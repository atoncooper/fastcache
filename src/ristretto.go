package src

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// RistrettoCache high performance cache
type RistrettoCache struct {
	config  *Config
	cache   *LRUCache
	freq    *Frequency
	metrics *Metrics
	closed  atomic.Bool

	// async Set buffer
	setBuf chan *setItem
	waitCh chan struct{}

	// callbacks
	onEvict  func(key string, value any, cost int64)
	onReject func(key string, value any, cost int64)
	onExit   func(value any)

	// GC configuration (from ShardedCacheV2)
	gcInterval     time.Duration
	gcMemThreshold int
	// Shared stop channel for ShardedCacheV2 GC
	stopCh chan struct{}

	wg sync.WaitGroup
}

type setItem struct {
	key        string
	value      any
	cost       int64
	expiration int64
}

// NewRistrettoCache creates a new cache
func NewRistrettoCache(config *Config) (*RistrettoCache, error) {
	if config == nil {
		config = defaultConfig()
	}

	// Set defaults
	if config.NumCounters <= 0 {
		config.NumCounters = 1e7
	}
	if config.MaxCost <= 0 {
		config.MaxCost = 1 << 30
	}
	if config.BufferItems <= 0 {
		config.BufferItems = 64
	}

	c := &RistrettoCache{
		config:         config,
		cache:          NewLRUCache(config.MaxCost),
		freq:           NewFrequency(config.NumCounters),
		metrics:        NewMetrics(),
		setBuf:         make(chan *setItem, config.BufferItems*10),
		waitCh:         make(chan struct{}),
		onEvict:        config.OnEvict,
		onReject:       config.OnReject,
		onExit:         config.OnExit,
		gcInterval:     config.GCInterval,
		gcMemThreshold: config.GcMemThreshold,
		stopCh:         make(chan struct{}),
	}

	// Start async write processor
	c.wg.Add(1)
	go c.processSets()

	// Start TTL cleaner
	if config.TTL > 0 {
		c.wg.Add(1)
		go c.ttlCleaner(config.TTL)
	}

	// Start GC if enabled (for standalone RistrettoCache)
	// ShardedCacheV2 will manage GC separately
	if config.GCInterval > 0 && config.GcMemThreshold > 0 {
		c.wg.Add(1)
		go c.gcRunner()
	}

	return c, nil
}

// Set sets a value
// returns accepted - may be dropped due to contention
func (c *RistrettoCache) Set(key string, value any, cost int64) bool {
	return c.setWithOptions(key, value, cost, 0)
}

// SetWithTTL sets a value with TTL
func (c *RistrettoCache) SetWithTTL(key string, value any, cost int64, ttl time.Duration) bool {
	var expiration int64
	if ttl > 0 {
		expiration = time.Now().UnixNano() + int64(ttl)
	}
	return c.setWithOptions(key, value, cost, expiration)
}

// setWithOptions internal set method
func (c *RistrettoCache) setWithOptions(key string, value any, cost int64, expiration int64) bool {
	if c.closed.Load() {
		return false
	}

	// Validate cost
	if cost < 0 {
		cost = 1
	}
	if cost == 0 {
		cost = 1
	}

	// Reject if cost exceeds max cost
	if int64(cost) > c.config.MaxCost {
		c.metrics.setsRejected.Add(1)
		if c.onReject != nil {
			c.onReject(key, value, cost)
		}
		if c.onExit != nil {
			c.onExit(value)
		}
		return false
	}

	// Send to buffer
	select {
	case c.setBuf <- &setItem{key, value, cost, expiration}:
		return true
	default:
		// Buffer full, drop
		c.metrics.setsDropped.Add(1)
		return false
	}
}

// processSets processes async Sets
func (c *RistrettoCache) processSets() {
	defer c.wg.Done()

	for {
		select {
		case item := <-c.setBuf:
			c.processOneSet(item)
		case <-c.waitCh:
			// Process all buffered items
			for {
				select {
				case item := <-c.setBuf:
					c.processOneSet(item)
				default:
					return
				}
			}
		}
	}
}

