package cost

import (
	"context"

	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
)

// Repository interface for federation cost operations
type Repository interface {
	// Cost tracking
	RecordCost(ctx context.Context, cost *FederationCost) error
	GetInstanceCost(ctx context.Context, domain string, period string) (*FederationCost, error)
	GetCostMetrics(ctx context.Context, period string) (*CostMetrics, error)

	// Health tracking
	UpdateInstanceHealth(ctx context.Context, health *InstanceHealth) error
	GetInstanceHealth(ctx context.Context, domain string) (*InstanceHealth, error)
	ListUnhealthyInstances(ctx context.Context) ([]*InstanceHealth, error)

	// Configuration
	SaveInstanceConfig(ctx context.Context, config *InstanceConfig) error
	GetInstanceConfig(ctx context.Context, domain string) (*InstanceConfig, error)
	ListInstanceConfigs(ctx context.Context) ([]*InstanceConfig, error)
}

// dynamoStorage implements the Storage interface using DynamoDB via repository pattern
type dynamoStorage struct {
	repo        Repository
	logger      *zap.Logger
	costTracker *cost.Tracker
}

// NewDynamoStorage creates a new DynamoDB-backed storage implementation
func NewDynamoStorage(
	repo Repository,
	logger *zap.Logger,
	costTracker *cost.Tracker,
) Storage {
	return &dynamoStorage{
		repo:        repo,
		logger:      logger,
		costTracker: costTracker,
	}
}

// RecordCost saves federation cost data to DynamoDB
func (s *dynamoStorage) RecordCost(ctx context.Context, cost *FederationCost) error {
	return s.repo.RecordCost(ctx, cost)
}

// GetInstanceCost retrieves cost data for a specific instance and period
func (s *dynamoStorage) GetInstanceCost(ctx context.Context, domain string, period string) (*FederationCost, error) {
	return s.repo.GetInstanceCost(ctx, domain, period)
}

// GetCostMetrics retrieves aggregated cost metrics for a period
func (s *dynamoStorage) GetCostMetrics(ctx context.Context, period string) (*CostMetrics, error) {
	return s.repo.GetCostMetrics(ctx, period)
}

// UpdateInstanceHealth updates health metrics for an instance
func (s *dynamoStorage) UpdateInstanceHealth(ctx context.Context, health *InstanceHealth) error {
	return s.repo.UpdateInstanceHealth(ctx, health)
}

// GetInstanceHealth retrieves health data for an instance
func (s *dynamoStorage) GetInstanceHealth(ctx context.Context, domain string) (*InstanceHealth, error) {
	return s.repo.GetInstanceHealth(ctx, domain)
}

// ListUnhealthyInstances returns all unhealthy instances
func (s *dynamoStorage) ListUnhealthyInstances(ctx context.Context) ([]*InstanceHealth, error) {
	return s.repo.ListUnhealthyInstances(ctx)
}

// SaveInstanceConfig saves federation configuration for an instance
func (s *dynamoStorage) SaveInstanceConfig(ctx context.Context, config *InstanceConfig) error {
	return s.repo.SaveInstanceConfig(ctx, config)
}

// GetInstanceConfig retrieves configuration for an instance
func (s *dynamoStorage) GetInstanceConfig(ctx context.Context, domain string) (*InstanceConfig, error) {
	return s.repo.GetInstanceConfig(ctx, domain)
}

// ListInstanceConfigs returns all instance configurations
func (s *dynamoStorage) ListInstanceConfigs(ctx context.Context) ([]*InstanceConfig, error) {
	return s.repo.ListInstanceConfigs(ctx)
}
