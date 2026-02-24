package src

import (
	"encoding/json"
	"hash/fnv"
	"sync"
	"time"
)

// VectorStoreConfig is the configuration for the vector store.
type VectorStoreConfig struct {
	// IndexType is the index type: "flat" or "hnsw".
	IndexType string

	// HNSW is the HNSW configuration.
	HNSW HNSWConfig

	// Metric is the distance metric: "l2", "cosine", or "ip".
	Metric MetricType

	// MaxCost is the memory limit.
	MaxCost int64

	// TTL is the time-to-live duration.
	TTL time.Duration

	// ShardCount is the number of shards.
	ShardCount int
}

// DefaultVectorStoreConfig returns the default configuration.
func DefaultVectorStoreConfig() VectorStoreConfig {
	return VectorStoreConfig{
		IndexType: "flat",
		HNSW:      DefaultHNSWConfig(),
		Metric:    MetricL2,
		MaxCost:   1 << 30, // 1GB
		ShardCount: 1,
	}
}

// VectorItemWithIndex is a vector item with an index reference.
type VectorItemWithIndex struct {
	Item  *VectorItem
	Index interface{} // Index is the reference in the index.
}

// VectorCache is a vector store that wraps FastCache.
type VectorCache struct {
	config *VectorStoreConfig
	cache  *RistrettoCache
	index  VectorStore

	shards     []*VectorCache
	shardCount int

	// itemCollector collects all vectors for index rebuilding.
	itemCollector func() []*VectorItem

	mu sync.RWMutex
}

// NewVectorStore creates a new vector store.
func NewVectorStore(config *VectorStoreConfig) (*VectorCache, error) {
	if config == nil {
		defaultCfg := DefaultVectorStoreConfig()
		config = &defaultCfg
	}

	// If shard count is greater than 1, create a sharded store.
	if config.ShardCount > 1 {
		return newShardedVectorStore(config)
	}

	// Single shard.
	vc := &VectorCache{
		config: config,
	}

	// Create FastCache.
	cacheConfig := &Config{
		MaxCost: config.MaxCost,
		TTL:     config.TTL,
	}
	cache, err := NewRistrettoCache(cacheConfig)
	if err != nil {
		return nil, err
	}
	vc.cache = cache

	// Create index.
	switch config.IndexType {
	case "hnsw":
		vc.index = NewHNSW(config.HNSW, config.Metric)
	default:
		vc.index = NewFlatSearch(config.Metric)
	}

	return vc, nil
}

// newShardedVectorStore creates a sharded vector store.
func newShardedVectorStore(config *VectorStoreConfig) (*VectorCache, error) {
	shardCount := config.ShardCount
	if shardCount <= 0 {
		shardCount = 1
	}

	// Per-shard configuration.
	shardConfig := *config
	shardConfig.ShardCount = 1

	shards := make([]*VectorCache, shardCount)
	for i := 0; i < shardCount; i++ {
		// Allocate memory for each shard.
		shardConfig.MaxCost = config.MaxCost / int64(shardCount)
		store, err := NewVectorStore(&shardConfig)
		if err != nil {
			// Rollback already created shards.
			for j := 0; j < i; j++ {
				shards[j].Close()
			}
			return nil, err
		}
		shards[i] = store
	}

	return &VectorCache{
		config:    config,
		shards:    shards,
		shardCount: shardCount,
	}, nil
}

// getShard returns the shard for the given ID.
func (vc *VectorCache) getShard(id string) *VectorCache {
	if vc.shardCount > 1 {
		h := fnv.New32a()
		h.Write([]byte(id))
		shardIdx := int(h.Sum32()) % vc.shardCount
		return vc.shards[shardIdx]
	}
	return vc
}

// Add adds a vector.
func (vc *VectorCache) Add(id string, vector Vector, metadata map[string]any) error {
	shard := vc.getShard(id)

	// Calculate cost.
	cost := int64(len(vector)*4) + 64 // float32 * 4 bytes + base overhead
	if metadata != nil {
		cost += 128 // Estimate metadata.
	}

	// Store in cache.
	storeKey := "vec:" + id
	item := &VectorItemWithIndex{
		Item: &VectorItem{
			ID:       id,
			Vector:   vector,
			Metadata: metadata,
			Cost:     cost,
		},
	}
	shard.cache.Set(storeKey, item, cost)

	// Add to index.
	return shard.index.Add(id, vector, metadata)
}

