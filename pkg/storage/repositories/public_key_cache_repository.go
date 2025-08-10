package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// PublicKeyCacheRepository handles caching of public keys for HTTP signature verification
type PublicKeyCacheRepository struct {
	*BaseRepository[*models.PublicKeyCache]
}

// NewPublicKeyCacheRepository creates a new public key cache repository
func NewPublicKeyCacheRepository(db core.DB, tableName string, logger *zap.Logger) *PublicKeyCacheRepository {
	return &PublicKeyCacheRepository{
		BaseRepository: NewBaseRepository[*models.PublicKeyCache](db, tableName, logger),
	}
}

// GetByActorURL retrieves a cached public key by actor URL
func (r *PublicKeyCacheRepository) GetByActorURL(ctx context.Context, actorURL string) (*models.PublicKeyCache, error) {
	var cache models.PublicKeyCache
	cache.ActorURL = actorURL
	if err := cache.UpdateKeys(); err != nil {
		return nil, fmt.Errorf("failed to update keys: %w", err)
	}

	err := r.db.WithContext(ctx).Model(&models.PublicKeyCache{}).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(&cache)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("public key cache miss",
				zap.String("actor_url", actorURL))
			return nil, fmt.Errorf("public key cache not found for actor: %s", actorURL)
		}
		r.logger.Error("failed to get cached public key",
			zap.Error(err),
			zap.String("actor_url", actorURL))
		return nil, fmt.Errorf("failed to get cached public key: %w", err)
	}

	// Check if cache entry is still valid
	if !cache.IsValid() {
		r.logger.Debug("cached public key expired",
			zap.String("actor_url", actorURL),
			zap.Time("ttl", time.Unix(cache.TTL, 0)))
		// Delete expired entry to keep table clean
		go func() {
			deleteCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := r.Delete(deleteCtx, cache.PK, cache.SK); err != nil {
				r.logger.Warn("failed to delete expired cache entry",
					zap.Error(err),
					zap.String("actor_url", actorURL))
			}
		}()
		return nil, fmt.Errorf("public key cache expired for actor: %s", actorURL)
	}

	r.logger.Debug("public key cache hit",
		zap.String("actor_url", actorURL),
		zap.String("key_id", cache.KeyID))

	return &cache, nil
}

// Store caches a public key with automatic TTL
func (r *PublicKeyCacheRepository) Store(ctx context.Context, actorURL, keyID, publicKeyPEM, algorithm string) (*models.PublicKeyCache, error) {
	cache := models.NewPublicKeyCache(actorURL, keyID, publicKeyPEM, algorithm)

	err := r.Create(ctx, cache)
	if err != nil {
		r.logger.Error("failed to store public key cache",
			zap.Error(err),
			zap.String("actor_url", actorURL),
			zap.String("key_id", keyID))
		return nil, fmt.Errorf("failed to store public key cache: %w", err)
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
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// First get the current entry
	err := r.db.WithContext(ctx).Model(&models.PublicKeyCache{}).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(&cache)

	if err != nil {
		if errors.IsNotFound(err) {
			// Entry doesn't exist, nothing to update
			return nil
		}
		return fmt.Errorf("failed to get cache entry for stats update: %w", err)
	}

	// Update stats
	if success {
		cache.RecordSuccess()
	} else {
		cache.RecordFailure()
	}

	// Update the entry
	err = r.db.WithContext(ctx).Model(&cache).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		Update()

	if err != nil {
		r.logger.Error("failed to update cache entry stats",
			zap.Error(err),
			zap.String("actor_url", actorURL),
			zap.Bool("success", success))
		return fmt.Errorf("failed to update cache entry stats: %w", err)
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
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Check if entry exists first
	err := r.db.WithContext(ctx).Model(&models.PublicKeyCache{}).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(&cache)

	if err != nil {
		if errors.IsNotFound(err) {
			// Entry doesn't exist, create new one
			_, err := r.Store(ctx, actorURL, keyID, publicKeyPEM, algorithm)
			return err
		}
		return fmt.Errorf("failed to get existing cache entry: %w", err)
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
		return fmt.Errorf("failed to refresh cached public key: %w", err)
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
		return fmt.Errorf("failed to update keys: %w", err)
	}

	err := r.Delete(ctx, cache.PK, cache.SK)
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to invalidate cache",
			zap.Error(err),
			zap.String("actor_url", actorURL))
		return fmt.Errorf("failed to invalidate cache: %w", err)
	}

	r.logger.Debug("invalidated cache entry",
		zap.String("actor_url", actorURL))

	return nil
}