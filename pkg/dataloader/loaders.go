// Package dataloader provides efficient batching and caching mechanisms for database operations.
// This implements the DataLoader pattern to solve N+1 query problems commonly found in GraphQL
// and REST API implementations, with specific optimizations for DynamoDB access patterns.
package dataloader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// BatchFunc defines the signature for batch loading functions
type BatchFunc[K comparable, V any] func(context.Context, []K) ([]V, []error)

// DataLoader provides batching and caching for database operations
type DataLoader[K comparable, V any] struct {
	// Configuration
	batchFn     BatchFunc[K, V]
	wait        time.Duration
	maxBatch    int
	cache       bool
	cacheExpiry time.Duration

	// Internal state
	mu           sync.Mutex
	batch        []K
	batchCh      chan struct{}
	cache_data   map[K]*cacheItem[V]
	singleflight singleflight.Group

	// Metrics
	logger  *zap.Logger
	hits    int64
	misses  int64
	batches int64
	errors  int64
}

type cacheItem[V any] struct {
	value     V
	err       error
	timestamp time.Time
}

// Config holds configuration for DataLoader
type Config struct {
	Wait        time.Duration // How long to wait before executing a batch (default: 1ms)
	MaxBatch    int           // Maximum batch size (default: 100)
	Cache       bool          // Enable caching (default: true)
	CacheExpiry time.Duration // Cache expiry time (default: 5 minutes)
}

// DefaultConfig returns default DataLoader configuration
func DefaultConfig() Config {
	return Config{
		Wait:        1 * time.Millisecond,
		MaxBatch:    100,
		Cache:       true,
		CacheExpiry: 5 * time.Minute,
	}
}

