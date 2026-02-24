package src

import (
	"hash/fnv"
	"sync"
	"time"
)

const DefaultSize = 512
const LoadFactor = 0.75

type KeyLink struct {
	Key      string
	value    string
	ExpireAt int64
	Start    int64
	LastAccess int64  // Last access time, used for LRU
	Next     *KeyLink
}

type KeyLinkList struct {
	Head *KeyLink
}

func NewKLL() *KeyLinkList {
	return &KeyLinkList{}
}

func (k *KeyLinkList) add(key string, value string, exp int64) {
	node := &KeyLink{
		Key:        key,
		value:      value,
		Start:      time.Now().UnixNano(),
		LastAccess: time.Now().UnixNano(),
		ExpireAt:   exp,
	}
	// Head insertion method
	node.Next = k.Head
	k.Head = node
}

func (k *KeyLinkList) find(key string) (string, bool) {
	c := k.Head
	for c != nil {
		if c.Key == key {
			// Check if expired
			if time.Now().UnixNano() > c.ExpireAt {
				return "", false
			}
			return c.value, true
		}
		c = c.Next
	}
	return "", false
}

func (k *KeyLinkList) delete(key string) {
	if k.Head == nil {
		return
	}
	if k.Head.Key == key {
		k.Head = k.Head.Next
		return
	}
	prev := k.Head
	curr := k.Head.Next
	for curr != nil {
		if curr.Key == key {
			prev.Next = curr.Next
			return
		}
		prev = curr
		curr = curr.Next
	}
}

type HashMapAkBucket struct {
	mu          sync.RWMutex
	table       []KeyLinkList
	oldTable    []KeyLinkList
	size        int
	count       int64
	rehashIndex int
}

func NewHashMapAKBucket() *HashMapAkBucket {
	return &HashMapAkBucket{
		table: make([]KeyLinkList, DefaultSize),
		size:  DefaultSize,
	}
}

// set inserts a key-value-expire item.
func (h *HashMapAkBucket) set(key string, value string, exp int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.doReHashStep()

	if float64(h.count)/float64(h.size) > LoadFactor {
		h.startExpansion()
	}

	index := HashKey(key, h.size)

	// Insert into linked list
	h.table[index].add(key, value, exp)
	h.count++
	return nil
}

// get retrieves the value.
func (h *HashMapAkBucket) get(key string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	index := HashKey(key, h.size)

	node := h.table[index].Head
	var value string
	var ok bool
	now := time.Now().UnixNano()
	for node != nil {
		if node.Key == key {
			if now > node.ExpireAt {
				// Mark for deletion, don't call delete under read lock
				return "", false
			}
			value = node.value
			ok = true
			// Update last access time
			node.LastAccess = now
			break
		}
		node = node.Next
	}
	return value, ok
}

// deleteExpired deletes expired keys (internal use, requires write lock).
func (h *HashMapAkBucket) deleteExpired(key string) {
	index := HashKey(key, h.size)
	h.table[index].delete(key)
	h.count--
}

// delete deletes the key.
func (h *HashMapAkBucket) delete(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	index := HashKey(key, h.size)
	h.table[index].delete(key)
	h.count--
}

// startExpansion initiates hash table expansion.
func (h *HashMapAkBucket) startExpansion() {
	if h.oldTable != nil {
		return
	}
	h.oldTable = h.table
	h.size = h.size * 2
	h.table = make([]KeyLinkList, h.size)
	h.rehashIndex = 0
}

// doReHashStep migrates one step.
func (h *HashMapAkBucket) doReHashStep() {
	if h.oldTable == nil {
		return
	}
	for i := h.rehashIndex; i < len(h.oldTable); i++ {
		oldList := h.oldTable[i]
		node := oldList.Head
		for node != nil {
			next := node.Next
			index := HashKey(node.Key, h.size)
			h.table[index].add(node.Key, node.value, node.ExpireAt)
			node = next
		}
		h.rehashIndex = i + 1
		break // Only process one bucket each time, step size can be adjusted
	}
	if h.rehashIndex >= len(h.oldTable) {
		h.oldTable = nil
		h.rehashIndex = 0
	}
}

