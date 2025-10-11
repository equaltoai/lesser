// Package cache provides efficient caching mechanisms for federation operations,
// particularly focused on caching public keys and instance metadata to improve
// ActivityPub federation performance and reduce external API calls.
package cache

import (
	"crypto"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// FederationCache provides caching for federation-related data with TTL support
type FederationCache struct {
	// Public key cache
	publicKeys map[string]*PublicKeyEntry
	pkMu       sync.RWMutex

	// Instance metadata cache
	instances map[string]*InstanceEntry
	instMu    sync.RWMutex

	// Actor profile cache
	actors  map[string]*ActorEntry
	actorMu sync.RWMutex

	// Configuration
	publicKeyTTL time.Duration
	instanceTTL  time.Duration
	actorTTL     time.Duration
	maxEntries   int

	// Storage backend (optional, for persistence) - interface to be defined by implementer
	storage interface{} // Should implement persistence methods
	logger  *zap.Logger

	// Cleanup goroutine control
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// PublicKeyEntry represents a cached public key with metadata
type PublicKeyEntry struct {
	Key       crypto.PublicKey `json:"key"`
	KeyID     string           `json:"key_id"`
	Owner     string           `json:"owner"`     // Actor URI who owns this key
	Algorithm string           `json:"algorithm"` // Key algorithm (RSA, Ed25519, etc.)
	CreatedAt time.Time        `json:"created_at"`
	ExpiresAt time.Time        `json:"expires_at"`
}

// InstanceEntry represents cached instance metadata
type InstanceEntry struct {
	Domain      string                 `json:"domain"`
	Metadata    map[string]interface{} `json:"metadata"`
	Available   bool                   `json:"available"`
	LastChecked time.Time              `json:"last_checked"`
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
}

// ActorEntry represents a cached actor profile
type ActorEntry struct {
	ActorID     string                 `json:"actor_id"`
	Username    string                 `json:"username"`
	Domain      string                 `json:"domain"`
	PublicKeyID string                 `json:"public_key_id"`
	Profile     map[string]interface{} `json:"profile"`
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
}

// Config holds configuration for FederationCache
type CacheConfig struct {
	PublicKeyTTL    time.Duration // Default: 1 hour
	InstanceTTL     time.Duration // Default: 30 minutes
	ActorTTL        time.Duration // Default: 15 minutes
	MaxEntries      int           // Default: 10000
	CleanupInterval time.Duration // Default: 5 minutes
}

// DefaultCacheConfig returns sensible defaults for federation caching
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		PublicKeyTTL:    1 * time.Hour,
		InstanceTTL:     30 * time.Minute,
		ActorTTL:        15 * time.Minute,
		MaxEntries:      10000,
		CleanupInterval: 5 * time.Minute,
	}
}

// NewFederationCache creates a new federation cache with the given configuration
func NewFederationCache(config CacheConfig, storage interface{}, logger *zap.Logger) *FederationCache {
	if config.PublicKeyTTL == 0 {
		config.PublicKeyTTL = 1 * time.Hour
	}
	if config.InstanceTTL == 0 {
		config.InstanceTTL = 30 * time.Minute
	}
	if config.ActorTTL == 0 {
		config.ActorTTL = 15 * time.Minute
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = 10000
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 5 * time.Minute
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	fc := &FederationCache{
		publicKeys:   make(map[string]*PublicKeyEntry),
		instances:    make(map[string]*InstanceEntry),
		actors:       make(map[string]*ActorEntry),
		publicKeyTTL: config.PublicKeyTTL,
		instanceTTL:  config.InstanceTTL,
		actorTTL:     config.ActorTTL,
		maxEntries:   config.MaxEntries,
		storage:      storage,
		logger:       logger,
		stopCleanup:  make(chan struct{}),
		cleanupDone:  make(chan struct{}),
	}

	// Start background cleanup goroutine
	go fc.cleanupLoop(config.CleanupInterval)

	return fc
}

// Public Key Cache Methods

// GetPublicKey retrieves a cached public key by key ID
func (fc *FederationCache) GetPublicKey(keyID string) (*PublicKeyEntry, bool) {
	fc.pkMu.RLock()
	entry, exists := fc.publicKeys[keyID]
	fc.pkMu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		fc.InvalidatePublicKey(keyID)
		return nil, false
	}

	return entry, true
}

