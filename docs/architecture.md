# Architecture

This document describes the internal architecture of FastCache.

## Overview

FastCache is designed with two main components:
1. **Cache Layer**: High-performance key-value cache
2. **Vector Layer**: In-memory vector database for similarity search

## Cache Architecture

### Core Components

```
FastCache
├── KeyMap       (Hash table for key lookup)
├── ValueMap     (Separate value storage)
├── LRU List     (Least Recently Used eviction)
└── Write Buffer (Async write queue)
```

### KeyMap

KeyMap is a hash table that stores:
- Key string
- Pointer to value
- Pointer to LRU list node
- Flags and metadata

Provides O(1) key lookup and insertion.

### ValueMap

ValueMap stores the actual values separately from keys:
- Reduces memory duplication when multiple keys point to same value
- Enables efficient value sharing (SetM2One feature)

### LRU Eviction

When memory cost exceeds MaxCost:
1. Items are sorted by last access time
2. Least recently used items are evicted
3. Space is freed for new items

### Sharded Cache

For high concurrency, cache is divided into shards:

```
ShardedCache
├── Shard 0  (independent cache)
├── Shard 1  (independent cache)
├── ...      (...)
└── Shard N  (independent cache)
```

Each shard:
- Has its own KeyMap, ValueMap, LRU
- Operates independently
- Reduces lock contention

## Vector Architecture

### Components

```
VectorStore
├── RistrettoCache  (Vector data storage)
├── Index           (Search index: Flat or HNSW)
└── Metadata        (Optional metadata)
```

### Index Types

#### FlatSearch

Brute-force linear scan:
- O(n) search complexity
- Exact nearest neighbors
- Simple implementation
- Best for small datasets

#### HNSW Index

Hierarchical Navigable Small World graph:

```
Level 3: [Entry Point] ----> [Node A] ----> [Node B]
                         |
Level 2:                +----> [Node C] ----> [Node D]
                         |
Level 1:                +----> [Node E] ----> [Node F]
                         |
Level 0:                +----> [Node G] ----> [Node H] ----> ...
```

**Properties:**
- Multi-layer graph structure
- Higher layers have fewer nodes, longer edges
- Lower layers have more nodes, shorter edges
- Entry point at highest layer

**Search Algorithm:**
1. Start from entry point at highest level
2. Greedy traverse to nearest neighbor
3. Drop to lower level at each step
4. Continue until level 0

**Construction:**
1. Generate random level for new node
2. Search to find insertion point at each level
3. Add edges to nearest neighbors
4. Prune edges to maintain M connections

### Distance Metrics

#### L2 Distance (Euclidean)

```
d = sqrt(sum((x_i - y_i)^2))
```

- Range: [0, infinity)
- Best for: General similarity

#### Cosine Distance

```
d = 1 - (x . y) / (||x|| * ||y||)
```

- Range: [0, 2]
- Best for: Text embeddings, directional similarity

#### Inner Product

```
d = -(x . y)
```

- Range: [-infinity, infinity)
- Best for: Recommendations, unnormalized embeddings

Note: Returns negative value so larger inner product = better match.

## Memory Management

### Cost Model

Each item has a cost value:
- Key-value cache: configurable cost per item
- Vector store: `dimensions * 4 + overhead`

### Eviction Policy

1. **LRU (Least Recently Used)**
   - Default policy
   - Evicts least recently accessed items
   - Good for temporal access patterns

2. **Sampled LFU (Least Frequently Used)**
   - Samples random items
   - Evicts least frequently used
   - Good for popular items

### Memory Limits

- MaxCost: Maximum memory budget
- When exceeded, eviction is triggered
- Write buffer size affects eviction frequency

## Concurrency

### Lock Strategy

- Per-shard locking for sharded cache
- Read-write locks for index
- Minimal lock hold time

### Async Writes

- Writes go to buffer first
- Background goroutine processes buffer
- Wait() ensures all writes complete

## Performance Characteristics

### Cache

| Operation | Complexity |
|-----------|------------|
| Get | O(1) |
| Set | O(1) amortized |
| Del | O(1) |
| Eviction | O(n) |

### Vector Search

| Index | Add | Search | Memory |
|-------|-----|--------|--------|
| Flat | O(1) | O(n) | Low |
| HNSW | O(M log n) | O(M log n) | Medium |

Where M = number of connections (EF)

## Zero-GC Design

To minimize garbage collection overhead:

### ByteSlicePool

Reuses byte slices instead of allocating new ones:

```go
buf := ByteSlicePool.Get(size)
defer ByteSlicePool.Put(buf)
// use buf
```

### StringBuilderPool

Reuses strings.Builder for string concatenation:

```go
sb := StringBuilderPool.Get()
defer StringBuilderPool.Put(sb)
// use sb
```

## File Structure

```
src/
├── cache.go           # Basic cache implementation
├── ristretto.go       # Ristretto cache (high perf)
├── sharded.go         # Sharded cache wrapper
├── lru.go            # LRU list implementation
├── frequency.go       # LFU frequency tracking
├── pool.go           # Memory pools
├── config.go         # Configuration types
├── vector.go         # Vector types & distance
├── hnsw.go           # HNSW index
└── vector_store.go   # Vector storage layer
```

## Configuration Best Practices

### Cache

- Set MaxCost to available memory minus overhead
- Use ShardCount = CPU cores for best concurrency
- BufferSize = MaxCost / 4 to 2

### Vector Store

- Flat: Use for < 10K vectors
- HNSW: Use M=16, EFSearch=50 for balanced performance
- Set MaxCost with headroom for index structure
