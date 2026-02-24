# API Reference

## Cache API

### NewFastCache

```go
cache := src.NewFastCache()
```

Creates a basic FastCache instance with default settings.

### NewRistrettoCache

```go
cache, err := src.NewRistrettoCache(config *Config) (*RistrettoCache, error)
```

Creates a Ristretto cache instance with custom configuration.

**Config Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| MaxCost | int64 | 1GB | Maximum memory cost |
| BufferSize | int | 512MB | Write buffer size |
| ShardCount | int | 8 | Number of shards |
| TTL | time.Duration | 0 | Default TTL |
| MetricsEnabled | bool | false | Enable metrics |

### Set

```go
cache.Set(key string, value interface{}, cost int64) bool
```

Sets a value in the cache.

**Parameters:**
- key: Cache key
- value: Value to store
- cost: Memory cost of this item

**Returns:** true if successfully set

### Get

```go
val := cache.Get(key string) (interface{}, bool)
```

Gets a value from the cache.

**Returns:** (value, found)

### SetM2One

```go
cache.SetM2One(keys []string, value interface{}, cost int64) bool
```

Maps multiple keys to a single value. All keys will return the same value.

### SetM

```go
cache.SetM(items []CacheItem)
```

Batch set multiple items.

### Del

```go
cache.Del(key string) bool
```

Deletes a key from the cache.

### Wait

```go
cache.Wait()
```

Waits for all pending writes to complete.

### Cost

```go
cost := cache.Cost() int64
```

Returns the total memory cost of all items in the cache.

### Clear

```go
cache.Clear()
```

Clears all items from the cache.

---

## Vector Store API

### NewVectorStore

```go
store, err := src.NewVectorStore(config *VectorStoreConfig) (*VectorCache, error)
```

Creates a vector store instance.

**Config Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| IndexType | string | "flat" | Index type: "flat" or "hnsw" |
| Metric | MetricType | MetricL2 | Distance metric |
| MaxCost | int64 | 1GB | Maximum memory cost |
| ShardCount | int | 1 | Number of shards |
| TTL | time.Duration | 0 | Vector TTL |
| HNSW | HNSWConfig | default | HNSW configuration |

### Add

```go
err := store.Add(id string, vector Vector, metadata map[string]any) error
```

Adds a vector to the store.

**Parameters:**
- id: Unique identifier
- vector: Float32 vector
- metadata: Optional metadata

### Get

```go
item, found := store.Get(id string) (*VectorItem, bool)
```

Retrieves a vector by ID.

### Delete

```go
err := store.Delete(id string) error
```

Deletes a vector by ID.

### Search

```go
results, err := store.Search(query Vector, k int) ([]SearchResult, error)
```

Searches for k nearest vectors.

**Returns:** Array of SearchResult sorted by distance

### SearchWithFilter

```go
results, err := store.SearchWithFilter(query Vector, k int, filter FilterFunc) ([]SearchResult, error)
```

Searches with metadata filtering.

**FilterFunc:**
```go
type FilterFunc func(metadata map[string]any) bool
```

### BatchAdd

```go
err := store.BatchAdd(items []VectorItem) error
```

Batch add multiple vectors.

### BatchGet

```go
result := store.BatchGet(ids []string) map[string]*VectorItem
```

Batch get multiple vectors.

### BatchDelete

```go
count := store.BatchDelete(ids []string) int
```

Batch delete vectors.

### ExportToBytes

```go
data, err := store.ExportToBytes() ([]byte, error)
```

Exports vectors to JSON bytes.

### ImportFromBytes

```go
err := store.ImportFromBytes(data []byte) error
```

Imports vectors from JSON bytes.

### Len

```go
count := store.Len() int
```

Returns the number of vectors.

### Cost

```go
cost := store.Cost() int64
```

Returns current memory cost.

### Close

```go
store.Close() error
```

Closes the vector store.

---

## Vector Types

### Vector

```go
type Vector []float32
```

Represents a vector as a slice of float32 values.

### VectorItem

```go
type VectorItem struct {
    ID       string
    Vector   Vector
    Metadata map[string]any
    Cost     int64
}
```

Represents a stored vector with metadata.

### SearchResult

```go
type SearchResult struct {
    ID       string
    Vector   Vector
    Score    float32
    Metadata map[string]any
}
```

Represents a search result.

### MetricType

```go
type MetricType string

const (
    MetricL2     MetricType = "l2"      // Euclidean distance
    MetricCosine MetricType = "cosine"  // Cosine similarity
    MetricIP     MetricType = "ip"       // Inner product
)
```

---

## Distance Functions

### L2Distance

```go
dist := src.L2Distance(v1, v2 Vector) float32
```

Calculates Euclidean distance.

### CosineDistance

```go
dist := src.CosineDistance(v1, v2 Vector) float32
```

Calculates cosine distance (1 - similarity).

### IPDistance

```go
dist := src.IPDistance(v1, v2 Vector) float32
```

Calculates inner product (returns negative for sorting).

---

## HNSW Configuration

### HNSWConfig

```go
type HNSWConfig struct {
    M              int     // Number of connections per node
    EFConstruction int    // Candidate list size during construction
    EFSearch       int    // Candidate list size during search
    LevelMult      float64 // Level multiplier factor
}
```

**Default Configuration:**
```go
func DefaultHNSWConfig() HNSWConfig {
    return HNSWConfig{
        M:              16,
        EFConstruction: 200,
        EFSearch:       50,
        LevelMult:      1 / math.Ln2,
    }
}
```