// SetPublicKey stores a public key in the cache
func (fc *FederationCache) SetPublicKey(keyID string, key crypto.PublicKey, owner, algorithm string) {
	now := time.Now()
	entry := &PublicKeyEntry{
		Key:       key,
		KeyID:     keyID,
		Owner:     owner,
		Algorithm: algorithm,
		CreatedAt: now,
		ExpiresAt: now.Add(fc.publicKeyTTL),
	}

	fc.pkMu.Lock()
	fc.publicKeys[keyID] = entry
	fc.pkMu.Unlock()

	// Persist to storage if available
	if fc.storage != nil {
		go fc.persistPublicKey(keyID, entry)
	}

	fc.logger.Debug("cached public key",
		zap.String("key_id", keyID),
		zap.String("owner", owner),
		zap.String("algorithm", algorithm),
		zap.Time("expires_at", entry.ExpiresAt))
}

// InvalidatePublicKey removes a public key from the cache
func (fc *FederationCache) InvalidatePublicKey(keyID string) {
	fc.pkMu.Lock()
	delete(fc.publicKeys, keyID)
	fc.pkMu.Unlock()

	// Remove from persistent storage if available
	if fc.storage != nil {
		go fc.removePersistentPublicKey(keyID)
	}

	fc.logger.Debug("invalidated public key", zap.String("key_id", keyID))
}

// Instance Metadata Cache Methods

// GetInstance retrieves cached instance metadata by domain
func (fc *FederationCache) GetInstance(domain string) (*InstanceEntry, bool) {
	fc.instMu.RLock()
	entry, exists := fc.instances[domain]
	fc.instMu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		fc.InvalidateInstance(domain)
		return nil, false
	}

	return entry, true
}

// SetInstance stores instance metadata in the cache
func (fc *FederationCache) SetInstance(domain string, metadata map[string]interface{}, available bool) {
	now := time.Now()
	entry := &InstanceEntry{
		Domain:      domain,
		Metadata:    metadata,
		Available:   available,
		LastChecked: now,
		CreatedAt:   now,
		ExpiresAt:   now.Add(fc.instanceTTL),
	}

	fc.instMu.Lock()
	fc.instances[domain] = entry
	fc.instMu.Unlock()

	// Persist to storage if available
	if fc.storage != nil {
		go fc.persistInstance(domain, entry)
	}

	fc.logger.Debug("cached instance metadata",
		zap.String("domain", domain),
		zap.Bool("available", available),
		zap.Time("expires_at", entry.ExpiresAt))
}

// InvalidateInstance removes instance metadata from the cache
func (fc *FederationCache) InvalidateInstance(domain string) {
	fc.instMu.Lock()
	delete(fc.instances, domain)
	fc.instMu.Unlock()

	// Remove from persistent storage if available
	if fc.storage != nil {
		go fc.removePersistentInstance(domain)
	}

	fc.logger.Debug("invalidated instance", zap.String("domain", domain))
}

// Actor Profile Cache Methods

// GetActor retrieves a cached actor profile by actor ID
func (fc *FederationCache) GetActor(actorID string) (*ActorEntry, bool) {
	fc.actorMu.RLock()
	entry, exists := fc.actors[actorID]
	fc.actorMu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		fc.InvalidateActor(actorID)
		return nil, false
	}

	return entry, true
}

