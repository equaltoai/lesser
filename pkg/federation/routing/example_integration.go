package routing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// ExampleRouteManagerIntegration demonstrates how to set up the complete route management system
func ExampleRouteManagerIntegration(db core.DB, tableName string, logger *zap.Logger) *Manager {
	// Create repositories
	routeOptimRepo := repositories.NewRouteOptimizerRepository(db, tableName, logger)
	circuitBreakerRepo := repositories.NewCircuitBreakerRepositoryBasic(db, tableName, logger)
	routingMetricsRepo := repositories.NewRoutingMetricsRepository(db, tableName, logger)
	costTrackingBaseRepo := repositories.NewBaseRepository[*models.FederationCostTracking](db, tableName, logger)
	budgetBaseRepo := repositories.NewBaseRepository[*models.FederationBudget](db, tableName, logger)
	federationCostRepo := repositories.NewFederationCostRepository(costTrackingBaseRepo, budgetBaseRepo)

	// Create a mock federation instance repository for the example
	instanceRepo := &MockFederationInstanceRepository{
		logger: logger,
	}

	// Configure manager with guidance document thresholds
	config := &ManagerConfig{
		RoutingConfig: &types.RoutingConfig{
			HealthCheckInterval:     1 * time.Minute,
			HealthCheckTimeout:      5 * time.Second,
			UnhealthyThreshold:      3,
			HealthyThreshold:        2,
			CircuitBreakerThreshold: 5,
			CircuitBreakerTimeout:   30 * time.Second,
			HalfOpenMaxAttempts:     3,
			MaxRoutesPerInstance:    10,
			RouteSelectionAlgorithm: "weighted_random",
			EnableLoadBalancing:     true,
			EnableCostOptimization:  true,
			DefaultTimeout:          10 * time.Second,
			MaxRetries:              3,
			RetryBackoff:            1 * time.Second,
			MaxQueueDepth:           10000,
			EnableCompression:       true,
			BatchDeliverySize:       100,
			ParallelDeliveries:      10,
		},
		OptimizerConfig: &OptimizerConfig{
			// Values from guidance document
			LatencyWeight:        0.4, // 40% weight
			ReliabilityWeight:    0.5, // 50% weight (most important)
			CostWeight:           0.1, // 10% weight
			MaxAcceptableLatency: 10 * time.Second,
			MinAcceptableSuccess: 0.5, // 50% minimum
			HistoryWindow:        15 * time.Minute,
			MinSamplesRequired:   10,
			AdaptationRate:       0.3, // 30% weight to new data
		},
		CacheTTL: 1 * time.Minute,
	}

	// Create the route manager with all components wired up
	manager := NewManager(
		instanceRepo,
		nil, // instanceHealthRepo - analytics service integration
		circuitBreakerRepo,
		routeOptimRepo,
		routingMetricsRepo,
		federationCostRepo,
		logger,
		config,
	)

	logger.Info("Route manager initialized with threshold-based optimization",
		zap.Duration("healthyRouteTTL", config.OptimizerConfig.HistoryWindow),
		zap.Float64("criticalSuccessRate", 0.5),
		zap.Float64("degradedSuccessRate", 0.7),
		zap.Float64("preferredSuccessRate", 0.95))

	return manager
}

