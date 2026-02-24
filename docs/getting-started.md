# Getting Started

This guide will help you get started with FastCache.

## Installation

```bash
go get github.com/atoncooper/fastcache
```

## Quick Start

### Basic Cache Usage

```go
package main

import (
    "fmt"
    "github.com/atoncooper/fastcache/src"
)

func main() {
    // Create cache
    cache := src.NewFastCache()

    // Set a value with 10 seconds TTL
    cache.Set("key", "value", 10)

    // Get the value
    val := cache.Get("key")
    fmt.Println(val)
}
```

### Using Ristretto Cache

Ristretto is a high-performance concurrent cache.

```go
config := &src.Config{
    MaxCost:    1024 * 1024 * 1024, // 1GB
    BufferSize: 512 * 1024 * 1024,  // 512MB
    ShardCount: 8,
}

cache, err := src.NewRistrettoCache(config)
if err != nil {
    panic(err)
}
defer cache.Close()

// Set
cache.Set("key", "value", 60)

// Get
if val, found := cache.Get("key"); found {
    fmt.Println(val)
}
```

### Batch Operations

```go
// Multiple keys to single value
cache.SetM2One([]string{"key1", "key2"}, "shared_value", 60)

// Multiple keys to multiple values
items := []src.CacheItem{
    {Key: "a", Value: "1", Cost: 1},
    {Key: "b", Value: "2", Cost: 1},
}
cache.SetM(items)
```

## Vector Database Quick Start

```go
// Create vector store
config := &src.VectorStoreConfig{
    IndexType:  "hnsw",           // or "flat"
    Metric:     src.MetricL2,     // l2, cosine, ip
    MaxCost:    1024 * 1024 * 1024,
    ShardCount: 4,
}

store, err := src.NewVectorStore(config)
if err != nil {
    panic(err)
}
defer store.Close()

// Add vectors
dim := 128
for i := 0; i < 1000; i++ {
    vec := make(src.Vector, dim)
    for j := 0; j < dim; j++ {
        vec[j] = float32(i * j)
    }
    store.Add(fmt.Sprintf("doc_%d", i), vec, map[string]any{
        "index": i,
    })
}

// Search
query := make(src.Vector, dim)
// ... fill query vector ...

results, _ := store.Search(query, 10)
for _, r := range results {
    fmt.Printf("ID: %s, Score: %f\n", r.ID, r.Score)
}
```

## Next Steps

- Read [API Reference](api.md) for detailed documentation
- Read [Vector Database](vector.md) for vector search features
- Read [Architecture](architecture.md) for internal design details
