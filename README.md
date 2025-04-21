# FastCache

FastCache 是一个高性能的本地缓存库，支持多种缓存操作，如存储、查询和过期删除。它采用哈希表和链表的混合结构，保证了在大多数情况下的高效性，并且支持分片缓存系统。

## 特性

- **高性能**：通过优化的哈希表和链表实现，实现快速的插入和查询操作。
- **过期控制**：支持基于过期时间的缓存删除。
- **支持批量存储**：通过 `SetM2One` 方法，可以将多个键映射到同一个值，简化了批量操作。同时简化了内存的的占用。
- **分片缓存**：支持通过哈希表分片来提高缓存的扩展性。

## 安装

```bash
go get github.com/atoncooper/fastcache
```

## 使用
### 初始化缓存
~~~go
import "github.com/atoncooper/fastcache/src"
cache := NewFastCache()
~~~
### 多缓存项目
~~~go
cache.SetM2One([]string{"key1", "key2"}, "value2", 10)
value := cache.Get("key1")
fmt.Println(value) // 输出: value2
~~~
### 单缓存设计
~~~go
cache.Set("key", "value", 10)
value := cache.Get("key")
fmt.Println(value) // 输出: value
~~~
## 数据结构
 - KeyMap：用于存储缓存的键和对应的值。采用哈希表结构，提供 O(1) 的查找速度。

 - ValueMap：用于存储缓存的实际值，确保值与键的分离，优化存储空间。

 - 分片设计：为了提高缓存的扩展性，系统支持通过哈希表的分片来管理多个缓存实例。
## 性能
 - 查询性能：对于大多数操作，查询和插入操作都在 O(1) 的时间复杂度内完成。

 - 过期删除：支持基于过期时间的缓存删除，保证缓存不被无谓占用。