package cost

import (
	"context"
	"time"
)

// FederationCost tracks the cost metrics for a specific federated instance
type FederationCost struct {
	InstanceDomain string    `json:"instance_domain" dynamodbav:"InstanceDomain"`
	IngressBytes   int64     `json:"ingress_bytes" dynamodbav:"IngressBytes"`
	EgressBytes    int64     `json:"egress_bytes" dynamodbav:"EgressBytes"`
	RequestCount   int       `json:"request_count" dynamodbav:"RequestCount"`
	ErrorCount     int       `json:"error_count" dynamodbav:"ErrorCount"`
	ErrorRate      float64   `json:"error_rate" dynamodbav:"ErrorRate"`
	AverageCostUSD float64   `json:"average_cost_usd" dynamodbav:"AverageCostUSD"`
	LastUpdated    time.Time `json:"last_updated" dynamodbav:"LastUpdated"`
	BillingPeriod  string    `json:"billing_period" dynamodbav:"BillingPeriod"` // YYYY-MM format
}

// InstanceHealth represents the health and reliability metrics for a federated instance
type InstanceHealth struct {
	Domain           string    `json:"domain" dynamodbav:"Domain"`
	HealthScore      float64   `json:"health_score" dynamodbav:"HealthScore"`          // 0.0 to 1.0
	ResponseTimeP95  int64     `json:"response_time_p95" dynamodbav:"ResponseTimeP95"` // milliseconds
	SuccessRate      float64   `json:"success_rate" dynamodbav:"SuccessRate"`          // 0.0 to 1.0
	LastHealthCheck  time.Time `json:"last_health_check" dynamodbav:"LastHealthCheck"`
	ConsecutiveFails int       `json:"consecutive_fails" dynamodbav:"ConsecutiveFails"`
	IsHealthy        bool      `json:"is_healthy" dynamodbav:"IsHealthy"`
}

// FederationBudget defines spending limits for federation activities
type FederationBudget struct {
	TotalBudgetUSD       float64            `json:"total_budget_usd"`
	PerInstanceBudgetUSD float64            `json:"per_instance_budget_usd"`
	BudgetPeriod         string             `json:"budget_period"` // "monthly", "daily", "hourly"
	InstanceOverrides    map[string]float64 `json:"instance_overrides"`
}

// Thresholds defines alerting thresholds
type Thresholds struct {
	WarnThresholdPercent  float64 `json:"warn_threshold_percent"`  // e.g., 80
	BlockThresholdPercent float64 `json:"block_threshold_percent"` // e.g., 95
}

// FederationTier represents different service tiers for federated instances
type FederationTier string

// Federation tier constants
const (
	TierPremium  FederationTier = "premium"  // Unlimited, priority processing
	TierStandard FederationTier = "standard" // Normal limits
	TierLimited  FederationTier = "limited"  // Reduced limits, lower priority
	TierBlocked  FederationTier = "blocked"  // No federation
)

// InstanceConfig holds per-instance federation configuration
type InstanceConfig struct {
	Domain            string         `json:"domain" dynamodbav:"Domain"`
	Tier              FederationTier `json:"tier" dynamodbav:"Tier"`
	CustomBudgetUSD   *float64       `json:"custom_budget_usd,omitempty" dynamodbav:"CustomBudgetUSD,omitempty"`
	RateLimitOverride *int           `json:"rate_limit_override,omitempty" dynamodbav:"RateLimitOverride,omitempty"`
	RetryPolicy       *RetryPolicy   `json:"retry_policy,omitempty" dynamodbav:"RetryPolicy,omitempty"`
	Created           time.Time      `json:"created" dynamodbav:"Created"`
	LastModified      time.Time      `json:"last_modified" dynamodbav:"LastModified"`
}

// RetryPolicy defines how to retry failed federation attempts
type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries" dynamodbav:"MaxRetries"`
	InitialBackoff time.Duration `json:"initial_backoff" dynamodbav:"InitialBackoff"`
	MaxBackoff     time.Duration `json:"max_backoff" dynamodbav:"MaxBackoff"`
	BackoffFactor  float64       `json:"backoff_factor" dynamodbav:"BackoffFactor"`
}

// CostMetrics holds aggregated cost metrics
//
//nolint:revive // Cost prefix clarifies this is for cost metrics
type CostMetrics struct {
	Period         string             `json:"period" dynamodbav:"Period"` // YYYY-MM-DD
	TotalCostUSD   float64            `json:"total_cost_usd" dynamodbav:"TotalCostUSD"`
	InstanceCosts  map[string]float64 `json:"instance_costs" dynamodbav:"InstanceCosts"`
	ActivityCosts  map[string]float64 `json:"activity_costs" dynamodbav:"ActivityCosts"` // by activity type
	DataTransferGB float64            `json:"data_transfer_gb" dynamodbav:"DataTransferGB"`
	RequestCount   int64              `json:"request_count" dynamodbav:"RequestCount"`
}

// Storage interface for cost tracking persistence
type Storage interface {
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

// Controller interface for cost-aware federation decisions
type Controller interface {
	// Decision making
	ShouldFederate(ctx context.Context, instance string) (bool, error)
	GetInstanceTier(ctx context.Context, instance string) (FederationTier, error)
	GetRetryPolicy(ctx context.Context, instance string) (*RetryPolicy, error)

	// Cost tracking
	TrackActivity(ctx context.Context, instance string, activityType string, sizeBytes int64) error
	GetRemainingBudget(ctx context.Context, instance string) (float64, error)

	// Health monitoring
	RecordSuccess(ctx context.Context, instance string, responseTimeMs int64) error
	RecordFailure(ctx context.Context, instance string, err error) error
	IsHealthy(ctx context.Context, instance string) (bool, error)
}

// DefaultRetryPolicy defines the default retry configuration
var DefaultRetryPolicy = &RetryPolicy{
	MaxRetries:     3,
	InitialBackoff: 1 * time.Second,
	MaxBackoff:     30 * time.Second,
	BackoffFactor:  2.0,
}
