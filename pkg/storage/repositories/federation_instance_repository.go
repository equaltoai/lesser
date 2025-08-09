package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// FederationInstanceRepository handles federation instance operations using DynamORM
type FederationInstanceRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewFederationInstanceRepository creates a new federation instance repository
func NewFederationInstanceRepository(db core.DB, logger *zap.Logger) *FederationInstanceRepository {
	return &FederationInstanceRepository{
		db:     db,
		logger: logger,
	}
}

// CreateInstance registers a new federated instance
func (r *FederationInstanceRepository) CreateInstance(ctx context.Context, instance *types.Instance) error {
	model := r.toModel(instance)

	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}

	r.logger.Info("created federation instance",
		zap.String("instanceID", instance.ID),
		zap.String("domain", instance.Domain),
		zap.String("tier", string(instance.TierLevel)))

	return nil
}

// GetInstance retrieves an instance by ID
func (r *FederationInstanceRepository) GetInstance(ctx context.Context, instanceID string) (*types.Instance, error) {
	var model models.FederationInstanceRegistry
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", instanceID)).
		Where("SK", "=", "METADATA").
		First(&model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("instance not found: %s", instanceID)
		}
		return nil, fmt.Errorf("get instance: %w", err)
	}

	return r.fromModel(&model), nil
}

// GetInstanceByDomain retrieves an instance by domain name
func (r *FederationInstanceRepository) GetInstanceByDomain(ctx context.Context, domain string) (*types.Instance, error) {
	var model models.FederationInstanceRegistry
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", domain)).
		Where("SK", "=", "METADATA").
		First(&model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("instance not found: %s", domain)
		}
		return nil, fmt.Errorf("get instance by domain: %w", err)
	}

	return r.fromModel(&model), nil
}

// UpdateInstance updates an existing instance
func (r *FederationInstanceRepository) UpdateInstance(ctx context.Context, instance *types.Instance) error {
	model := r.toModel(instance)

	err := r.db.WithContext(ctx).Model(model).
		Where("PK", "=", model.PK).
		Where("SK", "=", model.SK).
		Update()
	if err != nil {
		return fmt.Errorf("update instance: %w", err)
	}

	return nil
}

// DeleteInstance removes an instance
func (r *FederationInstanceRepository) DeleteInstance(ctx context.Context, instanceID string) error {
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", instanceID)).
		Where("SK", "=", "METADATA").
		Delete()
	if err != nil {
		return fmt.Errorf("delete instance: %w", err)
	}

	return nil
}

// ListInstancesByStatus retrieves instances by status using GSI1
func (r *FederationInstanceRepository) ListInstancesByStatus(ctx context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error) {
	var instances []models.FederationInstanceRegistry
	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("STATUS#%s", status))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&instances)
	if err != nil {
		return nil, fmt.Errorf("list instances by status: %w", err)
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	return result, nil
}

// ListHealthyInstances returns all healthy instances
func (r *FederationInstanceRepository) ListHealthyInstances(ctx context.Context) ([]*types.Instance, error) {
	return r.ListInstancesByStatus(ctx, types.InstanceStatusActive, 100)
}

// GetInstancesByTier retrieves instances by tier level using GSI2
func (r *FederationInstanceRepository) GetInstancesByTier(ctx context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error) {
	var instances []models.FederationInstanceRegistry
	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Index("GSI2").
		Where("GSI2PK", "=", fmt.Sprintf("TIER#%s", tier)).
		OrderBy("GSI2SK", "ASC") // Sort by usage ascending (least used first)

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&instances)
	if err != nil {
		return nil, fmt.Errorf("get instances by tier: %w", err)
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	return result, nil
}

