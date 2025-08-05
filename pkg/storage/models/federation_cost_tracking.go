package models

import (
	"fmt"
	"time"
)

// FederationCostTracking represents comprehensive cost tracking for federation activities
type FederationCostTracking struct {
	// Primary keys - federation cost tracking uses FED_COST#{domain}#{timestamp} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`
	
	// GSI1 for time-based queries - FED_COSTS#{date}, TS#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`
	
	// GSI2 for activity type queries - FED_TYPE#{activity_type}, DOMAIN#{domain}#{timestamp}
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2_sk"`
	
	// Federation activity metadata
	ActivityID     string    `json:"activity_id"`
	Domain         string    `json:"domain"`         // Remote instance domain
	InstanceDomain string    `json:"instance_domain"` // Alias for Domain for compatibility
	ActivityType   string    `json:"activity_type"`  // Create, Follow, Like, etc.
	Direction      string    `json:"direction"`      // inbound, outbound
	OperationType  string    `json:"operation_type"` // inbox_processing, outbox_delivery, signature_verification
	
	// Billing period tracking
	BillingPeriod string    `json:"billing_period"` // YYYY-MM format
	LastUpdated   time.Time `json:"last_updated"`   // Last update timestamp
	
	// Legacy compatibility fields for aggregated metrics
	IngressBytes   int64   `json:"ingress_bytes"`   // Inbound data bytes
	EgressBytes    int64   `json:"egress_bytes"`    // Outbound data bytes
	RequestCount   int     `json:"request_count"`   // Number of requests
	ErrorCount     int     `json:"error_count"`     // Number of errors
	ErrorRate      float64 `json:"error_rate"`      // Error rate percentage
	AverageCostUSD float64 `json:"average_cost_usd"` // Average cost in USD
	
	// Success/failure tracking
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	
	// Lambda execution costs (all in microdollars)
	LambdaExecutionCost   int64 `json:"lambda_execution_cost"`   // Lambda compute cost
	LambdaDurationMs      int64 `json:"lambda_duration_ms"`      // Lambda execution time
	LambdaMemoryMB        int64 `json:"lambda_memory_mb"`        // Lambda memory allocation
	
	// HTTP signature verification costs (CPU intensive)
	SignatureVerificationMs     int64 `json:"signature_verification_ms"`   // Time spent verifying signatures
	SignatureVerificationCost   int64 `json:"signature_verification_cost"` // CPU cost for signature verification
	
	// Network costs
	HTTPRequestCount    int64 `json:"http_request_count"`    // Number of HTTP requests made
	HTTPRequestCost     int64 `json:"http_request_cost"`     // Cost of HTTP requests ($0.0001 per request)
	DataTransferBytes   int64 `json:"data_transfer_bytes"`   // Bytes transferred (inbound/outbound)
	DataTransferCost    int64 `json:"data_transfer_cost"`    // Data transfer costs ($0.09 per GB outbound)
	
	// DynamoDB costs
	DynamoDBWriteCount    int64   `json:"dynamodb_write_count"`     // Number of write operations
	DynamoDBReadCount     int64   `json:"dynamodb_read_count"`      // Number of read operations
	DynamoDBWriteUnits    float64 `json:"dynamodb_write_units"`     // Write capacity consumed
	DynamoDBReadUnits     float64 `json:"dynamodb_read_units"`      // Read capacity consumed
	DynamoDBWriteCost     int64   `json:"dynamodb_write_cost"`      // Write operation costs
	DynamoDBReadCost      int64   `json:"dynamodb_read_cost"`       // Read operation costs
	
	// DNS/WebFinger costs
	DNSLookupCount   int64 `json:"dns_lookup_count"`   // Number of DNS lookups
	DNSLookupCost    int64 `json:"dns_lookup_cost"`    // DNS lookup costs ($0.0004 per query)
	WebFingerCount   int64 `json:"webfinger_count"`    // WebFinger lookups
	WebFingerCost    int64 `json:"webfinger_cost"`     // WebFinger lookup costs
	
	// SQS costs (for outbound delivery queue)
	SQSMessageCount int64 `json:"sqs_message_count"` // SQS messages sent
	SQSMessageCost  int64 `json:"sqs_message_cost"`  // SQS message costs ($0.0000004 per message)
	
	// Retry penalty costs
	RetryCount     int   `json:"retry_count"`      // Number of retries performed
	RetryCost      int64 `json:"retry_cost"`       // Additional cost penalties for retries
	
	// Performance metrics
	ResponseTimeMs       int64 `json:"response_time_ms"`       // Total response time
	ProcessingTimeMs     int64 `json:"processing_time_ms"`     // Time spent processing
	QueueWaitTimeMs      int64 `json:"queue_wait_time_ms"`     // Time spent waiting in queue
	
	// Data volume metrics
	PayloadSize       int64 `json:"payload_size"`        // Size of activity payload
	CompressedSize    int64 `json:"compressed_size"`     // Size after compression
	CompressionRatio  float64 `json:"compression_ratio"` // Compression efficiency
	
	// Total cost breakdown
	TotalCostMicroCents int64 `json:"total_cost_micro_cents"` // Total cost in microcents
	
	// Timestamps
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	// TTL for automatic cleanup (30 days for detailed cost records)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the primary keys for the FederationCostTracking model
func (f *FederationCostTracking) UpdateKeys() {
	timestampStr := f.Timestamp.Format("20060102150405")
	f.PK = fmt.Sprintf("FED_COST#%s#%s", f.Domain, timestampStr)
	f.SK = fmt.Sprintf("ACTIVITY#%s#%s", f.ActivityType, f.ActivityID)
	f.GSI1PK = fmt.Sprintf("FED_COSTS#%s", f.Timestamp.Format("20060102"))
	f.GSI1SK = fmt.Sprintf("TS#%s#%s", timestampStr, f.Domain)
	f.GSI2PK = fmt.Sprintf("FED_TYPE#%s", f.ActivityType)
	f.GSI2SK = fmt.Sprintf("DOMAIN#%s#%s", f.Domain, timestampStr)
}

// BeforeCreate is called before creating the record
func (f *FederationCostTracking) BeforeCreate() error {
	now := time.Now()
	if f.Timestamp.IsZero() {
		f.Timestamp = now
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	
	// Set TTL to 30 days from creation (detailed records)
	f.TTL = now.AddDate(0, 0, 30).Unix()
	
	f.UpdateKeys()
	f.CalculateTotalCost()
	return nil
}

// BeforeUpdate is called before updating the record
func (f *FederationCostTracking) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	f.UpdateKeys()
	f.CalculateTotalCost()
	return nil
}

// CalculateTotalCost calculates the total cost from all components
func (f *FederationCostTracking) CalculateTotalCost() {
	f.TotalCostMicroCents = f.LambdaExecutionCost +
		f.SignatureVerificationCost +
		f.HTTPRequestCost +
		f.DataTransferCost +
		f.DynamoDBWriteCost +
		f.DynamoDBReadCost +
		f.DNSLookupCost +
		f.WebFingerCost +
		f.SQSMessageCost +
		f.RetryCost
}

// GetTotalCostDollars returns the total cost in dollars
func (f *FederationCostTracking) GetTotalCostDollars() float64 {
	return float64(f.TotalCostMicroCents) / 1_000_000.0
}

// TableName returns the DynamoDB table name
func (f *FederationCostTracking) TableName() string {
	return "" // Will be set by the repository
}

// FederationBudget represents budget limits for federation operations per instance
type FederationBudget struct {
	// Primary keys - federation budgets use FED_BUDGET#{domain}#{period} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`
	
	// GSI1 for active budget queries - ACTIVE_BUDGETS, DOMAIN#{domain}#{period}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`
	
	// Budget configuration
	Domain string `json:"domain"`
	Period string `json:"period"` // daily, weekly, monthly
	
	// Budget limits (in microcents)
	InboundLimitMicroCents  int64 `json:"inbound_limit_micro_cents"`
	OutboundLimitMicroCents int64 `json:"outbound_limit_micro_cents"`
	CombinedLimitMicroCents int64 `json:"combined_limit_micro_cents"`
	
	// Per-activity type limits
	ActivityTypeLimits map[string]int64 `json:"activity_type_limits,omitempty"`
	
	// Current usage (reset per period)
	CurrentInboundCost  int64 `json:"current_inbound_cost"`
	CurrentOutboundCost int64 `json:"current_outbound_cost"`
	CurrentCombinedCost int64 `json:"current_combined_cost"`
	
	// Activity type usage
	ActivityTypeUsage map[string]int64 `json:"activity_type_usage,omitempty"`
	
	// Usage tracking
	InboundActivityCount  int64 `json:"inbound_activity_count"`
	OutboundActivityCount int64 `json:"outbound_activity_count"`
	LastInboundAt         *time.Time `json:"last_inbound_at,omitempty"`
	LastOutboundAt        *time.Time `json:"last_outbound_at,omitempty"`
	
	// Period tracking
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	
	// Alert settings
	AlertThresholdPercent float64    `json:"alert_threshold_percent"`  // Send alert at this % of limit
	AlertSendingEnabled   bool       `json:"alert_sending_enabled"`
	LastAlertSentAt       *time.Time `json:"last_alert_sent_at,omitempty"`
	
	// Enforcement settings
	BlockOnLimitExceeded bool `json:"block_on_limit_exceeded"`
	RateLimitOnThreshold bool `json:"rate_limit_on_threshold"`
	
	// Status
	IsActive bool   `json:"is_active"`
	Status   string `json:"status"` // active, suspended, over_limit
	
	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateKeys sets the primary keys for the FederationBudget model
func (f *FederationBudget) UpdateKeys() {
	f.PK = fmt.Sprintf("FED_BUDGET#%s#%s", f.Domain, f.Period)
	f.SK = "CONFIG"
	f.GSI1PK = "ACTIVE_BUDGETS"
	f.GSI1SK = fmt.Sprintf("DOMAIN#%s#%s", f.Domain, f.Period)
}

// BeforeCreate is called before creating the record
func (f *FederationBudget) BeforeCreate() error {
	now := time.Now()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	
	// Initialize maps if nil
	if f.ActivityTypeLimits == nil {
		f.ActivityTypeLimits = make(map[string]int64)
	}
	if f.ActivityTypeUsage == nil {
		f.ActivityTypeUsage = make(map[string]int64)
	}
	
	f.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (f *FederationBudget) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	f.UpdateKeys()
	return nil
}

// IsOverInboundLimit checks if the domain is over their inbound limit
func (f *FederationBudget) IsOverInboundLimit() bool {
	return f.CurrentInboundCost >= f.InboundLimitMicroCents
}

// IsOverOutboundLimit checks if the domain is over their outbound limit
func (f *FederationBudget) IsOverOutboundLimit() bool {
	return f.CurrentOutboundCost >= f.OutboundLimitMicroCents
}

// IsOverCombinedLimit checks if the domain is over their combined limit
func (f *FederationBudget) IsOverCombinedLimit() bool {
	return f.CurrentCombinedCost >= f.CombinedLimitMicroCents
}

// IsOverActivityTypeLimit checks if the domain is over a specific activity type limit
func (f *FederationBudget) IsOverActivityTypeLimit(activityType string) bool {
	limit, exists := f.ActivityTypeLimits[activityType]
	if !exists {
		return false
	}
	usage := f.ActivityTypeUsage[activityType]
	return usage >= limit
}

// GetInboundUsagePercent returns inbound usage as a percentage of limit
func (f *FederationBudget) GetInboundUsagePercent() float64 {
	if f.InboundLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(f.CurrentInboundCost) / float64(f.InboundLimitMicroCents)) * 100.0
}

// GetOutboundUsagePercent returns outbound usage as a percentage of limit
func (f *FederationBudget) GetOutboundUsagePercent() float64 {
	if f.OutboundLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(f.CurrentOutboundCost) / float64(f.OutboundLimitMicroCents)) * 100.0
}

