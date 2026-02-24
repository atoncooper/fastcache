# FastCache

[![Go Version](https://img.shields.io/github/go-mod/go-version/atoncooper/fastcache)](https://github.com/atoncooper/fastcache)
[![License](https://img.shields.io/github/license/atoncooper/fastcache)](LICENSE)
[![Status](https://img.shields.io/badge/status-production--ready-brightgreen)](https://github.com/atoncooper/fastcache)

FastCache is a high-performance local cache library that supports various cache operations such as storage, retrieval, and expiration-based deletion. It uses a hybrid structure combining hash tables and linked lists to ensure efficient operations in most cases, and supports a sharded cache system. It also provides an in-memory vector database feature that supports vector storage and similarity search.

> **Note**: This project includes both Chinese and English documentation. For Chinese version, see [../README.md](../README.md)

## Features

### Core Cache

| Feature | Description |
|---------|-------------|
| High Performance | Optimized hash table and linked list for fast insert/query |
| Expiration Control | Supports TTL-based cache deletion |
| Batch Operations | Multiple keys to single value mapping |
| Sharded Cache | Hash-based sharding for scalability |
| Sampled LFU | Sampled Least Frequently Used eviction |

### Vector Database

| Feature | Description |
|---------|-------------|
| Vector Storage | In-memory storage, supports billions of vectors |
| Multiple Metrics | L2, Cosine, Inner Product |
| ANN Index | HNSW approximate nearest neighbor search |
| Sharding | Horizontal scaling, high concurrency |
| Metadata | Vector + metadata combined storage |

## Installation

```bash
go get github.com/atoncooper/fastcache
```

## Quick Start

### Initialize Cache

```go
import "github.com/atoncooper/fastcache/src"

cache := src.NewFastCache()
```

### Basic Operations

```go
// Set cache with 10 seconds TTL
cache.Set("key", "value", 10)

// Get cache
value := cache.Get("key")
fmt.Println(value) // Output: value
```

### Batch Operations

```go
// Multiple keys to single value
cache.SetM2One([]string{"key1", "key2"}, "value2", 10)
value := cache.Get("key1")
fmt.Println(value) // Output: value2
```

### Sharded Cache

```go
config := &src.Config{
    ShardCount: 8,
    MaxCost:    1024 * 1024 * 1024, // 1GB per shard
}
cache, _ := src.NewRistrettoCache(config)
```

### Vector Database

```go
// Initialize vector store
config := &src.VectorStoreConfig{
    IndexType:  "flat",    // or "hnsw"
    Metric:     src.MetricL2,
    MaxCost:    1024 * 1024 * 1024, // 1GB
    ShardCount: 4,
}
store, _ := src.NewVectorStore(config)
defer store.Close()

// Add vector
vector := make(src.Vector, 128)
for i := range vector {
    vector[i] = float32(i)
}
store.Add("doc_1", vector, map[string]any{
    "title": "Document 1",
    "category": "tech",
})

// Search
query := make(src.Vector, 128)
results, _ := store.Search(query, 10)
for _, r := range results {
    fmt.Printf("ID: %s, Score: %f\n", r.ID, r.Score)
}
```

## Performance

### Cache Operations

| Operation | Complexity |
|----------|------------|
| Get | O(1) |
| Set | O(1) amortized |
| Del | O(1) |

### Vector Search

| Index Type | Complexity | Use Case |
|------------|------------|----------|
| FlatSearch | O(n) | Small scale < 10K |
| HNSW | O(log n) | Large scale |

## Configuration

### Cache Config

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| MaxCost | int64 | 1GB | Maximum memory cost |
| BufferSize | int | 512MB | Write buffer size |
| ShardCount | int | 8 | Number of shards |
| TTL | time.Duration | 0 | Default TTL |

### VectorStore Config

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| IndexType | string | "flat" | flat or hnsw |
| Metric | string | "l2" | l2, cosine, ip |
| MaxCost | int64 | 1GB | Maximum memory cost |
| ShardCount | int | 1 | Number of shards |

### HNSW Config

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| M | int | 16 | Connections per node |
| EFConstruction | int | 200 | Construction list size |
| EFSearch | int | 50 | Search list size |

## Project Structure

```
fast-cache/
├── src/
│   ├── cache.go           # Core cache implementation
│   ├── ristretto.go      # Ristretto cache
│   ├── sharded.go        # Sharded cache
│   ├── lru.go            # LRU eviction
│   ├── frequency.go      # LFU frequency
│   ├── pool.go           # Memory pool
│   ├── config.go         # Configuration
│   ├── vector.go         # Vector types
│   ├── hnsw.go           # HNSW index
│   └── vector_store.go    # Vector storage
├── test/
│   ├── cache_test.go    # Cache tests
│   └── vector_test.go   # Vector tests
├── docs/                  # Documentation
│   ├── README.md
│   ├── api.md
│   ├── architecture.md
│   ├── getting-started.md
│   └── vector.md
└── README.md
```

## Testing

```bash
# Run all tests
go test ./...

# Run cache tests
go test -v -run "TestCache" ./test/

# Run vector tests
go test -v -run "TestVector" ./test/
```

## Documentation

- [Chinese Documentation](../README.md)
- [API Reference](api.md)
- [Architecture](architecture.md)
- [Getting Started](getting-started.md)
- [Vector Database](vector.md)

## License

MIT License - see [LICENSE](../LICENSE) for details.
