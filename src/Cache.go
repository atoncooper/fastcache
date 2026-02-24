package src

import (
	"sync"
	"time"
)

const (
	DefaultMaxKeys = 100000 // Default max number of keys
)

type FastCache struct {
	KeyMap   *ShardedCache
	ValueMap *ShardedCacheValue
	mu       sync.RWMutex

	// MaxKeys is the maximum number of keys allowed
	MaxKeys int64

	// closed is the flag indicating if the cache is closed
	closed bool
	closeCh chan struct{}
}

func NewFastCache() *FastCache {
	return NewFastCacheWithMaxKeys(DefaultMaxKeys)
}

func NewFastCacheWithMaxKeys(maxKeys int64) *FastCache {
	fc := &FastCache{
		KeyMap:   NewShardedCache(512),
		ValueMap: NewShardedCacheRowValue(512),
		MaxKeys:  maxKeys,
		closeCh:  make(chan struct{}),
	}
	fc.KeyMap.StartGC(10 * time.Second)
	return fc
}

// Close closes the cache, stops GC and releases resources.
func (fc *FastCache) Close() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.closed {
		return nil
	}
	fc.closed = true
	close(fc.closeCh)
	return nil
}

func (fc *FastCache) Set(key string, value any, exp time.Duration) {
	// Check for empty key
	if key == "" {
		return
	}

	// Check if closed
	if fc.closed {
		return
	}

	// Expiration time cannot be negative
	if exp < 0 {
		exp = 0
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	// If key already exists, delete old value reference first
	oldKeyValue, ok := fc.KeyMap.Get(key)
	if ok && oldKeyValue != "" {
		fc.ValueMap.DecrRefCount(oldKeyValue)
	}

	// Capacity check, trigger LRU eviction
	if fc.MaxKeys > 0 {
		for fc.KeyMap.Count() >= fc.MaxKeys {
			evictedKey := fc.KeyMap.EvictOne()
			if evictedKey == "" {
				break
			}
			// Try to get evicted key's value and decrement reference
			evictedKeyValue, _ := fc.KeyMap.Get(evictedKey)
			if evictedKeyValue != "" {
				fc.ValueMap.DecrRefCount(evictedKeyValue)
			}
		}
	}

	// Store new value
	keyValue := fc.ValueMap.SetValue(value)
	expTime := time.Now().UnixNano() + int64(exp)
	fc.KeyMap.Set(key, keyValue, expTime)
}

// Get retrieves a value, returns (value, exists).
// exists is true if the value exists, false if it does not.
// Even if value is nil, exists is true as long as the key exists.
func (fc *FastCache) Get(key string) (any, bool) {
	// Check for empty key
	if key == "" {
		return nil, false
	}

	KeyValue, ok := fc.KeyMap.Get(key)
	if !ok || KeyValue == "" {
		return nil, false
	}
	value := fc.ValueMap.GetValue(KeyValue)
	if value == nil {
		// If value doesn't exist, delete key and decrement reference count
		fc.KeyMap.Delete(key)
		fc.ValueMap.DecrRefCount(KeyValue)
		return nil, false
	}
	return value, true
}

func (fc *FastCache) Delete(key string) {
	// Check for empty key
	if key == "" {
		return
	}

	// Delete key first, then delete value
	KeyValue, ok := fc.KeyMap.Get(key)
	if ok == false && KeyValue == "" {
		return
	}
	fc.KeyMap.Delete(key)
	// Decrement reference count, automatically delete value when count reaches 0
	fc.ValueMap.DecrRefCount(KeyValue)
}

func (fc *FastCache) SetM2One(key []string, value any, exp time.Duration) {
	// Check if closed
	if fc.closed {
		return
	}

	// Expiration time cannot be negative
	if exp < 0 {
		exp = 0
	}

	keyValue := fc.ValueMap.SetValue(value)
	expTime := time.Now().UnixNano() + int64(exp)
	// Map multiple keys to the same value, each key increments reference count
	for _, k := range key {
		// Skip empty key
		if k == "" {
			continue
		}
		// If key already exists, delete old value reference first
		oldKeyValue, ok := fc.KeyMap.Get(k)
		if ok && oldKeyValue != "" {
			fc.ValueMap.DecrRefCount(oldKeyValue)
		}
		fc.KeyMap.Set(k, keyValue, expTime)
		fc.ValueMap.IncrRefCount(keyValue)
	}
}
