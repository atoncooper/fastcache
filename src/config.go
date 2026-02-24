package src

import (
	"time"
)

// Config cache configuration
type Config struct {
	// NumCounters number of keys to track for frequency (recommend: expected keys * 10)
	NumCounters int64
	// MaxCost maximum cost of cache
	MaxCost int64
	// BufferItems number of keys per Set buffer
	BufferItems int64
	// Metrics enable metrics collection
	Metrics bool
	// TTL default TTL (0 means no expiration)
	TTL time.Duration
	// OnEvict eviction callback
	OnEvict func(key string, value any, cost int64)
	// OnReject rejection callback
	OnReject func(key string, value any, cost int64)
	// OnExit exit callback (eviction + rejection)
	OnExit func(value any)

	// GCInterval GC interval (0 = disabled)
	GCInterval time.Duration
	// GcMemThreshold cost threshold for triggering GC (0-100)
	GcMemThreshold int
}

// defaultConfig returns default configuration
func defaultConfig() *Config {
	return &Config{
		NumCounters:    1e7, // 10M
		MaxCost:        1 << 30, // 1GB
		BufferItems:    64,
		Metrics:        false,
		TTL:            0,
		GCInterval:     0,
		GcMemThreshold: 80,
	}
}
