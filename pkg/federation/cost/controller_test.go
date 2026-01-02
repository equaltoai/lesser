package cost

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type fakeCalculator struct {
	dataTransferCost float64
	estimateFunc     func(bytes int64, region string) float64
}

func (c fakeCalculator) EstimateDataTransferCost(bytes int64, region string) float64 {
	if c.estimateFunc != nil {
		return c.estimateFunc(bytes, region)
	}
	return c.dataTransferCost
}
func (c fakeCalculator) EstimateLambdaCost(_ int, _ int64) float64 { return 0 }
func (c fakeCalculator) EstimateDynamoDBCost(_, _ int) float64     { return 0 }
func (c fakeCalculator) EstimateS3Cost(_, _ int64) float64         { return 0 }

type memoryStorage struct {
	mu sync.Mutex

	costs   map[string]*FederationCost
	health  map[string]*InstanceHealth
	configs map[string]*InstanceConfig

	recordCostCalls int
	recordCostErr   error

	getCostCalls int
	getCostErr   error

	updateHealthCalls int
	updateHealthErr   error

	getHealthCalls int
	getHealthErr   error

	getConfigCalls int
	getConfigErr   error
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{
		costs:   make(map[string]*FederationCost),
		health:  make(map[string]*InstanceHealth),
		configs: make(map[string]*InstanceConfig),
	}
}

func (s *memoryStorage) RecordCost(_ context.Context, cost *FederationCost) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.recordCostCalls++
	if s.recordCostErr != nil {
		return s.recordCostErr
	}

	s.costs[cost.InstanceDomain+":"+cost.BillingPeriod] = cost
	return nil
}

func (s *memoryStorage) GetInstanceCost(_ context.Context, domain string, period string) (*FederationCost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.getCostCalls++
	if s.getCostErr != nil {
		return nil, s.getCostErr
	}
	return s.costs[domain+":"+period], nil
}

func (s *memoryStorage) GetCostMetrics(_ context.Context, _ string) (*CostMetrics, error) {
	return nil, nil
}

func (s *memoryStorage) UpdateInstanceHealth(_ context.Context, health *InstanceHealth) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateHealthCalls++
	if s.updateHealthErr != nil {
		return s.updateHealthErr
	}

	s.health[health.Domain] = health
	return nil
}

func (s *memoryStorage) GetInstanceHealth(_ context.Context, domain string) (*InstanceHealth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.getHealthCalls++
	if s.getHealthErr != nil {
		return nil, s.getHealthErr
	}
	return s.health[domain], nil
}

func (s *memoryStorage) ListUnhealthyInstances(_ context.Context) ([]*InstanceHealth, error) {
	return nil, nil
}

func (s *memoryStorage) SaveInstanceConfig(_ context.Context, config *InstanceConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[config.Domain] = config
	return nil
}

func (s *memoryStorage) GetInstanceConfig(_ context.Context, domain string) (*InstanceConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.getConfigCalls++
	if s.getConfigErr != nil {
		return nil, s.getConfigErr
	}
	return s.configs[domain], nil
}

func (s *memoryStorage) ListInstanceConfigs(_ context.Context) ([]*InstanceConfig, error) {
	return nil, nil
}

