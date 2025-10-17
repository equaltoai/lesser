package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// DNSCacheRepository handles DNS cache operations with enhanced patterns
type DNSCacheRepository struct {
	*EnhancedBaseRepository[*models.DNSCache]
	logger *zap.Logger
	db     core.DB
}

// NewDNSCacheRepository creates a new DNS cache repository with enhanced functionality
func NewDNSCacheRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *DNSCacheRepository {
	// Create enhanced repository optimized for DNS cache operations
	enhancedRepo := NewEnhancedBaseRepository[*models.DNSCache](db, tableName, logger, costService, "DNSCacheRepository", "dns_cache")

	// Set up enhanced services for DNS cache operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // DNS entries cached in memory
	enhancedRepo.SetEventService(NewDefaultEventService())      // DNS resolution events

	return &DNSCacheRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
		db:                     db,
	}
}

// GetDNSCache retrieves a cached DNS lookup result
// Pattern: PK=DNSCACHE#hostname, SK=ENTRY
func (r *DNSCacheRepository) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	// Create model with keys set
	dnsCache := &models.DNSCache{
		Hostname: hostname,
	}
	_ = dnsCache.UpdateKeys() // Ignore error as this is internal model operation

	// Query for the entry using DynamORM pattern
	err := r.db.WithContext(ctx).Model(&models.DNSCache{}).
		Where("PK", "=", dnsCache.PK).
		Where("SK", "=", dnsCache.SK).
		First(dnsCache)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityDNSCache, hostname)
		}
		r.logger.Error("failed to get DNS cache entry",
			zap.String("hostname", hostname),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrDNSCacheGetFailed, err)
	}

	// Check if the entry has expired
	if dnsCache.ExpiresAt > 0 && time.Now().Unix() > dnsCache.ExpiresAt {
		_ = r.InvalidateDNSCache(ctx, hostname)
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityDNSCache, hostname)
	}

	// Convert to storage model
	entry := &storage.DNSCacheEntry{
		Hostname:   dnsCache.Hostname,
		IPs:        dnsCache.IPs,
		ResolvedAt: dnsCache.ResolvedAt,
		TTL:        int64(dnsCache.TTL),
	}

	return entry, nil
}

// SetDNSCache stores a DNS lookup result in the cache
// Pattern: PK=DNSCACHE#hostname, SK=ENTRY
func (r *DNSCacheRepository) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	if entry == nil {
		return ErrDNSCacheEntryRequired
	}

	// Calculate expiration time (current time + TTL)
	expiresAt := time.Now().Unix() + entry.TTL

	// Create DynamORM model
	dnsCache := &models.DNSCache{
		Hostname:   entry.Hostname,
		IPs:        entry.IPs,
		ResolvedAt: entry.ResolvedAt,
		TTL:        int(entry.TTL),
		ExpiresAt:  expiresAt,
	}
	_ = dnsCache.UpdateKeys() // Ignore error as this is internal model operation

	// Save to DynamoDB using DynamORM pattern
	if err := r.db.WithContext(ctx).Model(dnsCache).Create(); err != nil {
		r.logger.Error("failed to set DNS cache entry",
			zap.String("hostname", entry.Hostname),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrDNSCacheSetFailed, err)
	}

	r.logger.Debug("DNS cache entry stored",
		zap.String("hostname", entry.Hostname),
		zap.Int("ip_count", len(entry.IPs)),
		zap.Duration("ttl", time.Duration(entry.TTL)*time.Second))

	return nil
}

// InvalidateDNSCache removes a DNS cache entry
// Pattern: PK=DNSCACHE#hostname, SK=ENTRY
func (r *DNSCacheRepository) InvalidateDNSCache(ctx context.Context, hostname string) error {
	// Create model with keys set
	dnsCache := &models.DNSCache{
		Hostname: hostname,
	}
	_ = dnsCache.UpdateKeys() // Ignore error as this is internal model operation

	// Delete the entry
	err := r.db.WithContext(ctx).Model(&models.DNSCache{}).
		Where("PK", "=", dnsCache.PK).
		Where("SK", "=", dnsCache.SK).
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			// Not found is not an error for cache invalidation
			return nil
		}
		r.logger.Error("failed to invalidate DNS cache entry",
			zap.String("hostname", hostname),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrDNSCacheInvalidateFailed, err)
	}

	r.logger.Debug("DNS cache entry invalidated",
		zap.String("hostname", hostname))

	return nil
}
