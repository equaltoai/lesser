package repositories

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// FederationInstanceRepository handles federation instance operations using enhanced repository patterns
type FederationInstanceRepository struct {
	*EnhancedBaseRepository[*models.FederationInstanceRegistry]
}

// NewFederationInstanceRepository creates a new federation instance repository with enhanced functionality
func NewFederationInstanceRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *FederationInstanceRepository {
	// Create enhanced repository optimized for federation instance operations
	enhancedRepo := NewEnhancedBaseRepository[*models.FederationInstanceRegistry](db, tableName, logger, costService, "FederationInstanceRepository", "federation_instance")

	// Set up enhanced services for federation instance operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Instance registry permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Cache instance data for performance
	enhancedRepo.SetEventService(NewDefaultEventService())           // Federation registry events

	return &FederationInstanceRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateInstance registers a new federated instance
func (r *FederationInstanceRepository) CreateInstance(ctx context.Context, instance *types.Instance) error {
	model := r.toModel(instance)

	err := r.ValidateAndCreate(ctx, model)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "federation instance", "create")
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
	err := r.Get(ctx, fmt.Sprintf("INSTANCE#%s", instanceID), "METADATA", &model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(fmt.Errorf("%w: %s", ErrEntityNotFound, instanceID), EntityFederationInstance, instanceID)
		}
		return nil, ErrorHandler.HandleGetError(err, "federation instance", instanceID)
	}

	return r.fromModel(&model), nil
}

// GetInstanceByDomain retrieves an instance by domain name
func (r *FederationInstanceRepository) GetInstanceByDomain(ctx context.Context, domain string) (*types.Instance, error) {
	var model models.FederationInstanceRegistry
	err := r.Get(ctx, fmt.Sprintf("INSTANCE#%s", domain), "METADATA", &model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(fmt.Errorf("%w: %s", ErrEntityNotFound, domain), EntityFederationInstance, domain)
		}
		return nil, ErrorHandler.HandleGetError(err, "federation instance", domain)
	}

	return r.fromModel(&model), nil
}

// UpdateInstance updates an existing instance
func (r *FederationInstanceRepository) UpdateInstance(ctx context.Context, instance *types.Instance) error {
	model := r.toModel(instance)

	err := r.Update(ctx, model)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "federation instance", "update")
	}

	return nil
}

// DeleteInstance removes an instance
func (r *FederationInstanceRepository) DeleteInstance(ctx context.Context, instanceID string) error {
	err := r.Delete(ctx, fmt.Sprintf("INSTANCE#%s", instanceID), "METADATA")
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "federation instance", instanceID)
	}

	return nil
}

// ListInstancesByStatus retrieves instances by status using GSI1 (backward compatible)
func (r *FederationInstanceRepository) ListInstancesByStatus(ctx context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error) {
	instances, _, err := r.ListInstancesByStatusWithCursor(ctx, status, limit, "")
	return instances, err
}

// ListInstancesByStatusWithCursor retrieves instances by status using GSI1 with cursor pagination
func (r *FederationInstanceRepository) ListInstancesByStatusWithCursor(ctx context.Context, status types.InstanceStatus, limit int, cursor string) ([]*types.Instance, string, error) {
	var instances []models.FederationInstanceRegistry
	// Validate pagination parameters
	if err := r.validatePaginationParams(limit, cursor); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "federation instance", "pagination validation")
	}

	// Log pagination query for debugging
	r.logPaginationQuery("ListInstancesByStatus", map[string]interface{}{
		"status": string(status),
		"limit":  limit,
		"cursor": cursor,
	})

	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("STATUS#%s", status))

	// Add cursor for pagination
	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one extra item to determine if there are more results
	actualLimit := limit
	if limit > 0 {
		query = query.Limit(limit + 1)
	}

	err := query.All(&instances)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "federation instance", "by status")
	}

	// Determine next cursor and trim results if needed
	var nextCursor string
	if limit > 0 && len(instances) > actualLimit {
		// We got more results than requested, so there are more pages
		nextCursor = instances[actualLimit-1].GSI1SK // Use the last item's sort key as cursor
		instances = instances[:actualLimit]          // Trim to requested limit
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	r.logger.Debug("listed instances by status with pagination",
		zap.String("status", string(status)),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
		zap.String("next_cursor", nextCursor),
		zap.Int("result_count", len(result)))

	return result, nextCursor, nil
}

