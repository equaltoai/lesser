package security

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptedSecretCache(t *testing.T) {
	// Generate a random encryption key for testing
	encryptionKey := make([]byte, 32)
	_, err := rand.Read(encryptionKey)
	require.NoError(t, err)

	cache, err := NewEncryptedSecretCache(5*time.Minute, encryptionKey)
	require.NoError(t, err)
	require.NotNil(t, cache)

	t.Run("Set and Get secret", func(t *testing.T) {
		key := "test-key"
		value := "test-secret-value"

		// Set secret
		err := cache.Set(key, value)
		require.NoError(t, err)

		// Get secret
		retrieved, err := cache.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("Get non-existent secret", func(t *testing.T) {
		retrieved, err := cache.Get("non-existent")
		require.NoError(t, err)
		assert.Equal(t, "", retrieved)
	})

	t.Run("Cache expiration", func(t *testing.T) {
		// Create cache with short TTL
		shortCache, err := NewEncryptedSecretCache(100*time.Millisecond, encryptionKey)
		require.NoError(t, err)

		key := "expiring-key"
		value := "expiring-value"

		// Set secret
		err = shortCache.Set(key, value)
		require.NoError(t, err)

		// Should be retrievable immediately
		retrieved, err := shortCache.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// Wait for expiration
		time.Sleep(150 * time.Millisecond)

		// Should be expired
		retrieved, err = shortCache.Get(key)
		require.NoError(t, err)
		assert.Equal(t, "", retrieved)
	})

	t.Run("Delete secret", func(t *testing.T) {
		key := "delete-key"
		value := "delete-value"

		// Set secret
		err := cache.Set(key, value)
		require.NoError(t, err)

		// Verify it exists
		retrieved, err := cache.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// Delete secret
		cache.Delete(key)

		// Verify it's gone
		retrieved, err = cache.Get(key)
		require.NoError(t, err)
		assert.Equal(t, "", retrieved)
	})

	t.Run("Clear cache", func(t *testing.T) {
		// Create fresh cache for this test
		freshCache, err := NewEncryptedSecretCache(5*time.Minute, encryptionKey)
		require.NoError(t, err)

		// Set multiple secrets
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("key-%d", i)
			value := fmt.Sprintf("value-%d", i)
			err := freshCache.Set(key, value)
			require.NoError(t, err)
		}

		// Verify cache has secrets
		assert.Equal(t, 5, freshCache.Size())

		// Clear cache
		freshCache.Clear()

		// Verify cache is empty
		assert.Equal(t, 0, freshCache.Size())
	})

	t.Run("Cache size tracking", func(t *testing.T) {
		initialSize := cache.Size()

		// Add secrets
		for i := 0; i < 3; i++ {
			key := fmt.Sprintf("size-key-%d", i)
			value := fmt.Sprintf("size-value-%d", i)
			err := cache.Set(key, value)
			require.NoError(t, err)
		}

		assert.Equal(t, initialSize+3, cache.Size())

		// Delete one secret
		cache.Delete("size-key-1")
		assert.Equal(t, initialSize+2, cache.Size())
	})

	t.Run("Cache info", func(t *testing.T) {
		info := cache.GetCacheInfo()
		require.NotNil(t, info)

		assert.Contains(t, info, "size")
		assert.Contains(t, info, "ttl_sec")
		assert.Contains(t, info, "encrypted")
		assert.Equal(t, true, info["encrypted"])
		assert.Equal(t, 300.0, info["ttl_sec"]) // 5 minutes
	})

	t.Run("Encryption with different keys", func(t *testing.T) {
		// Create another cache with different key
		differentKey := make([]byte, 32)
		_, err := rand.Read(differentKey)
		require.NoError(t, err)

		cache2, err := NewEncryptedSecretCache(5*time.Minute, differentKey)
		require.NoError(t, err)

		key := "cross-cache-key"
		value := "cross-cache-value"

		// Set in first cache
		err = cache.Set(key, value)
		require.NoError(t, err)

		// Should not be accessible from second cache (different encryption key)
		retrieved, err := cache2.Get(key)
		require.NoError(t, err)
		assert.Equal(t, "", retrieved)
	})

	t.Run("Concurrent access", func(t *testing.T) {
		const numGoroutines = 10
		const numOperations = 100

		done := make(chan bool, numGoroutines)

		// Launch goroutines for concurrent access
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() { done <- true }()

				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("concurrent-key-%d-%d", id, j)
					value := fmt.Sprintf("concurrent-value-%d-%d", id, j)

					// Set
					err := cache.Set(key, value)
					assert.NoError(t, err)

					// Get
					retrieved, err := cache.Get(key)
					assert.NoError(t, err)
					assert.Equal(t, value, retrieved)

					// Delete
					cache.Delete(key)
				}
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})
}

func TestEncryptedCacheVsPlainTextCache(t *testing.T) {
	encryptionKey := make([]byte, 32)
	_, err := rand.Read(encryptionKey)
	require.NoError(t, err)

	// Create both cache types
	encryptedCache, err := NewEncryptedSecretCache(5*time.Minute, encryptionKey)
	require.NoError(t, err)

	plainCache := NewSecretCache(5 * time.Minute)

	// Test data
	testSecrets := map[string]string{
		"api-key":     "sk-1234567890abcdef",
		"db-password": "super-secret-password",
		"jwt-secret":  "jwt-signing-secret-key",
		"private-key": "-----BEGIN PRIVATE KEY-----\nMIIEvQ...",
	}

	t.Run("Both caches work with same data", func(t *testing.T) {
		// Set in both caches
		for key, value := range testSecrets {
			err := encryptedCache.Set(key, value)
			require.NoError(t, err)

			plainCache.Set(key, value)
		}

		// Verify retrieval from both caches
		for key, expectedValue := range testSecrets {
			// Encrypted cache
			encryptedValue, err := encryptedCache.Get(key)
			require.NoError(t, err)
			assert.Equal(t, expectedValue, encryptedValue)

			// Plain cache
			plainValue := plainCache.Get(key)
			assert.Equal(t, expectedValue, plainValue)
		}
	})

	t.Run("Memory inspection security", func(t *testing.T) {
		// This test demonstrates that encrypted cache is more secure
		// In a real attack, plain text cache would expose secrets in memory
		// while encrypted cache would only show encrypted data

		// #nosec G101 -- Test API key for security testing demonstration
		secretValue := "super-sensitive-api-key"

		// Set in both caches
		err := encryptedCache.Set("memory-test", secretValue)
		require.NoError(t, err)

		plainCache.Set("memory-test", secretValue)

		// The encrypted cache stores the value encrypted in memory
		// The plain cache stores the value as plain text in memory
		// This test verifies the encrypted cache works correctly

		retrieved, err := encryptedCache.Get("memory-test")
		require.NoError(t, err)
		assert.Equal(t, secretValue, retrieved)
	})
}

func BenchmarkEncryptedCache(b *testing.B) {
	encryptionKey := make([]byte, 32)
	if _, err := rand.Read(encryptionKey); err != nil {
		b.Fatal("Failed to generate encryption key:", err)
	}

	cache, err := NewEncryptedSecretCache(5*time.Minute, encryptionKey)
	require.NoError(b, err)

	testValue := "benchmark-secret-value-that-is-reasonably-long"

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("bench-key-%d", i)
			cache.Set(key, testValue)
		}
	})

	// Pre-populate cache for Get benchmark
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("get-bench-key-%d", i)
		cache.Set(key, testValue)
	}

	b.Run("Get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("get-bench-key-%d", i%1000)
			cache.Get(key)
		}
	})
}