// GetCombinedUsagePercent returns combined usage as a percentage of limit
func (f *FederationBudget) GetCombinedUsagePercent() float64 {
	if f.CombinedLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(f.CurrentCombinedCost) / float64(f.CombinedLimitMicroCents)) * 100.0
}

// GetActivityTypeUsagePercent returns activity type usage as a percentage of limit
func (f *FederationBudget) GetActivityTypeUsagePercent(activityType string) float64 {
	limit, exists := f.ActivityTypeLimits[activityType]
	if !exists || limit == 0 {
		return 0.0
	}
	usage := f.ActivityTypeUsage[activityType]
	return (float64(usage) / float64(limit)) * 100.0
}

// ShouldSendAlert checks if an alert should be sent
func (f *FederationBudget) ShouldSendAlert() bool {
	if !f.AlertSendingEnabled {
		return false
	}
	
	// Check if we've exceeded the alert threshold
	if f.GetCombinedUsagePercent() < f.AlertThresholdPercent {
		return false
	}
	
	// Check if we've already sent an alert recently (within 1 hour)
	if f.LastAlertSentAt != nil && time.Since(*f.LastAlertSentAt) < time.Hour {
		return false
	}
	
	return true
}

// ShouldBlock checks if federation should be blocked for this domain
func (f *FederationBudget) ShouldBlock() bool {
	if !f.BlockOnLimitExceeded {
		return false
	}
	
	return f.IsOverCombinedLimit()
}