// ListHealthyInstances returns all healthy instances
func (r *FederationInstanceRepository) ListHealthyInstances(ctx context.Context) ([]*types.Instance, error) {
	instances, err := r.ListInstancesByStatus(ctx, types.InstanceStatusActive, 100)
	return instances, err
}

// GetInstancesByTier retrieves instances by tier level using GSI2 (backward compatible)
func (r *FederationInstanceRepository) GetInstancesByTier(ctx context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error) {
	instances, _, err := r.GetInstancesByTierWithCursor(ctx, tier, limit, "")
	return instances, err
}

// GetInstancesByTierWithCursor retrieves instances by tier level using GSI2 with cursor pagination
func (r *FederationInstanceRepository) GetInstancesByTierWithCursor(ctx context.Context, tier types.TierLevel, limit int, cursor string) ([]*types.Instance, string, error) {
	var instances []models.FederationInstanceRegistry
	// Validate pagination parameters
	if err := r.validatePaginationParams(limit, cursor); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "federation instance", "pagination validation")
	}

	// Log pagination query for debugging
	r.logPaginationQuery("GetInstancesByTier", map[string]interface{}{
		"tier":   string(tier),
		"limit":  limit,
		"cursor": cursor,
	})

	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("TIER#%s", tier)).
		OrderBy("gsi2SK", "ASC") // Sort by usage ascending (least used first)

	// Add cursor for pagination
	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one extra item to determine if there are more results
	actualLimit := limit
	if limit > 0 {
		query = query.Limit(limit + 1)
	}

	err := query.All(&instances)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "federation instance", "by tier")
	}

	// Determine next cursor and trim results if needed
	var nextCursor string
	if limit > 0 && len(instances) > actualLimit {
		// We got more results than requested, so there are more pages
		nextCursor = instances[actualLimit-1].GSI2SK // Use the last item's sort key as cursor
		instances = instances[:actualLimit]          // Trim to requested limit
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	r.logger.Debug("get instances by tier with pagination",
		zap.String("tier", string(tier)),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
		zap.String("next_cursor", nextCursor),
		zap.Int("result_count", len(result)))

	return result, nextCursor, nil
}

// BatchGetInstances retrieves multiple instances efficiently using BaseRepository batch operations
func (r *FederationInstanceRepository) BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*types.Instance, error) {
	if common.ValidateSliceNotEmpty("instanceIDs", instanceIDs) != nil {
		return []*types.Instance{}, nil
	}

	// Handle batch size limits (DynamoDB limit is 100 items per batch)
	const maxBatchSize = 100
	if len(instanceIDs) > maxBatchSize {
		return r.batchGetInstancesInChunks(ctx, instanceIDs, maxBatchSize)
	}

	// Create batch keys for BaseRepository
	batchKeys := make([]struct{ PK, SK string }, 0, len(instanceIDs))
	for _, instanceID := range instanceIDs {
		batchKeys = append(batchKeys, struct{ PK, SK string }{
			PK: fmt.Sprintf("INSTANCE#%s", instanceID),
			SK: "METADATA",
		})
	}

	// Use BaseRepository batch get
	instanceModels, err := r.BatchGet(ctx, batchKeys)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "federation instance", "batch get")
	}

	// Convert models to types.Instance
	instances := make([]*types.Instance, 0, len(instanceModels))
	for _, model := range instanceModels {
		instance := r.fromModel(model)
		instances = append(instances, instance)
	}

	r.logger.Debug("batch get instances completed",
		zap.Int("requested", len(instanceIDs)),
		zap.Int("found", len(instances)))

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
		return ErrorHandler.HandleUpdateError(err, "federation instance", instanceID)
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
		return ErrorHandler.HandleGetError(err, "instance usage", instanceID)
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
		return ErrorHandler.HandleUpdateError(err, "instance usage", instanceID)
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

