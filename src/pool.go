package src

import (
	"sync"
	"sync/atomic"
)

// CacheItemPool is a pool for reusing CacheItem objects
var CacheItemPool = sync.Pool{
	New: func() interface{} {
		return &CacheItem{}
	},
}

// GetCacheItem gets a CacheItem from the pool
func GetCacheItem() *CacheItem {
	return CacheItemPool.Get().(*CacheItem)
}

// PutCacheItem returns a CacheItem to the pool
func PutCacheItem(item *CacheItem) {
	if item != nil {
		item.Key = ""
		item.Value = nil
		item.element = nil
		CacheItemPool.Put(item)
	}
}

// SetItemPool is a pool for reusing setItem objects
var SetItemPool = sync.Pool{
	New: func() interface{} {
		return &setItem{}
	},
}

// GetSetItem gets a setItem from the pool
func GetSetItem() *setItem {
	return SetItemPool.Get().(*setItem)
}

// PutSetItem returns a setItem to the pool
func PutSetItem(item *setItem) {
	if item != nil {
		item.key = ""
		item.value = nil
		SetItemPool.Put(item)
	}
}

// ByteSlicePool is a pool for reusing []byte slices
// Use for values that are []byte to reduce GC
var ByteSlicePool = sync.Pool{
	New: func() interface{} {
		return &[]byte{}
	},
}

// GetByteSlice gets a []byte from the pool with specified capacity
func GetByteSlice(cap int) []byte {
	ptr := ByteSlicePool.Get().(*[]byte)
	*ptr = make([]byte, 0, cap)
	return *ptr
}

// PutByteSlice returns a []byte to the pool
func PutByteSlice(b []byte) {
	if b != nil {
		ptr := &b
		*ptr = (*ptr)[:0]
		ByteSlicePool.Put(ptr)
	}
}

// StringBuilderPool is a pool for reusing strings.Builder
var StringBuilderPool = sync.Pool{
	New: func() interface{} {
		return &stringBuilderWrapper{}
	},
}

type stringBuilderWrapper struct {
	buf []byte
}

func (s *stringBuilderWrapper) Reset() {
	s.buf = s.buf[:0]
}

func (s *stringBuilderWrapper) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	return len(p), nil
}

func (s *stringBuilderWrapper) String() string {
	return string(s.buf)
}

// GetStringBuilder gets a string builder from the pool
func GetStringBuilder() *stringBuilderPoolWrapper {
	return stringBuilderPool.Get().(*stringBuilderPoolWrapper)
}

var stringBuilderPool = sync.Pool{
	New: func() interface{} {
		return &stringBuilderPoolWrapper{
			buf: make([]byte, 0, 128),
		}
	},
}

type stringBuilderPoolWrapper struct {
	buf []byte
}

func (s *stringBuilderPoolWrapper) Reset() {
	s.buf = s.buf[:0]
}

func (s *stringBuilderPoolWrapper) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	return len(p), nil
}

func (s *stringBuilderPoolWrapper) WriteByte(b byte) error {
	s.buf = append(s.buf, b)
	return nil
}

func (s *stringBuilderPoolWrapper) WriteString(p string) (int, error) {
	s.buf = append(s.buf, p...)
	return len(p), nil
}

func (s *stringBuilderPoolWrapper) String() string {
	return string(s.buf)
}

func (s *stringBuilderPoolWrapper) Len() int {
	return len(s.buf)
}

func (s *stringBuilderPoolWrapper) Cap() int {
	return cap(s.buf)
}

// PutStringBuilder returns a string builder to the pool
func PutStringBuilder(sb *stringBuilderPoolWrapper) {
	if sb != nil {
		sb.Reset()
		stringBuilderPool.Put(sb)
	}
}

// GCStats holds GC statistics for monitoring
type GCStats struct {
	lastNumGC     uint32
	atomicPauseNs uint64
}

// NewGCStats creates a new GC stats tracker
func NewGCStats() *GCStats {
	return &GCStats{}
}

// RecordGC records GC pause time
func (g *GCStats) RecordGC(numGC uint32, pauseNs uint64) {
	atomic.StoreUint32(&g.lastNumGC, numGC)
	atomic.StoreUint64(&g.atomicPauseNs, pauseNs)
}

// LastNumGC returns the last GC count
func (g *GCStats) LastNumGC() uint32 {
	return atomic.LoadUint32(&g.lastNumGC)
}

// PauseNs returns the last GC pause time in nanoseconds
func (g *GCStats) PauseNs() uint64 {
	return atomic.LoadUint64(&g.atomicPauseNs)
}