// ShouldRateLimit checks if federation should be rate limited for this domain
func (f *FederationBudget) ShouldRateLimit() bool {
	if !f.RateLimitOnThreshold {
		return false
	}
	
	return f.GetCombinedUsagePercent() >= f.AlertThresholdPercent
}

// AddUsage adds usage for a specific activity type and direction
func (f *FederationBudget) AddUsage(activityType, direction string, cost int64) {
	// Initialize maps if nil
	if f.ActivityTypeUsage == nil {
		f.ActivityTypeUsage = make(map[string]int64)
	}
	
	// Add to activity type usage
	f.ActivityTypeUsage[activityType] += cost
	
	// Add to directional usage
	switch direction {
	case "inbound":
		f.CurrentInboundCost += cost
		f.InboundActivityCount++
		now := time.Now()
		f.LastInboundAt = &now
	case "outbound":
		f.CurrentOutboundCost += cost
		f.OutboundActivityCount++
		now := time.Now()
		f.LastOutboundAt = &now
	}
	
	// Update combined cost
	f.CurrentCombinedCost = f.CurrentInboundCost + f.CurrentOutboundCost
}

// ResetPeriod resets usage counters for a new period
func (f *FederationBudget) ResetPeriod(newPeriodStart, newPeriodEnd time.Time) {
	f.CurrentInboundCost = 0
	f.CurrentOutboundCost = 0
	f.CurrentCombinedCost = 0
	f.InboundActivityCount = 0
	f.OutboundActivityCount = 0
	f.LastInboundAt = nil
	f.LastOutboundAt = nil
	f.PeriodStart = newPeriodStart
	f.PeriodEnd = newPeriodEnd
	
	// Reset activity type usage
	if f.ActivityTypeUsage != nil {
		for key := range f.ActivityTypeUsage {
			f.ActivityTypeUsage[key] = 0
		}
	}
	
	f.Status = "active"
}

// TableName returns the DynamoDB table name
func (f *FederationBudget) TableName() string {
	return "" // Will be set by the repository
}