// HashKey is a hash function.
func HashKey(key string, size int) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := int(h.Sum32() & 0x7fffffff) // Ensure positive
	return hash % size
}

// startGC launches a goroutine to periodically clean expired keys.
func (h *HashMapAkBucket) startGC(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			h.mu.Lock()
			now := time.Now().UnixNano()

			for i := 0; i < len(h.table); i++ {
				prev := (*KeyLink)(nil)
				curr := h.table[i].Head
				for curr != nil {
					if now > curr.ExpireAt {
						if prev == nil {
							h.table[i].Head = curr.Next
							curr = h.table[i].Head
						} else {
							prev.Next = curr.Next
							curr = prev.Next
						}
						h.count--
					} else {
						prev = curr
						curr = curr.Next
					}
				}
			}

			// Clean up oldTable (if expanding)
			if h.oldTable != nil {
				for i := 0; i < len(h.oldTable); i++ {
					prev := (*KeyLink)(nil)
					curr := h.oldTable[i].Head
					for curr != nil {
						if now > curr.ExpireAt {
							if prev == nil {
								h.oldTable[i].Head = curr.Next
								curr = h.oldTable[i].Head
							} else {
								prev.Next = curr.Next
								curr = prev.Next
							}
							h.count--
						} else {
							prev = curr
							curr = curr.Next
						}
					}
				}
			}

			h.mu.Unlock()
		}
	}()
}

type ShardedCache struct {
	shards     []*HashMapAkBucket
	shardCount int
}

func NewShardedCache(count int) *ShardedCache {
	sc := &ShardedCache{
		shards:     make([]*HashMapAkBucket, count),
		shardCount: count,
	}
	for i := 0; i < count; i++ {
		sc.shards[i] = NewHashMapAKBucket()
	}
	return sc
}
func (sc *ShardedCache) getShard(key string) *HashMapAkBucket {
	index := HashKey(key, sc.shardCount)
	return sc.shards[index]
}
func (sc *ShardedCache) Set(key string, value string, exp int64) {
	shard := sc.getShard(key)
	shard.set(key, value, exp)
}

func (sc *ShardedCache) Get(key string) (string, bool) {
	shard := sc.getShard(key)
	return shard.get(key)
}

func (sc *ShardedCache) Delete(key string) {
	shard := sc.getShard(key)
	shard.delete(key)
}
func (sc *ShardedCache) StartGC(interval time.Duration) {
	for _, shard := range sc.shards {
		shard.startGC(interval)
	}
}

// EvictOne evicts one least recently used key, returns evicted key.
func (sc *ShardedCache) EvictOne() string {
	// Randomly select a shard
	for i := 0; i < sc.shardCount; i++ {
		shard := sc.shards[i]
		evicted := shard.evictOne()
		if evicted != "" {
			return evicted
		}
	}
	return ""
}

// evictOne evicts one least recently used key from current shard.
func (h *HashMapAkBucket) evictOne() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now().UnixNano()
	var oldestKey string
	var oldestTime int64 = now + 1

	// Iterate through all buckets, find the oldest non-expired key
	for i := 0; i < len(h.table); i++ {
		node := h.table[i].Head
		for node != nil {
			if node.ExpireAt > now && node.LastAccess < oldestTime {
				oldestTime = node.LastAccess
				oldestKey = node.Key
			}
			node = node.Next
		}
	}

	if oldestKey != "" {
		index := HashKey(oldestKey, h.size)
		h.table[index].delete(oldestKey)
		h.count--
	}

	return oldestKey
}

// Count returns the current number of keys.
func (sc *ShardedCache) Count() int64 {
	var total int64
	for _, shard := range sc.shards {
		total += shard.count
	}
	return total
}
