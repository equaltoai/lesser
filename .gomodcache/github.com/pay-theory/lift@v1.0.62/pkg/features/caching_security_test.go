package features

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachingSecurityFix(t *testing.T) {
	t.Run("Cache middleware uses SHA-256 instead of MD5", func(t *testing.T) {
		// Create cache middleware
		config := CacheConfig{
			Store:      NewMemoryCache(MemoryCacheConfig{MaxSize: 100, TTL: 5 * time.Minute}),
			DefaultTTL: 5 * time.Minute,
		}
		middleware := NewCacheMiddleware(config)

		// Test the hash function directly - this is the core security fix
		testString := "test-string-for-hashing"
		hash := middleware.hashString(testString)

		// Verify it's a SHA-256 hash (32 bytes)
		assert.Equal(t, 32, len(hash), "Hash should be 32 bytes for SHA-256")

		// Verify it matches expected SHA-256 output
		expectedHash := sha256.Sum256([]byte(testString))
		assert.Equal(t, expectedHash, hash, "Hash should match SHA-256 output")

		// Verify it's NOT the old MD5 length (16 bytes would indicate MD5)
		assert.NotEqual(t, 16, len(hash), "Hash should not be 16 bytes (MD5 length)")

		// Test with different strings to ensure proper hashing
		hash1 := middleware.hashString("string1")
		hash2 := middleware.hashString("string2")
		assert.NotEqual(t, hash1, hash2, "Different strings should produce different hashes")

		// Test deterministic behavior
		hash3 := middleware.hashString("string1")
		assert.Equal(t, hash1, hash3, "Same string should produce same hash")
	})

	t.Run("Basic cache middleware functionality", func(t *testing.T) {
		config := CacheConfig{
			Store:      NewMemoryCache(MemoryCacheConfig{MaxSize: 100, TTL: 5 * time.Minute}),
			DefaultTTL: 5 * time.Minute,
		}
		middleware := NewCacheMiddleware(config)

		// Create test context
		ctx := createCacheTestContext("GET", "/test", nil)

		// Generate cache key - this should work without User-Agent
		key := middleware.generateKey(ctx)
		assert.NotEmpty(t, key, "Cache key should not be empty")
		assert.Contains(t, key, "GET:/test", "Cache key should contain method and path")
	})

	t.Run("Cache middleware handles hash collisions securely", func(t *testing.T) {
		config := CacheConfig{
			Store:      NewMemoryCache(MemoryCacheConfig{MaxSize: 100, TTL: 5 * time.Minute}),
			DefaultTTL: 5 * time.Minute,
		}
		middleware := NewCacheMiddleware(config)

		// Test with strings that might have MD5 collisions but won't with SHA-256
		testStrings := []string{
			"collision-test-1",
			"collision-test-2",
		}

		hashes := make([][32]byte, len(testStrings))
		for i, str := range testStrings {
			hashes[i] = middleware.hashString(str)
		}

		// With SHA-256, these should produce different hashes
		assert.NotEqual(t, hashes[0], hashes[1], "SHA-256 should not have easy collisions")
	})
}

func TestMemoryCache(t *testing.T) {
	t.Run("Memory cache basic operations", func(t *testing.T) {
		cache := NewMemoryCache(MemoryCacheConfig{
			MaxSize: 10,
			TTL:     5 * time.Minute,
		})

		ctx := context.Background()

		// Test Set and Get
		err := cache.Set(ctx, "test-key", "test-value", 5*time.Minute)
		require.NoError(t, err)

		value, found, err := cache.Get(ctx, "test-key")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "test-value", value)

		// Test existence
		exists := cache.Exists("test-key")
		assert.True(t, exists)

		// Test non-existent key
		_, found, err = cache.Get(ctx, "non-existent")
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("Memory cache delete operation", func(t *testing.T) {
		cache := NewMemoryCache(MemoryCacheConfig{
			MaxSize: 10,
			TTL:     5 * time.Minute,
		})

		ctx := context.Background()

		// Set and verify
		err := cache.Set(ctx, "delete-test", "value", 5*time.Minute)
		require.NoError(t, err)

		exists := cache.Exists("delete-test")
		assert.True(t, exists)

		// Delete and verify
		err = cache.Delete(ctx, "delete-test")
		require.NoError(t, err)

		exists = cache.Exists("delete-test")
		assert.False(t, exists)
	})
}

// Helper function to create test context for caching tests
func createCacheTestContext(method, path string, body []byte) *lift.Context {
	adapterReq := &adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		PathParams:  make(map[string]string),
		Body:        body,
	}
	req := lift.NewRequest(adapterReq)
	return lift.NewContext(context.Background(), req)
}
