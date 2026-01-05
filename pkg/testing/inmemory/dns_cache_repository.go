// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// DNSCacheRepository is a thread-safe in-memory implementation of interfaces.DNSCacheRepository.
type DNSCacheRepository struct {
	mu sync.RWMutex

	// Cache entries: hostname -> DNSCacheEntry
	entries map[string]*storage.DNSCacheEntry
}

// NewDNSCacheRepository creates a new in-memory DNS cache repository
func NewDNSCacheRepository() *DNSCacheRepository {
	return &DNSCacheRepository{
		entries: make(map[string]*storage.DNSCacheEntry),
	}
}

// GetDNSCache retrieves a cached DNS lookup result
func (r *DNSCacheRepository) GetDNSCache(_ context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[hostname]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// Check if entry has expired
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		return nil, storage.ErrNotFound
	}

	return entry, nil
}

// SetDNSCache stores a DNS lookup result in the cache
func (r *DNSCacheRepository) SetDNSCache(_ context.Context, entry *storage.DNSCacheEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry == nil {
		return storage.ErrInvalidInput
	}

	// Calculate expiration time if TTL is set
	if entry.TTL > 0 && entry.ExpiresAt.IsZero() {
		entry.ExpiresAt = time.Now().Add(time.Duration(entry.TTL) * time.Second)
	}

	r.entries[entry.Hostname] = entry
	return nil
}

// InvalidateDNSCache removes a DNS cache entry
func (r *DNSCacheRepository) InvalidateDNSCache(_ context.Context, hostname string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.entries, hostname)
	return nil
}

// Clear clears all data (test helper)
func (r *DNSCacheRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = make(map[string]*storage.DNSCacheEntry)
}

// CleanupExpired removes expired entries (test helper)
func (r *DNSCacheRepository) CleanupExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for hostname, entry := range r.entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(r.entries, hostname)
		}
	}
}

// Ensure DNSCacheRepository implements interfaces.DNSCacheRepository
var _ interfaces.DNSCacheRepository = (*DNSCacheRepository)(nil)
