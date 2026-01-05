package repositories

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTrendingCache(t *testing.T) {
	cache := NewTrendingCache(5 * time.Minute)

	assert.NotNil(t, cache)
	assert.NotNil(t, cache.results)
	assert.NotNil(t, cache.metrics)
	assert.Equal(t, 5*time.Minute, cache.expiration)
}

func TestTrendingCache_GetSetTrendingResult(t *testing.T) {
	t.Run("cache hit returns result", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		result := &CachedTrendingResult{
			Results: []*storage.TrendingHashtag{
				{Name: "test", UsageCount: 100},
			},
			GeneratedAt: time.Now(),
			HitCount:    1,
		}

		cache.setTrendingResult("key1", result)

		retrieved := cache.getTrendingResult("key1")
		require.NotNil(t, retrieved)
		assert.Equal(t, result.Results, retrieved.Results)
		assert.Equal(t, int64(1), retrieved.HitCount)
	})

	t.Run("cache miss returns nil", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		retrieved := cache.getTrendingResult("nonexistent")
		assert.Nil(t, retrieved)
	})

	t.Run("expired entry returns nil", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		result := &CachedTrendingResult{
			Results:     []*storage.TrendingHashtag{{Name: "expired"}},
			GeneratedAt: time.Now().Add(-10 * time.Minute), // Expired
			HitCount:    1,
		}

		// Manually set to bypass cleanup in setter
		cache.mu.Lock()
		cache.results["expired_key"] = result
		cache.mu.Unlock()

		retrieved := cache.getTrendingResult("expired_key")
		assert.Nil(t, retrieved, "expired entry should return nil")
	})

	t.Run("entry just before expiration is returned", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		result := &CachedTrendingResult{
			Results:     []*storage.TrendingHashtag{{Name: "fresh"}},
			GeneratedAt: time.Now().Add(-4 * time.Minute), // 4 min old, 5 min expiry
			HitCount:    1,
		}

		cache.mu.Lock()
		cache.results["fresh_key"] = result
		cache.mu.Unlock()

		retrieved := cache.getTrendingResult("fresh_key")
		assert.NotNil(t, retrieved, "entry just before expiration should be returned")
	})
}

func TestTrendingCache_GetSetHashtagMetrics(t *testing.T) {
	t.Run("cache hit returns metrics", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		metrics := &CachedHashtagMetrics{
			Metrics: &EnhancedHashtagMetrics{
				HashtagName: "golang",
				TotalUsage:  500,
			},
			GeneratedAt: time.Now(),
			ValidUntil:  time.Now().Add(10 * time.Minute),
		}

		cache.setHashtagMetrics("golang", metrics)

		retrieved := cache.getHashtagMetrics("golang")
		require.NotNil(t, retrieved)
		assert.Equal(t, "golang", retrieved.Metrics.HashtagName)
		assert.Equal(t, int64(500), retrieved.Metrics.TotalUsage)
	})

	t.Run("cache miss returns nil", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		retrieved := cache.getHashtagMetrics("nonexistent")
		assert.Nil(t, retrieved)
	})

	t.Run("expired entry by ValidUntil returns nil", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		metrics := &CachedHashtagMetrics{
			Metrics: &EnhancedHashtagMetrics{
				HashtagName: "expired",
			},
			GeneratedAt: time.Now().Add(-1 * time.Hour),
			ValidUntil:  time.Now().Add(-30 * time.Minute), // ValidUntil in past
		}

		// Manually set to bypass cleanup
		cache.mu.Lock()
		cache.metrics["expired"] = metrics
		cache.mu.Unlock()

		retrieved := cache.getHashtagMetrics("expired")
		assert.Nil(t, retrieved, "entry with ValidUntil in past should return nil")
	})

	t.Run("entry with future ValidUntil is returned", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		metrics := &CachedHashtagMetrics{
			Metrics: &EnhancedHashtagMetrics{
				HashtagName: "valid",
			},
			GeneratedAt: time.Now(),
			ValidUntil:  time.Now().Add(1 * time.Hour),
		}

		cache.setHashtagMetrics("valid", metrics)

		retrieved := cache.getHashtagMetrics("valid")
		assert.NotNil(t, retrieved)
	})
}

func TestTrendingCache_CleanupExpiredResults(t *testing.T) {
	t.Run("removes only expired entries", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		// Add fresh entry
		cache.mu.Lock()
		cache.results["fresh"] = &CachedTrendingResult{
			GeneratedAt: time.Now(),
		}
		// Add expired entry
		cache.results["expired1"] = &CachedTrendingResult{
			GeneratedAt: time.Now().Add(-10 * time.Minute),
		}
		cache.results["expired2"] = &CachedTrendingResult{
			GeneratedAt: time.Now().Add(-1 * time.Hour),
		}
		cache.mu.Unlock()

		// Trigger cleanup via setTrendingResult
		cache.setTrendingResult("trigger", &CachedTrendingResult{
			GeneratedAt: time.Now(),
		})

		cache.mu.RLock()
		defer cache.mu.RUnlock()

		assert.Contains(t, cache.results, "fresh")
		assert.Contains(t, cache.results, "trigger")
		assert.NotContains(t, cache.results, "expired1")
		assert.NotContains(t, cache.results, "expired2")
	})

	t.Run("empty cache cleanup is safe", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		// Should not panic
		cache.mu.Lock()
		cache.cleanupExpiredResults()
		cache.mu.Unlock()

		assert.Empty(t, cache.results)
	})
}

func TestTrendingCache_CleanupExpiredMetrics(t *testing.T) {
	t.Run("removes only entries past ValidUntil", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		// Add valid entry
		cache.mu.Lock()
		cache.metrics["valid"] = &CachedHashtagMetrics{
			ValidUntil: time.Now().Add(1 * time.Hour),
		}
		// Add expired entries
		cache.metrics["expired1"] = &CachedHashtagMetrics{
			ValidUntil: time.Now().Add(-10 * time.Minute),
		}
		cache.metrics["expired2"] = &CachedHashtagMetrics{
			ValidUntil: time.Now().Add(-1 * time.Hour),
		}
		cache.mu.Unlock()

		// Trigger cleanup via setHashtagMetrics
		cache.setHashtagMetrics("trigger", &CachedHashtagMetrics{
			ValidUntil: time.Now().Add(1 * time.Hour),
		})

		cache.mu.RLock()
		defer cache.mu.RUnlock()

		assert.Contains(t, cache.metrics, "valid")
		assert.Contains(t, cache.metrics, "trigger")
		assert.NotContains(t, cache.metrics, "expired1")
		assert.NotContains(t, cache.metrics, "expired2")
	})

	t.Run("empty metrics cleanup is safe", func(t *testing.T) {
		cache := NewTrendingCache(5 * time.Minute)

		// Should not panic
		cache.mu.Lock()
		cache.cleanupExpiredMetrics()
		cache.mu.Unlock()

		assert.Empty(t, cache.metrics)
	})
}

func TestTrendingCache_ConcurrentAccess(t *testing.T) {
	// This test verifies no race conditions occur with concurrent access
	// Run with -race flag to detect issues
	cache := NewTrendingCache(100 * time.Millisecond)

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.setTrendingResult("key", &CachedTrendingResult{
				GeneratedAt: time.Now(),
			})
			cache.setHashtagMetrics("hashtag", &CachedHashtagMetrics{
				ValidUntil: time.Now().Add(1 * time.Minute),
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = cache.getTrendingResult("key")
			_ = cache.getHashtagMetrics("hashtag")
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}