// processOneSet processes a single Set
func (c *RistrettoCache) processOneSet(item *setItem) {
	key := item.key

	// Update frequency first (for admission control)
	c.freq.Increment(key)

	// TinyLFU admission policy: sample and compare
	// Only apply when cache is near capacity
	currentCost := c.cache.Cost()
	isNearCapacity := c.config.MaxCost > 0 && currentCost > c.config.MaxCost*7/10

	if isNearCapacity && c.cache.Len() > 0 {
		// Get current key's frequency
		currentFreq := c.freq.Get(key)

		// Sample existing keys and find minimum frequency
		minFreq, evictKey := c.sampleMinFrequency(5)

		// If new key frequency is higher, admit it and potentially evict sample
		if currentFreq > minFreq && evictKey != "" {
			// Evict the sampled key to make room
			c.cache.Delete(evictKey)
		}
	}

	// Check current cost
	availCost := c.config.MaxCost - c.cache.Cost()

	// If new item cost exceeds available cost, evict
	if int64(item.cost) > availCost {
		// Evict until enough space
		for c.cache.Cost()+int64(item.cost) > c.config.MaxCost && c.cache.Len() > 0 {
			evicted := c.evictOne()
			if evicted == nil {
				break
			}
		}
	}

	// Try to get old value
	oldItem, found := c.cache.GetItem(key)
	if found {
		// Update existing item
		c.cache.mu.Lock()
		oldValue := oldItem.Value
		oldCost := oldItem.Cost
		oldItem.Value = item.value
		oldItem.Cost = item.cost
		oldItem.Expiration = item.expiration
		c.cache.cost = c.cache.cost - oldCost + item.cost
		c.cache.list.MoveToFront(oldItem.element)
		c.cache.mu.Unlock()

		c.metrics.costAdded.Add(item.cost)

		if c.onExit != nil && oldValue != nil {
			c.onExit(oldValue)
		}
	} else {
		// Add new item
		c.cache.Add(key, item.value, item.cost, item.expiration)
		c.metrics.keysAdded.Add(1)
		c.metrics.costAdded.Add(item.cost)
	}
}

// sampleMinFrequency samples keys and returns the minimum frequency
func (c *RistrettoCache) sampleMinFrequency(sampleSize int) (minFreq int64, evictKey string) {
	items := c.cache.Items()
	if len(items) == 0 {
		return 0, ""
	}

	// Limit sample size
	if len(items) < sampleSize {
		sampleSize = len(items)
	}

	minFreq = 1<<63 - 1

	// Sample random keys
	for i := 0; i < sampleSize; i++ {
		key := items[i].Key
		freq := c.freq.Get(key)
		if freq < minFreq {
			minFreq = freq
			evictKey = key
		}
	}

	return minFreq, evictKey
}

// evictOne evicts one item
func (c *RistrettoCache) evictOne() *CacheItem {
	// Evict from LRU tail (oldest)
	item := c.cache.GetList().Back()
	if item == nil {
		return nil
	}

	evicted := item.Value.(*CacheItem)

	// Call callbacks
	if c.onEvict != nil {
		c.onEvict(evicted.Key, evicted.Value, evicted.Cost)
	}
	if c.onExit != nil {
		c.onExit(evicted.Value)
	}

	c.cache.RemoveElement(evicted)

	c.metrics.keysEvicted.Add(1)
	c.metrics.costEvicted.Add(evicted.Cost)

	return evicted
}

// Get gets a value
func (c *RistrettoCache) Get(key string) (any, bool) {
	if c.closed.Load() {
		return nil, false
	}

	// Use GetAndUpdate to update LRU
	item, found := c.cache.GetAndUpdate(key)
	if !found {
		c.metrics.misses.Add(1)
		return nil, false
	}

	// Increment frequency
	c.freq.Increment(key)

	c.metrics.hits.Add(1)
	return item.Value, true
}