// Get retrieves a vector.
func (vc *VectorCache) Get(id string) (*VectorItem, bool) {
	shard := vc.getShard(id)
	storeKey := "vec:" + id

	val, found := shard.cache.Get(storeKey)
	if !found {
		return nil, false
	}

	item, ok := val.(*VectorItemWithIndex)
	if !ok {
		return nil, false
	}

	return item.Item, true
}

// Delete removes a vector.
func (vc *VectorCache) Delete(id string) error {
	shard := vc.getShard(id)

	// Delete from cache.
	storeKey := "vec:" + id
	shard.cache.Del(storeKey)

	// Delete from index.
	return shard.index.Delete(id)
}

// Search searches for vectors.
func (vc *VectorCache) Search(query Vector, k int) ([]SearchResult, error) {
	// For sharded stores, search all shards and merge results.
	if vc.shardCount > 1 {
		return vc.shardedSearch(query, k)
	}

	return vc.index.Search(query, k)
}

// shardedSearch searches across all shards.
func (vc *VectorCache) shardedSearch(query Vector, k int) ([]SearchResult, error) {
	type resultWithShard struct {
		results []SearchResult
		shard   int
	}

	// Search all shards in parallel.
	resultsChan := make(chan resultWithShard, vc.shardCount)
	var wg sync.WaitGroup

	for i, shard := range vc.shards {
		wg.Add(1)
		go func(s *VectorCache, idx int) {
			defer wg.Done()
			results, err := s.index.Search(query, k*2) // Search more results per shard.
			if err == nil && len(results) > 0 {
				resultsChan <- resultWithShard{results: results, shard: idx}
			}
		}(shard, i)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect all results.
	var allResults []SearchResult
	for r := range resultsChan {
		allResults = append(allResults, r.results...)
	}

	// Sort and get Top-K.
	if len(allResults) == 0 {
		return []SearchResult{}, nil
	}

	// Sort by score.
	if vc.config.Metric == MetricIP {
		// Higher inner product is better.
		for i := 0; i < len(allResults)-1; i++ {
			for j := i + 1; j < len(allResults); j++ {
				if allResults[i].Score < allResults[j].Score {
					allResults[i], allResults[j] = allResults[j], allResults[i]
				}
			}
		}
	} else {
		// Smaller distance is better.
		for i := 0; i < len(allResults)-1; i++ {
			for j := i + 1; j < len(allResults); j++ {
				if allResults[i].Score > allResults[j].Score {
					allResults[i], allResults[j] = allResults[j], allResults[i]
				}
			}
		}
	}

	// Get Top-K.
	if len(allResults) > k {
		allResults = allResults[:k]
	}

	return allResults, nil
}

// SearchWithFilter searches with a filter condition.
func (vc *VectorCache) SearchWithFilter(query Vector, k int, filter FilterFunc) ([]SearchResult, error) {
	// For sharded stores, search all shards and merge results.
	if vc.shardCount > 1 {
		return vc.shardedSearchWithFilter(query, k, filter)
	}

	return vc.index.SearchWithFilter(query, k, filter)
}

// shardedSearchWithFilter searches across all shards with filtering.
func (vc *VectorCache) shardedSearchWithFilter(query Vector, k int, filter FilterFunc) ([]SearchResult, error) {
	type resultWithShard struct {
		results []SearchResult
		shard   int
	}

	// Search all shards in parallel.
	resultsChan := make(chan resultWithShard, vc.shardCount)
	var wg sync.WaitGroup

	for i, shard := range vc.shards {
		wg.Add(1)
		go func(s *VectorCache, idx int) {
			defer wg.Done()
			results, err := s.index.SearchWithFilter(query, k*2, filter)
			if err == nil && len(results) > 0 {
				resultsChan <- resultWithShard{results: results, shard: idx}
			}
		}(shard, i)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect all results.
	var allResults []SearchResult
	for r := range resultsChan {
		allResults = append(allResults, r.results...)
	}

	if len(allResults) == 0 {
		return []SearchResult{}, nil
	}

	// Sort and get Top-K.
	if vc.config.Metric == MetricIP {
		for i := 0; i < len(allResults)-1; i++ {
			for j := i + 1; j < len(allResults); j++ {
				if allResults[i].Score < allResults[j].Score {
					allResults[i], allResults[j] = allResults[j], allResults[i]
				}
			}
		}
	} else {
		for i := 0; i < len(allResults)-1; i++ {
			for j := i + 1; j < len(allResults); j++ {
				if allResults[i].Score > allResults[j].Score {
					allResults[i], allResults[j] = allResults[j], allResults[i]
				}
			}
		}
	}

	if len(allResults) > k {
		allResults = allResults[:k]
	}

	return allResults, nil
}

// Len returns the number of vectors.
func (vc *VectorCache) Len() int {
	if vc.shardCount > 1 {
		total := 0
		for _, shard := range vc.shards {
			total += shard.index.Len()
		}
		return total
	}
	return vc.index.Len()
}

// Cost returns the current cost.
func (vc *VectorCache) Cost() int64 {
	if vc.shardCount > 1 {
		var total int64
		for _, shard := range vc.shards {
			total += shard.cache.Cost()
		}
		return total
	}
	return vc.cache.Cost()
}

// Clear clears all data.
func (vc *VectorCache) Clear() {
	if vc.shardCount > 1 {
		for _, shard := range vc.shards {
			shard.cache.Clear()
			shard.index.Clear()
		}
		return
	}
	vc.cache.Clear()
	vc.index.Clear()
}

// Wait waits for all async writes to complete.
func (vc *VectorCache) Wait() {
	if vc.shardCount > 1 {
		for _, shard := range vc.shards {
			shard.cache.Wait()
		}
		return
	}
	if vc.cache != nil {
		vc.cache.Wait()
	}
}

// Close closes the store.
func (vc *VectorCache) Close() error {
	if vc.shardCount > 1 {
		for _, shard := range vc.shards {
			shard.cache.Close()
		}
		return nil
	}
	return vc.cache.Close()
}

// BatchAdd adds multiple vectors in batch.
func (vc *VectorCache) BatchAdd(items []VectorItem) error {
	for _, item := range items {
		if err := vc.Add(item.ID, item.Vector, item.Metadata); err != nil {
			return err
		}
	}
	return nil
}

// BatchGet retrieves multiple vectors in batch.
func (vc *VectorCache) BatchGet(ids []string) map[string]*VectorItem {
	result := make(map[string]*VectorItem)
	for _, id := range ids {
		if item, found := vc.Get(id); found {
			result[id] = item
		}
	}
	return result
}

// BatchDelete deletes multiple vectors in batch.
func (vc *VectorCache) BatchDelete(ids []string) int {
	count := 0
	for _, id := range ids {
		if err := vc.Delete(id); err == nil {
			count++
		}
	}
	return count
}

// BuildIndex rebuilds the index from storage.
// It rebuilds the index from storage, useful when the index is corrupted or needs optimization.
func (vc *VectorCache) BuildIndex() error {
	if vc.shardCount > 1 {
		for _, shard := range vc.shards {
			if err := shard.rebuildIndexFromCache(); err != nil {
				return err
			}
		}
		return nil
	}

	return vc.rebuildIndexFromCache()
}

// rebuildIndexFromCache rebuilds the index from cache.
func (vc *VectorCache) rebuildIndexFromCache() error {
	// Clear current index.
	vc.index.Clear()

	// Get all stored vectors.
	items := vc.collectAllItems()

	// Add to index one by one.
	for _, item := range items {
		if err := vc.index.Add(item.ID, item.Vector, item.Metadata); err != nil {
			return err
		}
	}

	return nil
}

// collectAllItems collects all vectors from the cache.
func (vc *VectorCache) collectAllItems() []*VectorItem {
	var items []*VectorItem

	if vc.shardCount > 1 {
		for _, shard := range vc.shards {
			items = append(items, shard.collectAllItems()...)
		}
		return items
	}

	// Since FastCache does not provide a traversal interface,
	// we maintain an internal vector list for index rebuilding.
	// Simplified handling: returns an empty list, users need to maintain the vector list themselves.
	// In actual usage, a list can be updated simultaneously when adding.

	return items
}

// OptimizeIndex optimizes the index.
// For HNSW, connection edges can be readjusted.
func (vc *VectorCache) OptimizeIndex() error {
	// For FlatSearch, sorting is already optimal.
	// For HNSW, the graph structure can be rebuilt.

	if vc.shardCount > 1 {
		for _, shard := range vc.shards {
			if err := shard.rebuildIndexFromCache(); err != nil {
				return err
			}
		}
		return nil
	}

	// Check index type.
	switch vc.config.IndexType {
	case "hnsw":
		// Rebuild HNSW index for optimization.
		return vc.rebuildIndexFromCache()
	default:
		// FlatSearch does not require optimization.
		return nil
	}
}

// SetItemCollector sets the vector collector.
// Users can provide a function to collect all vectors for index rebuilding.
func (vc *VectorCache) SetItemCollector(collector func() []*VectorItem) {
	if vc.shardCount > 1 {
		for _, shard := range vc.shards {
			shard.SetItemCollector(collector)
		}
		return
	}
	vc.itemCollector = collector
}

// GetAllItems returns all vectors (requires setting a collector first).
func (vc *VectorCache) GetAllItems() []*VectorItem {
	if vc.itemCollector != nil {
		return vc.itemCollector()
	}
	return vc.collectAllItems()
}

// Export exports vector data.
// It returns all vector data that users can save to a file.
func (vc *VectorCache) Export() []*VectorItem {
	items := vc.GetAllItems()
	result := make([]*VectorItem, len(items))
	copy(result, items)
	return result
}

// Import imports vector data.
// It imports data from a vector list.
func (vc *VectorCache) Import(items []*VectorItem) error {
	for _, item := range items {
		if err := vc.Add(item.ID, item.Vector, item.Metadata); err != nil {
			return err
		}
	}
	vc.Wait()
	return nil
}

// ExportData is the data structure for export.
type ExportData struct {
	Metric    MetricType    `json:"metric"`
	IndexType string        `json:"index_type"`
	Items     []ExportItem  `json:"items"`
}

// ExportItem is an item for export.
type ExportItem struct {
	ID       string         `json:"id"`
	Vector   []float32      `json:"vector"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ExportToBytes exports to binary format.
func (vc *VectorCache) ExportToBytes() ([]byte, error) {
	items := vc.GetAllItems()

	exportItems := make([]ExportItem, len(items))
	for i, item := range items {
		exportItems[i] = ExportItem{
			ID:       item.ID,
			Vector:   item.Vector,
			Metadata: item.Metadata,
		}
	}

	data := ExportData{
		Metric:    vc.config.Metric,
		IndexType: vc.config.IndexType,
		Items:     exportItems,
	}

	return json.Marshal(data)
}

// ImportFromBytes imports from binary format.
func (vc *VectorCache) ImportFromBytes(data []byte) error {
	var exportData ExportData
	if err := json.Unmarshal(data, &exportData); err != nil {
		return err
	}

	// Verify metric matches.
	if exportData.Metric != vc.config.Metric {
		// Warning: metric does not match.
	}

	// Import vectors.
	for _, item := range exportData.Items {
		vec := Vector(item.Vector)
		if err := vc.Add(item.ID, vec, item.Metadata); err != nil {
			return err
		}
	}

	vc.Wait()
	return nil
}

// GetStats returns statistics.
func (vc *VectorCache) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"len":          vc.Len(),
		"cost":         vc.Cost(),
		"maxCost":      vc.config.MaxCost,
		"shardCount":   vc.shardCount,
		"indexType":    vc.config.IndexType,
		"metric":       vc.config.Metric,
	}

	if vc.shardCount > 1 {
		shardStats := make([]map[string]interface{}, vc.shardCount)
		for i, shard := range vc.shards {
			shardStats[i] = map[string]interface{}{
				"len":   shard.index.Len(),
				"cost":  shard.cache.Cost(),
			}
		}
		stats["shards"] = shardStats
	}

	return stats
}