// SetActor stores an actor profile in the cache
func (fc *FederationCache) SetActor(actorID, username, domain, publicKeyID string, profile map[string]interface{}) {
	now := time.Now()
	entry := &ActorEntry{
		ActorID:     actorID,
		Username:    username,
		Domain:      domain,
		PublicKeyID: publicKeyID,
		Profile:     profile,
		CreatedAt:   now,
		ExpiresAt:   now.Add(fc.actorTTL),
	}

	fc.actorMu.Lock()
	fc.actors[actorID] = entry
	fc.actorMu.Unlock()

	// Persist to storage if available
	if fc.storage != nil {
		go fc.persistActor(actorID, entry)
	}

	fc.logger.Debug("cached actor profile",
		zap.String("actor_id", actorID),
		zap.String("username", username),
		zap.String("domain", domain),
		zap.Time("expires_at", entry.ExpiresAt))
}

// InvalidateActor removes an actor profile from the cache
func (fc *FederationCache) InvalidateActor(actorID string) {
	fc.actorMu.Lock()
	delete(fc.actors, actorID)
	fc.actorMu.Unlock()

	// Remove from persistent storage if available
	if fc.storage != nil {
		go fc.removePersistentActor(actorID)
	}

	fc.logger.Debug("invalidated actor", zap.String("actor_id", actorID))
}

// Cache Management Methods

// GetStats returns cache statistics
func (fc *FederationCache) GetStats() CacheStats {
	fc.pkMu.RLock()
	publicKeyCount := len(fc.publicKeys)
	fc.pkMu.RUnlock()

	fc.instMu.RLock()
	instanceCount := len(fc.instances)
	fc.instMu.RUnlock()

	fc.actorMu.RLock()
	actorCount := len(fc.actors)
	fc.actorMu.RUnlock()

	return CacheStats{
		PublicKeyEntries: publicKeyCount,
		InstanceEntries:  instanceCount,
		ActorEntries:     actorCount,
		TotalEntries:     publicKeyCount + instanceCount + actorCount,
	}
}

// CacheStats holds cache usage statistics
type CacheStats struct {
	PublicKeyEntries int `json:"public_key_entries"`
	InstanceEntries  int `json:"instance_entries"`
	ActorEntries     int `json:"actor_entries"`
	TotalEntries     int `json:"total_entries"`
}

// Clear removes all entries from the cache
func (fc *FederationCache) Clear() {
	fc.pkMu.Lock()
	fc.publicKeys = make(map[string]*PublicKeyEntry)
	fc.pkMu.Unlock()

	fc.instMu.Lock()
	fc.instances = make(map[string]*InstanceEntry)
	fc.instMu.Unlock()

	fc.actorMu.Lock()
	fc.actors = make(map[string]*ActorEntry)
	fc.actorMu.Unlock()

	fc.logger.Info("cleared all cache entries")
}

// Close stops the cache cleanup goroutine and cleans up resources
func (fc *FederationCache) Close() {
	close(fc.stopCleanup)
	<-fc.cleanupDone
}

// Background cleanup and persistence methods

func (fc *FederationCache) cleanupLoop(interval time.Duration) {
	defer close(fc.cleanupDone)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fc.cleanup()
		case <-fc.stopCleanup:
			return
		}
	}
}

func (fc *FederationCache) cleanup() {
	now := time.Now()

	// Cleanup expired public keys
	fc.pkMu.Lock()
	for keyID, entry := range fc.publicKeys {
		if now.After(entry.ExpiresAt) {
			delete(fc.publicKeys, keyID)
		}
	}
	fc.pkMu.Unlock()

	// Cleanup expired instances
	fc.instMu.Lock()
	for domain, entry := range fc.instances {
		if now.After(entry.ExpiresAt) {
			delete(fc.instances, domain)
		}
	}
	fc.instMu.Unlock()

	// Cleanup expired actors
	fc.actorMu.Lock()
	for actorID, entry := range fc.actors {
		if now.After(entry.ExpiresAt) {
			delete(fc.actors, actorID)
		}
	}
	fc.actorMu.Unlock()

	// Log cleanup stats
	stats := fc.GetStats()
	fc.logger.Debug("cache cleanup completed",
		zap.Int("total_entries", stats.TotalEntries),
		zap.Int("public_keys", stats.PublicKeyEntries),
		zap.Int("instances", stats.InstanceEntries),
		zap.Int("actors", stats.ActorEntries))
}