// ExampleUsageScenario demonstrates the complete routing workflow with thresholds
func ExampleUsageScenario(manager *Manager, logger *zap.Logger) {
	ctx := context.Background()

	// Register a test instance
	testInstance := &types.Instance{
		ID:             "mastodon.social",
		Domain:         "mastodon.social",
		InboxURL:       "https://mastodon.social/inbox",
		SharedInboxURL: "https://mastodon.social/inbox",
		Status:         types.InstanceStatusActive,
		TierLevel:      types.TierStandard,
		MonthlyQuota:   1000000,
		CurrentUsage:   0,
		SupportedTypes: []types.MessageType{
			types.MessageTypeCreate,
			types.MessageTypeUpdate,
			types.MessageTypeDelete,
			types.MessageTypeAnnounce,
			types.MessageTypeFollow,
			types.MessageTypeLike,
		},
	}

	if err := manager.RegisterInstance(testInstance); err != nil {
		logger.Error("Failed to register instance", zap.Error(err))
		return
	}

	// Simulate route selection for different message types
	messageTypes := []types.MessageType{
		types.MessageTypeCreate, // Normal priority
		types.MessageTypeFollow, // High priority
		types.MessageTypeDelete, // Low priority
	}

	for _, msgType := range messageTypes {
		route, err := manager.SelectRoute("mastodon.social", msgType)
		if err != nil {
			logger.Error("Route selection failed",
				zap.String("messageType", string(msgType)),
				zap.Error(err))
			continue
		}

		logger.Info("Route selected",
			zap.String("routeID", route.ID),
			zap.String("messageType", string(msgType)),
			zap.Duration("latency", route.Latency),
			zap.Float64("successRate", route.SuccessRate),
			zap.String("circuitStatus", string(route.CircuitStatus)))

		// Simulate delivery and record result
		message := &types.FederationMessage{
			ID:          fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Type:        msgType,
			Target:      []string{"mastodon.social"},
			PayloadSize: 1024, // 1KB message
		}

		result, err := manager.DeliverMessage(ctx, message, types.DeliveryOptions{})
		if err != nil {
			logger.Error("Message delivery failed", zap.Error(err))
			continue
		}

		logger.Info("Message delivered",
			zap.String("messageID", message.ID),
			zap.Bool("success", result.Success),
			zap.Duration("duration", result.Duration),
			zap.Int64("bytesSent", result.BytesSent),
			zap.Float64("cost", result.Cost))
	}

	// Demonstrate route optimization
	logger.Info("Triggering route optimization...")
	if err := manager.OptimizeRoutes(); err != nil {
		logger.Error("Route optimization failed", zap.Error(err))
	} else {
		logger.Info("Route optimization completed")
	}

	// Get route metrics
	metrics, err := manager.GetRouteMetrics("mastodon.social")
	if err != nil {
		logger.Error("Failed to get route metrics", zap.Error(err))
	} else {
		logger.Info("Route metrics retrieved",
			zap.Int64("totalMessages", metrics.TotalMessages),
			zap.Int64("successfulCount", metrics.SuccessfulCount),
			zap.Int64("failedCount", metrics.FailedCount),
			zap.Duration("avgLatency", metrics.AvgLatency),
			zap.Duration("p95Latency", metrics.P95Latency),
			zap.Duration("p99Latency", metrics.P99Latency),
			zap.Float64("totalCost", metrics.TotalCost))
	}
}

// MockFederationInstanceRepository is a simple mock for demonstration
type MockFederationInstanceRepository struct {
	logger    *zap.Logger
	instances map[string]*types.Instance
}

// GetInstance retrieves an instance by ID from the mock repository
func (m *MockFederationInstanceRepository) GetInstance(_ context.Context, instanceID string) (*types.Instance, error) {
	if m.instances == nil {
		m.instances = make(map[string]*types.Instance)
	}

	if instance, exists := m.instances[instanceID]; exists {
		return instance, nil
	}

	m.logger.Error("instance not found in mock repository", zap.String("instance_id", instanceID))
	return nil, errors.Join(ErrInstanceNotFound, errors.New("instance "+instanceID))
}

// RegisterInstance adds a new instance to the mock repository
func (m *MockFederationInstanceRepository) RegisterInstance(_ context.Context, instance *types.Instance) error {
	if m.instances == nil {
		m.instances = make(map[string]*types.Instance)
	}

	m.instances[instance.ID] = instance
	m.logger.Info("Mock instance registered", zap.String("instanceID", instance.ID))
	return nil
}

// UpdateInstanceHealth updates the health status of an instance in the mock repository
func (m *MockFederationInstanceRepository) UpdateInstanceHealth(_ context.Context, instanceID string, health *types.HealthStatus) error {
	m.logger.Debug("Mock health update",
		zap.String("instanceID", instanceID),
		zap.Bool("reachable", health.Reachable),
		zap.Float64("errorRate", health.ErrorRate))
	return nil
}