// GetWithTTL gets a value and remaining TTL
func (c *RistrettoCache) GetWithTTL(key string) (any, bool, time.Duration) {
	if c.closed.Load() {
		return nil, false, 0
	}

	item, found := c.cache.GetAndUpdate(key)
	if !found {
		c.metrics.misses.Add(1)
		return nil, false, 0
	}

	c.freq.Increment(key)
	c.metrics.hits.Add(1)

	var ttl time.Duration
	if item.Expiration > 0 {
		ttl = time.Duration(item.Expiration - time.Now().UnixNano())
		if ttl < 0 {
			ttl = 0
		}
	}

	return item.Value, true, ttl
}

// GetTTL gets remaining TTL
func (c *RistrettoCache) GetTTL(key string) (time.Duration, bool) {
	item, found := c.cache.GetAndUpdate(key)
	if !found {
		return 0, false
	}

	if item.Expiration <= 0 {
		return 0, false
	}

	ttl := time.Duration(item.Expiration - time.Now().UnixNano())
	if ttl < 0 {
		return 0, false
	}

	return ttl, true
}

// MGet gets multiple values
// Returns a map of key -> value, only found keys are included
func (c *RistrettoCache) MGet(keys ...string) map[string]any {
	if c.closed.Load() {
		return nil
	}

	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, found := c.Get(key); found {
			result[key] = value
		}
	}
	return result
}

// MSet sets multiple values
// Returns the number of successfully set items
func (c *RistrettoCache) MSet(items map[string]any, defaultCost int64) int {
	if c.closed.Load() {
		return 0
	}

	successCount := 0
	for key, value := range items {
		cost := defaultCost
		if cost <= 0 {
			cost = 1
		}
		if c.Set(key, value, cost) {
			successCount++
		}
	}
	return successCount
}

// MSetWithCosts sets multiple values with individual costs
// items map[key]struct{value any, cost int64}
// Returns the number of successfully set items
func (c *RistrettoCache) MSetWithCosts(items map[string]struct {
	Value any
	Cost  int64
}) int {
	if c.closed.Load() {
		return 0
	}

	successCount := 0
	for key, item := range items {
		cost := item.Cost
		if cost <= 0 {
			cost = 1
		}
		if c.Set(key, item.Value, cost) {
			successCount++
		}
	}
	return successCount
}

// Exists checks if a key exists (without updating LRU)
func (c *RistrettoCache) Exists(key string) bool {
	_, found := c.cache.Get(key)
	return found
}

// CAS performs compare-and-swap operation
// Only sets the value if the current value matches the old value
// Returns true if the operation succeeded
func (c *RistrettoCache) CAS(key string, oldValue any, newValue any, cost int64) bool {
	if c.closed.Load() {
		return false
	}

	if cost < 0 {
		cost = 1
	}

	// Get current item
	item, found := c.cache.GetItem(key)
	if !found {
		// Key doesn't exist, can't CAS
		return false
	}

	// Compare old value
	if oldValue != item.Value {
		// Value doesn't match
		return false
	}

	// Perform CAS by setting new value
	return c.Set(key, newValue, cost)
}

// Del deletes a value
func (c *RistrettoCache) Del(key string) {
	value, found := c.cache.Delete(key)
	if found {
		if c.onExit != nil && value != nil {
			c.onExit(value)
		}
	}
}

// Wait waits for all buffered writes to complete
func (c *RistrettoCache) Wait() {
	if c.closed.Load() {
		return
	}

	// Send signal
	close(c.waitCh)
	c.wg.Wait()

	// Recreate waitCh (since it was closed)
	c.waitCh = make(chan struct{})
	c.wg.Add(1)
	go c.processSets()
}

