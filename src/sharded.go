package src

import (
	"hash/fnv"
	"sync"
	"time"
)

// ShardedCacheV2 is a sharded cache implementation for high concurrency
type ShardedCacheV2 struct {
	shards      []*RistrettoCache
	shardCount  int
	numCounters int64
	maxCost     int64
	bufferItems int64
	metrics     bool
	ttl         time.Duration
	onEvict     func(key string, value any, cost int64)
	onReject    func(key string, value any, cost int64)
	onExit      func(value any)

	// GC management
	gcInterval     time.Duration
	gcMemThreshold int

	// Internal
	closed bool
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewShardedCacheV2 creates a new sharded cache
func NewShardedCacheV2(shardCount int, config *Config) (*ShardedCacheV2, error) {
	if shardCount <= 0 {
		shardCount = 32 // default shard count
	}

	// Use config values or defaults
	numCounters := int64(1e6)
	maxCost := int64(1 << 20) // 1MB per shard
	bufferItems := int64(64)
	metrics := false
	var ttl time.Duration
	var onEvict func(key string, value any, cost int64)
	var onReject func(key string, value any, cost int64)
	var onExit func(value any)
	gcInterval := time.Duration(0)
	gcMemThreshold := 80

	if config != nil {
		if config.NumCounters > 0 {
			numCounters = config.NumCounters / int64(shardCount)
		}
		if config.MaxCost > 0 {
			// Auto-shard MaxCost across all shards
			maxCost = config.MaxCost / int64(shardCount)
		}
		if config.BufferItems > 0 {
			bufferItems = config.BufferItems
		}
		metrics = config.Metrics
		ttl = config.TTL
		onEvict = config.OnEvict
		onReject = config.OnReject
		onExit = config.OnExit
		gcInterval = config.GCInterval
		if config.GcMemThreshold > 0 {
			gcMemThreshold = config.GcMemThreshold
		}
	}

	sc := &ShardedCacheV2{
		shards:         make([]*RistrettoCache, shardCount),
		shardCount:     shardCount,
		numCounters:    numCounters,
		maxCost:        maxCost,
		bufferItems:    bufferItems,
		metrics:        metrics,
		ttl:            ttl,
		onEvict:        onEvict,
		onReject:       onReject,
		onExit:         onExit,
		gcInterval:     gcInterval,
		gcMemThreshold: gcMemThreshold,
		stopCh:         make(chan struct{}),
	}

	// Initialize shards
	for i := 0; i < shardCount; i++ {
		shardConfig := &Config{
			NumCounters:    sc.numCounters,
			MaxCost:        sc.maxCost,
			BufferItems:    sc.bufferItems,
			Metrics:        sc.metrics,
			TTL:            sc.ttl,
			OnEvict:        sc.onEvict,
			OnReject:       sc.onReject,
			OnExit:         sc.onExit,
			GCInterval:     0, // ShardedCacheV2 manages GC centrally
			GcMemThreshold: 0,  // ShardedCacheV2 manages GC centrally
		}
		cache, err := NewRistrettoCache(shardConfig)
		if err != nil {
			// Rollback already created shards
			for j := 0; j < i; j++ {
				sc.shards[j].Close()
			}
			return nil, err
		}
		sc.shards[i] = cache
	}

	// Start unified GC goroutine (only one for all shards)
	if sc.gcInterval > 0 {
		sc.wg.Add(1)
		go sc.gcRunner()
	}

	return sc, nil
}

// getShard returns the shard for a given key
func (sc *ShardedCacheV2) getShard(key string) *RistrettoCache {
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := int(h.Sum32())
	return sc.shards[hash%sc.shardCount]
}

// Set sets a value
func (sc *ShardedCacheV2) Set(key string, value any, cost int64) bool {
	shard := sc.getShard(key)
	return shard.Set(key, value, cost)
}

// SetWithTTL sets a value with TTL
func (sc *ShardedCacheV2) SetWithTTL(key string, value any, cost int64, ttl time.Duration) bool {
	shard := sc.getShard(key)
	return shard.SetWithTTL(key, value, cost, ttl)
}

// Get gets a value
func (sc *ShardedCacheV2) Get(key string) (any, bool) {
	shard := sc.getShard(key)
	return shard.Get(key)
}

// GetWithTTL gets a value and remaining TTL
func (sc *ShardedCacheV2) GetWithTTL(key string) (any, bool, time.Duration) {
	shard := sc.getShard(key)
	return shard.GetWithTTL(key)
}

// GetTTL gets remaining TTL
func (sc *ShardedCacheV2) GetTTL(key string) (time.Duration, bool) {
	shard := sc.getShard(key)
	return shard.GetTTL(key)
}

// MGet gets multiple values from all shards
// Returns a map of key -> value, only found keys are included
func (sc *ShardedCacheV2) MGet(keys ...string) map[string]any {
	if len(keys) == 0 {
		return nil
	}

	// Group keys by shard
	shardKeys := make(map[*RistrettoCache][]string)
	for _, key := range keys {
		shard := sc.getShard(key)
		shardKeys[shard] = append(shardKeys[shard], key)
	}

	// Query each shard
	result := make(map[string]any)
	var wg sync.WaitGroup
	mu := sync.Mutex{}

	for shard, keys := range shardKeys {
		wg.Add(1)
		go func(s *RistrettoCache, ks []string) {
			defer wg.Done()
			values := s.MGet(ks...)
			mu.Lock()
			for k, v := range values {
				result[k] = v
			}
			mu.Unlock()
		}(shard, keys)
	}
	wg.Wait()

	return result
}

// MSet sets multiple values
// Returns the number of successfully set items
func (sc *ShardedCacheV2) MSet(items map[string]any, defaultCost int64) int {
	if len(items) == 0 {
		return 0
	}

	// Group keys by shard
	shardItems := make(map[*RistrettoCache]map[string]any)
	for key, value := range items {
		shard := sc.getShard(key)
		if shardItems[shard] == nil {
			shardItems[shard] = make(map[string]any)
		}
		shardItems[shard][key] = value
	}

	// Set on each shard
	successCount := 0
	var wg sync.WaitGroup
	mu := sync.Mutex{}

	for shard, items := range shardItems {
		wg.Add(1)
		go func(s *RistrettoCache, its map[string]any) {
			defer wg.Done()
			count := s.MSet(its, defaultCost)
			mu.Lock()
			successCount += count
			mu.Unlock()
		}(shard, items)
	}
	wg.Wait()

	return successCount
}

// MSetWithCosts sets multiple values with individual costs
// Returns the number of successfully set items
func (sc *ShardedCacheV2) MSetWithCosts(items map[string]struct {
	Value any
	Cost  int64
}) int {
	if len(items) == 0 {
		return 0
	}

	// Group keys by shard
	type itemData struct {
		Value any
		Cost  int64
	}
	shardItems := make(map[*RistrettoCache]map[string]itemData)
	for key, item := range items {
		shard := sc.getShard(key)
		if shardItems[shard] == nil {
			shardItems[shard] = make(map[string]itemData)
		}
		shardItems[shard][key] = itemData{Value: item.Value, Cost: item.Cost}
	}

	// Set on each shard
	successCount := 0
	var wg sync.WaitGroup
	mu := sync.Mutex{}

	for shard, items := range shardItems {
		wg.Add(1)
		go func(s *RistrettoCache, its map[string]itemData) {
			defer wg.Done()
			// Convert to map format expected by MSetWithCosts
			converted := make(map[string]struct{ Value any; Cost int64 })
			for k, v := range its {
				converted[k] = struct{ Value any; Cost int64 }{Value: v.Value, Cost: v.Cost}
			}
			count := s.MSetWithCosts(converted)
			mu.Lock()
			successCount += count
			mu.Unlock()
		}(shard, items)
	}
	wg.Wait()

	return successCount
}

// Exists checks if a key exists (without updating LRU)
func (sc *ShardedCacheV2) Exists(key string) bool {
	shard := sc.getShard(key)
	return shard.Exists(key)
}

// CAS performs compare-and-swap operation
// Only sets the value if the current value matches the old value
// Returns true if the operation succeeded
func (sc *ShardedCacheV2) CAS(key string, oldValue any, newValue any, cost int64) bool {
	shard := sc.getShard(key)
	return shard.CAS(key, oldValue, newValue, cost)
}

// Del deletes a value
func (sc *ShardedCacheV2) Del(key string) {
	shard := sc.getShard(key)
	shard.Del(key)
}

// Wait waits for all buffered writes to complete
func (sc *ShardedCacheV2) Wait() {
	var wg sync.WaitGroup
	wg.Add(sc.shardCount)
	for _, shard := range sc.shards {
		go func(s *RistrettoCache) {
			s.Wait()
			wg.Done()
		}(shard)
	}
	wg.Wait()
}

// Close closes all shards
func (sc *ShardedCacheV2) Close() error {
	if sc.closed {
		return nil
	}
	sc.closed = true

	// Stop GC goroutine
	close(sc.stopCh)
	sc.wg.Wait()

	// Close all shards
	var wg sync.WaitGroup
	wg.Add(sc.shardCount)
	for _, shard := range sc.shards {
		go func(s *RistrettoCache) {
			s.Close()
			wg.Done()
		}(shard)
	}
	wg.Wait()
	return nil
}

// Clear clears all shards
func (sc *ShardedCacheV2) Clear() {
	for _, shard := range sc.shards {
		shard.Clear()
	}
}

// Len returns the total number of items
func (sc *ShardedCacheV2) Len() int {
	total := 0
	for _, shard := range sc.shards {
		total += shard.Len()
	}
	return total
}

// Cost returns the total cost
func (sc *ShardedCacheV2) Cost() int64 {
	var total int64
	for _, shard := range sc.shards {
		total += shard.Cost()
	}
	return total
}

// Metrics returns aggregated metrics from all shards
func (sc *ShardedCacheV2) Metrics() *Metrics {
	total := &Metrics{}

	for _, shard := range sc.shards {
		m := shard.Metrics()
		if m != nil {
			total.hits.Add(m.Hits())
			total.misses.Add(m.Misses())
			total.keysAdded.Add(m.KeysAdded())
			total.keysEvicted.Add(m.KeysEvicted())
			total.setsDropped.Add(m.SetsDropped())
			total.setsRejected.Add(m.SetsRejected())
			total.costAdded.Add(m.CostAdded())
			total.costEvicted.Add(m.CostEvicted())
		}
	}

	return total
}

// ShardLen returns the number of shards
func (sc *ShardedCacheV2) ShardLen() int {
	return sc.shardCount
}

// ShardStats returns statistics for each shard
func (sc *ShardedCacheV2) ShardStats() []ShardStat {
	stats := make([]ShardStat, sc.shardCount)
	for i, shard := range sc.shards {
		stats[i] = ShardStat{
			Shard: i,
			Len:   shard.Len(),
			Cost:  shard.Cost(),
		}
	}
	return stats
}

// ShardStat represents statistics for a single shard
type ShardStat struct {
	Shard int
	Len   int
	Cost  int64
}

// GetMemStats returns aggregated memory statistics from all shards
func (sc *ShardedCacheV2) GetMemStats() map[string]interface{} {
	var totalAlloc, totalCost, totalMaxCost int64
	var totalLen int

	for _, shard := range sc.shards {
		stats := shard.GetMemStats()
		totalAlloc += stats["alloc"].(int64)
		totalCost += stats["cacheCost"].(int64)
		totalMaxCost += stats["maxCost"].(int64)
		totalLen += stats["cacheLen"].(int)
	}

	result := map[string]interface{}{
		"totalAlloc":   totalAlloc,
		"totalCost":    totalCost,
		"totalMaxCost": totalMaxCost,
		"totalLen":     totalLen,
		"numShards":    sc.shardCount,
	}

	if totalMaxCost > 0 {
		result["costPercent"] = int(totalCost * 100 / totalMaxCost)
	}

	return result
}

// gcRunner runs periodic GC for all shards (unified management)
func (sc *ShardedCacheV2) gcRunner() {
	defer sc.wg.Done()

	ticker := time.NewTicker(sc.gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if sc.closed {
				return
			}
			// Run GC on all shards
			for _, shard := range sc.shards {
				shard.doGC()
			}
		case <-sc.stopCh:
			return
		}
	}
}
