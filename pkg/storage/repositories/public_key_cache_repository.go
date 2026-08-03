package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// PublicKeyCacheRepository handles caching of public keys for HTTP signature verification
type PublicKeyCacheRepository struct {
	*EnhancedBaseRepository[*models.PublicKeyCache]
}

// NewPublicKeyCacheRepository creates a new public key cache repository with enhanced functionality
func NewPublicKeyCacheRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *PublicKeyCacheRepository {
	// Create enhanced repository optimized for public key cache operations
	enhancedRepo := NewEnhancedBaseRepository[*models.PublicKeyCache](db, tableName, logger, costService, "PublicKeyCacheRepository", "public_key_cache")

	// Set up enhanced services for public key cache operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Keys cached in memory
	enhancedRepo.SetEventService(NewDefaultEventService())      // Key cache events

	return &PublicKeyCacheRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// GetByActorURL retrieves a cached public key by actor URL
func (r *PublicKeyCacheRepository) GetByActorURL(ctx context.Context, actorURL string) (*models.PublicKeyCache, error) {
	var cache models.PublicKeyCache
	cache.ActorURL = actorURL
	if err := cache.UpdateKeys(); err != nil {
		return nil, ErrorHandler.HandleUpdateError(err, EntityPublicKeyCache, actorURL)
	}

	err := r.db.WithContext(ctx).Model(&models.PublicKeyCache{}).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(&cache)

	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			r.logger.Debug("public key cache miss",
				zap.String("actor_url", actorURL))
			return nil, ErrorHandler.HandleGetError(err, EntityPublicKeyCache, actorURL)
		}
		r.logger.Error("failed to get cached public key",
			zap.Error(err),
			zap.String("actor_url", actorURL))
		return nil, ErrorHandler.HandleGetError(err, EntityPublicKeyCache, actorURL)
	}

	// Check if cache entry is still valid
	if !cache.IsValid() {
		r.logger.Debug("cached public key expired",
			zap.String("actor_url", actorURL),
			zap.Time("ttl", time.Unix(cache.TTL, 0)))
		// Delete expired entry to keep table clean
		deleteCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := r.ValidateAndDelete(deleteCtx, cache.PK, cache.SK); err != nil {
			r.logger.Warn("failed to delete expired cache entry",
				zap.Error(err),
				zap.String("actor_url", actorURL))
		}
		cancel()
		return nil, ErrorHandler.HandleGetError(errors.New("public key cache expired"), EntityPublicKeyCache, actorURL)
	}

	r.logger.Debug("public key cache hit",
		zap.String("actor_url", actorURL),
		zap.String("key_id", cache.KeyID))

	return &cache, nil
}

// Store caches a public key with automatic TTL
func (r *PublicKeyCacheRepository) Store(ctx context.Context, actorURL, keyID, publicKeyPEM, algorithm string) (*models.PublicKeyCache, error) {
	cache := models.NewPublicKeyCache(actorURL, keyID, publicKeyPEM, algorithm)

	err := r.ValidateAndCreate(ctx, cache)
	if err != nil {
		r.logger.Error("failed to store public key cache",
			zap.Error(err),
			zap.String("actor_url", actorURL),
			zap.String("key_id", keyID))
		return nil, ErrorHandler.HandleCreateError(err, EntityPublicKeyCache, actorURL)
	}

	r.logger.Debug("stored public key in cache",
		zap.String("actor_url", actorURL),
		zap.String("key_id", keyID),
		zap.Time("ttl", time.Unix(cache.TTL, 0)))

	return cache, nil
}

// UpdateStats updates the success/failure statistics for a cached key
func (r *PublicKeyCacheRepository) UpdateStats(ctx context.Context, actorURL string, success bool) error {
	var cache models.PublicKeyCache
	cache.ActorURL = actorURL
	if err := cache.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityPublicKeyCache, actorURL)
	}

	// First get the current entry
	err := r.db.WithContext(ctx).Model(&models.PublicKeyCache{}).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(&cache)

	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			// Entry doesn't exist, nothing to update
			return nil
		}
		return ErrorHandler.HandleGetError(err, EntityPublicKeyCache, actorURL)
	}

	// Update stats
	if success {
		cache.RecordSuccess()
	} else {
		cache.RecordFailure()
	}

	// Update only the mutable counters and timestamps to avoid rewriting the PEM payload.
	update := r.db.WithContext(ctx).Model(&cache).UpdateBuilder().
		Set("SuccessCount", cache.SuccessCount).
		Set("FailureCount", cache.FailureCount)
	if success {
		update.Set("LastUsed", cache.LastUsed)
	}

	err = update.Execute()

	if err != nil {
		r.logger.Error("failed to update cache entry stats",
			zap.Error(err),
			zap.String("actor_url", actorURL),
			zap.Bool("success", success))
		return ErrorHandler.HandleUpdateError(err, EntityPublicKeyCache, actorURL)
	}

	r.logger.Debug("updated cache entry stats",
		zap.String("actor_url", actorURL),
		zap.Bool("success", success),
		zap.Int("success_count", cache.SuccessCount),
		zap.Int("failure_count", cache.FailureCount))

	return nil
}

// RefreshKey updates an existing cache entry with a new public key
func (r *PublicKeyCacheRepository) RefreshKey(ctx context.Context, actorURL, keyID, publicKeyPEM, algorithm string) error {
	var cache models.PublicKeyCache
	cache.ActorURL = actorURL
	if err := cache.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityPublicKeyCache, actorURL)
	}

	// Check if entry exists first
	err := r.db.WithContext(ctx).Model(&models.PublicKeyCache{}).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(&cache)

	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			// Entry doesn't exist, create new one
			_, err := r.Store(ctx, actorURL, keyID, publicKeyPEM, algorithm)
			return err
		}
		return ErrorHandler.HandleGetError(err, EntityPublicKeyCache, actorURL)
	}

	// Update the key data
	cache.KeyID = keyID
	cache.PublicKeyPEM = publicKeyPEM
	cache.Algorithm = algorithm
	cache.FetchedAt = time.Now()
	cache.ExtendTTL(24 * time.Hour)
	// Reset failure count on refresh
	cache.FailureCount = 0

	err = r.db.WithContext(ctx).Model(&cache).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		Update()

	if err != nil {
		r.logger.Error("failed to refresh cached public key",
			zap.Error(err),
			zap.String("actor_url", actorURL),
			zap.String("key_id", keyID))
		return ErrorHandler.HandleUpdateError(err, EntityPublicKeyCache, actorURL)
	}

	r.logger.Debug("refreshed cached public key",
		zap.String("actor_url", actorURL),
		zap.String("key_id", keyID))

	return nil
}

// InvalidateCache removes a cached key (useful when verification fails consistently)
func (r *PublicKeyCacheRepository) InvalidateCache(ctx context.Context, actorURL string) error {
	var cache models.PublicKeyCache
	cache.ActorURL = actorURL
	if err := cache.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityPublicKeyCache, actorURL)
	}

	err := r.ValidateAndDelete(ctx, cache.PK, cache.SK)
	if err != nil && !dynamormerrors.IsNotFound(err) {
		r.logger.Error("failed to invalidate cache",
			zap.Error(err),
			zap.String("actor_url", actorURL))
		return ErrorHandler.HandleDeleteError(err, EntityPublicKeyCache, actorURL)
	}

	r.logger.Debug("invalidated cache entry",
		zap.String("actor_url", actorURL))

	return nil
}