// Close closes the cache
func (c *RistrettoCache) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	// Wait for all writes to complete
	close(c.waitCh)
	c.wg.Wait()

	return nil
}

// Clear clears the cache
func (c *RistrettoCache) Clear() {
	c.cache.Clear()
}

// Len returns the number of items in the cache
func (c *RistrettoCache) Len() int {
	return c.cache.Len()
}

// Cost returns the current cost
func (c *RistrettoCache) Cost() int64 {
	return c.cache.Cost()
}

// Metrics returns the metrics
func (c *RistrettoCache) Metrics() *Metrics {
	return c.metrics
}

// ttlCleaner TTL cleaner
func (c *RistrettoCache) ttlCleaner(ttl time.Duration) {
	defer c.wg.Done()

	ticker := time.NewTicker(ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.closed.Load() {
				return
			}
			c.cleanupExpired()
		case <-c.waitCh:
			return
		}
	}
}

// cleanupExpired cleans up expired items
func (c *RistrettoCache) cleanupExpired() {
	now := time.Now().UnixNano()
	items := c.cache.Items()

	for _, item := range items {
		if item.Expiration > 0 && now > item.Expiration {
			value, found := c.cache.Delete(item.Key)
			if found {
				c.metrics.keysEvicted.Add(1)
				c.metrics.costEvicted.Add(item.Cost)
				if c.onEvict != nil {
					c.onEvict(item.Key, value, item.Cost)
				}
				if c.onExit != nil {
					c.onExit(value)
				}
			}
		}
	}
}

// GC manually triggers GC (for testing)
func (c *RistrettoCache) GC() {
	runtime.GC()
}

// gcRunner runs periodic GC and memory management
func (c *RistrettoCache) gcRunner() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.closed.Load() {
				return
			}
			c.doGC()
		case <-c.waitCh:
			return
		case <-c.stopCh:
			return
		}
	}
}

// doGC performs garbage collection and memory management
func (c *RistrettoCache) doGC() {
	// Check cache cost vs max cost
	if c.config.MaxCost > 0 {
		currentCost := c.cache.Cost()
		costPercent := int(currentCost * 100 / c.config.MaxCost)

		// If cache cost exceeds threshold, trigger cleanup
		if costPercent > c.gcMemThreshold {
			// Clean up expired items
			c.cleanupExpired()

			// If still over cost limit, evict more items
			currentCost = c.cache.Cost()
			for currentCost > c.config.MaxCost && c.cache.Len() > 0 {
				// Evict 10% of cache items
				toEvict := c.cache.Len() / 10
				if toEvict < 1 {
					toEvict = 1
				}
				for i := 0; i < toEvict; i++ {
					c.evictOne()
				}
				currentCost = c.cache.Cost()
			}
		}
	} else if c.config.TTL > 0 {
		// Only cleanup expired items if no cost limit
		c.cleanupExpired()
	}

	// Also cleanup expired items periodically if TTL is enabled
	if c.config.TTL > 0 {
		c.cleanupExpired()
	}
}

// GetMemStats returns current memory statistics
func (c *RistrettoCache) GetMemStats() map[string]interface{} {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	cost := c.cache.Cost()
	maxCost := c.config.MaxCost

	stats := map[string]interface{}{
		"alloc":       int64(memStats.Alloc),
		"totalAlloc":  int64(memStats.TotalAlloc),
		"sys":         int64(memStats.Sys),
		"numGC":       memStats.NumGC,
		"cacheLen":    c.cache.Len(),
		"cacheCost":   cost,
		"maxCost":     maxCost,
	}

	if maxCost > 0 {
		stats["costPercent"] = int(cost * 100 / maxCost)
	}

	return stats
}

// String returns a string representation of the cache
func (c *RistrettoCache) String() string {
	return fmt.Sprintf("RistrettoCache: Len=%d, Cost=%d, Metrics:\n%s",
		c.Len(), c.Cost(), c.Metrics())
}
