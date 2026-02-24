package src

import (
	"container/list"
	"sync"
	"time"
)

// CacheItem represents a cache entry
type CacheItem struct {
	Key        string
	Value      any
	Cost       int64
	Expiration int64 // expiration time in nanoseconds, 0 means no expiration
	element    *list.Element // element in LRU linked list
}

// LRUCache LRU cache implementation
type LRUCache struct {
	mu      sync.RWMutex
	items   map[string]*CacheItem
	list    *list.List // doubly linked list, head is most recently used
	cost    int64
	maxCost int64
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(maxCost int64) *LRUCache {
	return &LRUCache{
		items:   make(map[string]*CacheItem),
		list:    list.New(),
		maxCost: maxCost,
	}
}

// Add adds an item to the cache
func (c *LRUCache) Add(key string, value any, cost int64, expiration int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update if exists
	if item, ok := c.items[key]; ok {
		c.cost -= item.Cost
		c.cost += cost
		item.Value = value
		item.Cost = cost
		item.Expiration = expiration
		c.list.MoveToFront(item.element)
		return
	}

	// Get item from pool
	item := GetCacheItem()
	item.Key = key
	item.Value = value
	item.Cost = cost
	item.Expiration = expiration

	item.element = c.list.PushFront(item)
	c.items[key] = item
	c.cost += cost

	// Evict if over max cost
	for c.cost > c.maxCost && c.list.Len() > 0 {
		c.evictOldest()
	}
}

// Get gets an item (read-only, does not update LRU)
func (c *LRUCache) Get(key string) (*CacheItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// Check expiration (simplified, does not delete under read lock)
	if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
		return nil, false
	}

	return item, true
}

// GetAndUpdate gets an item and updates LRU (for read operations)
func (c *LRUCache) GetAndUpdate(key string) (*CacheItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// Check expiration
	if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
		c.removeElement(item)
		return nil, false
	}

	// Move to front
	c.list.MoveToFront(item.element)
	return item, true
}

// Delete removes an item from the cache
func (c *LRUCache) Delete(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	c.removeElement(item)
	return item.Value, true
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(item *CacheItem) {
	if item.element != nil {
		c.list.Remove(item.element)
	}
	delete(c.items, item.Key)
	c.cost -= item.Cost
	// Return item to pool
	PutCacheItem(item)
}

// evictOldest evicts the oldest item
func (c *LRUCache) evictOldest() {
	if elem := c.list.Back(); elem != nil {
		item := elem.Value.(*CacheItem)
		c.removeElement(item)
	}
}

// Len returns the number of items
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Cost returns the current cost
func (c *LRUCache) Cost() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cost
}

// Clear clears the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*CacheItem)
	c.list.Init()
	c.cost = 0
}

// Items returns all items (for iteration)
func (c *LRUCache) Items() []*CacheItem {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items := make([]*CacheItem, 0, len(c.items))
	for _, item := range c.items {
		items = append(items, item)
	}
	return items
}

// GetItem returns the internal item map (for advanced operations)
func (c *LRUCache) GetItem(key string) (*CacheItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[key]
	return item, ok
}

// RemoveElement removes an element (caller must hold lock)
func (c *LRUCache) RemoveElement(item *CacheItem) {
	c.removeElement(item)
}

// GetList returns the list (caller must hold lock)
func (c *LRUCache) GetList() *list.List {
	return c.list
}
