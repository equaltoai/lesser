package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// QueryCacheRepository handles query cache operations
type QueryCacheRepository struct {
	db           core.DB
	tableName    string
	logger       *zap.Logger
	instanceRepo *FederationInstanceRepository
}

// NewQueryCacheRepository creates a new query cache repository
func NewQueryCacheRepository(db core.DB, tableName string, logger *zap.Logger, instanceRepo *FederationInstanceRepository) *QueryCacheRepository {
	return &QueryCacheRepository{
		db:           db,
		tableName:    tableName,
		logger:       logger,
		instanceRepo: instanceRepo,
	}
}

// GetCachedValue retrieves a cached value by key
func (r *QueryCacheRepository) GetCachedValue(ctx context.Context, cacheKey string) (interface{}, error) {
	var entry models.QueryCacheEntry
	pk := fmt.Sprintf("CACHE#%s", cacheKey)

	err := r.db.WithContext(ctx).Model(&models.QueryCacheEntry{}).
		Where("PK", "=", pk).
		Where("SK", "=", "ENTRY").
		First(&entry)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Cache miss
		}
		r.logger.Error("Failed to get cached value",
			zap.String("cacheKey", cacheKey),
			zap.Error(err))
		return nil, fmt.Errorf("get cached value: %w", err)
	}

	// Check if expired
	if entry.IsExpired() {
		r.logger.Debug("Cache entry expired", zap.String("cacheKey", cacheKey))
		return nil, nil
	}

	// Deserialize the value based on cache key pattern
	var result interface{}
	if err := json.Unmarshal([]byte(entry.Value), &result); err != nil {
		r.logger.Error("Failed to unmarshal cached value",
			zap.String("cacheKey", cacheKey),
			zap.Error(err))
		return nil, fmt.Errorf("unmarshal cached value: %w", err)
	}

	r.logger.Debug("Cache hit", zap.String("cacheKey", cacheKey))
	return result, nil
}

// SetCachedValue stores a value in the cache
func (r *QueryCacheRepository) SetCachedValue(ctx context.Context, cacheKey string, value interface{}, size int, ttl time.Duration) error {
	// Serialize the value
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal cache value: %w", err)
	}

	entry := &models.QueryCacheEntry{
		CacheKey:  cacheKey,
		Value:     string(valueBytes),
		Size:      size,
		ExpiresAt: time.Now().Add(ttl),
	}
	entry.UpdateKeys()

	err = r.db.WithContext(ctx).Model(entry).Create()
	if err != nil {
		r.logger.Error("Failed to set cached value",
			zap.String("cacheKey", cacheKey),
			zap.Error(err))
		return fmt.Errorf("set cached value: %w", err)
	}

	r.logger.Debug("Cached value",
		zap.String("cacheKey", cacheKey),
		zap.Int("size", size))
	return nil
}

// InvalidateCachePattern removes cache entries matching a pattern
func (r *QueryCacheRepository) InvalidateCachePattern(ctx context.Context, pattern string) error {
	// For simple patterns ending with *, do a prefix scan
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return r.invalidateCachePrefix(ctx, prefix)
	}

	// For exact match, delete specific key
	pk := fmt.Sprintf("CACHE#%s", pattern)
	err := r.db.WithContext(ctx).Model(&models.QueryCacheEntry{}).
		Where("PK", "=", pk).
		Where("SK", "=", "ENTRY").
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("Failed to invalidate cache entry",
			zap.String("pattern", pattern),
			zap.Error(err))
		return fmt.Errorf("invalidate cache entry: %w", err)
	}

	r.logger.Debug("Invalidated cache entry", zap.String("pattern", pattern))
	return nil
}

// invalidateCachePrefix removes all cache entries with keys starting with prefix
func (r *QueryCacheRepository) invalidateCachePrefix(ctx context.Context, prefix string) error {
	var entries []models.QueryCacheEntry

	pk := fmt.Sprintf("CACHE#%s", prefix)

	// Scan for entries with keys starting with prefix
	query := r.db.WithContext(ctx).Model(&models.QueryCacheEntry{}).
		Where("PK", "begins_with", pk).
		Where("SK", "=", "ENTRY")

	err := query.All(&entries)
	if err != nil {
		r.logger.Error("Failed to scan cache entries for prefix",
			zap.String("prefix", prefix),
			zap.Error(err))
		return fmt.Errorf("scan cache entries: %w", err)
	}

	// Delete each entry
	for _, entry := range entries {
		deleteErr := r.db.WithContext(ctx).Model(&entry).Delete()
		if deleteErr != nil && !errors.IsNotFound(deleteErr) {
			r.logger.Warn("Failed to delete cache entry",
				zap.String("cacheKey", entry.CacheKey),
				zap.Error(deleteErr))
		}
	}

	r.logger.Debug("Invalidated cache entries by prefix",
		zap.String("prefix", prefix),
		zap.Int("count", len(entries)))
	return nil
}

// GetInstance retrieves a cached instance or nil if not found
func (r *QueryCacheRepository) GetInstance(ctx context.Context, instanceID string) (*types.Instance, error) {
	cacheKey := fmt.Sprintf("instance:%s", instanceID)
	cached, err := r.GetCachedValue(ctx, cacheKey)
	if err != nil {
		return nil, err
	}
	if cached == nil {
		return nil, nil
	}

	// Convert from generic interface to Instance
	instanceMap, ok := cached.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("cached instance has invalid format")
	}

	// Deserialize to Instance struct
	instanceBytes, err := json.Marshal(instanceMap)
	if err != nil {
		return nil, fmt.Errorf("marshal instance map: %w", err)
	}

	var instance types.Instance
	if err := json.Unmarshal(instanceBytes, &instance); err != nil {
		return nil, fmt.Errorf("unmarshal instance: %w", err)
	}

	return &instance, nil
}

