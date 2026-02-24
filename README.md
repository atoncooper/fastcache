# FastCache

[![Go Version](https://img.shields.io/github/go-mod/go-version/atoncooper/fastcache)](https://github.com/atoncooper/fastcache)
[![License](https://img.shields.io/github/license/atoncooper/fastcache)](LICENSE)
[![Status](https://img.shields.io/badge/status-production--ready-brightgreen)](https://github.com/atoncooper/fastcache)

FastCache 是一个高性能的本地缓存库，支持多种缓存操作，如存储、查询和过期删除。它采用哈希表和链表的混合结构，保证了在大多数情况下的高效性，并且支持分片缓存系统。同时还提供内存向量数据库功能，支持向量存储和相似度搜索。

> **注意**: 本项目包含中文和英文文档。英文版本请参阅 [docs/README.md](docs/README.md)

## 特性

### 核心缓存

| 特性 | 描述 |
|------|------|
| 高性能 | 通过优化的哈希表和链表实现，实现快速的插入和查询操作 |
| 过期控制 | 支持基于过期时间的缓存删除 |
| 批量存储 | 支持多个键映射到同一个值，简化批量操作 |
| 分片缓存 | 支持通过哈希表分片来提高缓存的扩展性 |
| LFU 采样 | 支持采样 LFU 淘汰策略 |

### 向量数据库

| 特性 | 描述 |
|------|------|
| 向量存储 | 高性能内存向量存储，支持数十亿向量规模 |
| 多种相似度度量 | 欧氏距离 (L2)、余弦相似度 (Cosine)、内积 (IP) |
| ANN 索引 | 支持 HNSW 近似最近邻搜索 |
| 分片支持 | 水平扩展，支持高并发 |
| 元数据存储 | 支持向量 + 元数据的联合存储 |

## 安装

```bash
go get github.com/atoncooper/fastcache
```

## 快速开始

### 初始化缓存

```go
import "github.com/atoncooper/fastcache/src"

cache := src.NewFastCache()
```

### 基本缓存操作

```go
// 设置缓存，10 秒过期
cache.Set("key", "value", 10)

// 获取缓存
value := cache.Get("key")
fmt.Println(value) // 输出: value
```

### 批量操作

```go
// 多键单值映射
cache.SetM2One([]string{"key1", "key2"}, "value2", 10)
value := cache.Get("key1")
fmt.Println(value) // 输出: value2
```

### 分片缓存

```go
config := &src.Config{
    ShardCount: 8,
    MaxCost:    1024 * 1024 * 1024, // 1GB per shard
}
cache, _ := src.NewRistrettoCache(config)
```

### 向量数据库

```go
// 初始化向量存储
config := &src.VectorStoreConfig{
    IndexType:  "flat",    // 或 "hnsw"
    Metric:     src.MetricL2,
    MaxCost:    1024 * 1024 * 1024, // 1GB
    ShardCount: 4,
}
store, _ := src.NewVectorStore(config)
defer store.Close()

// 添加向量
vector := make(src.Vector, 128)
for i := range vector {
    vector[i] = float32(i)
}
store.Add("doc_1", vector, map[string]any{
    "title": "Document 1",
    "category": "tech",
})

// 搜索向量
query := make(src.Vector, 128)
results, _ := store.Search(query, 10)
for _, r := range results {
    fmt.Printf("ID: %s, Score: %f\n", r.ID, r.Score)
}
```

## 性能对比

### 缓存性能

| 操作 | 时间复杂度 |
|------|----------|
| Get | O(1) |
| Set | O(1) 均摊 |
| Del | O(1) |

### 向量搜索性能

| 索引类型 | 搜索复杂度 | 适用场景 |
|----------|------------|----------|
| FlatSearch | O(n) | 小规模数据 < 10K |
| HNSW | O(log n) | 大规模数据 |

## 配置选项

### 缓存配置

| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| MaxCost | int64 | 1GB | 最大内存成本 |
| BufferSize | int | 512MB | 写入缓冲大小 |
| ShardCount | int | 8 | 分片数量 |
| TTL | time.Duration | 0 | 默认过期时间 |

### 向量存储配置

| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| IndexType | string | "flat" | 索引类型: flat, hnsw |
| Metric | string | "l2" | 距离度量: l2, cosine, ip |
| MaxCost | int64 | 1GB | 最大内存成本 |
| ShardCount | int | 1 | 分片数量 |

### HNSW 配置

| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| M | int | 16 | 每个节点的边数 |
| EFConstruction | int | 200 | 建图时候选列表大小 |
| EFSearch | int | 50 | 搜索时候选列表大小 |

## 项目结构

```
fast-cache/
├── src/
│   ├── cache.go           # 缓存核心实现
│   ├── ristretto.go      # Ristretto 缓存实现
│   ├── sharded.go        # 分片缓存
│   ├── lru.go           # LRU 淘汰策略
│   ├── frequency.go      # LFU 频率统计
│   ├── pool.go          # 内存池
│   ├── config.go        # 配置定义
│   ├── vector.go        # 向量类型和距离计算
│   ├── hnsw.go          # HNSW 索引实现
│   └── vector_store.go  # 向量存储层
├── test/
│   ├── cache_test.go    # 缓存测试
│   └── vector_test.go   # 向量测试
├── docs/                 # 英文文档
│   ├── README.md
│   ├── api.md
│   ├── architecture.md
│   ├── getting-started.md
│   └── vector.md
└── README.md
```

## 测试

```bash
# 运行所有测试
go test ./...

# 运行缓存测试
go test -v -run "TestCache" ./test/

# 运行向量测试
go test -v -run "TestVector" ./test/
```

## 文档

- [English Documentation](docs/README.md)
- [API 参考](docs/api.md)
- [架构设计](docs/architecture.md)
- [快速开始](docs/getting-started.md)
- [向量数据库](docs/vector.md)

## 许可证

MIT License - see [LICENSE](LICENSE) for details.