// SearchInstances searches for instances by domain pattern (backward compatible)
func (r *FederationInstanceRepository) SearchInstances(ctx context.Context, domainPattern string, limit int) ([]*types.Instance, error) {
	instances, _, err := r.SearchInstancesWithCursor(ctx, domainPattern, limit, "")
	return instances, err
}

// SearchInstancesWithCursor searches for instances by domain pattern with cursor pagination
func (r *FederationInstanceRepository) SearchInstancesWithCursor(ctx context.Context, domainPattern string, limit int, cursor string) ([]*types.Instance, string, error) {
	var instances []models.FederationInstanceRegistry
	// Validate pagination parameters
	if err := r.validatePaginationParams(limit, cursor); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "federation instance", "pagination validation")
	}

	// Log pagination query for debugging
	r.logPaginationQuery("SearchInstances", map[string]interface{}{
		"domain_pattern": domainPattern,
		"limit":          limit,
		"cursor":         cursor,
	})

	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("SK", "=", "METADATA").
		Filter("Domain", "contains", domainPattern)

	// Add cursor for pagination
	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one extra item to determine if there are more results
	actualLimit := limit
	if limit > 0 {
		query = query.Limit(limit + 1)
	}

	err := query.All(&instances)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrFederationInstanceSearchFailed, err)
	}

	// Determine next cursor and trim results if needed
	var nextCursor string
	if limit > 0 && len(instances) > actualLimit {
		// We got more results than requested, so there are more pages
		nextCursor = instances[actualLimit-1].PK // Use the last item's primary key as cursor
		instances = instances[:actualLimit]      // Trim to requested limit
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	r.logger.Debug("search instances with pagination",
		zap.String("domain_pattern", domainPattern),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
		zap.String("next_cursor", nextCursor),
		zap.Int("result_count", len(result)))

	return result, nextCursor, nil
}

// storeHealthHistory stores health status in history table
func (r *FederationInstanceRepository) storeHealthHistory(ctx context.Context, instanceID string, health *types.HealthStatus) error {
	history := r.toHealthHistoryModel(instanceID, health)

	// Create a temporary BaseRepository for health history
	healthRepo := NewBaseRepository[*models.FederationInstanceRegistryHealthHistory](r.db, "FederationInstances", r.logger)
	err := healthRepo.Create(ctx, history)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFederationInstanceHealthStoreFailed, err)
	}

	return nil
}

// GetHealthHistory retrieves health history for an instance
func (r *FederationInstanceRepository) GetHealthHistory(ctx context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error) {
	since := time.Now().Add(-duration)

	// Create a temporary BaseRepository for health history
	healthRepo := NewBaseRepository[*models.FederationInstanceRegistryHealthHistory](r.db, "FederationInstances", r.logger)
	historyModels, err := healthRepo.QueryBetween(ctx,
		fmt.Sprintf("INSTANCE#%s", instanceID),
		fmt.Sprintf("HEALTH#%d", since.UnixNano()),
		fmt.Sprintf("HEALTH#%d", time.Now().UnixNano()),
		0) // no limit
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFederationInstanceHealthQueryFailed, err)
	}

	history := make([]*types.HealthStatus, len(historyModels))
	for i, model := range historyModels {
		history[i] = r.fromHealthHistoryModel(model)
	}

	return history, nil
}

