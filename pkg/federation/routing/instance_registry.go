package routing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"go.uber.org/zap"
)

// FederationInstanceRepository interface for dependency injection
type FederationInstanceRepository interface {
	// Instance CRUD operations
	CreateInstance(ctx context.Context, instance *types.Instance) error
	GetInstance(ctx context.Context, instanceID string) (*types.Instance, error)
	GetInstanceByDomain(ctx context.Context, domain string) (*types.Instance, error)
	UpdateInstance(ctx context.Context, instance *types.Instance) error
	DeleteInstance(ctx context.Context, instanceID string) error

	// Instance queries
	ListInstancesByStatus(ctx context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error)
	ListHealthyInstances(ctx context.Context) ([]*types.Instance, error)
	GetInstancesByTier(ctx context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error)
	BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*types.Instance, error)
	SearchInstances(ctx context.Context, domainPattern string, limit int) ([]*types.Instance, error)
	ListAllInstances(ctx context.Context, limit int, startKey map[string]interface{}) ([]*types.Instance, map[string]interface{}, error)

	// Batch operations for efficiency
	BatchCreateInstances(ctx context.Context, instances []*types.Instance) error
	BatchUpdateInstancesHealth(ctx context.Context, healthUpdates map[string]*types.HealthStatus) error
	BatchUpdateInstancesUsage(ctx context.Context, usageUpdates map[string]int64) error

	// Instance health and metrics
	UpdateInstanceHealth(ctx context.Context, instanceID string, health *types.HealthStatus) error
	UpdateInstanceUsage(ctx context.Context, instanceID string, bytesUsed int64) error
	GetHealthHistory(ctx context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error)
}

// InstanceRegistry manages federated instance data using DynamORM
type InstanceRegistry struct {
	repo   FederationInstanceRepository
	logger *zap.Logger

	// Local cache for frequently accessed instances
	cache    sync.Map
	cacheTTL time.Duration
}

type cachedInstance struct {
	instance *types.Instance
	cachedAt time.Time
}

// NewInstanceRegistry creates a new instance registry
func NewInstanceRegistry(repo FederationInstanceRepository, logger *zap.Logger) *InstanceRegistry {
	return &InstanceRegistry{
		repo:     repo,
		logger:   logger,
		cacheTTL: 5 * time.Minute,
	}
}

// RegisterInstance registers a new federated instance
func (ir *InstanceRegistry) RegisterInstance(ctx context.Context, instance *types.Instance) error {
	if err := common.ValidateRequiredParam("instance.ID", instance.ID); err != nil {
		instance.ID = generateInstanceID(instance.Domain)
	}

	instance.RegisteredAt = time.Now()
	instance.LastSeen = time.Now()
	instance.Status = types.InstanceStatusActive

	err := ir.repo.CreateInstance(ctx, instance)
	if err != nil {
		ir.logger.Error("instance registration failed",
			zap.String("instance_id", instance.ID),
			zap.String("domain", instance.Domain),
			zap.String("operation", "register"),
			zap.Error(err))
		return errors.Join(ErrInstanceRegistrationFailed, err)
	}

	// Update cache
	ir.cache.Store(instance.ID, &cachedInstance{
		instance: instance,
		cachedAt: time.Now(),
	})

	ir.logger.Info("registered instance",
		zap.String("instanceID", instance.ID),
		zap.String("domain", instance.Domain),
		zap.String("tier", string(instance.TierLevel)))

	return nil
}

// GetInstance retrieves an instance by ID with caching
func (ir *InstanceRegistry) GetInstance(ctx context.Context, instanceID string) (*types.Instance, error) {
	// Check cache first
	if cached, ok := ir.cache.Load(instanceID); ok {
		if ci, ok := cached.(*cachedInstance); ok && time.Since(ci.cachedAt) < ir.cacheTTL {
			return ci.instance, nil
		}
	}

	// Query repository
	instance, err := ir.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	// Update cache
	ir.cache.Store(instanceID, &cachedInstance{
		instance: instance,
		cachedAt: time.Now(),
	})

	return instance, nil
}

// GetInstanceByDomain retrieves an instance by domain name
func (ir *InstanceRegistry) GetInstanceByDomain(ctx context.Context, domain string) (*types.Instance, error) {
	return ir.repo.GetInstanceByDomain(ctx, domain)
}