// BatchGetInstances retrieves multiple instances efficiently
func (r *FederationInstanceRepository) BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*types.Instance, error) {
	if len(instanceIDs) == 0 {
		return []*types.Instance{}, nil
	}

	instances := make([]*types.Instance, 0, len(instanceIDs))

	// For now, do individual queries (can optimize later with proper batch API)
	for _, instanceID := range instanceIDs {
		instance, err := r.GetInstance(ctx, instanceID)
		if err != nil {
			// Skip not found errors, continue with others
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("batch get instances: %w", err)
			}
			continue
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// UpdateInstanceHealth updates instance health metrics
func (r *FederationInstanceRepository) UpdateInstanceHealth(ctx context.Context, instanceID string, health *types.HealthStatus) error {
	// Calculate new status based on health
	status := types.InstanceStatusActive
	if !health.Reachable {
		status = types.InstanceStatusUnreachable
	} else if health.ErrorRate > 0.1 {
		status = types.InstanceStatusDegraded
	}

	// Create update model with new health metrics
	updateModel := &models.FederationInstanceRegistry{
		PK:              fmt.Sprintf("INSTANCE#%s", instanceID),
		SK:              "METADATA",
		Status:          string(status),
		LastSeen:        time.Now(),
		AvgResponseTime: health.ResponseTime.Milliseconds(),
		ErrorRate:       health.ErrorRate,
		GSI1PK:          fmt.Sprintf("STATUS#%s", status),
	}

	err := r.db.WithContext(ctx).Model(updateModel).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", instanceID)).
		Where("SK", "=", "METADATA").
		Update()
	if err != nil {
		return fmt.Errorf("update instance health: %w", err)
	}

	// Store health history
	if err := r.storeHealthHistory(ctx, instanceID, health); err != nil {
		r.logger.Warn("failed to store health history",
			zap.String("instanceID", instanceID),
			zap.Error(err))
	}

	return nil
}

// UpdateInstanceUsage updates usage counters efficiently
func (r *FederationInstanceRepository) UpdateInstanceUsage(ctx context.Context, instanceID string, bytesUsed int64) error {
	// Get current usage first to calculate new GSI2SK
	var currentModel models.FederationInstanceRegistry
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", instanceID)).
		Where("SK", "=", "METADATA").
		First(&currentModel)
	if err != nil {
		return fmt.Errorf("get current usage: %w", err)
	}

	newUsage := currentModel.CurrentUsage + bytesUsed
	updateModel := &models.FederationInstanceRegistry{
		PK:           fmt.Sprintf("INSTANCE#%s", instanceID),
		SK:           "METADATA",
		CurrentUsage: newUsage,
		GSI2SK:       fmt.Sprintf("USAGE#%010d", newUsage),
	}

	err = r.db.WithContext(ctx).Model(updateModel).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", instanceID)).
		Where("SK", "=", "METADATA").
		Update()
	if err != nil {
		return fmt.Errorf("update instance usage: %w", err)
	}

	// Check if quota exceeded and update status if needed
	if newUsage > currentModel.MonthlyQuota {
		statusUpdateModel := &models.FederationInstanceRegistry{
			PK:     fmt.Sprintf("INSTANCE#%s", instanceID),
			SK:     "METADATA",
			Status: string(types.InstanceStatusBlocked),
			GSI1PK: "STATUS#blocked",
		}

		err = r.db.WithContext(ctx).Model(statusUpdateModel).
			Where("PK", "=", fmt.Sprintf("INSTANCE#%s", instanceID)).
			Where("SK", "=", "METADATA").
			Update()
		if err != nil {
			r.logger.Warn("failed to update instance status to blocked",
				zap.String("instanceID", instanceID),
				zap.Error(err))
		}
	}

	return nil
}

// SearchInstances searches for instances by domain pattern
func (r *FederationInstanceRepository) SearchInstances(ctx context.Context, domainPattern string, limit int) ([]*types.Instance, error) {
	var instances []models.FederationInstanceRegistry
	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("SK", "=", "METADATA").
		Filter("Domain", "contains", domainPattern)

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&instances)
	if err != nil {
		return nil, fmt.Errorf("search instances: %w", err)
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	return result, nil
}

// storeHealthHistory stores health status in history table
func (r *FederationInstanceRepository) storeHealthHistory(ctx context.Context, instanceID string, health *types.HealthStatus) error {
	history := r.toHealthHistoryModel(instanceID, health)

	err := r.db.WithContext(ctx).Model(history).Create()
	if err != nil {
		return fmt.Errorf("store health history: %w", err)
	}

	return nil
}

// GetHealthHistory retrieves health history for an instance
func (r *FederationInstanceRepository) GetHealthHistory(ctx context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error) {
	since := time.Now().Add(-duration)

	var historyModels []models.FederationInstanceRegistryHealthHistory
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistryHealthHistory{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", instanceID)).
		Where("SK", ">=", fmt.Sprintf("HEALTH#%d", since.UnixNano())).
		OrderBy("SK", "DESC"). // Descending order (newest first)
		All(&historyModels)
	if err != nil {
		return nil, fmt.Errorf("get health history: %w", err)
	}

	history := make([]*types.HealthStatus, len(historyModels))
	for i, model := range historyModels {
		history[i] = r.fromHealthHistoryModel(&model)
	}

	return history, nil
}

// ListAllInstances returns all instances with pagination
func (r *FederationInstanceRepository) ListAllInstances(ctx context.Context, limit int, _ map[string]interface{}) ([]*types.Instance, map[string]interface{}, error) {
	var instances []models.FederationInstanceRegistry
	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("SK", "=", "METADATA")

	if limit > 0 {
		query = query.Limit(limit)
	}

	// Note: DynamORM doesn't support StartKey - would need to implement with LastEvaluatedKey pattern
	// For now, we'll skip the startKey functionality and implement basic pagination

	err := query.All(&instances)
	if err != nil {
		return nil, nil, fmt.Errorf("list all instances: %w", err)
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	// Get the last evaluated key for pagination (simplified - would need actual implementation)
	var lastKey map[string]interface{}
	if len(instances) == limit {
		lastModel := instances[len(instances)-1]
		lastKey = map[string]interface{}{
			"PK": lastModel.PK,
			"SK": lastModel.SK,
		}
	}

	return result, lastKey, nil
}

// Conversion methods

// toModel converts a types.Instance to a models.FederationInstanceRegistry
func (r *FederationInstanceRepository) toModel(instance *types.Instance) *models.FederationInstanceRegistry {
	model := &models.FederationInstanceRegistry{
		ID:              instance.ID,
		Domain:          instance.Domain,
		InboxURL:        instance.InboxURL,
		SharedInboxURL:  instance.SharedInboxURL,
		PublicKeyPEM:    instance.PublicKeyPEM,
		Status:          string(instance.Status),
		LastSeen:        instance.LastSeen,
		RegisteredAt:    instance.RegisteredAt,
		AvgResponseTime: instance.AvgResponseTime.Milliseconds(),
		SuccessRate:     instance.SuccessRate,
		ErrorRate:       instance.ErrorRate,
		TierLevel:       string(instance.TierLevel),
		MonthlyQuota:    instance.MonthlyQuota,
		CurrentUsage:    instance.CurrentUsage,
		MaxMessageSize:  instance.MaxMessageSize,
	}

	// Convert supported types
	if len(instance.SupportedTypes) > 0 {
		model.SupportedTypes = make([]string, len(instance.SupportedTypes))
		for i, t := range instance.SupportedTypes {
			model.SupportedTypes[i] = string(t)
		}
	}

	// Convert rate limits to map
	model.RateLimits = map[string]interface{}{
		"MessagesPerMinute": instance.RateLimits.MessagesPerMinute,
		"MessagesPerHour":   instance.RateLimits.MessagesPerHour,
		"BytesPerMinute":    instance.RateLimits.BytesPerMinute,
		"BytesPerHour":      instance.RateLimits.BytesPerHour,
		"BurstSize":         instance.RateLimits.BurstSize,
	}

	// Update keys after setting fields
	model.UpdateKeys()

	return model
}

// fromModel converts a models.FederationInstanceRegistry to a types.Instance
func (r *FederationInstanceRepository) fromModel(model *models.FederationInstanceRegistry) *types.Instance {
	instance := &types.Instance{
		ID:              model.ID,
		Domain:          model.Domain,
		InboxURL:        model.InboxURL,
		SharedInboxURL:  model.SharedInboxURL,
		PublicKeyPEM:    model.PublicKeyPEM,
		Status:          types.InstanceStatus(model.Status),
		LastSeen:        model.LastSeen,
		RegisteredAt:    model.RegisteredAt,
		AvgResponseTime: time.Duration(model.AvgResponseTime) * time.Millisecond,
		SuccessRate:     model.SuccessRate,
		ErrorRate:       model.ErrorRate,
		TierLevel:       types.TierLevel(model.TierLevel),
		MonthlyQuota:    model.MonthlyQuota,
		CurrentUsage:    model.CurrentUsage,
		MaxMessageSize:  model.MaxMessageSize,
	}

	// Convert supported types
	if len(model.SupportedTypes) > 0 {
		instance.SupportedTypes = make([]types.MessageType, len(model.SupportedTypes))
		for i, t := range model.SupportedTypes {
			instance.SupportedTypes[i] = types.MessageType(t)
		}
	}

	// Convert rate limits from map
	if model.RateLimits != nil {
		limits := &types.RateLimits{}
		if v, ok := model.RateLimits["MessagesPerMinute"]; ok {
			if rpm, ok := v.(float64); ok {
				limits.MessagesPerMinute = int(rpm)
			}
		}
		if v, ok := model.RateLimits["MessagesPerHour"]; ok {
			if rph, ok := v.(float64); ok {
				limits.MessagesPerHour = int(rph)
			}
		}
		if v, ok := model.RateLimits["BytesPerMinute"]; ok {
			if bpm, ok := v.(float64); ok {
				limits.BytesPerMinute = int64(bpm)
			}
		}
		if v, ok := model.RateLimits["BytesPerHour"]; ok {
			if bph, ok := v.(float64); ok {
				limits.BytesPerHour = int64(bph)
			}
		}
		if v, ok := model.RateLimits["BurstSize"]; ok {
			if bs, ok := v.(float64); ok {
				limits.BurstSize = int(bs)
			}
		}
		instance.RateLimits = *limits
	}

	return instance
}

// toHealthHistoryModel converts routing types to health history model
func (r *FederationInstanceRepository) toHealthHistoryModel(instanceID string, health *types.HealthStatus) *models.FederationInstanceRegistryHealthHistory {
	model := &models.FederationInstanceRegistryHealthHistory{
		Reachable:       health.Reachable,
		ResponseTime:    health.ResponseTime.Milliseconds(),
		StatusCode:      health.StatusCode,
		ErrorRate:       health.ErrorRate,
		InboxBacklog:    health.InboxBacklog,
		ProcessingDelay: health.ProcessingDelay.Milliseconds(),
		ErrorMessage:    health.ErrorMessage,
		Timestamp:       health.Timestamp,
	}

	// Set keys
	model.PK = fmt.Sprintf("INSTANCE#%s", instanceID)
	model.UpdateKeys()

	return model
}

// fromHealthHistoryModel converts health history model to routing types
func (r *FederationInstanceRepository) fromHealthHistoryModel(model *models.FederationInstanceRegistryHealthHistory) *types.HealthStatus {
	return &types.HealthStatus{
		Timestamp:       model.Timestamp,
		Reachable:       model.Reachable,
		ResponseTime:    time.Duration(model.ResponseTime) * time.Millisecond,
		StatusCode:      model.StatusCode,
		ErrorMessage:    model.ErrorMessage,
		InboxBacklog:    model.InboxBacklog,
		ProcessingDelay: time.Duration(model.ProcessingDelay) * time.Millisecond,
		ErrorRate:       model.ErrorRate,
	}
}