// batchGetInstancesInChunks handles large batch requests by splitting them into chunks
func (r *FederationInstanceRepository) batchGetInstancesInChunks(ctx context.Context, instanceIDs []string, chunkSize int) ([]*types.Instance, error) {
	allInstances := make([]*types.Instance, 0, len(instanceIDs))

	for i := 0; i < len(instanceIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(instanceIDs) {
			end = len(instanceIDs)
		}

		chunk := instanceIDs[i:end]
		chunkInstances, err := r.BatchGetInstances(ctx, chunk)
		if err != nil {
			return nil, fmt.Errorf("%w at index %d: %w", ErrFederationInstanceBatchGetFailed, i, err)
		}

		allInstances = append(allInstances, chunkInstances...)

		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return allInstances, nil
}

// BatchCreateInstances creates multiple instances efficiently for federation discovery
func (r *FederationInstanceRepository) BatchCreateInstances(ctx context.Context, instances []*types.Instance) error {
	if common.ValidateSliceNotEmpty("instances", instances) != nil {
		return nil
	}

	// Handle batch size limits (DynamoDB limit is 25 items per batch for writes)
	const maxBatchSize = 25
	if len(instances) > maxBatchSize {
		return r.batchCreateInstancesInChunks(ctx, instances, maxBatchSize)
	}

	// Convert to models
	instanceModels := make([]*models.FederationInstanceRegistry, 0, len(instances))
	for _, instance := range instances {
		model := r.toModel(instance)
		instanceModels = append(instanceModels, model)
	}

	// Use BaseRepository batch create
	err := r.BatchCreate(ctx, instanceModels)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFederationInstanceBatchCreateFailed, err)
	}

	r.logger.Info("batch created federation instances",
		zap.Int("count", len(instances)))

	return nil
}

// batchCreateInstancesInChunks handles large batch create requests by splitting them into chunks
func (r *FederationInstanceRepository) batchCreateInstancesInChunks(ctx context.Context, instances []*types.Instance, chunkSize int) error {
	for i := 0; i < len(instances); i += chunkSize {
		end := i + chunkSize
		if end > len(instances) {
			end = len(instances)
		}

		chunk := instances[i:end]
		if err := r.BatchCreateInstances(ctx, chunk); err != nil {
			return fmt.Errorf("%w at index %d: %w", ErrFederationInstanceBatchCreateChunkFailed, i, err)
		}

		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// BatchUpdateInstancesHealth updates health status for multiple instances efficiently
func (r *FederationInstanceRepository) BatchUpdateInstancesHealth(ctx context.Context, healthUpdates map[string]*types.HealthStatus) error {
	if len(healthUpdates) == 0 {
		return nil
	}

	// Handle batch size limits
	const maxBatchSize = 25
	if len(healthUpdates) > maxBatchSize {
		return r.batchUpdateHealthInChunks(ctx, healthUpdates, maxBatchSize)
	}

	// Convert to update models
	updateModels := make([]interface{}, 0, len(healthUpdates))
	for instanceID, health := range healthUpdates {
		// Calculate new status based on health
		status := types.InstanceStatusActive
		if !health.Reachable {
			status = types.InstanceStatusUnreachable
		} else if health.ErrorRate > 0.1 {
			status = types.InstanceStatusDegraded
		}

		updateModel := &models.FederationInstanceRegistry{
			PK:              fmt.Sprintf("INSTANCE#%s", instanceID),
			SK:              "METADATA",
			Status:          string(status),
			LastSeen:        time.Now(),
			AvgResponseTime: health.ResponseTime.Milliseconds(),
			ErrorRate:       health.ErrorRate,
			GSI1PK:          fmt.Sprintf("STATUS#%s", status),
		}
		updateModels = append(updateModels, updateModel)
	}

	// Use DynamORM batch update (implemented as batch put)
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).BatchCreate(updateModels)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFederationInstanceBatchUpdateHealthFailed, err)
	}

	// Store health history for each instance
	for instanceID, health := range healthUpdates {
		if err := r.storeHealthHistory(ctx, instanceID, health); err != nil {
			r.logger.Warn("failed to store health history",
				zap.String("instanceID", instanceID),
				zap.Error(err))
		}
	}

	r.logger.Info("batch updated instance health",
		zap.Int("count", len(healthUpdates)))

	return nil
}