func TestController_ShouldFederate_BlockedTier(t *testing.T) {
	store := newMemoryStorage()
	_ = store.SaveInstanceConfig(context.Background(), &InstanceConfig{
		Domain: "blocked.example",
		Tier:   TierBlocked,
	})

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	ok, err := ctrl.ShouldFederate(context.Background(), "blocked.example")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestController_ShouldFederate_UnhealthyInstance(t *testing.T) {
	store := newMemoryStorage()
	store.health["unhealthy.example"] = &InstanceHealth{
		Domain:          "unhealthy.example",
		IsHealthy:       false,
		LastHealthCheck: time.Now(),
	}

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	ok, err := ctrl.ShouldFederate(context.Background(), "unhealthy.example")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestController_ShouldFederate_BudgetExhausted(t *testing.T) {
	store := newMemoryStorage()
	store.health["expensive.example"] = &InstanceHealth{
		Domain:          "expensive.example",
		IsHealthy:       true,
		LastHealthCheck: time.Now(),
	}

	period := time.Now().Format(common.MonthFormat)
	store.costs["expensive.example:"+period] = &FederationCost{
		InstanceDomain: "expensive.example",
		BillingPeriod:  period,
		AverageCostUSD: 1.0,
		RequestCount:   10,
	}

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 5, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	ok, err := ctrl.ShouldFederate(context.Background(), "expensive.example")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestController_ShouldFederate_ApproachingThreshold_NonPremium(t *testing.T) {
	store := newMemoryStorage()
	_ = store.SaveInstanceConfig(context.Background(), &InstanceConfig{
		Domain: "standard.example",
		Tier:   TierStandard,
	})
	store.health["standard.example"] = &InstanceHealth{
		Domain:          "standard.example",
		IsHealthy:       true,
		LastHealthCheck: time.Now(),
	}

	period := time.Now().Format(common.MonthFormat)
	store.costs["standard.example:"+period] = &FederationCost{
		InstanceDomain: "standard.example",
		BillingPeriod:  period,
		AverageCostUSD: 0.95,
		RequestCount:   10, // spent = 9.5, remaining = 0.5 => 95% used
	}

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	ok, err := ctrl.ShouldFederate(context.Background(), "standard.example")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestController_ShouldFederate_ApproachingThreshold_PremiumAllowed(t *testing.T) {
	store := newMemoryStorage()
	_ = store.SaveInstanceConfig(context.Background(), &InstanceConfig{
		Domain: "premium.example",
		Tier:   TierPremium,
	})
	store.health["premium.example"] = &InstanceHealth{
		Domain:          "premium.example",
		IsHealthy:       true,
		LastHealthCheck: time.Now(),
	}

	period := time.Now().Format(common.MonthFormat)
	store.costs["premium.example:"+period] = &FederationCost{
		InstanceDomain: "premium.example",
		BillingPeriod:  period,
		AverageCostUSD: 0.95,
		RequestCount:   10, // spent = 9.5, remaining = 0.5 => 95% used
	}

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	ok, err := ctrl.ShouldFederate(context.Background(), "premium.example")
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestController_GetRetryPolicy_DefaultWhenNoConfig(t *testing.T) {
	store := newMemoryStorage()
	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	policy, err := ctrl.GetRetryPolicy(context.Background(), "unknown.example")
	assert.NoError(t, err)
	assert.Same(t, DefaultRetryPolicy, policy)
}

func TestController_RecordSuccessThenFailure_UpdatesHealthAndCost(t *testing.T) {
	store := newMemoryStorage()
	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	instance := "example.com"
	assert.NoError(t, ctrl.RecordSuccess(context.Background(), instance, 250))
	assert.Equal(t, 1, store.updateHealthCalls)
	assert.Equal(t, 1.0, store.health[instance].SuccessRate)

	period := time.Now().Format(common.MonthFormat)
	store.costs[instance+":"+period] = &FederationCost{
		InstanceDomain: instance,
		BillingPeriod:  period,
		RequestCount:   2,
		ErrorCount:     0,
		AverageCostUSD: 1.0,
	}

	assert.NoError(t, ctrl.RecordFailure(context.Background(), instance, errors.New("boom")))
	assert.Equal(t, 2, store.updateHealthCalls)
	assert.Equal(t, 1, store.costs[instance+":"+period].ErrorCount)
	assert.InDelta(t, 0.5, store.costs[instance+":"+period].ErrorRate, 0.0001)
}

func TestController_IsHealthy_Branches(t *testing.T) {
	store := newMemoryStorage()
	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	// Unknown instance defaults to healthy.
	ok, err := ctrl.IsHealthy(context.Background(), "unknown.example")
	assert.NoError(t, err)
	assert.True(t, ok)

	// Consecutive failures threshold.
	store.health["fails.example"] = &InstanceHealth{
		Domain:           "fails.example",
		ConsecutiveFails: 5,
		LastHealthCheck:  time.Now(),
		IsHealthy:        true,
	}
	ok, err = ctrl.IsHealthy(context.Background(), "fails.example")
	assert.NoError(t, err)
	assert.False(t, ok)

	// Stale data triggers async refresh but returns healthy.
	store.health["stale.example"] = &InstanceHealth{
		Domain:          "stale.example",
		IsHealthy:       false,
		LastHealthCheck: time.Now().Add(-25 * time.Hour),
	}
	ok, err = ctrl.IsHealthy(context.Background(), "stale.example")
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestController_TrackActivity_PropagatesStorageError(t *testing.T) {
	store := newMemoryStorage()
	store.recordCostErr = errors.New("write failed")

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 1.0},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	err := ctrl.TrackActivity(context.Background(), "example.com", "delivery", 1024)
	assert.Error(t, err)
}

func TestController_TrackActivity_Success_UsesCacheAndWarns(t *testing.T) {
	store := newMemoryStorage()

	ctrl := NewController(
		store,
		fakeCalculator{
			estimateFunc: func(bytes int64, _ string) float64 { return float64(bytes) / 1000.0 },
		},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 1, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	instance := "example.com"
	period := time.Now().Format(common.MonthFormat)

	assert.NoError(t, ctrl.TrackActivity(context.Background(), instance, "delivery", 1000))
	assert.Equal(t, 1, store.getCostCalls)
	assert.Equal(t, 1, store.recordCostCalls)
	assert.Equal(t, 1, store.costs[instance+":"+period].RequestCount)
	assert.InDelta(t, 1.0, store.costs[instance+":"+period].AverageCostUSD, 0.0001)

	assert.NoError(t, ctrl.TrackActivity(context.Background(), instance, "delivery", 2000))
	assert.Equal(t, 1, store.getCostCalls, "expected second call to hit controller cache")
	assert.Equal(t, 2, store.costs[instance+":"+period].RequestCount)
	assert.Equal(t, int64(3000), store.costs[instance+":"+period].EgressBytes)
	assert.InDelta(t, 1.5, store.costs[instance+":"+period].AverageCostUSD, 0.0001)
}

func TestController_GetRetryPolicy_Custom(t *testing.T) {
	store := newMemoryStorage()
	custom := &RetryPolicy{MaxRetries: 9, InitialBackoff: 0, MaxBackoff: 0, BackoffFactor: 1.0}
	_ = store.SaveInstanceConfig(context.Background(), &InstanceConfig{
		Domain:      "custom.example",
		Tier:        TierStandard,
		RetryPolicy: custom,
	})

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	policy, err := ctrl.GetRetryPolicy(context.Background(), "custom.example")
	assert.NoError(t, err)
	assert.Same(t, custom, policy)
}

func TestController_getInstanceBudget_UsesOverride(t *testing.T) {
	store := newMemoryStorage()
	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{
			PerInstanceBudgetUSD: 10,
			InstanceOverrides:    map[string]float64{"override.example": 2.5},
		},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	assert.InDelta(t, 2.5, ctrl.getInstanceBudget("override.example"), 0.0001)
	assert.InDelta(t, 10.0, ctrl.getInstanceBudget("default.example"), 0.0001)
}

func TestController_calculateHealthScore_ClampsPenalties(t *testing.T) {
	store := newMemoryStorage()
	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	score := ctrl.calculateHealthScore(&InstanceHealth{
		SuccessRate:      0.5,
		ConsecutiveFails: 15,     // triggers clamp to 0
		ResponseTimeP95:  100000, // triggers clamp to 0
	})
	assert.GreaterOrEqual(t, score, 0.0)
}

func TestController_GetRemainingBudget_NoCostReturnsBudget(t *testing.T) {
	store := newMemoryStorage()
	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	remaining, err := ctrl.GetRemainingBudget(context.Background(), "new.example")
	assert.NoError(t, err)
	assert.InDelta(t, 10.0, remaining, 0.0001)
}

func TestController_RecordSuccess_ExistingHealth_UsesMovingAverage(t *testing.T) {
	store := newMemoryStorage()
	store.health["example.com"] = &InstanceHealth{
		Domain:          "example.com",
		SuccessRate:     0.5,
		LastHealthCheck: time.Now(),
		IsHealthy:       true,
	}

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	assert.NoError(t, ctrl.RecordSuccess(context.Background(), "example.com", 123))
	assert.InDelta(t, (0.5*0.95)+0.05, store.health["example.com"].SuccessRate, 0.0001)
}

func TestController_RecordFailure_WhenCostRecordingFailsStillSucceeds(t *testing.T) {
	store := newMemoryStorage()
	store.recordCostErr = errors.New("record failed")

	period := time.Now().Format(common.MonthFormat)
	store.costs["example.com:"+period] = &FederationCost{
		InstanceDomain: "example.com",
		BillingPeriod:  period,
		RequestCount:   2,
		ErrorCount:     0,
	}

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	assert.NoError(t, ctrl.RecordFailure(context.Background(), "example.com", errors.New("boom")))
}

func TestController_GetInstanceTier_PropagatesStorageError(t *testing.T) {
	store := newMemoryStorage()
	store.getConfigErr = errors.New("config read failed")

	ctrl := NewController(
		store,
		fakeCalculator{dataTransferCost: 0.5},
		zap.NewNop(),
		&FederationBudget{PerInstanceBudgetUSD: 10, InstanceOverrides: map[string]float64{}},
		&Thresholds{WarnThresholdPercent: 80, BlockThresholdPercent: 95},
	).(*controller)

	_, err := ctrl.GetInstanceTier(context.Background(), "example.com")
	assert.Error(t, err)
}
