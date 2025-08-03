package cost

import (
	"context"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// repositoryAdapter implements the Repository interface using DynamORM directly
type repositoryAdapter struct {
	db          core.DB
	logger      *zap.Logger
	costTracker *cost.Tracker
}

// NewRepositoryAdapter creates a new repository adapter
func NewRepositoryAdapter(db core.DB, logger *zap.Logger, costTracker *cost.Tracker) Repository {
	return &repositoryAdapter{
		db:          db,
		logger:      logger,
		costTracker: costTracker,
	}
}

// RecordCost saves federation cost data to DynamoDB
func (r *repositoryAdapter) RecordCost(ctx context.Context, cost *FederationCost) error {
	model := &models.FederationCostTracking{
		InstanceDomain: cost.InstanceDomain,
		IngressBytes:   cost.IngressBytes,
		EgressBytes:    cost.EgressBytes,
		RequestCount:   cost.RequestCount,
		ErrorCount:     cost.ErrorCount,
		ErrorRate:      cost.ErrorRate,
		AverageCostUSD: cost.AverageCostUSD,
		LastUpdated:    cost.LastUpdated,
		BillingPeriod:  cost.BillingPeriod,
	}
	model.UpdateKeys()
	
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to record federation cost",
			zap.Error(err),
			zap.String("instance", cost.InstanceDomain))
		return err
	}

	r.costTracker.TrackDynamoWrite(1)
	return nil
}

// GetInstanceCost retrieves cost data for a specific instance and period
func (r *repositoryAdapter) GetInstanceCost(ctx context.Context, domain string, period string) (*FederationCost, error) {
	var model models.FederationCostTracking
	err := r.db.WithContext(ctx).Model(&models.FederationCostTracking{}).
		Where("PK", "=", "FEDCOST#"+domain).
		Where("SK", "=", "PERIOD#"+period).
		First(&model)

	r.costTracker.TrackDynamoRead(1)

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &FederationCost{
		InstanceDomain: model.InstanceDomain,
		IngressBytes:   model.IngressBytes,
		EgressBytes:    model.EgressBytes,
		RequestCount:   model.RequestCount,
		ErrorCount:     model.ErrorCount,
		ErrorRate:      model.ErrorRate,
		AverageCostUSD: model.AverageCostUSD,
		LastUpdated:    model.LastUpdated,
		BillingPeriod:  model.BillingPeriod,
	}, nil
}

// GetCostMetrics retrieves aggregated cost metrics for a period
func (r *repositoryAdapter) GetCostMetrics(ctx context.Context, period string) (*CostMetrics, error) {
	metrics := &CostMetrics{
		Period:        period,
		InstanceCosts: make(map[string]float64),
		ActivityCosts: make(map[string]float64),
	}

	var costs []models.FederationCostTracking
	err := r.db.WithContext(ctx).Model(&models.FederationCostTracking{}).
		Index("gsi1").
		Where("GSI1PK", "=", "PERIOD#"+period).
		All(&costs)

	if err != nil {
		return nil, err
	}

	readUnits := len(costs)/100 + 1 // Estimate read units
	r.costTracker.TrackDynamoRead(readUnits)

	// Aggregate metrics
	for _, cost := range costs {
		instanceTotal := cost.AverageCostUSD * float64(cost.RequestCount)
		metrics.InstanceCosts[cost.InstanceDomain] = instanceTotal
		metrics.TotalCostUSD += instanceTotal
		metrics.DataTransferGB += float64(cost.EgressBytes+cost.IngressBytes) / (1024 * 1024 * 1024)
		metrics.RequestCount += int64(cost.RequestCount)
	}

	return metrics, nil
}

// UpdateInstanceHealth updates health metrics for an instance
func (r *repositoryAdapter) UpdateInstanceHealth(ctx context.Context, health *InstanceHealth) error {
	model := &models.FederationInstanceHealthTracking{
		Domain:           health.Domain,
		HealthScore:      health.HealthScore,
		ResponseTimeP95:  health.ResponseTimeP95,
		SuccessRate:      health.SuccessRate,
		LastHealthCheck:  health.LastHealthCheck,
		ConsecutiveFails: health.ConsecutiveFails,
		IsHealthy:        health.IsHealthy,
	}
	model.UpdateKeys()

	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		return err
	}

	r.costTracker.TrackDynamoWrite(1)
	return nil
}

// GetInstanceHealth retrieves health data for an instance
func (r *repositoryAdapter) GetInstanceHealth(ctx context.Context, domain string) (*InstanceHealth, error) {
	var model models.FederationInstanceHealthTracking
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceHealthTracking{}).
		Where("PK", "=", "INSTANCE#"+domain).
		Where("SK", "=", "HEALTH").
		First(&model)

	r.costTracker.TrackDynamoRead(1)

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &InstanceHealth{
		Domain:           model.Domain,
		HealthScore:      model.HealthScore,
		ResponseTimeP95:  model.ResponseTimeP95,
		SuccessRate:      model.SuccessRate,
		LastHealthCheck:  model.LastHealthCheck,
		ConsecutiveFails: model.ConsecutiveFails,
		IsHealthy:        model.IsHealthy,
	}, nil
}

