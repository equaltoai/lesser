// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// DNSCacheRepository defines the interface for DNS cache operations.
// This handles caching of DNS lookup results for performance optimization.
type DNSCacheRepository interface {
	// GetDNSCache retrieves a cached DNS lookup result
	GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error)

	// SetDNSCache stores a DNS lookup result in the cache
	SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error

	// InvalidateDNSCache removes a DNS cache entry
	InvalidateDNSCache(ctx context.Context, hostname string) error
}
