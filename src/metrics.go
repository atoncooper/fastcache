package src

import (
	"fmt"
	"sync/atomic"
)

// Metrics cache metrics statistics
type Metrics struct {
	hits         atomic.Int64
	misses       atomic.Int64
	keysAdded    atomic.Int64
	keysEvicted  atomic.Int64
	setsDropped  atomic.Int64
	setsRejected atomic.Int64
	costAdded    atomic.Int64
	costEvicted  atomic.Int64
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// Hits returns the number of cache hits
func (m *Metrics) Hits() int64 {
	return m.hits.Load()
}

// Misses returns the number of cache misses
func (m *Metrics) Misses() int64 {
	return m.misses.Load()
}

// KeysAdded returns the number of keys added
func (m *Metrics) KeysAdded() int64 {
	return m.keysAdded.Load()
}

// KeysEvicted returns the number of keys evicted
func (m *Metrics) KeysEvicted() int64 {
	return m.keysEvicted.Load()
}

// SetsDropped returns the number of dropped SET operations
func (m *Metrics) SetsDropped() int64 {
	return m.setsDropped.Load()
}

// SetsRejected returns the number of rejected SET operations
func (m *Metrics) SetsRejected() int64 {
	return m.setsRejected.Load()
}

// CostAdded returns the total cost added
func (m *Metrics) CostAdded() int64 {
	return m.costAdded.Load()
}

// CostEvicted returns the total cost evicted
func (m *Metrics) CostEvicted() int64 {
	return m.costEvicted.Load()
}

// Ratio returns the hit ratio
func (m *Metrics) Ratio() float64 {
	total := m.hits.Load() + m.misses.Load()
	if total == 0 {
		return 0
	}
	return float64(m.hits.Load()) / float64(total)
}

// String returns a string representation of metrics
func (m *Metrics) String() string {
	return fmt.Sprintf(`
Cache Metrics:
  Hits: %d
  Misses: %d
  Hit Ratio: %.2f%%
  Keys Added: %d
  Keys Evicted: %d
  Sets Dropped: %d
  Sets Rejected: %d
  Cost Added: %d
  Cost Evicted: %d
`,
		m.hits.Load(),
		m.misses.Load(),
		m.Ratio()*100,
		m.keysAdded.Load(),
		m.keysEvicted.Load(),
		m.setsDropped.Load(),
		m.setsRejected.Load(),
		m.costAdded.Load(),
		m.costEvicted.Load(),
	)
}