// ListHealthyInstances returns all healthy instances from the mock repository
func (m *MockFederationInstanceRepository) ListHealthyInstances(_ context.Context) ([]*types.Instance, error) {
	var healthy []*types.Instance

	if m.instances != nil {
		for _, instance := range m.instances {
			if instance.Status == types.InstanceStatusActive {
				healthy = append(healthy, instance)
			}
		}
	}

	return healthy, nil
}

// BatchGetInstances retrieves multiple instances by IDs from the mock repository
func (m *MockFederationInstanceRepository) BatchGetInstances(_ context.Context, instanceIDs []string) ([]*types.Instance, error) {
	var instances []*types.Instance

	if m.instances != nil {
		for _, id := range instanceIDs {
			if instance, exists := m.instances[id]; exists {
				instances = append(instances, instance)
			}
		}
	}

	return instances, nil
}

// CreateInstance creates a new instance in the mock repository
func (m *MockFederationInstanceRepository) CreateInstance(ctx context.Context, instance *types.Instance) error {
	return m.RegisterInstance(ctx, instance)
}

// GetInstanceByDomain retrieves an instance by domain from the mock repository
func (m *MockFederationInstanceRepository) GetInstanceByDomain(ctx context.Context, domain string) (*types.Instance, error) {
	return m.GetInstance(ctx, domain)
}

// UpdateInstance updates an existing instance in the mock repository
func (m *MockFederationInstanceRepository) UpdateInstance(ctx context.Context, instance *types.Instance) error {
	return m.RegisterInstance(ctx, instance)
}

// DeleteInstance removes an instance from the mock repository
func (m *MockFederationInstanceRepository) DeleteInstance(_ context.Context, instanceID string) error {
	delete(m.instances, instanceID)
	return nil
}

// ListInstancesByStatus returns instances filtered by status (mock implementation)
func (m *MockFederationInstanceRepository) ListInstancesByStatus(ctx context.Context, _ types.InstanceStatus, _ int) ([]*types.Instance, error) {
	return m.ListHealthyInstances(ctx)
}

// GetInstancesByTier returns instances filtered by tier level (mock implementation)
func (m *MockFederationInstanceRepository) GetInstancesByTier(ctx context.Context, _ types.TierLevel, _ int) ([]*types.Instance, error) {
	return m.ListHealthyInstances(ctx)
}

// SearchInstances searches for instances matching a pattern (mock implementation)
func (m *MockFederationInstanceRepository) SearchInstances(ctx context.Context, _ string, _ int) ([]*types.Instance, error) {
	return m.ListHealthyInstances(ctx)
}

// ListAllInstances returns all instances with pagination support (mock implementation)
func (m *MockFederationInstanceRepository) ListAllInstances(ctx context.Context, _ int, _ map[string]interface{}) ([]*types.Instance, map[string]interface{}, error) {
	instances, err := m.ListHealthyInstances(ctx)
	return instances, nil, err
}

// UpdateInstanceUsage updates the usage metrics for an instance (mock implementation)
func (m *MockFederationInstanceRepository) UpdateInstanceUsage(_ context.Context, _ string, _ int64) error {
	return nil
}

// GetHealthHistory retrieves health history for an instance (mock implementation)
func (m *MockFederationInstanceRepository) GetHealthHistory(_ context.Context, _ string, _ time.Duration) ([]*types.HealthStatus, error) {
	return nil, nil
}

// BatchCreateInstances creates multiple instances efficiently (mock implementation)
func (m *MockFederationInstanceRepository) BatchCreateInstances(_ context.Context, instances []*types.Instance) error {
	if m.instances == nil {
		m.instances = make(map[string]*types.Instance)
	}
	for _, instance := range instances {
		m.instances[instance.ID] = instance
	}
	return nil
}

// BatchUpdateInstancesHealth updates health status for multiple instances (mock implementation)
func (m *MockFederationInstanceRepository) BatchUpdateInstancesHealth(_ context.Context, _ map[string]*types.HealthStatus) error {
	return nil
}

// BatchUpdateInstancesUsage updates usage counters for multiple instances (mock implementation)  
func (m *MockFederationInstanceRepository) BatchUpdateInstancesUsage(_ context.Context, _ map[string]int64) error {
	return nil
}
