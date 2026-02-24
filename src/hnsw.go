package src

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
)

// HNSWConfig contains configuration parameters for the HNSW index.
type HNSWConfig struct {
	M              int     // Number of connections per node.
	EFConstruction int    // Size of the candidate list during index construction.
	EFSearch       int    // Size of the candidate list during search.
	LevelMult      float64 // Multiplier for determining node levels.
}

// DefaultHNSWConfig returns the default HNSW configuration.
func DefaultHNSWConfig() HNSWConfig {
	return HNSWConfig{
		M:              16,
		EFConstruction: 200,
		EFSearch:       50,
		LevelMult:      1 / math.Ln2,
	}
}

// HNSWNode represents a node in the HNSW graph structure.
type HNSWNode struct {
	ID       string
	Vector   Vector
	Metadata map[string]any

	// Neighbor nodes organized by level.
	// neighbors[level] = map[string]*HNSWNode
	neighbors []map[string]*HNSWNode

	// Flag indicating whether this node has been deleted.
	deleted bool
}

// NewHNSWNode creates a new HNSW node with the specified level.
func NewHNSWNode(id string, vector Vector, metadata map[string]any, level int) *HNSWNode {
	neighbors := make([]map[string]*HNSWNode, level+1)
	for i := 0; i <= level; i++ {
		neighbors[i] = make(map[string]*HNSWNode)
	}
	return &HNSWNode{
		ID:       id,
		Vector:   vector,
		Metadata: metadata,
		neighbors: neighbors,
	}
}

// HNSW implements a Hierarchical Navigable Small World graph index.
type HNSW struct {
	mu       sync.RWMutex
	config   HNSWConfig
	metric   MetricType
	distance DistanceFunc

	// Node storage.
	nodes map[string]*HNSWNode

	// Entry point (node at the highest level).
	entryPoint *HNSWNode

	// Maximum level in the graph.
	maxLevel int32

	// Number of nodes in the index.
	count int64

	// Random number generator for level calculation.
	rand *rand.Rand

	// Memory tracking.
	maxMemory int64
	currentMem int64
}

// nodeDist pairs a node with its distance to a query vector.
type nodeDist struct {
	node *HNSWNode
	dist float32
}

// NewHNSW creates a new HNSW index with the specified configuration and metric.
func NewHNSW(config HNSWConfig, metric MetricType) *HNSW {
	if config.M <= 0 {
		config.M = 16
	}
	if config.EFConstruction <= 0 {
		config.EFConstruction = 200
	}
	if config.EFSearch <= 0 {
		config.EFSearch = 50
	}
	if config.LevelMult <= 0 {
		config.LevelMult = 1 / math.Ln2
	}

	return &HNSW{
		config:    config,
		metric:    metric,
		distance:  GetDistanceFunc(metric),
		nodes:     make(map[string]*HNSWNode),
		maxLevel:  -1,
		rand:      rand.New(rand.NewSource(rand.Int63())),
	}
}

// getLevel calculates a random level for a new node using exponential distribution.
func (h *HNSW) getLevel() int {
	// Exponential distribution: P(l) = exp(-l/levelMult)
	// level = floor(-ln(random) * levelMult)
	r := h.rand.Float64()
	level := int(-math.Log(r) * h.config.LevelMult)
	return level
}

// Add inserts a vector into the HNSW index.
func (h *HNSW) Add(id string, vector Vector, metadata map[string]any) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if the node already exists.
	if _, exists := h.nodes[id]; exists {
		// Update the existing node.
		h.updateNode(id, vector, metadata)
		return nil
	}

	// Generate a random level for the new node.
	level := h.getLevel()
	if level > 32 {
		level = 32 // Cap the maximum level.
	}

	// Create a new node.
	node := NewHNSWNode(id, vector, metadata, level)

	// Update memory statistics.
	nodeMem := int64(len(vector)*4 + len(id) + 64)
	h.currentMem += nodeMem

	// If this is the first node.
	if h.entryPoint == nil {
		h.entryPoint = node
		h.maxLevel = int32(level)
		h.nodes[id] = node
		h.count++
		return nil
	}

	// Start searching from the entry point.
	ep := h.entryPoint

	// Search from the highest level down to the new node's level.
	for l := int(h.maxLevel); l > level; l-- {
		res := h.searchLayer(ep, vector, 1, l)
		if len(res) > 0 {
			ep = res[0]
		}
	}

	// Insert the node at each level.
	for l := min(int(level), int(h.maxLevel)); l >= 0; l-- {
		// Search for nearest neighbors at this level.
		candidates := h.searchLayer(ep, vector, h.config.EFConstruction, l)

		// Connect to the nearest neighbors.
		for _, candidate := range candidates {
			if candidate.ID == node.ID {
				continue
			}
			// Create bidirectional edges.
			h.addEdge(node, candidate, l)
			h.pruneNeighbors(node, l)

			// Update reverse edges.
			h.addEdge(candidate, node, l)
			h.pruneNeighbors(candidate, l)
		}

		// Update the entry point.
		if len(candidates) > 0 {
			ep = candidates[0]
		}
	}

	// Update the entry point if the new node has a higher level.
	if level > int(h.maxLevel) {
		h.entryPoint = node
		h.maxLevel = int32(level)
	}

	h.nodes[id] = node
	h.count++

	return nil
}

