package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"
	"time"
)

// EncryptedSecretCache provides encrypted in-memory caching for secrets with TTL
type EncryptedSecretCache struct {
	gcm     cipher.AEAD
	secrets map[string]*EncryptedCachedSecret
	key     []byte
	ttl     time.Duration
	mu      sync.RWMutex
}

// EncryptedCachedSecret represents an encrypted cached secret with expiration
type EncryptedCachedSecret struct {
	ExpiresAt      time.Time
	EncryptedValue []byte
	Nonce          []byte
}

// NewEncryptedSecretCache creates a new encrypted secret cache with the specified TTL
func NewEncryptedSecretCache(ttl time.Duration, encryptionKey []byte) (*EncryptedSecretCache, error) {
	// Derive a 32-byte key from the provided key using SHA-256
	hash := sha256.Sum256(encryptionKey)
	key := hash[:]

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	cache := &EncryptedSecretCache{
		secrets: make(map[string]*EncryptedCachedSecret),
		ttl:     ttl,
		gcm:     gcm,
		key:     key,
	}

	// Start background cleanup goroutine
	go cache.startCleanupRoutine()

	return cache, nil
}

// Set encrypts and stores a value in the cache with TTL
func (c *EncryptedSecretCache) Set(key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Generate random nonce
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the value
	plaintext := []byte(value)
	ciphertext := c.gcm.Seal(nil, nonce, plaintext, nil)

	c.secrets[key] = &EncryptedCachedSecret{
		EncryptedValue: ciphertext,
		Nonce:          nonce,
		ExpiresAt:      time.Now().Add(c.ttl),
	}

	// Clear the plaintext from memory (best effort)
	for i := range plaintext {
		plaintext[i] = 0
	}

	return nil
}

// Get retrieves and decrypts a value from the cache
func (c *EncryptedSecretCache) Get(key string) (string, error) {
	c.mu.RLock()
	secret, exists := c.secrets[key]
	c.mu.RUnlock()

	if !exists {
		return "", nil
	}

	// Check if expired (atomic check without race condition)
	if time.Now().After(secret.ExpiresAt) {
		c.mu.Lock()
		delete(c.secrets, key)
		c.mu.Unlock()
		return "", nil
	}

	// Decrypt the value
	plaintext, err := c.gcm.Open(nil, secret.Nonce, secret.EncryptedValue, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret: %w", err)
	}

	// Convert to string and clear the plaintext from memory
	value := string(plaintext)
	for i := range plaintext {
		plaintext[i] = 0
	}

	return value, nil
}

// Delete removes a value from the cache
func (c *EncryptedSecretCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.secrets, key)
}

// Clear removes all values from the cache and clears encryption keys
func (c *EncryptedSecretCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.secrets = make(map[string]*EncryptedCachedSecret)

	// Clear encryption key from memory
	for i := range c.key {
		c.key[i] = 0
	}
}

// Size returns the number of cached secrets
func (c *EncryptedSecretCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.secrets)
}

// startCleanupRoutine starts a background goroutine to clean expired secrets
func (c *EncryptedSecretCache) startCleanupRoutine() {
	ticker := time.NewTicker(c.ttl / 2) // Cleanup every half TTL
	defer ticker.Stop()

	for range ticker.C {
		c.cleanupExpired()
	}
}

// cleanupExpired removes expired secrets from the cache (thread-safe)
func (c *EncryptedSecretCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	// Collect expired keys first to avoid map iteration issues
	for key, secret := range c.secrets {
		if now.After(secret.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Delete expired keys
	for _, key := range expiredKeys {
		delete(c.secrets, key)
	}
}

// GetCacheInfo returns cache statistics (for monitoring)
func (c *EncryptedSecretCache) GetCacheInfo() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]any{
		"size":      len(c.secrets),
		"ttl_sec":   c.ttl.Seconds(),
		"encrypted": true,
	}
}
