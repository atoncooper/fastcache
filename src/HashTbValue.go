package src

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

type ValueLink struct {
	Key   string
	value any
	Next  *ValueLink
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

func (r *RowValueLinkList) addWithKey(key string, value any) {
	node := &ValueLink{
		Key:   key,
		value: value,
		Next:  r.Head,
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

func createKeyId() string {
	now := time.Now().UnixNano()
	h := sha1.New()
	h.Write([]byte(strconv.FormatInt(now, 10)))
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
	// 计算key的hash索引值
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

// getValue 获取value
//
// key : 存储的key
// return : value 返回实际的value值
func (h *HashMapValueBucket) getValue(key string) any {
	index := HashKey(key, h.size)
	return h.table[index].find(key)
}

// DeleteValue 删除value
//
// key : 存储的key
// return : bool 返回是否删除成功
//
//	true : 删除成功
//	false : 删除失败
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

// startExpansion 启动扩容
func (h *HashMapValueBucket) startExpansion() {
	if h.oldTable != nil {
		// 已存在扩容中
		return
	}
	h.oldTable = h.table
	h.size = h.size * 2
	h.table = make([]RowValueLinkList, h.size)
	// 初始化index
	h.rehashIndex = 0
}

// doReHashStep 执行扩容步骤
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
			h.rehashIndex = i + 1 // 下次从下一个桶位开始
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
	// 将值存入指定的分片
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
