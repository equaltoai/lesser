package cost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// controller implements the Controller interface for cost-aware federation
type controller struct {
	storage    Storage
	calculator CostCalculator
	logger     *zap.Logger
	budget     *FederationBudget
	thresholds *CostThresholds

	// In-memory cache for performance
	mu          sync.RWMutex
	costCache   map[string]*FederationCost
	healthCache map[string]*InstanceHealth
	configCache map[string]*InstanceConfig
	cacheExpiry time.Duration
}

// NewController creates a new cost-aware federation controller
func NewController(
	storage Storage,
	calculator CostCalculator,
	logger *zap.Logger,
	budget *FederationBudget,
	thresholds *CostThresholds,
) Controller {
	return &controller{
		storage:     storage,
		calculator:  calculator,
		logger:      logger,
		budget:      budget,
		thresholds:  thresholds,
		costCache:   make(map[string]*FederationCost),
		healthCache: make(map[string]*InstanceHealth),
		configCache: make(map[string]*InstanceConfig),
		cacheExpiry: 5 * time.Minute,
	}
}

// ShouldFederate determines if federation with an instance should proceed
func (c *controller) ShouldFederate(ctx context.Context, instance string) (bool, error) {
	// Check instance tier
	tier, err := c.GetInstanceTier(ctx, instance)
	if err != nil {
		return false, fmt.Errorf("get instance tier: %w", err)
	}

	if tier == TierBlocked {
		c.logger.Debug("federation blocked for instance",
			zap.String("instance", instance),
			zap.String("tier", string(tier)))
		return false, nil
	}

	// Check health status
	healthy, err := c.IsHealthy(ctx, instance)
	if err != nil {
		return false, fmt.Errorf("check health: %w", err)
	}

	if !healthy {
		c.logger.Debug("instance unhealthy",
			zap.String("instance", instance))
		return false, nil
	}

	// Check budget
	remaining, err := c.GetRemainingBudget(ctx, instance)
	if err != nil {
		return false, fmt.Errorf("get remaining budget: %w", err)
	}

	if remaining <= 0 {
		c.logger.Warn("budget exhausted for instance",
			zap.String("instance", instance))
		return false, nil
	}

	// Check if we're approaching budget threshold
	budgetUsedPercent := (c.getInstanceBudget(instance) - remaining) / c.getInstanceBudget(instance) * 100
	if budgetUsedPercent >= c.thresholds.BlockThresholdPercent {
		c.logger.Warn("approaching budget threshold",
			zap.String("instance", instance),
			zap.Float64("budget_used_percent", budgetUsedPercent))

		// Only allow critical activities when near threshold
		if tier != TierPremium {
			return false, nil
		}
	}

	return true, nil
}

// GetInstanceTier returns the service tier for an instance
func (c *controller) GetInstanceTier(ctx context.Context, instance string) (FederationTier, error) {
	config, err := c.getInstanceConfig(ctx, instance)
	if err != nil {
		return TierStandard, fmt.Errorf("get instance config: %w", err)
	}

	if config == nil {
		// Default tier for unknown instances
		return TierStandard, nil
	}

	return config.Tier, nil
}

// GetRetryPolicy returns the retry policy for an instance
func (c *controller) GetRetryPolicy(ctx context.Context, instance string) (*RetryPolicy, error) {
	config, err := c.getInstanceConfig(ctx, instance)
	if err != nil {
		return DefaultRetryPolicy, fmt.Errorf("get instance config: %w", err)
	}

	if config == nil || config.RetryPolicy == nil {
		return DefaultRetryPolicy, nil
	}

	return config.RetryPolicy, nil
}