// NewDataLoader creates a new DataLoader with the given batch function
func NewDataLoader[K comparable, V any](batchFn BatchFunc[K, V], cfg Config, logger *zap.Logger) *DataLoader[K, V] {
	if cfg.Wait == 0 {
		cfg.Wait = 1 * time.Millisecond
	}
	if cfg.MaxBatch == 0 {
		cfg.MaxBatch = 100
	}
	if cfg.CacheExpiry == 0 {
		cfg.CacheExpiry = 5 * time.Minute
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	dl := &DataLoader[K, V]{
		batchFn:     batchFn,
		wait:        cfg.Wait,
		maxBatch:    cfg.MaxBatch,
		cache:       cfg.Cache,
		cacheExpiry: cfg.CacheExpiry,
		cache_data:  make(map[K]*cacheItem[V]),
		logger:      logger,
	}

	return dl
}

// Load loads a single value by key, batching with other concurrent requests
func (dl *DataLoader[K, V]) Load(ctx context.Context, key K) (V, error) {
	// Check cache first if enabled
	if dl.cache {
		if item := dl.getCached(key); item != nil {
			if !dl.isExpired(item) {
				dl.hits++
				return item.value, item.err
			}
		}
	}

	dl.misses++

	// Use singleflight to deduplicate concurrent requests for the same key
	result, err, _ := dl.singleflight.Do(fmt.Sprintf("%v", key), func() (interface{}, error) {
		return dl.loadBatch(ctx, key)
	})

	if err != nil {
		var zero V
		return zero, err
	}

	return result.(V), nil
}

// LoadMany loads multiple values by keys, batching efficiently
func (dl *DataLoader[K, V]) LoadMany(ctx context.Context, keys []K) ([]V, []error) {
	if len(keys) == 0 {
		return nil, nil
	}

	// For small requests, use individual Load calls to benefit from caching
	if len(keys) <= 10 {
		results := make([]V, len(keys))
		errors := make([]error, len(keys))

		for i, key := range keys {
			results[i], errors[i] = dl.Load(ctx, key)
		}

		return results, errors
	}

	// For larger requests, batch directly
	uncachedKeys := make([]K, 0, len(keys))
	keyToIndex := make(map[K]int)

	results := make([]V, len(keys))
	errors := make([]error, len(keys))

	// Check cache for each key if caching is enabled
	if dl.cache {
		for i, key := range keys {
			if item := dl.getCached(key); item != nil && !dl.isExpired(item) {
				results[i] = item.value
				errors[i] = item.err
				dl.hits++
			} else {
				keyToIndex[key] = i
				uncachedKeys = append(uncachedKeys, key)
				dl.misses++
			}
		}
	} else {
		uncachedKeys = keys
		for i, key := range keys {
			keyToIndex[key] = i
		}
	}

	// Load uncached keys
	if len(uncachedKeys) > 0 {
		batchResults, batchErrors := dl.batchFn(ctx, uncachedKeys)

		// Map results back to original positions
		for i, key := range uncachedKeys {
			originalIndex := keyToIndex[key]

			var value V
			var err error

			if i < len(batchResults) {
				value = batchResults[i]
			}
			if i < len(batchErrors) {
				err = batchErrors[i]
			}

			results[originalIndex] = value
			errors[originalIndex] = err

			// Cache the result
			if dl.cache {
				dl.setCached(key, value, err)
			}
		}

		dl.batches++
	}

	return results, errors
}

// Prime adds a value to the cache without loading
func (dl *DataLoader[K, V]) Prime(key K, value V, err error) {
	if !dl.cache {
		return
	}
	dl.setCached(key, value, err)
}

// Clear removes a key from the cache
func (dl *DataLoader[K, V]) Clear(key K) {
	if !dl.cache {
		return
	}

	dl.mu.Lock()
	delete(dl.cache_data, key)
	dl.mu.Unlock()
}

// ClearAll clears the entire cache
func (dl *DataLoader[K, V]) ClearAll() {
	if !dl.cache {
		return
	}

	dl.mu.Lock()
	dl.cache_data = make(map[K]*cacheItem[V])
	dl.mu.Unlock()
}

// GetStats returns loader statistics
func (dl *DataLoader[K, V]) GetStats() LoaderStats {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	cacheSize := len(dl.cache_data)

	return LoaderStats{
		Hits:      dl.hits,
		Misses:    dl.misses,
		Batches:   dl.batches,
		Errors:    dl.errors,
		CacheSize: cacheSize,
	}
}

// LoaderStats holds statistics about loader performance
type LoaderStats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Batches   int64 `json:"batches"`
	Errors    int64 `json:"errors"`
	CacheSize int   `json:"cache_size"`
}

// Internal methods

func (dl *DataLoader[K, V]) loadBatch(ctx context.Context, key K) (V, error) {
	results, errors := dl.batchFn(ctx, []K{key})

	var value V
	var err error

	if len(results) > 0 {
		value = results[0]
	}
	if len(errors) > 0 {
		err = errors[0]
	}

	if dl.cache {
		dl.setCached(key, value, err)
	}

	dl.batches++

	return value, err
}

func (dl *DataLoader[K, V]) getCached(key K) *cacheItem[V] {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	return dl.cache_data[key]
}

func (dl *DataLoader[K, V]) setCached(key K, value V, err error) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	dl.cache_data[key] = &cacheItem[V]{
		value:     value,
		err:       err,
		timestamp: time.Now(),
	}
}

func (dl *DataLoader[K, V]) isExpired(item *cacheItem[V]) bool {
	return time.Since(item.timestamp) > dl.cacheExpiry
}

// Example Usage:
//
// To create domain-specific loaders, import the necessary interfaces and create loaders like:
//
//   type UserLoader struct {
//       *DataLoader[string, *YourUserType]
//   }
//
//   func NewUserLoader(repos YourRepositoryInterface, logger *zap.Logger) *UserLoader {
//       batchFn := func(ctx context.Context, usernames []string) ([]*YourUserType, []error) {
//           // Implementation specific to your repository
//       }
//       config := DefaultConfig()
//       loader := NewDataLoader(batchFn, config, logger)
//       return &UserLoader{loader}
//   }
