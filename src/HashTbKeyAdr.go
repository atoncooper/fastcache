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
		Key:      key,
		value:    value,
		Start:    0,
		ExpireAt: exp,
	}
	// 头插法
	node.Next = k.Head
	k.Head = node
}

func (k *KeyLinkList) find(key string) (string, bool) {
	c := k.Head
	for c != nil {
		if c.Key == key {
			// 检查是否过期
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

// set 插入一个key-value-expire项
func (h *HashMapAkBucket) set(key string, value string, exp int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.doReHashStep()

	if float64(h.count)/float64(h.size) > LoadFactor {
		h.startExpansion()
	}

	index := HashKey(key, h.size)

	// 插入链表
	h.table[index].add(key, value, exp)
	h.count++
	return nil
}

// get 获取值
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
				// 不能在 RLock 下调用 delete，延迟处理
				go h.delete(key) // 异步清理
				return "", false
			}
			value = node.value
			ok = true
			break
		}
		node = node.Next
	}
	return value, ok
}

// delete 删除key
func (h *HashMapAkBucket) delete(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	index := HashKey(key, h.size)
	h.table[index].delete(key)
	h.count--
}

// Rehash 启动扩容
func (h *HashMapAkBucket) startExpansion() {
	if h.oldTable != nil {
		return
	}
	h.oldTable = h.table
	h.size = h.size * 2
	h.table = make([]KeyLinkList, h.size)
	h.rehashIndex = 0
}

// doReHashStep 搬运一步
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
		break // 每次只处理一个 bucket，可调节步长
	}
	if h.rehashIndex >= len(h.oldTable) {
		h.oldTable = nil
		h.rehashIndex = 0
	}
}

// HashKey 哈希函数
func HashKey(key string, size int) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := int(h.Sum32() & 0x7fffffff) // 保证正数
	return hash % size
}

// startGC 启动协程定期清理过期键
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

			// 清理 oldTable 里的（如果正在扩容）
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