// UpdateInstance updates an existing instance
func (ir *InstanceRegistry) UpdateInstance(ctx context.Context, instance *types.Instance) error {
	err := ir.repo.UpdateInstance(ctx, instance)
	if err != nil {
		ir.logger.Error("instance update failed",
			zap.String("instance_id", instance.ID),
			zap.String("domain", instance.Domain),
			zap.String("operation", "update"),
			zap.Error(err))
		return errors.Join(ErrInstanceUpdateFailed, err)
	}

	// Invalidate cache
	ir.cache.Delete(instance.ID)

	return nil
}

// UnregisterInstance removes an instance
func (ir *InstanceRegistry) UnregisterInstance(ctx context.Context, instanceID string) error {
	err := ir.repo.DeleteInstance(ctx, instanceID)
	if err != nil {
		ir.logger.Error("instance unregistration failed",
			zap.String("instance_id", instanceID),
			zap.String("operation", "unregister"),
			zap.Error(err))
		return errors.Join(ErrInstanceUnregistrationFailed, err)
	}

	// Remove from cache
	ir.cache.Delete(instanceID)

	ir.logger.Info("unregistered instance", zap.String("instanceID", instanceID))

	return nil
}

// ListHealthyInstances returns all healthy instances
func (ir *InstanceRegistry) ListHealthyInstances(ctx context.Context) ([]*types.Instance, error) {
	return ir.repo.ListHealthyInstances(ctx)
}

// ListInstances returns all instances with optional pagination
func (ir *InstanceRegistry) ListInstances(ctx context.Context, limit int, startKey map[string]interface{}) ([]*types.Instance, map[string]interface{}, error) {
	return ir.repo.ListAllInstances(ctx, limit, startKey)
}

// GetInstancesByStatus retrieves instances by status
func (ir *InstanceRegistry) GetInstancesByStatus(ctx context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error) {
	return ir.repo.ListInstancesByStatus(ctx, status, limit)
}

// UpdateInstanceHealth updates instance health metrics
func (ir *InstanceRegistry) UpdateInstanceHealth(ctx context.Context, instanceID string, health *types.HealthStatus) error {
	err := ir.repo.UpdateInstanceHealth(ctx, instanceID, health)
	if err != nil {
		ir.logger.Error("instance health update failed",
			zap.String("instance_id", instanceID),
			zap.String("operation", "health_update"),
			zap.Error(err))
		return errors.Join(ErrInstanceHealthUpdateFailed, err)
	}

	// Invalidate cache
	ir.cache.Delete(instanceID)

	return nil
}

// GetInstancesByTier retrieves instances by tier level
func (ir *InstanceRegistry) GetInstancesByTier(ctx context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error) {
	return ir.repo.GetInstancesByTier(ctx, tier, limit)
}

// BatchGetInstances retrieves multiple instances efficiently
func (ir *InstanceRegistry) BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*types.Instance, error) {
	if err := common.ValidateSliceNotEmpty("instanceIDs", instanceIDs); err != nil {
		return []*types.Instance{}, nil
	}

	// Check cache first
	instances := make([]*types.Instance, 0, len(instanceIDs))
	uncachedIDs := []string{}

	for _, id := range instanceIDs {
		if cached, ok := ir.cache.Load(id); ok {
			if ci, ok := cached.(*cachedInstance); ok && time.Since(ci.cachedAt) < ir.cacheTTL {
				instances = append(instances, ci.instance)
				continue
			}
		}
		uncachedIDs = append(uncachedIDs, id)
	}

	// Batch get uncached instances
	if len(uncachedIDs) > 0 {
		uncachedInstances, err := ir.repo.BatchGetInstances(ctx, uncachedIDs)
		if err != nil {
			ir.logger.Error("instance batch get failed",
				zap.Strings("uncached_ids", uncachedIDs),
				zap.String("operation", "batch_get"),
				zap.Error(err))
			return nil, errors.Join(ErrInstanceBatchGetFailed, err)
		}

		for _, instance := range uncachedInstances {
			instances = append(instances, instance)

			// Update cache
			ir.cache.Store(instance.ID, &cachedInstance{
				instance: instance,
				cachedAt: time.Now(),
			})
		}
	}

	return instances, nil
}

