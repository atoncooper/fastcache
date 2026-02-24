# Vector Database

FastCache provides a high-performance in-memory vector database with support for vector storage and similarity search.

## Overview

The vector database feature allows you to:
- Store billions of vectors in memory
- Search for similar vectors using various distance metrics
- Use HNSW index for approximate nearest neighbor search
- Scale horizontally with sharding
- Store metadata alongside vectors

## Creating a Vector Store

### Basic Configuration

```go
config := &src.VectorStoreConfig{
    IndexType:  "flat",    // or "hnsw"
    Metric:     src.MetricL2,
    MaxCost:    1024 * 1024 * 1024, // 1GB
    ShardCount: 1,
}
store, _ := src.NewVectorStore(config)
defer store.Close()
```

### HNSW Configuration

```go
hnswConfig := src.HNSWConfig{
    M:              16,     // Connections per node
    EFConstruction: 200,   // Construction candidate list size
    EFSearch:       50,    // Search candidate list size
}

config := &src.VectorStoreConfig{
    IndexType:  "hnsw",
    Metric:     src.MetricL2,
    MaxCost:    10 * 1024 * 1024 * 1024, // 10GB
    HNSW:       hnswConfig,
}
```

## Adding Vectors

### Single Vector

```go
// Create a 128-dimensional vector
vector := make(src.Vector, 128)
for i := range vector {
    vector[i] = float32(i)
}

// Add with metadata
store.Add("doc_1", vector, map[string]any{
    "title": "Document 1",
    "category": "tech",
    "tags": []string{"ai", "ml"},
})
```

### Batch Vectors

```go
items := []src.VectorItem{
    {
        ID:       "doc_1",
        Vector:   vector1,
        Metadata: map[string]any{"category": "a"},
    },
    {
        ID:       "doc_2",
        Vector:   vector2,
        Metadata: map[string]any{"category": "b"},
    },
}
store.BatchAdd(items)
```

## Searching Vectors

### Basic Search

```go
// Create query vector
query := make(src.Vector, 128)
// ... fill query ...

// Search for top 10 similar vectors
results, err := store.Search(query, 10)
if err != nil {
    // handle error
}

for _, r := range results {
    fmt.Printf("ID: %s, Score: %f\n", r.ID, r.Score)
}
```

### Search with Filter

```go
results, err := store.SearchWithFilter(query, 10, func(m map[string]any) bool {
    // Only return vectors with category "tech"
    if cat, ok := m["category"].(string); ok {
        return cat == "tech"
    }
    return false
})
```

### Search Options

**Distance Metrics:**

| Metric | Description | Best For |
|--------|-------------|----------|
| MetricL2 | Euclidean distance | General purpose |
| MetricCosine | Cosine similarity | Text embeddings |
| MetricIP | Inner product | Recommendations |

**Index Types:**

| Index | Search Complexity | Use Case |
|-------|-------------------|----------|
| flat | O(n) | Small datasets < 10K |
| hnsw | O(log n) | Large datasets |

## Flat Search vs HNSW

### Flat Search

- Linear scan through all vectors
- Exact nearest neighbor results
- Best for small datasets (< 10,000 vectors)
- No index building time

```go
config := &src.VectorStoreConfig{
    IndexType: "flat",
    Metric:    src.MetricL2,
}
```

### HNSW (Hierarchical Navigable Small World)

- Graph-based approximate nearest neighbor search
- Sub-linear search time
- Configurable accuracy/speed tradeoff
- Best for large datasets (> 10,000 vectors)

```go
config := &src.VectorStoreConfig{
    IndexType: "hnsw",
    Metric:    src.MetricL2,
    HNSW: src.HNSWConfig{
        M:           16,     // Higher = more accurate, slower
        EFSearch:    50,     // Higher = more accurate, slower
    },
}
```

## Sharding

For horizontal scaling, use sharded vector store:

```go
config := &src.VectorStoreConfig{
    IndexType:  "hnsw",
    Metric:    src.MetricL2,
    MaxCost:    10 * 1024 * 1024 * 1024, // 10GB total
    ShardCount: 8,                        // 8 shards
}
store, _ := src.NewVectorStore(config)
```

Sharding automatically distributes vectors across shards based on ID hash.

## Persistence

### Export

```go
// Export to JSON bytes
data, err := store.ExportToBytes()
if err != nil {
    // handle error
}

// Save to file
ioutil.WriteFile("vectors.json", data, 0644)
```

### Import

```go
// Read from file
data, err := ioutil.ReadFile("vectors.json")
if err != nil {
    // handle error
}

// Import
err = store.ImportFromBytes(data)
if err != nil {
    // handle error
}
```

### Export Format

```json
{
    "metric": "l2",
    "index_type": "hnsw",
    "items": [
        {
            "id": "doc_1",
            "vector": [0.1, 0.2, ...],
            "metadata": {"category": "tech"}
        }
    ]
}
```

## Memory Management

### Cost Calculation

Each vector costs:
- Vector data: `dimensions * 4` bytes (float32)
- Metadata: ~128 bytes (estimated)
- Index overhead: varies by index type

```go
config := &src.VectorStoreConfig{
    MaxCost: 1024 * 1024 * 1024, // 1GB limit
}
```

When memory exceeds limit, least recently used vectors are evicted.

## Performance Tips

1. **Choose right index type**
   - < 10K vectors: use flat
   - > 10K vectors: use hnsw

2. **Set appropriate memory limit**
   - Set MaxCost based on available memory
   - Leave headroom for index overhead

3. **Use sharding for scale**
   - Use 4-8 shards for better concurrency
   - Each shard operates independently

4. **Batch operations**
   - Use BatchAdd for bulk insertions
   - Reduces individual operation overhead

5. **Filter early**
   - Use SearchWithFilter to reduce result set
   - More efficient than filtering after search

## Examples

### Semantic Search

```go
// Store document embeddings
dim := 768 // BERT dimension
for _, doc := range documents {
    embedding := getEmbedding(doc.Text) // vector(dim)
    store.Add(doc.ID, embedding, map[string]any{
        "text": doc.Text,
    })
}

// Search
queryEmbedding := getEmbedding(userQuery)
results, _ := store.Search(queryEmbedding, 5)
```

### Recommendation System

```go
// Store user/item embeddings
store.Add("user_1", userEmbedding, map[string]any{
    "type": "user",
})

// Find similar users or items
results, _ := store.SearchWithFilter(query, 10, func(m map[string]any) bool {
    return m["type"] == "item"
})
```

### Image Retrieval

```go
// Store image features
store.Add("img_1", features, map[string]any{
    "url": "https://...",
    "tags": []string{"nature", "landscape"},
})

// Search with tag filter
results, _ := store.SearchWithFilter(query, 10, func(m map[string]any) bool {
    tags, _ := m["tags"].([]string)
    for _, t := range tags {
        if t == "landscape" {
            return true
        }
    }
    return false
})
```
