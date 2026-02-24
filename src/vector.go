package src

import (
	"fmt"
	"math"
	"sync"
)

// Vector represents a vector of float32 values.
type Vector []float32

// ErrDimensionMismatch is returned when vector dimensions do not match.
var ErrDimensionMismatch = fmt.Errorf("vector dimension mismatch")

// VectorError represents an error that occurred during a vector operation.
type VectorError struct {
	Op  string
	Err error
}

func (e *VectorError) Error() string {
	return fmt.Sprintf("vector %s: %v", e.Op, e.Err)
}

// VectorItem represents a vector element with its metadata.
type VectorItem struct {
	ID       string
	Vector   Vector
	Metadata map[string]any
	Cost     int64 // Memory cost in bytes.
}

// SearchResult represents a search result from vector similarity search.
type SearchResult struct {
	ID       string
	Vector   Vector
	Score    float32
	Metadata map[string]any
}

// DistanceFunc is a function that computes the distance between two vectors.
type DistanceFunc func(v1, v2 Vector) float32

// MetricType represents the type of distance metric used for similarity search.
type MetricType string

const (
	MetricL2     MetricType = "l2"      // L2 (Euclidean) distance.
	MetricCosine MetricType = "cosine"  // Cosine similarity.
	MetricIP     MetricType = "ip"      // Inner product.
)

// GetDistanceFunc returns the distance function for the given metric type.
func GetDistanceFunc(metric MetricType) DistanceFunc {
	switch metric {
	case MetricL2:
		return L2Distance
	case MetricCosine:
		return CosineDistance
	case MetricIP:
		return IPDistance
	default:
		return L2Distance
	}
}

// MaxFloat32 is the maximum value used to represent invalid distance calculations.
const MaxFloat32 = float32(1e38)

// L2Distance computes the Euclidean distance between two vectors.
// It returns MaxFloat32 if the vectors have different dimensions.
func L2Distance(v1, v2 Vector) float32 {
	if len(v1) != len(v2) {
		return MaxFloat32
	}
	var sum float64 = 0
	for i := 0; i < len(v1); i++ {
		diff := float64(v1[i]) - float64(v2[i])
		sum += diff * diff
	}
	return float32(math.Sqrt(sum))
}

// L2DistanceSquared computes the squared Euclidean distance between two vectors.
// This is faster than L2Distance and can be used for comparisons.
// It returns MaxFloat32 if the vectors have different dimensions.
func L2DistanceSquared(v1, v2 Vector) float32 {
	if len(v1) != len(v2) {
		return MaxFloat32
	}
	var sum float64 = 0
	for i := 0; i < len(v1); i++ {
		diff := float64(v1[i]) - float64(v2[i])
		sum += diff * diff
	}
	return float32(sum)
}

// CosineDistance computes the cosine distance between two vectors.
// It returns 1 - similarity, with range [0, 2].
// It returns MaxFloat32 if the vectors have different dimensions.
func CosineDistance(v1, v2 Vector) float32 {
	if len(v1) != len(v2) {
		return MaxFloat32
	}
	dot := float64(0)
	norm1 := float64(0)
	norm2 := float64(0)
	for i := 0; i < len(v1); i++ {
		dot += float64(v1[i]) * float64(v2[i])
		norm1 += float64(v1[i]) * float64(v1[i])
		norm2 += float64(v2[i]) * float64(v2[i])
	}
	if norm1 == 0 || norm2 == 0 {
		return 1.0 // Returns maximum distance for zero vectors.
	}
	similarity := dot / (math.Sqrt(norm1) * math.Sqrt(norm2))
	return float32(1.0 - similarity)
}

// CosineSimilarity computes the cosine similarity between two vectors.
// The result ranges from -1 to 1.
// It returns 0 if the vectors have different dimensions.
func CosineSimilarity(v1, v2 Vector) float32 {
	if len(v1) != len(v2) {
		return 0
	}
	dot := float64(0)
	norm1 := float64(0)
	norm2 := float64(0)
	for i := 0; i < len(v1); i++ {
		dot += float64(v1[i]) * float64(v2[i])
		norm1 += float64(v1[i]) * float64(v1[i])
		norm2 += float64(v2[i]) * float64(v2[i])
	}
	if norm1 == 0 || norm2 == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(norm1) * math.Sqrt(norm2)))
}

// IPDistance computes the inner product between two vectors.
// It returns the negative of the inner product so that larger inner products
// appear first when sorting (i.e., better matches have higher scores).
// It returns MaxFloat32 if the vectors have different dimensions.
func IPDistance(v1, v2 Vector) float32 {
	if len(v1) != len(v2) {
		return MaxFloat32
	}
	var sum float64 = 0
	for i := 0; i < len(v1); i++ {
		sum += float64(v1[i]) * float64(v2[i])
	}
	return float32(-sum) // Negative so larger inner products rank higher.
}

// scoredItem is an internal type that pairs a vector item with its computed score.
type scoredItem struct {
	id    string
	item  *VectorItem
	score float32
}

// FilterFunc is a function that determines whether a vector's metadata meets certain criteria.
type FilterFunc func(metadata map[string]any) bool

// VectorStore is the interface for vector storage and retrieval implementations.
type VectorStore interface {
	Add(id string, vector Vector, metadata map[string]any) error
	Get(id string) (*VectorItem, bool)
	Delete(id string) error
	Search(query Vector, k int) ([]SearchResult, error)
	SearchWithFilter(query Vector, k int, filter FilterFunc) ([]SearchResult, error)
	Len() int
	Clear()
}