// ListUnhealthyInstances returns all unhealthy instances
func (r *repositoryAdapter) ListUnhealthyInstances(ctx context.Context) ([]*InstanceHealth, error) {
	var healthModels []models.FederationInstanceHealthTracking
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceHealthTracking{}).
		Index("gsi2").
		Where("GSI2PK", "=", "UNHEALTHY").
		OrderBy("GSI2SK", "ASC"). // Ascending order - lowest health scores first
		Limit(50).
		All(&healthModels)

	if err != nil {
		return nil, err
	}

	readUnits := len(healthModels)/100 + 1 // Estimate read units
	r.costTracker.TrackDynamoRead(readUnits)

	instances := make([]*InstanceHealth, len(healthModels))
	for i, model := range healthModels {
		instances[i] = &InstanceHealth{
			Domain:           model.Domain,
			HealthScore:      model.HealthScore,
			ResponseTimeP95:  model.ResponseTimeP95,
			SuccessRate:      model.SuccessRate,
			LastHealthCheck:  model.LastHealthCheck,
			ConsecutiveFails: model.ConsecutiveFails,
			IsHealthy:        model.IsHealthy,
		}
	}

	return instances, nil
}

// SaveInstanceConfig saves federation configuration for an instance
func (r *repositoryAdapter) SaveInstanceConfig(ctx context.Context, config *InstanceConfig) error {
	model := &models.FederationInstanceConfigTracking{
		Domain:            config.Domain,
		Tier:              models.FederationTier(config.Tier),
		CustomBudgetUSD:   config.CustomBudgetUSD,
		RateLimitOverride: config.RateLimitOverride,
		Created:           config.Created,
		LastModified:      config.LastModified,
	}
	
	// Convert retry policy if present
	if config.RetryPolicy != nil {
		model.RetryPolicy = &models.RetryPolicy{
			MaxRetries:     config.RetryPolicy.MaxRetries,
			InitialBackoff: config.RetryPolicy.InitialBackoff,
			MaxBackoff:     config.RetryPolicy.MaxBackoff,
			BackoffFactor:  config.RetryPolicy.BackoffFactor,
		}
	}
	
	model.UpdateKeys()
	
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		return err
	}

	r.costTracker.TrackDynamoWrite(1)
	return nil
}

// GetInstanceConfig retrieves configuration for an instance
func (r *repositoryAdapter) GetInstanceConfig(ctx context.Context, domain string) (*InstanceConfig, error) {
	var model models.FederationInstanceConfigTracking
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceConfigTracking{}).
		Where("PK", "=", "INSTANCE#"+domain).
		Where("SK", "=", "CONFIG").
		First(&model)

	r.costTracker.TrackDynamoRead(1)

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	config := &InstanceConfig{
		Domain:            model.Domain,
		Tier:              FederationTier(model.Tier),
		CustomBudgetUSD:   model.CustomBudgetUSD,
		RateLimitOverride: model.RateLimitOverride,
		Created:           model.Created,
		LastModified:      model.LastModified,
	}
	
	// Convert retry policy if present
	if model.RetryPolicy != nil {
		config.RetryPolicy = &RetryPolicy{
			MaxRetries:     model.RetryPolicy.MaxRetries,
			InitialBackoff: model.RetryPolicy.InitialBackoff,
			MaxBackoff:     model.RetryPolicy.MaxBackoff,
			BackoffFactor:  model.RetryPolicy.BackoffFactor,
		}
	}
	
	return config, nil
}

// ListInstanceConfigs returns all instance configurations
func (r *repositoryAdapter) ListInstanceConfigs(ctx context.Context) ([]*InstanceConfig, error) {
	var configModels []models.FederationInstanceConfigTracking
	err := r.db.WithContext(ctx).Model(&models.FederationInstanceConfigTracking{}).
		Where("Type", "=", "InstanceConfig").
		All(&configModels)

	if err != nil {
		return nil, err
	}

	readUnits := len(configModels)/100 + 1 // Estimate read units
	r.costTracker.TrackDynamoRead(readUnits)

	configs := make([]*InstanceConfig, len(configModels))
	for i, model := range configModels {
		config := &InstanceConfig{
			Domain:            model.Domain,
			Tier:              FederationTier(model.Tier),
			CustomBudgetUSD:   model.CustomBudgetUSD,
			RateLimitOverride: model.RateLimitOverride,
			Created:           model.Created,
			LastModified:      model.LastModified,
		}
		
		// Convert retry policy if present
		if model.RetryPolicy != nil {
			config.RetryPolicy = &RetryPolicy{
				MaxRetries:     model.RetryPolicy.MaxRetries,
				InitialBackoff: model.RetryPolicy.InitialBackoff,
				MaxBackoff:     model.RetryPolicy.MaxBackoff,
				BackoffFactor:  model.RetryPolicy.BackoffFactor,
			}
		}
		
		configs[i] = config
	}

	return configs, nil
}