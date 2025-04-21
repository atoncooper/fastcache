package src

import (
	"time"
)

type FastCache struct {
	KeyMap   *ShardedCache
	ValueMap *ShardedCacheValue
}

func NewFastCache() *FastCache {
	fc := &FastCache{
		KeyMap:   NewShardedCache(512),
		ValueMap: NewShardedCacheRowValue(512),
	}
	fc.KeyMap.StartGC(10 * time.Second)
	return fc
}

func (fc *FastCache) Set(key string, value any, exp time.Duration) {
	// 先存储value在存储key
	keyValue := fc.ValueMap.SetValue(value)
	expTime := time.Now().UnixNano() + int64(exp)
	fc.KeyMap.Set(key, keyValue, expTime)
}

func (fc *FastCache) Get(key string) any {
	KeyValue, ok := fc.KeyMap.Get(key)
	if ok == false && KeyValue == "" {
		return nil
	}
	value := fc.ValueMap.GetValue(KeyValue)
	if value == nil {
		// 如果value不存在，删除key
		fc.KeyMap.Delete(key)
		return nil
	}
	return value
}

func (fc *FastCache) Delete(key string) {
	// 先删除key在删除value
	KeyValue, ok := fc.KeyMap.Get(key)
	if ok == false && KeyValue == "" {
		return
	}
	fc.KeyMap.Delete(key)
	fc.ValueMap.DeleteValue(KeyValue)
}

func (fc *FastCache) SetM2One(key []string, value any, exp time.Duration) {
	keyValue := fc.ValueMap.SetValue(value)
	expTime := time.Now().UnixNano() + int64(exp)
	// 将多个key映射到同一个value
	for _, k := range key {
		fc.KeyMap.Set(k, keyValue, expTime)
	}
}
