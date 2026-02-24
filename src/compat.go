package src

import (
	"time"
)

// FastCacheV2 cache with legacy API compatibility
type FastCacheV2 struct {
	*RistrettoCache
}

// NewFastCacheV2 creates a new V2 cache
func NewFastCacheV2() (*FastCacheV2, error) {
	return NewFastCacheV2WithConfig(nil)
}

// NewFastCacheV2WithConfig creates a V2 cache with config
func NewFastCacheV2WithConfig(config *Config) (*FastCacheV2, error) {
	cache, err := NewRistrettoCache(config)
	if err != nil {
		return nil, err
	}
	return &FastCacheV2{RistrettoCache: cache}, nil
}

// Set sets a value (legacy API, cost defaults to 1, TTL defaults to never expire)
func (f *FastCacheV2) Set(key string, value any, ttl time.Duration) bool {
	if ttl <= 0 {
		return f.RistrettoCache.Set(key, value, 1)
	}
	return f.RistrettoCache.SetWithTTL(key, value, 1, ttl)
}

// Get gets a value (legacy API)
func (f *FastCacheV2) Get(key string) (any, bool) {
	return f.RistrettoCache.Get(key)
}

// Delete deletes a value
func (f *FastCacheV2) Delete(key string) {
	f.RistrettoCache.Del(key)
}

// Close closes the cache
func (f *FastCacheV2) Close() error {
	return f.RistrettoCache.Close()
}