// TrackActivity records a federation activity and its estimated cost
func (c *controller) TrackActivity(ctx context.Context, instance string, activityType string, sizeBytes int64) error {
	// Get current period
	period := time.Now().Format(common.MonthFormat)

	// Get current cost data
	cost, err := c.getInstanceCost(ctx, instance, period)
	if err != nil {
		return fmt.Errorf("get instance cost: %w", err)
	}

	if cost == nil {
		cost = &FederationCost{
			InstanceDomain: instance,
			BillingPeriod:  period,
		}
	}

	// Update metrics
	cost.RequestCount++
	cost.EgressBytes += sizeBytes

	// Estimate cost for this activity
	activityCost := c.calculator.EstimateDataTransferCost(sizeBytes, "us-east-1")

	// Update average cost
	totalCost := cost.AverageCostUSD * float64(cost.RequestCount-1)
	cost.AverageCostUSD = (totalCost + activityCost) / float64(cost.RequestCount)
	cost.LastUpdated = time.Now()

	// Save updated cost
	if err := c.storage.RecordCost(ctx, cost); err != nil {
		return fmt.Errorf("record cost: %w", err)
	}

	// Update cache
	c.mu.Lock()
	c.costCache[c.costCacheKey(instance, period)] = cost
	c.mu.Unlock()

	// Log if approaching threshold
	budgetUsedPercent := (cost.AverageCostUSD * float64(cost.RequestCount)) / c.getInstanceBudget(instance) * 100
	if budgetUsedPercent >= c.thresholds.WarnThresholdPercent {
		c.logger.Warn("instance approaching budget threshold",
			zap.String("instance", instance),
			zap.Float64("budget_used_percent", budgetUsedPercent),
			zap.String("activity_type", activityType))
	}

	return nil
}

