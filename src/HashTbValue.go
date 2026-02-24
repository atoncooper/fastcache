package src

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type ValueLink struct {
	Key        string
	value      any
	refCount   int    // Reference count
	Next       *ValueLink
}

type RowValueLinkList struct {
	Head *ValueLink
}

func NewRvll() *RowValueLinkList {
	return &RowValueLinkList{}
}

func (r *RowValueLinkList) add(key string, value any) {
	node := &ValueLink{
		Key:   key,
		value: value,
	}
	if r.Head == nil {
		r.Head = node
	}
}

func (r *RowValueLinkList) find(key string) any {
	c := r.Head
	for c != nil {
		if c.Key == key {
			return c.value
		}
		c = c.Next
	}
	return nil
}

// findNode finds a node and returns the pointer.
func (r *RowValueLinkList) findNode(key string) *ValueLink {
	c := r.Head
	for c != nil {
		if c.Key == key {
			return c
		}
		c = c.Next
	}
	return nil
}

func (r *RowValueLinkList) addWithKey(key string, value any) {
	node := &ValueLink{
		Key:      key,
		value:    value,
		refCount: 1,
		Next:     r.Head,
	}
	r.Head = node
}

func (r *RowValueLinkList) delete(key string) bool {
	if r.Head == nil {
		return false
	}
	if r.Head.Key == key {
		r.Head = r.Head.Next
		return true
	}
	prev := r.Head
	for curr := r.Head.Next; curr != nil; curr = curr.Next {
		if curr.Key == key {
			prev.Next = curr.Next
			return true
		}
		prev = curr
	}
	return false
}

var keyIdCounter int64

func createKeyId() string {
	// Use atomic counter to ensure uniqueness
	count := atomic.AddInt64(&keyIdCounter, 1)
	now := time.Now().UnixNano()
	h := sha1.New()
	h.Write([]byte(strconv.FormatInt(now, 10)))
	h.Write([]byte(strconv.FormatInt(count, 10)))
	return hex.EncodeToString(h.Sum(nil))[:8]
}

const VDefaultSize = 512
const VLoadFactor = 0.75

type HashMapValueBucket struct {
	mu          sync.RWMutex
	table       []RowValueLinkList
	oldTable    []RowValueLinkList
	size        int
	count       int
	rehashIndex int
}

func (h *HashMapValueBucket) setValue(key string, value any) {
	// Calculate key's hash index value
	index := HashKey(key, h.size)
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.table[index].Head == nil {
		h.table[index] = *NewRvll()
	}

	h.table[index].addWithKey(key, value)
	h.count++
	if float64(h.count)/float64(h.size) > VLoadFactor {
		h.startExpansion()
	}
}

// getValue retrieves the value.
//
// key: The stored key
// return: The actual value
func (h *HashMapValueBucket) getValue(key string) any {
	index := HashKey(key, h.size)
	return h.table[index].find(key)
}

// DeleteValue deletes the value.
//
// key: The stored key
// return: bool, returns true if deletion was successful
//
//	true: Delete successful
//	false: Delete failed
func (h *HashMapValueBucket) DeleteValue(key string) bool {
	index := HashKey(key, h.size)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.table[index].Head == nil {
		return false
	}

	ok := h.table[index].delete(key)
	if ok {
		h.count--
	}
	return ok
}

// incrRefCount increments the reference count.
func (h *HashMapValueBucket) incrRefCount(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	index := HashKey(key, h.size)
	node := h.table[index].findNode(key)
	if node != nil {
		node.refCount++
	}
}

// decrRefCount decrements reference count and returns whether value should be deleted.
func (h *HashMapValueBucket) decrRefCount(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	index := HashKey(key, h.size)
	node := h.table[index].findNode(key)
	if node == nil {
		return false
	}
	node.refCount--
	if node.refCount <= 0 {
		h.table[index].delete(key)
		h.count--
		return true
	}
	return false
}

// startExpansion starts the hash table expansion.
func (h *HashMapValueBucket) startExpansion() {
	if h.oldTable != nil {
		// Already expanding
		return
	}
	h.oldTable = h.table
	h.size = h.size * 2
	h.table = make([]RowValueLinkList, h.size)
	// Initialize index
	h.rehashIndex = 0
}

// doReHashStep performs one step of the expansion.
func (h *HashMapValueBucket) doReHashStep() {
	if h.oldTable == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for i := h.rehashIndex; i < len(h.oldTable); i++ {
		bucket := h.oldTable[i]
		if bucket.Head != nil {
			for c := bucket.Head; c != nil; c = c.Next {
				index := HashKey(c.Key, h.size)
				if h.table[index].Head == nil {
					h.table[index] = *NewRvll()
				}
				h.table[index].addWithKey(c.Key, c.value)
			}
			h.rehashIndex = i + 1 // Next time start from next bucket
			break
		}
	}

	if h.rehashIndex >= len(h.oldTable) {
		h.oldTable = nil
		h.rehashIndex = 0
	}
}

func NewHashMapValueBucket() *HashMapValueBucket {
	return &HashMapValueBucket{
		table: make([]RowValueLinkList, VDefaultSize),
		size:  VDefaultSize,
	}
}

type ShardedCacheValue struct {
	shards     []*HashMapValueBucket
	shardCount int
}

func NewShardedCacheRowValue(count int) *ShardedCacheValue {
	sh := &ShardedCacheValue{
		shards:     make([]*HashMapValueBucket, count),
		shardCount: count,
	}
	for i := 0; i < count; i++ {
		sh.shards[i] = NewHashMapValueBucket()
	}
	return sh
}
func (sc *ShardedCacheValue) getShard(key string) *HashMapValueBucket {
	index := HashKey(key, sc.shardCount)
	return sc.shards[index]
}

func (sc *ShardedCacheValue) SetValue(value any) string {
	// Store value in the specified shard
	key := createKeyId()
	shard := sc.getShard(key)
	shard.setValue(key, value)
	return key
}

func (sc *ShardedCacheValue) GetValue(key string) any {
	shard := sc.getShard(key)
	return shard.getValue(key)
}

func (sc *ShardedCacheValue) DeleteValue(key string) bool {
	shard := sc.getShard(key)
	return shard.DeleteValue(key)
}

// IncrRefCount increments the reference count.
func (sc *ShardedCacheValue) IncrRefCount(key string) {
	shard := sc.getShard(key)
	shard.incrRefCount(key)
}

// DecrRefCount decrements reference count and returns whether value should be deleted.
func (sc *ShardedCacheValue) DecrRefCount(key string) bool {
	shard := sc.getShard(key)
	return shard.decrRefCount(key)
}