// updateNode updates an existing node's vector and metadata.
func (h *HNSW) updateNode(id string, vector Vector, metadata map[string]any) {
	node := h.nodes[id]
	node.Vector = vector
	node.Metadata = metadata
}

// searchLayer searches for nearest neighbors at a specific level.
func (h *HNSW) searchLayer(entry *HNSWNode, query Vector, ef, level int) []*HNSWNode {
	if entry == nil {
		return nil
	}

	// Set of visited nodes.
	visited := make(map[string]bool)
	visited[entry.ID] = true

	// Candidate priority queue (min-heap).
	candidates := &nodeHeap{data: []nodeDist{{node: entry, dist: h.distance(entry.Vector, query)}}}
	// Results priority queue (max-heap for EF).
	results := &nodeHeapDesc{data: []nodeDist{{node: entry, dist: h.distance(entry.Vector, query)}}}

	for candidates.Len() > 0 {
		// Get the nearest candidate node.
		c := candidates.Pop().(nodeDist)

		// Get the farthest node in the results.
		r := results.Top().(nodeDist)

		// If the current node is farther than the farthest result, we can stop.
		if c.dist > r.dist && results.Len() >= ef {
			break
		}

		// Traverse neighbors of the current node.
		for _, neighbor := range c.node.neighbors[level] {
			if neighbor.deleted {
				continue
			}
			if visited[neighbor.ID] {
				continue
			}
			visited[neighbor.ID] = true

			dist := h.distance(neighbor.Vector, query)
			neighborNode := nodeDist{node: neighbor, dist: dist}

			// Add to candidate queue.
			candidates.Push(neighborNode)

			// Add to results queue.
			if results.Len() < ef {
				results.Push(neighborNode)
			} else if dist < r.dist {
				results.Pop()
				results.Push(neighborNode)
			}
		}
	}

	// Extract results.
	res := make([]*HNSWNode, results.Len())
	for i := results.Len() - 1; i >= 0; i-- {
		nd := results.Pop().(nodeDist)
		res[i] = nd.node
	}

	return res
}

// addEdge creates an edge from one node to another at a specific level.
func (h *HNSW) addEdge(from, to *HNSWNode, level int) {
	if level >= len(from.neighbors) {
		return
	}
	from.neighbors[level][to.ID] = to
}

// pruneNeighbors removes distant neighbors to maintain the maximum connection limit M.
func (h *HNSW) pruneNeighbors(node *HNSWNode, level int) {
	if level >= len(node.neighbors) {
		return
	}

	neighbors := node.neighbors[level]
	if len(neighbors) <= h.config.M {
		return
	}

	// Calculate distances to all neighbors.
	type nd struct {
		id   string
		node *HNSWNode
		dist float32
	}

	distList := make([]nd, 0, len(neighbors))
	for id, n := range neighbors {
		distList = append(distList, nd{id: id, node: n, dist: h.distance(node.Vector, n.Vector)})
	}

	// Sort by distance.
	for i := 0; i < len(distList)-1; i++ {
		for j := i + 1; j < len(distList); j++ {
			if distList[i].dist > distList[j].dist {
				distList[i], distList[j] = distList[j], distList[i]
			}
		}
	}

	// Keep only the closest M neighbors.
	for i := h.config.M; i < len(distList); i++ {
		delete(neighbors, distList[i].id)
	}
}

// Search finds the k nearest vectors to the query.
func (h *HNSW) Search(query Vector, k int) ([]SearchResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.entryPoint == nil {
		return []SearchResult{}, nil
	}

	if k <= 0 {
		k = 10
	}

	ef := k
	if ef < h.config.EFSearch {
		ef = h.config.EFSearch
	}

	// Search starting from the highest level.
	ep := h.entryPoint
	for l := int(h.maxLevel); l > 0; l-- {
		results := h.searchLayer(ep, query, 1, l)
		if len(results) > 0 {
			ep = results[0]
		}
	}

	// Search at level 0.
	results := h.searchLayer(ep, query, ef, 0)

	// Convert to SearchResult.
	topK := make([]SearchResult, 0, min(k, len(results)))
	for i := 0; i < min(k, len(results)); i++ {
		if results[i].deleted {
			continue
		}
		// Calculate distance.
		dist := h.distance(query, results[i].Vector)
		topK = append(topK, SearchResult{
			ID:       results[i].ID,
			Vector:   results[i].Vector,
			Score:    dist,
			Metadata: results[i].Metadata,
		})
		// Correct score to positive value (inner product uses negative values).
		if h.metric == MetricIP {
			topK[len(topK)-1].Score = -topK[len(topK)-1].Score
		}
	}

	return topK, nil
}

