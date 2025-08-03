package models

import (
	"fmt"
	"time"
)

// FederationCostTracking extends the existing FederationCost model with additional fields needed for the cost tracking system
type FederationCostTracking struct {
	// Primary keys
	PK string `dynamorm:"pk"` // FEDCOST#{InstanceDomain}
	SK string `dynamorm:"sk"` // PERIOD#{BillingPeriod}

	// GSI keys for period-based queries
	GSI1PK string `dynamorm:"index:gsi1,pk"` // PERIOD#{BillingPeriod}
	GSI1SK string `dynamorm:"index:gsi1,sk"` // INSTANCE#{InstanceDomain}

	// Type field
	Type string `json:"type"`

	// Business fields matching federation cost types
	InstanceDomain string    `json:"instance_domain"`
	IngressBytes   int64     `json:"ingress_bytes"`
	EgressBytes    int64     `json:"egress_bytes"`
	RequestCount   int       `json:"request_count"`
	ErrorCount     int       `json:"error_count"`
	ErrorRate      float64   `json:"error_rate"`
	AverageCostUSD float64   `json:"average_cost_usd"`
	LastUpdated    time.Time `json:"last_updated"`
	BillingPeriod  string    `json:"billing_period"` // YYYY-MM format
	UpdatedAt      time.Time `json:"updated_at"`

	// TTL for data retention (90 days)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the DynamoDB keys for federation cost tracking records
func (fc *FederationCostTracking) UpdateKeys() {
	fc.PK = fmt.Sprintf("FEDCOST#%s", fc.InstanceDomain)
	fc.SK = fmt.Sprintf("PERIOD#%s", fc.BillingPeriod)
	fc.GSI1PK = fmt.Sprintf("PERIOD#%s", fc.BillingPeriod)
	fc.GSI1SK = fmt.Sprintf("INSTANCE#%s", fc.InstanceDomain)
	fc.Type = "FederationCost"
	fc.UpdatedAt = time.Now()
	
	// Set TTL to 90 days from now
	fc.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
}

// FederationInstanceHealthTracking represents health metrics for federation cost tracking
type FederationInstanceHealthTracking struct {
	// Primary keys
	PK string `dynamorm:"pk"` // INSTANCE#{Domain}
	SK string `dynamorm:"sk"` // HEALTH

	// GSI keys for unhealthy instances
	GSI2PK string `dynamorm:"index:gsi2,pk"` // UNHEALTHY (only if !IsHealthy)
	GSI2SK string `dynamorm:"index:gsi2,sk"` // SCORE#{HealthScore}#{Domain}

	// Type field
	Type string `json:"type"`

	// Business fields matching InstanceHealth type
	Domain           string    `json:"domain"`
	HealthScore      float64   `json:"health_score"`          // 0.0 to 1.0
	ResponseTimeP95  int64     `json:"response_time_p95"`     // milliseconds
	SuccessRate      float64   `json:"success_rate"`          // 0.0 to 1.0
	LastHealthCheck  time.Time `json:"last_health_check"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	IsHealthy        bool      `json:"is_healthy"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// UpdateKeys sets the DynamoDB keys for instance health tracking records
func (ih *FederationInstanceHealthTracking) UpdateKeys() {
	ih.PK = fmt.Sprintf("INSTANCE#%s", ih.Domain)
	ih.SK = "HEALTH"
	ih.Type = "InstanceHealth"
	ih.UpdatedAt = time.Now()

	// Add to unhealthy index if needed
	if !ih.IsHealthy {
		ih.GSI2PK = "UNHEALTHY"
		ih.GSI2SK = fmt.Sprintf("SCORE#%.4f#%s", ih.HealthScore, ih.Domain)
	} else {
		ih.GSI2PK = ""
		ih.GSI2SK = ""
	}
}

// FederationTier represents different service tiers for federated instances
type FederationTier string

const (
	TierPremium  FederationTier = "premium"  // Unlimited, priority processing
	TierStandard FederationTier = "standard" // Normal limits
	TierLimited  FederationTier = "limited"  // Reduced limits, lower priority
	TierBlocked  FederationTier = "blocked"  // No federation
)

// RetryPolicy defines how to retry failed federation attempts
type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	BackoffFactor  float64       `json:"backoff_factor"`
}

// FederationInstanceConfigTracking holds per-instance federation configuration for cost tracking
type FederationInstanceConfigTracking struct {
	// Primary keys
	PK string `dynamorm:"pk"` // INSTANCE#{Domain}
	SK string `dynamorm:"sk"` // CONFIG

	// GSI keys for tier-based queries
	GSI3PK string `dynamorm:"index:gsi3,pk"` // TIER#{Tier}
	GSI3SK string `dynamorm:"index:gsi3,sk"` // {Domain}

	// Type field
	Type string `json:"type"`

	// Business fields matching InstanceConfig type
	Domain            string          `json:"domain"`
	Tier              FederationTier  `json:"tier"`
	CustomBudgetUSD   *float64        `json:"custom_budget_usd,omitempty"`
	RateLimitOverride *int            `json:"rate_limit_override,omitempty"`
	RetryPolicy       *RetryPolicy    `json:"retry_policy,omitempty"`
	Created           time.Time       `json:"created"`
	LastModified      time.Time       `json:"last_modified"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// UpdateKeys sets the DynamoDB keys for instance config tracking records
func (ic *FederationInstanceConfigTracking) UpdateKeys() {
	ic.PK = fmt.Sprintf("INSTANCE#%s", ic.Domain)
	ic.SK = "CONFIG"
	ic.GSI3PK = fmt.Sprintf("TIER#%s", ic.Tier)
	ic.GSI3SK = ic.Domain
	ic.Type = "InstanceConfig"
	ic.UpdatedAt = time.Now()
}