// Persistence methods (implementation depends on storage backend)

func (fc *FederationCache) persistPublicKey(keyID string, entry *PublicKeyEntry) {
	if fc.storage == nil {
		return
	}

	// Convert to storage format
	data, err := json.Marshal(entry)
	if err != nil {
		fc.logger.Warn("failed to marshal public key for persistence",
			zap.String("key_id", keyID),
			zap.Error(err))
		return
	}

	// Store using the configured storage backend
	// Implementation would depend on the specific storage interface provided
	// Example: fc.storage.SetPublicKey(ctx, keyID, data, entry.ExpiresAt)
	fc.logger.Debug("would persist public key to storage",
		zap.String("key_id", keyID),
		zap.Int("data_size", len(data)))
}

func (fc *FederationCache) persistInstance(domain string, entry *InstanceEntry) {
	// Similar implementation for instance persistence
	// Implementation would depend on the specific storage repository methods
}

func (fc *FederationCache) persistActor(actorID string, entry *ActorEntry) {
	// Similar implementation for actor persistence
	// Implementation would depend on the specific storage repository methods
}

func (fc *FederationCache) removePersistentPublicKey(keyID string) {
	if fc.storage == nil {
		return
	}

	// Implementation would depend on the specific storage interface provided
	// Example: fc.storage.DeletePublicKey(ctx, keyID)
	fc.logger.Debug("would remove persistent public key",
		zap.String("key_id", keyID))
}

func (fc *FederationCache) removePersistentInstance(domain string) {
	// Similar implementation for instance removal
}

func (fc *FederationCache) removePersistentActor(actorID string) {
	// Similar implementation for actor removal
}

// Utility methods for common federation operations

// GetOrFetchPublicKey attempts to get from cache, falls back to fetch function if not found
func (fc *FederationCache) GetOrFetchPublicKey(keyID string, fetchFn func() (crypto.PublicKey, string, string, error)) (crypto.PublicKey, error) {
	// Try cache first
	if entry, found := fc.GetPublicKey(keyID); found {
		fc.logger.Debug("public key cache hit", zap.String("key_id", keyID))
		return entry.Key, nil
	}

	// Cache miss, fetch from remote
	fc.logger.Debug("public key cache miss, fetching", zap.String("key_id", keyID))
	key, owner, algorithm, err := fetchFn()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch public key %s: %w", keyID, err)
	}

	// Cache the result
	fc.SetPublicKey(keyID, key, owner, algorithm)
	return key, nil
}

// GetOrFetchInstance attempts to get from cache, falls back to fetch function if not found
func (fc *FederationCache) GetOrFetchInstance(domain string, fetchFn func() (map[string]interface{}, bool, error)) (*InstanceEntry, error) {
	// Try cache first
	if entry, found := fc.GetInstance(domain); found {
		fc.logger.Debug("instance cache hit", zap.String("domain", domain))
		return entry, nil
	}

	// Cache miss, fetch from remote
	fc.logger.Debug("instance cache miss, fetching", zap.String("domain", domain))
	metadata, available, err := fetchFn()
	if err != nil {
		// Cache negative result for a shorter time
		fc.SetInstance(domain, nil, false)
		return nil, fmt.Errorf("failed to fetch instance %s: %w", domain, err)
	}

	// Cache the result
	fc.SetInstance(domain, metadata, available)

	// Return the newly cached entry
	entry, _ := fc.GetInstance(domain)
	return entry, nil
}
