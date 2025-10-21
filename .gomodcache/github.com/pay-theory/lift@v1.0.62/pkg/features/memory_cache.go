package features

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
)

// MemoryCacheConfig configures the memory cache
type MemoryCacheConfig struct {
	MaxSize int64
	TTL     time.Duration
}

// MemoryCache implements an in-memory cache store using go-cache
type MemoryCache struct {
	cache   *cache.Cache
	maxSize int64
}

// NewMemoryCache creates a new memory cache using go-cache
func NewMemoryCache(config MemoryCacheConfig) *MemoryCache {
	// Create cache with default TTL and cleanup interval
	ttl := config.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute // Default TTL
	}

	cleanupInterval := ttl / 2
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}

	return &MemoryCache{
		cache:   cache.New(ttl, cleanupInterval),
		maxSize: config.MaxSize,
	}
}

// Get retrieves a value from the cache
func (mc *MemoryCache) Get(_ context.Context, key string) (any, bool, error) {
	value, found := mc.cache.Get(key)
	return value, found, nil
}

// Set stores a value in the cache
func (mc *MemoryCache) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	if ttl == 0 {
		// Use default TTL
		mc.cache.Set(key, value, cache.DefaultExpiration)
	} else {
		mc.cache.Set(key, value, ttl)
	}
	return nil
}

// Delete removes a value from the cache
func (mc *MemoryCache) Delete(_ context.Context, key string) error {
	mc.cache.Delete(key)
	return nil
}

// Clear removes all items from the cache
func (mc *MemoryCache) Clear(_ context.Context) error {
	mc.cache.Flush()
	return nil
}

// Stats returns cache statistics
func (mc *MemoryCache) Stats() CacheStats {
	itemCount := mc.cache.ItemCount()

	return CacheStats{
		Size:        int64(itemCount),
		MaxSize:     mc.maxSize,
		MemoryUsage: int64(itemCount) * 64, // Rough estimate
	}
}

// Close closes the cache
func (mc *MemoryCache) Close() error {
	mc.cache.Flush()
	return nil
}

// Keys returns all keys (go-cache doesn't have pattern matching, so we return all keys)
func (mc *MemoryCache) Keys(_ string) ([]string, error) {
	items := mc.cache.Items()
	keys := make([]string, 0, len(items))

	for key := range items {
		keys = append(keys, key)
	}

	return keys, nil
}

// Exists checks if a key exists in the cache
func (mc *MemoryCache) Exists(key string) bool {
	_, found := mc.cache.Get(key)
	return found
}

// TTL returns the time-to-live for a key
func (mc *MemoryCache) TTL(key string) time.Duration {
	// go-cache doesn't expose TTL directly, so we return a default
	// In a real implementation, we might track this separately
	if mc.Exists(key) {
		return time.Minute // Placeholder
	}
	return 0
}
