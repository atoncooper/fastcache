package src

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
)

// Frequency frequency statistics for TinyLFU with sampling
type Frequency struct {
	mu       sync.RWMutex
	counters map[string]*counter
	// sliding window size
	windowSize int64
	// max counters
	maxCounters int64
	// total hits in window
	totalHits int64
	// decay counter
	decayCounter int64
}

// counter stores frequency count with metadata
type counter struct {
	count    int64
	lastHash uint64
}

// NewFrequency creates a new frequency tracker with TinyLFU sampling
func NewFrequency(numCounters int64) *Frequency {
	if numCounters <= 0 {
		numCounters = 1e6
	}
	return &Frequency{
		counters:    make(map[string]*counter, numCounters),
		windowSize:  numCounters,
		maxCounters: numCounters,
		totalHits:   0,
		decayCounter: 0,
	}
}

// Increment increments the frequency count for a key
// Uses CM Sketch-like approach for memory efficiency
func (f *Frequency) Increment(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Get or create counter
	c, exists := f.counters[key]
	if !exists {
		// Check if we need to evict
		if int64(len(f.counters)) >= f.maxCounters {
			f.evictOne()
		}
		c = &counter{count: 1}
		f.counters[key] = c
		atomic.AddInt64(&f.totalHits, 1)
		return
	}

	// Increment count
	c.count++

	// Check for periodic decay
	f.decayCounter++
	if f.decayCounter >= f.windowSize/10 {
		f.decay()
	}
}

// Get gets the frequency count for a key
func (f *Frequency) Get(key string) int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if c, exists := f.counters[key]; exists {
		return c.count
	}
	return 0
}

// evictOne evicts one counter to make room
func (f *Frequency) evictOne() {
	// Find a counter with count = 1 to evict
	for k, c := range f.counters {
		if c.count == 1 {
			delete(f.counters, k)
			return
		}
	}
	// If all counts > 1, evict random
	for k := range f.counters {
		delete(f.counters, k)
		return
	}
}

// decay performs counter decay to prevent stale entries from dominating
func (f *Frequency) decay() {
	f.decayCounter = 0

	// Halve all counters
	for _, c := range f.counters {
		c.count = (c.count + 1) / 2
	}
}

// Reset resets the frequency counts
func (f *Frequency) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.counters = make(map[string]*counter, f.maxCounters)
	f.totalHits = 0
	f.decayCounter = 0
}

// SampledLFU compares frequencies and returns true if new key should be admitted
// newKeyFreq: frequency of new key
// sampleSize: number of items to sample
// Returns the key to evict if admission is granted, empty string otherwise
func (f *Frequency) SampledLFU(sampleSize int) (evictKey string, admit bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.counters) == 0 {
		return "", true // Empty cache, admit
	}

	// Find minimum frequency in cache
	var minFreq int64 = 1<<63 - 1
	var minKey string

	// Sample keys
	count := 0
	for k, c := range f.counters {
		if c.count < minFreq {
			minFreq = c.count
			minKey = k
		}
		count++
		if count >= sampleSize {
			break
		}
	}

	return minKey, true
}

// CMFrequencyCountMin Sketch for memory-efficient frequency counting
type CMFrequencyCountMin struct {
	mu       sync.RWMutex
	width    int
	depth    int
	sketch   [][]int64
	hashSeeds []uint64
}

// NewCMFrequencyCountMin creates a new Count-Min sketch
func NewCMFrequencyCountMin(width, depth int) *CMFrequencyCountMin {
	cm := &CMFrequencyCountMin{
		width:    width,
		depth:    depth,
		sketch:   make([][]int64, depth),
		hashSeeds: make([]uint64, depth),
	}

	for i := 0; i < depth; i++ {
		cm.sketch[i] = make([]int64, width)
		// Generate unique hash seeds
		h := fnv.New64a()
		h.Write([]byte{byte(i)})
		cm.hashSeeds[i] = h.Sum64()
	}

	return cm
}

// Increment increments the count for a key
func (cm *CMFrequencyCountMin) Increment(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i := 0; i < cm.depth; i++ {
		h := fnv.New64a()
		// Convert uint64 seed to bytes
		seed := cm.hashSeeds[i]
		h.Write([]byte{
			byte(seed >> 56),
			byte(seed >> 48),
			byte(seed >> 40),
			byte(seed >> 32),
			byte(seed >> 24),
			byte(seed >> 16),
			byte(seed >> 8),
			byte(seed),
		})
		h.Write([]byte(key))
		idx := int(h.Sum64()) % cm.width
		cm.sketch[i][idx]++
	}
}

// Get gets the estimated count for a key (returns minimum across all hashes)
func (cm *CMFrequencyCountMin) Get(key string) int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var minCount int64 = 1<<63 - 1

	for i := 0; i < cm.depth; i++ {
		h := fnv.New64a()
		// Convert uint64 seed to bytes
		seed := cm.hashSeeds[i]
		h.Write([]byte{
			byte(seed >> 56),
			byte(seed >> 48),
			byte(seed >> 40),
			byte(seed >> 32),
			byte(seed >> 24),
			byte(seed >> 16),
			byte(seed >> 8),
			byte(seed),
		})
		h.Write([]byte(key))
		idx := int(h.Sum64()) % cm.width
		if cm.sketch[i][idx] < minCount {
			minCount = cm.sketch[i][idx]
		}
	}

	return minCount
}

// Reset resets the sketch
func (cm *CMFrequencyCountMin) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i := 0; i < cm.depth; i++ {
		for j := 0; j < cm.width; j++ {
			cm.sketch[i][j] = 0
		}
	}
}