// GetRemainingBudget returns the remaining budget for an instance
func (c *controller) GetRemainingBudget(ctx context.Context, instance string) (float64, error) {
	period := time.Now().Format(common.MonthFormat)

	cost, err := c.getInstanceCost(ctx, instance, period)
	if err != nil {
		return 0, fmt.Errorf("get instance cost: %w", err)
	}

	budget := c.getInstanceBudget(instance)

	if cost == nil {
		return budget, nil
	}

	spent := cost.AverageCostUSD * float64(cost.RequestCount)
	remaining := budget - spent

	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// RecordSuccess records a successful federation interaction
func (c *controller) RecordSuccess(ctx context.Context, instance string, responseTimeMs int64) error {
	health, err := c.getInstanceHealth(ctx, instance)
	if err != nil {
		return fmt.Errorf("get instance health: %w", err)
	}

	if health == nil {
		health = &InstanceHealth{
			Domain: instance,
		}
	}

	// Update success metrics
	health.ConsecutiveFails = 0
	health.LastHealthCheck = time.Now()
	health.ResponseTimeP95 = responseTimeMs // Simplified for now

	// Update success rate (simple moving average)
	if health.SuccessRate == 0 {
		health.SuccessRate = 1.0
	} else {
		health.SuccessRate = (health.SuccessRate * 0.95) + 0.05
	}

	// Update health score
	health.HealthScore = c.calculateHealthScore(health)
	health.IsHealthy = health.HealthScore >= 0.7

	// Save updated health
	if err := c.storage.UpdateInstanceHealth(ctx, health); err != nil {
		return fmt.Errorf("update instance health: %w", err)
	}

	// Update cache
	c.mu.Lock()
	c.healthCache[instance] = health
	c.mu.Unlock()

	return nil
}

// RecordFailure records a failed federation interaction
func (c *controller) RecordFailure(ctx context.Context, instance string, err error) error {
	health, healthErr := c.getInstanceHealth(ctx, instance)
	if healthErr != nil {
		return fmt.Errorf("get instance health: %w", healthErr)
	}

	if health == nil {
		health = &InstanceHealth{
			Domain: instance,
		}
	}

	// Update failure metrics
	health.ConsecutiveFails++
	health.LastHealthCheck = time.Now()

	// Update success rate
	if health.SuccessRate == 0 {
		health.SuccessRate = 0
	} else {
		health.SuccessRate = health.SuccessRate * 0.95
	}

	// Update error rate in cost tracking
	period := time.Now().Format(common.MonthFormat)
	cost, _ := c.getInstanceCost(ctx, instance, period)
	if cost != nil {
		cost.ErrorCount++
		cost.ErrorRate = float64(cost.ErrorCount) / float64(cost.RequestCount)
		if err := c.storage.RecordCost(ctx, cost); err != nil {
			c.logger.Warn("failed to record federation cost", zap.Error(err))
		}
	}

	// Update health score
	health.HealthScore = c.calculateHealthScore(health)
	health.IsHealthy = health.HealthScore >= 0.7

	// Save updated health
	if err := c.storage.UpdateInstanceHealth(ctx, health); err != nil {
		return fmt.Errorf("update instance health: %w", err)
	}

	// Update cache
	c.mu.Lock()
	c.healthCache[instance] = health
	c.mu.Unlock()

	c.logger.Debug("recorded federation failure",
		zap.String("instance", instance),
		zap.Error(err),
		zap.Int("consecutive_fails", health.ConsecutiveFails))

	return nil
}

// IsHealthy checks if an instance is healthy
func (c *controller) IsHealthy(ctx context.Context, instance string) (bool, error) {
	health, err := c.getInstanceHealth(ctx, instance)
	if err != nil {
		return false, fmt.Errorf("get instance health: %w", err)
	}

	if health == nil {
		// Unknown instances start as healthy
		return true, nil
	}

	// Check consecutive failures
	if health.ConsecutiveFails >= 5 {
		return false, nil
	}

	// Check if last health check is recent
	if time.Since(health.LastHealthCheck) > 24*time.Hour {
		// Stale health data, assume healthy but trigger update
		go c.triggerHealthCheck(instance)
		return true, nil
	}

	return health.IsHealthy, nil
}

// Helper methods

func (c *controller) getInstanceConfig(ctx context.Context, instance string) (*InstanceConfig, error) {
	// Check cache first
	c.mu.RLock()
	if cached, ok := c.configCache[instance]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	// Load from storage
	config, err := c.storage.GetInstanceConfig(ctx, instance)
	if err != nil {
		return nil, err
	}

	// Update cache
	if config != nil {
		c.mu.Lock()
		c.configCache[instance] = config
		c.mu.Unlock()
	}

	return config, nil
}

func (c *controller) getInstanceCost(ctx context.Context, instance string, period string) (*FederationCost, error) {
	key := c.costCacheKey(instance, period)

	// Check cache first
	c.mu.RLock()
	if cached, ok := c.costCache[key]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	// Load from storage
	cost, err := c.storage.GetInstanceCost(ctx, instance, period)
	if err != nil {
		return nil, err
	}

	// Update cache
	if cost != nil {
		c.mu.Lock()
		c.costCache[key] = cost
		c.mu.Unlock()
	}

	return cost, nil
}

func (c *controller) getInstanceHealth(ctx context.Context, instance string) (*InstanceHealth, error) {
	// Check cache first
	c.mu.RLock()
	if cached, ok := c.healthCache[instance]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	// Load from storage
	health, err := c.storage.GetInstanceHealth(ctx, instance)
	if err != nil {
		return nil, err
	}

	// Update cache
	if health != nil {
		c.mu.Lock()
		c.healthCache[instance] = health
		c.mu.Unlock()
	}

	return health, nil
}

func (c *controller) getInstanceBudget(instance string) float64 {
	// Check for instance-specific override
	if override, ok := c.budget.InstanceOverrides[instance]; ok {
		return override
	}

	// Return per-instance default
	return c.budget.PerInstanceBudgetUSD
}

func (c *controller) calculateHealthScore(health *InstanceHealth) float64 {
	score := 1.0

	// Factor in success rate (40% weight)
	score *= health.SuccessRate * 0.4

	// Factor in consecutive failures (30% weight)
	if health.ConsecutiveFails > 0 {
		failurePenalty := 1.0 - (float64(health.ConsecutiveFails) / 10.0)
		if failurePenalty < 0 {
			failurePenalty = 0
		}
		score += failurePenalty * 0.3
	} else {
		score += 0.3
	}

	// Factor in response time (30% weight)
	if health.ResponseTimeP95 > 0 {
		// Ideal response time is < 1000ms
		responsePenalty := 1.0 - (float64(health.ResponseTimeP95) / 5000.0)
		if responsePenalty < 0 {
			responsePenalty = 0
		}
		score += responsePenalty * 0.3
	} else {
		score += 0.3
	}

	return score
}

func (c *controller) costCacheKey(instance, period string) string {
	return fmt.Sprintf("%s:%s", instance, period)
}

func (c *controller) triggerHealthCheck(instance string) {
	// This would trigger an active health check
	// Implementation depends on your health check mechanism
	c.logger.Debug("triggering health check for stale instance",
		zap.String("instance", instance))
}