// SetInstance caches an instance
func (r *QueryCacheRepository) SetInstance(ctx context.Context, instance *types.Instance, ttl time.Duration) error {
	cacheKey := fmt.Sprintf("instance:%s", instance.ID)
	return r.SetCachedValue(ctx, cacheKey, instance, 1, ttl)
}

// GetInstancesByStatus retrieves cached instances by status with database fallback
func (r *QueryCacheRepository) GetInstancesByStatus(ctx context.Context, status types.InstanceStatus) ([]*types.Instance, error) {
	cacheKey := fmt.Sprintf("status:%s", status)
	cached, err := r.GetCachedValue(ctx, cacheKey)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		// Convert from generic interface to Instance slice
		instanceSlice, ok := cached.([]interface{})
		if ok {
			instances := make([]*types.Instance, 0, len(instanceSlice))
			for _, item := range instanceSlice {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				// Deserialize to Instance struct
				instanceBytes, err := json.Marshal(itemMap)
				if err != nil {
					continue
				}

				var instance types.Instance
				if err := json.Unmarshal(instanceBytes, &instance); err != nil {
					continue
				}

				instances = append(instances, &instance)
			}
			return instances, nil
		}
	}

	// Cache miss - fetch from database
	instances, err := r.instanceRepo.ListInstancesByStatus(ctx, status, 100)
	if err != nil {
		return nil, fmt.Errorf("get instances by status: %w", err)
	}

	// Cache the results
	if err := r.SetInstancesByStatus(ctx, status, instances, 5*time.Minute); err != nil {
		r.logger.Warn("Failed to cache instances by status",
			zap.String("status", string(status)),
			zap.Error(err))
	}

	return instances, nil
}

// SetInstancesByStatus caches instances by status
func (r *QueryCacheRepository) SetInstancesByStatus(ctx context.Context, status types.InstanceStatus, instances []*types.Instance, ttl time.Duration) error {
	cacheKey := fmt.Sprintf("status:%s", status)
	return r.SetCachedValue(ctx, cacheKey, instances, len(instances), ttl)
}

// BatchGetInstances performs batch get for multiple instances with cache and database fallback
func (r *QueryCacheRepository) BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*types.Instance, error) {
	instances := make([]*types.Instance, 0, len(instanceIDs))
	uncachedIDs := make([]string, 0)

	// Check cache for each instance
	for _, id := range instanceIDs {
		instance, err := r.GetInstance(ctx, id)
		if err != nil {
			r.logger.Warn("Error getting cached instance",
				zap.String("instanceID", id),
				zap.Error(err))
			uncachedIDs = append(uncachedIDs, id)
			continue
		}
		if instance != nil {
			instances = append(instances, instance)
		} else {
			uncachedIDs = append(uncachedIDs, id)
		}
	}

	// Fetch uncached instances from database if needed
	if len(uncachedIDs) > 0 {
		freshInstances, err := r.instanceRepo.BatchGetInstances(ctx, uncachedIDs)
		if err != nil {
			r.logger.Error("Failed to batch get instances from database",
				zap.Strings("instanceIDs", uncachedIDs),
				zap.Error(err))
			return instances, fmt.Errorf("batch get instances: %w", err)
		}

		// Cache the fresh instances and add to results
		for _, instance := range freshInstances {
			if err := r.SetInstance(ctx, instance, 5*time.Minute); err != nil {
				r.logger.Warn("Failed to cache instance",
					zap.String("instanceID", instance.ID),
					zap.Error(err))
			}
			instances = append(instances, instance)
		}
	}

	return instances, nil
}

// GetMetricsInRange retrieves delivery results for metrics queries
func (r *QueryCacheRepository) GetMetricsInRange(_ context.Context, routeID string, start, end time.Time, limit int) ([]*types.DeliveryResult, error) {
	// For metrics queries, we don't cache these as they're frequently updated
	// Use the route optimizer repository to get the data
	// This method would need to be implemented or we could integrate with route optimizer repository

	r.logger.Debug("Getting metrics in range (no caching for metrics)",
		zap.String("routeID", routeID),
		zap.Time("start", start),
		zap.Time("end", end),
		zap.Int("limit", limit))

	// For now, return empty slice - this would need actual implementation
	// or delegation to route optimizer repository
	return []*types.DeliveryResult{}, nil
}

// PrewarmActiveInstances preloads active instances into cache
func (r *QueryCacheRepository) PrewarmActiveInstances(ctx context.Context) error {
	r.logger.Info("Prewarming active instances cache")

	// Get active instances and cache them
	_, err := r.GetInstancesByStatus(ctx, types.InstanceStatusActive)
	if err != nil {
		return fmt.Errorf("prewarm active instances: %w", err)
	}

	return nil
}

// CleanupExpiredEntries removes expired cache entries (handled by TTL, but can be called manually)
func (r *QueryCacheRepository) CleanupExpiredEntries(_ context.Context) error {
	// Since we use TTL, this is mainly for manual cleanup if needed
	// In practice, DynamoDB will automatically remove expired items

	r.logger.Info("Cache cleanup requested - using TTL for automatic cleanup")
	return nil
}