// UpdateInstanceUsage updates usage counters
func (ir *InstanceRegistry) UpdateInstanceUsage(ctx context.Context, instanceID string, bytesUsed int64) error {
	err := ir.repo.UpdateInstanceUsage(ctx, instanceID, bytesUsed)
	if err != nil {
		ir.logger.Error("instance usage update failed",
			zap.String("instance_id", instanceID),
			zap.Int64("bytes_used", bytesUsed),
			zap.String("operation", "usage_update"),
			zap.Error(err))
		return errors.Join(ErrInstanceUsageUpdateFailed, err)
	}

	// Invalidate cache since usage metrics changed
	ir.cache.Delete(instanceID)

	return nil
}

// SearchInstances searches for instances by domain pattern
func (ir *InstanceRegistry) SearchInstances(ctx context.Context, domainPattern string, limit int) ([]*types.Instance, error) {
	return ir.repo.SearchInstances(ctx, domainPattern, limit)
}

// GetHealthHistory retrieves health history for an instance
func (ir *InstanceRegistry) GetHealthHistory(ctx context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error) {
	return ir.repo.GetHealthHistory(ctx, instanceID, duration)
}

// BatchCreateInstances creates multiple instances efficiently for federation discovery
func (ir *InstanceRegistry) BatchCreateInstances(ctx context.Context, instances []*types.Instance) error {
	err := ir.repo.BatchCreateInstances(ctx, instances)
	if err != nil {
		ir.logger.Error("instance batch create failed",
			zap.Int("instance_count", len(instances)),
			zap.String("operation", "batch_create"),
			zap.Error(err))
		return errors.Join(ErrInstanceBatchCreateFailed, err)
	}

	// Update cache for created instances
	for _, instance := range instances {
		ir.cache.Store(instance.ID, &cachedInstance{
			instance: instance,
			cachedAt: time.Now(),
		})
	}

	return nil
}

// BatchUpdateInstancesHealth updates health status for multiple instances efficiently
func (ir *InstanceRegistry) BatchUpdateInstancesHealth(ctx context.Context, healthUpdates map[string]*types.HealthStatus) error {
	err := ir.repo.BatchUpdateInstancesHealth(ctx, healthUpdates)
	if err != nil {
		ir.logger.Error("instance batch health update failed",
			zap.Int("update_count", len(healthUpdates)),
			zap.String("operation", "batch_health_update"),
			zap.Error(err))
		return errors.Join(ErrInstanceBatchHealthUpdateFailed, err)
	}

	// Invalidate cache for updated instances
	for instanceID := range healthUpdates {
		ir.cache.Delete(instanceID)
	}

	return nil
}

// BatchUpdateInstancesUsage updates usage counters for multiple instances efficiently  
func (ir *InstanceRegistry) BatchUpdateInstancesUsage(ctx context.Context, usageUpdates map[string]int64) error {
	err := ir.repo.BatchUpdateInstancesUsage(ctx, usageUpdates)
	if err != nil {
		ir.logger.Error("instance batch usage update failed",
			zap.Int("update_count", len(usageUpdates)),
			zap.String("operation", "batch_usage_update"),
			zap.Error(err))
		return errors.Join(ErrInstanceBatchUsageUpdateFailed, err)
	}

	// Invalidate cache for updated instances
	for instanceID := range usageUpdates {
		ir.cache.Delete(instanceID)
	}

	return nil
}

// Helper functions

func generateInstanceID(domain string) string {
	return fmt.Sprintf("%s-%d", domain, time.Now().Unix())
}

// ClearCache clears the local cache (useful for testing)
func (ir *InstanceRegistry) ClearCache() {
	ir.cache.Range(func(key, _ interface{}) bool {
		ir.cache.Delete(key)
		return true
	})
}

// GetCacheStats returns cache statistics for monitoring
func (ir *InstanceRegistry) GetCacheStats() map[string]interface{} {
	stats := map[string]interface{}{
		"cache_ttl_seconds": ir.cacheTTL.Seconds(),
	}

	// Count cache entries
	count := 0
	ir.cache.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	stats["cached_instances"] = count

	return stats
}