// SearchWithFilter performs a search with metadata filtering.
func (h *HNSW) SearchWithFilter(query Vector, k int, filter FilterFunc) ([]SearchResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.entryPoint == nil {
		return []SearchResult{}, nil
	}

	if k <= 0 {
		k = 10
	}

	// Search more results first, then filter.
	ef := k * 2
	if ef < h.config.EFSearch {
		ef = h.config.EFSearch
	}

	// Search starting from the highest level.
	ep := h.entryPoint
	for l := int(h.maxLevel); l > 0; l-- {
		results := h.searchLayer(ep, query, 1, l)
		if len(results) > 0 {
			ep = results[0]
		}
	}

	// Search at level 0.
	results := h.searchLayer(ep, query, ef, 0)

	// Filter and convert results.
	var filtered []SearchResult
	for _, node := range results {
		if node.deleted {
			continue
		}
		// Apply filter function.
		if filter != nil && !filter(node.Metadata) {
			continue
		}
		dist := h.distance(query, node.Vector)
		result := SearchResult{
			ID:       node.ID,
			Vector:   node.Vector,
			Score:    dist,
			Metadata: node.Metadata,
		}
		if h.metric == MetricIP {
			result.Score = -result.Score
		}
		filtered = append(filtered, result)
	}

	if len(filtered) == 0 {
		return []SearchResult{}, nil
	}

	// Sort and take top K.
	if len(filtered) > k {
		if h.metric == MetricIP {
			// Inner product: higher is better.
			for i := 0; i < len(filtered)-1; i++ {
				for j := i + 1; j < len(filtered); j++ {
					if filtered[i].Score < filtered[j].Score {
						filtered[i], filtered[j] = filtered[j], filtered[i]
					}
				}
			}
		} else {
			// Distance: lower is better.
			for i := 0; i < len(filtered)-1; i++ {
				for j := i + 1; j < len(filtered); j++ {
					if filtered[i].Score > filtered[j].Score {
						filtered[i], filtered[j] = filtered[j], filtered[i]
					}
				}
			}
		}
		filtered = filtered[:k]
	}

	return filtered, nil
}

// Get retrieves a vector by its ID.
func (h *HNSW) Get(id string) (*VectorItem, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	node, found := h.nodes[id]
	if !found || node.deleted {
		return nil, false
	}

	return &VectorItem{
		ID:       node.ID,
		Vector:   node.Vector,
		Metadata: node.Metadata,
		Cost:     int64(len(node.Vector) * 4),
	}, true
}

// Delete marks a vector as deleted (logical deletion).
func (h *HNSW) Delete(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	node, found := h.nodes[id]
	if !found {
		return nil
	}

	node.deleted = true
	h.count--
	return nil
}

// Len returns the number of vectors in the index.
func (h *HNSW) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return int(atomic.LoadInt64(&h.count))
}

// Clear removes all vectors from the index.
func (h *HNSW) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nodes = make(map[string]*HNSWNode)
	h.entryPoint = nil
	h.maxLevel = -1
	h.count = 0
	h.currentMem = 0
}

// nodeHeap is a min-heap for candidate priority queue.
type nodeHeap struct {
	data []nodeDist
}

func (h *nodeHeap) Len() int           { return len(h.data) }
func (h *nodeHeap) Less(i, j int) bool { return h.data[i].dist < h.data[j].dist }
func (h *nodeHeap) Swap(i, j int)      { h.data[i], h.data[j] = h.data[j], h.data[i] }
func (h *nodeHeap) Push(x interface{})  { h.data = append(h.data, x.(nodeDist)) }
func (h *nodeHeap) Pop() interface{} {
	res := h.data[len(h.data)-1]
	h.data = h.data[:len(h.data)-1]
	return res
}
func (h *nodeHeap) Top() interface{} {
	return h.data[0]
}

// nodeHeapDesc is a max-heap for results priority queue.
type nodeHeapDesc struct {
	data []nodeDist
}

func (h *nodeHeapDesc) Len() int           { return len(h.data) }
func (h *nodeHeapDesc) Less(i, j int) bool { return h.data[i].dist > h.data[j].dist }
func (h *nodeHeapDesc) Swap(i, j int)      { h.data[i], h.data[j] = h.data[j], h.data[i] }
func (h *nodeHeapDesc) Push(x interface{})  { h.data = append(h.data, x.(nodeDist)) }
func (h *nodeHeapDesc) Pop() interface{} {
	res := h.data[len(h.data)-1]
	h.data = h.data[:len(h.data)-1]
	return res
}
func (h *nodeHeapDesc) Top() interface{} {
	return h.data[0]
}
