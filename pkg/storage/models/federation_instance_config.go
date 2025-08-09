package models

import (
	"fmt"
	"time"
)

// FederationTier represents the tier level for federation
type FederationTier string

const (
	// FederationTierFree represents the free federation tier
	FederationTierFree       FederationTier = "free"
	// FederationTierBasic represents the basic federation tier
	FederationTierBasic      FederationTier = "basic"
	// FederationTierPremium represents the premium federation tier
	FederationTierPremium    FederationTier = "premium"
	// FederationTierEnterprise represents the enterprise federation tier
	FederationTierEnterprise FederationTier = "enterprise"
	// FederationTierBlocked represents a blocked federation tier
	FederationTierBlocked    FederationTier = "blocked"
)

// RetryPolicy defines retry behavior for federation operations
type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	BackoffFactor  float64       `json:"backoff_factor"`
}

// FederationInstanceConfigTracking stores configuration for federated instances
type FederationInstanceConfigTracking struct {
	// Primary keys - INSTANCE#{domain}, CONFIG
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for tier queries - TIER#{tier}, DOMAIN#{domain}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`

	// GSI2 for budget queries - BUDGET_OVERRIDE, BUDGET#{budget}#{domain}
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2_sk"`

	// Instance identification
	Domain string `json:"domain"` // Remote instance domain

	// Tier and budget configuration
	Tier              FederationTier `json:"tier"`                          // Federation tier
	CustomBudgetUSD   *float64       `json:"custom_budget_usd,omitempty"`   // Custom monthly budget (if set)
	RateLimitOverride *int           `json:"rate_limit_override,omitempty"` // Custom rate limit (requests/hour)

	// Retry configuration
	RetryPolicy *RetryPolicy `json:"retry_policy,omitempty"` // Custom retry policy

	// Feature flags
	EnableSignatureValidation bool `json:"enable_signature_validation"` // Whether to validate HTTP signatures
	EnableRateLimiting        bool `json:"enable_rate_limiting"`        // Whether to apply rate limits
	EnableBudgetEnforcement   bool `json:"enable_budget_enforcement"`   // Whether to enforce budget limits
	AllowPublicActivities     bool `json:"allow_public_activities"`     // Whether to accept public activities
	AllowFollowers            bool `json:"allow_followers"`             // Whether to accept follow requests
	AllowMentions             bool `json:"allow_mentions"`              // Whether to process mentions

	// Caching configuration
	CacheTTLSeconds int  `json:"cache_ttl_seconds"` // Actor/object cache TTL
	MaxCacheSize    int  `json:"max_cache_size"`    // Maximum cached items per instance
	EnableCaching   bool `json:"enable_caching"`    // Whether to cache instance data

	// Performance tuning
	MaxConcurrentRequests int `json:"max_concurrent_requests"` // Max concurrent outbound requests
	RequestTimeoutSeconds int `json:"request_timeout_seconds"` // HTTP request timeout
	ConnectionPoolSize    int `json:"connection_pool_size"`    // HTTP connection pool size
	CompressionThreshold  int `json:"compression_threshold"`   // Bytes threshold for compression

	// Trust and reputation
	TrustScore           float64 `json:"trust_score"`           // 0.0 to 1.0 trust score
	ReputationMultiplier float64 `json:"reputation_multiplier"` // Cost multiplier based on reputation
	RequireVouch         bool    `json:"require_vouch"`         // Whether vouching is required
	AutoAcceptThreshold  float64 `json:"auto_accept_threshold"` // Trust score for auto-accept

	// Admin notes
	Notes        string `json:"notes,omitempty"`         // Admin notes about the instance
	ContactEmail string `json:"contact_email,omitempty"` // Admin contact email

	// Type marker for queries
	Type string `json:"type"` // Always "InstanceConfig"

	// Timestamps
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"last_modified"`

	// No TTL - configurations are permanent until explicitly deleted
}

// UpdateKeys sets the primary and GSI keys for the config tracking model
func (f *FederationInstanceConfigTracking) UpdateKeys() {
	f.PK = fmt.Sprintf("INSTANCE#%s", f.Domain)
	f.SK = SKConfig
	f.Type = "InstanceConfig"

	// GSI1 for tier queries
	f.GSI1PK = fmt.Sprintf("TIER#%s", f.Tier)
	f.GSI1SK = fmt.Sprintf("DOMAIN#%s", f.Domain)

	// GSI2 for budget override queries (only if custom budget is set)
	if f.CustomBudgetUSD != nil {
		f.GSI2PK = "BUDGET_OVERRIDE"
		f.GSI2SK = fmt.Sprintf("BUDGET#%09.2f#%s", *f.CustomBudgetUSD, f.Domain)
	} else {
		f.GSI2PK = ""
		f.GSI2SK = ""
	}
}

// BeforeCreate is called before creating the record
func (f *FederationInstanceConfigTracking) BeforeCreate() error {
	now := time.Now()
	if f.Created.IsZero() {
		f.Created = now
	}
	f.LastModified = now

	// Set defaults if not specified
	if f.Tier == "" {
		f.Tier = FederationTierFree
	}
	if f.TrustScore == 0 {
		f.TrustScore = 0.5 // Neutral starting trust
	}
	if f.ReputationMultiplier == 0 {
		f.ReputationMultiplier = 1.0 // No cost adjustment by default
	}

	// Default feature flags (secure by default)
	if !f.EnableSignatureValidation {
		f.EnableSignatureValidation = true
	}
	if !f.EnableRateLimiting {
		f.EnableRateLimiting = true
	}
	if !f.EnableBudgetEnforcement {
		f.EnableBudgetEnforcement = true
	}

	// Default performance settings
	if f.CacheTTLSeconds == 0 {
		f.CacheTTLSeconds = 3600 // 1 hour default
	}
	if f.MaxCacheSize == 0 {
		f.MaxCacheSize = 1000
	}
	if f.MaxConcurrentRequests == 0 {
		f.MaxConcurrentRequests = 10
	}
	if f.RequestTimeoutSeconds == 0 {
		f.RequestTimeoutSeconds = 30
	}
	if f.ConnectionPoolSize == 0 {
		f.ConnectionPoolSize = 20
	}
	if f.CompressionThreshold == 0 {
		f.CompressionThreshold = 1024 // 1KB
	}

	f.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (f *FederationInstanceConfigTracking) BeforeUpdate() error {
	f.LastModified = time.Now()
	f.UpdateKeys()
	return nil
}

// GetBudgetLimit returns the effective budget limit for the instance
func (f *FederationInstanceConfigTracking) GetBudgetLimit() float64 {
	if f.CustomBudgetUSD != nil {
		return *f.CustomBudgetUSD
	}

	// Default budgets by tier
	switch f.Tier {
	case FederationTierFree:
		return 1.0 // $1/month
	case FederationTierBasic:
		return 10.0 // $10/month
	case FederationTierPremium:
		return 100.0 // $100/month
	case FederationTierEnterprise:
		return 1000.0 // $1000/month
	case FederationTierBlocked:
		return 0.0 // No budget
	default:
		return 1.0 // Default to free tier
	}
}

// GetRateLimit returns the effective rate limit for the instance
func (f *FederationInstanceConfigTracking) GetRateLimit() int {
	if f.RateLimitOverride != nil {
		return *f.RateLimitOverride
	}

	// Default rate limits by tier (requests per hour)
	switch f.Tier {
	case FederationTierFree:
		return 100
	case FederationTierBasic:
		return 1000
	case FederationTierPremium:
		return 10000
	case FederationTierEnterprise:
		return 100000
	case FederationTierBlocked:
		return 0
	default:
		return 100
	}
}

// TableName returns the DynamoDB table name
func (f *FederationInstanceConfigTracking) TableName() string {
	return MainTableName
}