// batchUpdateHealthInChunks handles large batch health update requests
func (r *FederationInstanceRepository) batchUpdateHealthInChunks(ctx context.Context, healthUpdates map[string]*types.HealthStatus, chunkSize int) error {
	updateSlice := make([]struct {
		instanceID string
		health     *types.HealthStatus
	}, 0, len(healthUpdates))

	// Convert map to slice for chunking
	for instanceID, health := range healthUpdates {
		updateSlice = append(updateSlice, struct {
			instanceID string
			health     *types.HealthStatus
		}{instanceID, health})
	}

	for i := 0; i < len(updateSlice); i += chunkSize {
		end := i + chunkSize
		if end > len(updateSlice) {
			end = len(updateSlice)
		}

		// Create chunk map
		chunkMap := make(map[string]*types.HealthStatus)
		for j := i; j < end; j++ {
			update := updateSlice[j]
			chunkMap[update.instanceID] = update.health
		}

		if err := r.BatchUpdateInstancesHealth(ctx, chunkMap); err != nil {
			return fmt.Errorf("%w at index %d: %w", ErrFederationInstanceBatchUpdateHealthChunkFailed, i, err)
		}

		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// BatchUpdateInstancesUsage updates usage counters for multiple instances efficiently
func (r *FederationInstanceRepository) BatchUpdateInstancesUsage(ctx context.Context, usageUpdates map[string]int64) error {
	if len(usageUpdates) == 0 {
		return nil
	}

	// For usage updates, we need to read current values first to calculate new GSI2SK
	// This is more complex than health updates, so we process them individually but efficiently
	instanceIDs := make([]string, 0, len(usageUpdates))
	for instanceID := range usageUpdates {
		instanceIDs = append(instanceIDs, instanceID)
	}

	// Batch get current instances
	currentInstances, err := r.BatchGetInstances(ctx, instanceIDs)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFederationInstanceUsageUpdateFailed, err)
	}

	// Create update models with new usage
	updateModels := make([]interface{}, 0, len(currentInstances))
	for _, instance := range currentInstances {
		if bytesUsed, exists := usageUpdates[instance.ID]; exists {
			newUsage := instance.CurrentUsage + bytesUsed
			status := instance.Status

			// Check if quota exceeded
			if newUsage > instance.MonthlyQuota {
				status = types.InstanceStatusBlocked
			}

			updateModel := &models.FederationInstanceRegistry{
				PK:           fmt.Sprintf("INSTANCE#%s", instance.ID),
				SK:           "METADATA",
				CurrentUsage: newUsage,
				Status:       string(status),
				GSI1PK:       fmt.Sprintf("STATUS#%s", status),
				GSI2SK:       fmt.Sprintf("USAGE#%010d", newUsage),
			}
			updateModels = append(updateModels, updateModel)
		}
	}

	if common.ValidateSliceNotEmpty("updateModels", updateModels) != nil {
		return nil
	}

	// Handle batch size limits
	const maxBatchSize = 25
	if len(updateModels) > maxBatchSize {
		return r.batchUpdateUsageInChunks(ctx, updateModels, maxBatchSize)
	}

	// Use DynamORM batch update
	err = r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).BatchCreate(updateModels)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFederationInstanceBatchUpdateUsageFailed, err)
	}

	r.logger.Info("batch updated instance usage",
		zap.Int("count", len(updateModels)))

	return nil
}

