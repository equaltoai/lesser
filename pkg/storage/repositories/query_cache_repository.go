package repositories

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// QueryCacheRepository handles query cache operations using enhanced patterns
type QueryCacheRepository struct {
	*EnhancedBaseRepository[*models.QueryCacheEntry]
	instanceRepo *FederationInstanceRepository
	routeRepo    *RouteOptimizerRepository
}

// NewQueryCacheRepository creates a new query cache repository with enhanced functionality
func NewQueryCacheRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService, instanceRepo *FederationInstanceRepository, routeRepo *RouteOptimizerRepository) *QueryCacheRepository {
	// Create enhanced repository optimized for query cache operations
	enhancedRepo := NewEnhancedBaseRepository[*models.QueryCacheEntry](db, tableName, logger, costService, "QueryCacheRepository", "query_cache")

	// Set up enhanced services for query cache operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Query cache cached in memory
	enhancedRepo.SetEventService(NewDefaultEventService())      // Cache events

	return &QueryCacheRepository{
		EnhancedBaseRepository: enhancedRepo,
		instanceRepo:           instanceRepo,
		routeRepo:              routeRepo,
	}
}

func queryCacheNamespace(cacheKey string) string {
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		return ""
	}
	if colon := strings.Index(cacheKey, ":"); colon > 0 {
		return cacheKey[:colon]
	}
	return cacheKey
}

func queryCacheKeys(cacheKey string) (pk, sk string) {
	namespace := queryCacheNamespace(cacheKey)
	return fmt.Sprintf("CACHE#%s", namespace), fmt.Sprintf("KEY#%s", cacheKey)
}

// GetCachedValue retrieves a cached value by key
func (r *QueryCacheRepository) GetCachedValue(ctx context.Context, cacheKey string) (interface{}, error) {
	var entry models.QueryCacheEntry
	pk, sk := queryCacheKeys(cacheKey)

	err := r.Get(ctx, pk, sk, &entry)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) || strings.Contains(err.Error(), "not found") {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityQueryCache, cacheKey)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityQueryCache, cacheKey)
	}

	// Check if expired
	if entry.IsExpired() {
		_ = r.Delete(ctx, entry.GetPK(), entry.GetSK())
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityQueryCache, cacheKey)
	}

	// Deserialize the value based on cache key pattern
	var result interface{}
	if err := json.Unmarshal([]byte(entry.Value), &result); err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityQueryCache, cacheKey)
	}

	return result, nil
}

// SetCachedValue stores a value in the cache
func (r *QueryCacheRepository) SetCachedValue(ctx context.Context, cacheKey string, value interface{}, size int, ttl time.Duration) error {
	// Serialize the value
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityQueryCache, cacheKey)
	}

	entry := &models.QueryCacheEntry{
		CacheKey:  cacheKey,
		Value:     string(valueBytes),
		Size:      size,
		ExpiresAt: time.Now().Add(ttl),
	}

	err = r.CreateOrUpdate(ctx, entry)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityQueryCache, cacheKey)
	}

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
	pk, sk := queryCacheKeys(pattern)
	err := r.Delete(ctx, pk, sk)

	if err != nil && !strings.Contains(err.Error(), "not found") {
		return ErrorHandler.HandleDeleteError(err, EntityQueryCache, pattern)
	}

	return nil
}

// invalidateCachePrefix removes all cache entries with keys starting with prefix
func (r *QueryCacheRepository) invalidateCachePrefix(ctx context.Context, prefix string) error {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}

	namespace := queryCacheNamespace(prefix)
	pk := fmt.Sprintf("CACHE#%s", namespace)
	skPrefix := fmt.Sprintf("KEY#%s", prefix)

	cursor := ""
	const pageLimit = 200

	for {
		var entries []models.QueryCacheEntry
		// One SK key condition (issue #1500): begins_with on the first page;
		// with a cursor key the exclusive `>` bound and demote begins_with to
		// a post-read FilterExpression.
		query := r.GetDB().WithContext(ctx).Model(&models.QueryCacheEntry{}).
			Where("PK", "=", pk).
			OrderBy("SK", "ASC").
			Limit(pageLimit + 1)

		if cursor != "" {
			query = query.Where("SK", ">", cursor).Filter("SK", "begins_with", skPrefix)
		} else {
			query = query.Where("SK", "begins_with", skPrefix)
		}

		err := query.All(&entries)
		if err != nil {
			return ErrorHandler.HandleQueryError(err, EntityQueryCache, "prefix query")
		}

		if len(entries) == 0 {
			break
		}

		hasMore := len(entries) > pageLimit
		if hasMore {
			entries = entries[:pageLimit]
		}

		keys := make([]struct{ PK, SK string }, len(entries))
		for i := range entries {
			keys[i] = struct{ PK, SK string }{PK: entries[i].PK, SK: entries[i].SK}
		}

		// Ignore individual deletion failures so invalidation doesn't fail the caller path.
		for _, key := range keys {
			_ = r.Delete(ctx, key.PK, key.SK)
		}

		if !hasMore {
			break
		}
		cursor = entries[len(entries)-1].SK
	}

	return nil
}

// GetInstance retrieves a cached instance or nil if not found
func (r *QueryCacheRepository) GetInstance(ctx context.Context, instanceID string) (*types.Instance, error) {
	cacheKey := fmt.Sprintf("instance:%s", instanceID)
	cached, err := r.GetCachedValue(ctx, cacheKey)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		return nil, err
	}

	// Convert from generic interface to Instance
	instanceMap, ok := cached.(map[string]interface{})
	if !ok {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityQueryCache, instanceID)
	}

	// Deserialize to Instance struct
	instanceBytes, err := json.Marshal(instanceMap)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityQueryCache, "instance")
	}

	var instance types.Instance
	if err := json.Unmarshal(instanceBytes, &instance); err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityQueryCache, "instance")
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
		if !stdErrors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	} else if cached != nil {
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
		return nil, ErrorHandler.HandleQueryError(err, EntityQueryCache, "instances by status")
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
			return instances, ErrorHandler.HandleQueryError(err, EntityQueryCache, "batch get instances")
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
func (r *QueryCacheRepository) GetMetricsInRange(ctx context.Context, routeID string, start, end time.Time, limit int) ([]*types.DeliveryResult, error) {
	r.logger.Debug("Getting metrics from cache repository",
		zap.String("routeID", routeID),
		zap.Time("start", start),
		zap.Time("end", end),
		zap.Int("limit", limit))

	// Metrics queries bypass cache and go directly to route repository for real-time data
	if r.routeRepo == nil {
		return nil, ErrorHandler.HandleQueryError(storage.ErrInvalidInput, EntityQueryCache, "metrics")
	}

	// Delegate to the route optimizer repository for actual metrics data
	return r.routeRepo.GetMetricsInRange(ctx, routeID, start, end, limit)
}

// PrewarmActiveInstances preloads active instances into cache
func (r *QueryCacheRepository) PrewarmActiveInstances(ctx context.Context) error {
	r.logger.Info("Prewarming active instances cache")

	// Get active instances and cache them
	_, err := r.GetInstancesByStatus(ctx, types.InstanceStatusActive)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityQueryCache, "prewarm")
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