// FlatSearch is a brute-force vector search implementation that scans all vectors.
type FlatSearch struct {
	mu       sync.RWMutex
	items    map[string]*VectorItem
	metric   MetricType
	distance DistanceFunc
}

// NewFlatSearch creates a new FlatSearch instance with the specified distance metric.
func NewFlatSearch(metric MetricType) *FlatSearch {
	return &FlatSearch{
		items:    make(map[string]*VectorItem),
		metric:   metric,
		distance: GetDistanceFunc(metric),
	}
}

// Add inserts a vector into the store.
func (f *FlatSearch) Add(id string, vector Vector, metadata map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	item := &VectorItem{
		ID:       id,
		Vector:   vector,
		Metadata: metadata,
		Cost:     int64(len(vector) * 4), // float32 occupies 4 bytes.
	}
	f.items[id] = item
	return nil
}

// Get retrieves a vector by its ID.
func (f *FlatSearch) Get(id string) (*VectorItem, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	item, found := f.items[id]
	if !found {
		return nil, false
	}
	return item, true
}

// Delete removes a vector from the store.
func (f *FlatSearch) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.items, id)
	return nil
}

// Search performs a brute-force search for the k nearest vectors to the query.
func (f *FlatSearch) Search(query Vector, k int) ([]SearchResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.items) == 0 {
		return []SearchResult{}, nil
	}

	if k <= 0 {
		k = 10
	}
	if k > len(f.items) {
		k = len(f.items)
	}

	// Compute distances for all vectors.
	results := make([]scoredItem, 0, len(f.items))
	for id, item := range f.items {
		score := f.distance(query, item.Vector)
		results = append(results, scoredItem{id: id, item: item, score: score})
	}

	// Sort by score (ascending for distance metrics, descending for inner product).
	if f.metric == MetricIP {
		// Inner product uses negative values, so larger is better.
		quickSortDesc(results, 0, len(results)-1)
	} else {
		quickSortAsc(results, 0, len(results)-1)
	}

	// Return the top k results.
	topK := make([]SearchResult, 0, k)
	for i := 0; i < k; i++ {
		topK = append(topK, SearchResult{
			ID:       results[i].item.ID,
			Vector:   results[i].item.Vector,
			Score:    results[i].score,
			Metadata: results[i].item.Metadata,
		})
	}

	return topK, nil
}

// SearchWithFilter performs a search with metadata filtering.
func (f *FlatSearch) SearchWithFilter(query Vector, k int, filter FilterFunc) ([]SearchResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.items) == 0 {
		return []SearchResult{}, nil
	}

	if k <= 0 {
		k = 10
	}

	// Filter items that match the filter criteria.
	filteredItems := make([]scoredItem, 0, len(f.items))
	for id, item := range f.items {
		// Apply filter function if provided.
		if filter != nil && !filter(item.Metadata) {
			continue
		}
		score := f.distance(query, item.Vector)
		filteredItems = append(filteredItems, scoredItem{id: id, item: item, score: score})
	}

	if len(filteredItems) == 0 {
		return []SearchResult{}, nil
	}

	if k > len(filteredItems) {
		k = len(filteredItems)
	}

	// Sort the filtered results.
	if f.metric == MetricIP {
		quickSortDesc(filteredItems, 0, len(filteredItems)-1)
	} else {
		quickSortAsc(filteredItems, 0, len(filteredItems)-1)
	}

	// Return the top k results.
	topK := make([]SearchResult, 0, k)
	for i := 0; i < k; i++ {
		topK = append(topK, SearchResult{
			ID:       filteredItems[i].item.ID,
			Vector:   filteredItems[i].item.Vector,
			Score:    filteredItems[i].score,
			Metadata: filteredItems[i].item.Metadata,
		})
	}

	return topK, nil
}

// Len returns the number of vectors in the store.
func (f *FlatSearch) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.items)
}

// Clear removes all vectors from the store.
func (f *FlatSearch) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = make(map[string]*VectorItem)
}

// quickSortAsc sorts scoredItems in ascending order by score.
func quickSortAsc(items []scoredItem, left, right int) {
	if left >= right {
		return
	}
	pivot := items[(left+right)/2].score
	i, j := left, right
	for i <= j {
		for i <= right && items[i].score < pivot {
			i++
		}
		for j >= left && items[j].score > pivot {
			j--
		}
		if i <= j {
			items[i], items[j] = items[j], items[i]
			i++
			j--
		}
	}
	if left < j {
		quickSortAsc(items, left, j)
	}
	if i < right {
		quickSortAsc(items, i, right)
	}
}

// quickSortDesc sorts scoredItems in descending order by score.
// This is used for inner product where higher values are better.
func quickSortDesc(items []scoredItem, left, right int) {
	if left >= right {
		return
	}
	pivot := items[(left+right)/2].score
	i, j := left, right
	for i <= j {
		for i <= right && items[i].score > pivot {
			i++
		}
		for j >= left && items[j].score < pivot {
			j--
		}
		if i <= j {
			items[i], items[j] = items[j], items[i]
			i++
			j--
		}
	}
	if left < j {
		quickSortDesc(items, left, j)
	}
	if i < right {
		quickSortDesc(items, i, right)
	}
}