// batchUpdateUsageInChunks handles large batch usage update requests
func (r *FederationInstanceRepository) batchUpdateUsageInChunks(ctx context.Context, updateModels []interface{}, chunkSize int) error {
	for i := 0; i < len(updateModels); i += chunkSize {
		end := i + chunkSize
		if end > len(updateModels) {
			end = len(updateModels)
		}

		chunk := updateModels[i:end]
		err := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).BatchCreate(chunk)
		if err != nil {
			return fmt.Errorf("%w at index %d: %w", ErrFederationInstanceBatchUpdateUsageChunkFailed, i, err)
		}

		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// ListAllInstances returns instances while honoring legacy startKey-based pagination
func (r *FederationInstanceRepository) ListAllInstances(ctx context.Context, limit int, startKey map[string]interface{}) ([]*types.Instance, map[string]interface{}, error) {
	// Convert old startKey format to cursor if needed
	cursor := ""
	if startKey != nil {
		if pk, ok := startKey["PK"].(string); ok {
			cursor = pk // Simple conversion for backward compatibility
		}
	}

	instances, nextCursor, err := r.ListAllInstancesWithCursor(ctx, limit, cursor)
	if err != nil {
		return nil, nil, err
	}

	// Convert cursor back to startKey format for backward compatibility
	var lastKey map[string]interface{}
	if nextCursor != "" {
		lastKey = map[string]interface{}{
			"PK": nextCursor,
			"SK": "METADATA",
		}
	}

	return instances, lastKey, nil
}

// ListAllInstancesWithCursor returns instances using internal cursor pagination
func (r *FederationInstanceRepository) ListAllInstancesWithCursor(ctx context.Context, limit int, cursor string) ([]*types.Instance, string, error) {
	var instances []models.FederationInstanceRegistry
	// Validate pagination parameters
	if err := r.validatePaginationParams(limit, cursor); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "federation instance", "pagination validation")
	}

	// Log pagination query for debugging
	r.logPaginationQuery("ListAllInstances", map[string]interface{}{
		"limit":  limit,
		"cursor": cursor,
	})

	query := r.db.WithContext(ctx).Model(&models.FederationInstanceRegistry{}).
		Where("SK", "=", "METADATA")

	// Add cursor for pagination
	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one extra item to determine if there are more results
	actualLimit := limit
	if limit > 0 {
		query = query.Limit(limit + 1)
	}

	err := query.All(&instances)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrFederationInstanceListFailed, err)
	}

	// Determine next cursor and trim results if needed
	var nextCursor string
	if limit > 0 && len(instances) > actualLimit {
		// We got more results than requested, so there are more pages
		nextCursor = instances[actualLimit-1].PK // Use the last item's primary key as cursor
		instances = instances[:actualLimit]      // Trim to requested limit
	}

	result := make([]*types.Instance, len(instances))
	for i, model := range instances {
		result[i] = r.fromModel(&model)
	}

	r.logger.Debug("listed all instances with pagination",
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
		zap.String("next_cursor", nextCursor),
		zap.Int("result_count", len(result)))

	return result, nextCursor, nil
}

// Helper methods for pagination

// validateCursor validates and sanitizes a pagination cursor
func (r *FederationInstanceRepository) validateCursor(cursor string) error {
	if err := common.ValidateRequiredParam("cursor", cursor); err != nil {
		return nil
	}

	// Check for reasonable length (base64 encoded cursors should be reasonable size)
	if err := common.ValidateStringLength("cursor", cursor, 1, 1024); err != nil {
		return ErrFederationInstanceCursorTooLong
	}

	// Validate base64 format if it appears to be encoded
	if strings.Contains(cursor, "#") || strings.Contains(cursor, "=") {
		// Try to decode as base64 to validate format
		if _, err := base64.URLEncoding.DecodeString(cursor); err == nil {
			return nil // Valid base64
		}
	}

	// For simple cursors (like primary keys), check for basic safety
	if !strings.Contains(cursor, "<script") && !strings.Contains(cursor, "javascript:") {
		return nil
	}

	return ErrFederationInstanceCursorInvalid
}

// validatePaginationParams validates pagination parameters
func (r *FederationInstanceRepository) validatePaginationParams(limit int, cursor string) error {
	// Validate limit
	if limit < 0 {
		return ErrFederationInstanceLimitNegative
	}
	if limit > 1000 {
		return ErrFederationInstanceLimitTooLarge
	}

	// Validate cursor
	return r.validateCursor(cursor)
}

// logPaginationQuery logs pagination query details for debugging
func (r *FederationInstanceRepository) logPaginationQuery(operation string, params map[string]interface{}) {
	r.logger.Debug("pagination query",
		zap.String("operation", operation),
		zap.Any("params", params))
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
	if common.ValidateSliceNotEmpty("instance.SupportedTypes", instance.SupportedTypes) == nil {
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
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

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
	if common.ValidateSliceNotEmpty("model.SupportedTypes", model.SupportedTypes) == nil {
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
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

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
