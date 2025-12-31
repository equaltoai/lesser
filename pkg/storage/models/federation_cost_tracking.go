package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// FederationCostTracking represents comprehensive cost tracking for federation activities
type FederationCostTracking struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - federation cost tracking uses FED_COST#{domain}#{timestamp} pattern
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI1 for time-based queries - FED_COSTS#{date}, TS#{timestamp}
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// GSI2 for activity type queries - FED_TYPE#{activity_type}, DOMAIN#{domain}#{timestamp}
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"`

	// Federation activity metadata
	ActivityID     string `dynamorm:"attr:activityID" json:"activity_id"`
	Domain         string `dynamorm:"attr:domain" json:"domain"`                  // Remote instance domain
	InstanceDomain string `dynamorm:"attr:instanceDomain" json:"instance_domain"` // Alias for Domain for compatibility
	ActivityType   string `dynamorm:"attr:activityType" json:"activity_type"`     // Create, Follow, Like, etc.
	Direction      string `dynamorm:"attr:direction" json:"direction"`            // inbound, outbound
	OperationType  string `dynamorm:"attr:operationType" json:"operation_type"`   // inbox_processing, outbox_delivery, signature_verification

	// Billing period tracking
	BillingPeriod string    `dynamorm:"attr:billingPeriod" json:"billing_period"` // YYYY-MM format
	LastUpdated   time.Time `dynamorm:"attr:lastUpdated" json:"last_updated"`     // Last update timestamp

	// Legacy compatibility fields for aggregated metrics
	IngressBytes   int64   `dynamorm:"attr:ingressBytes" json:"ingress_bytes"`      // Inbound data bytes
	EgressBytes    int64   `dynamorm:"attr:egressBytes" json:"egress_bytes"`        // Outbound data bytes
	RequestCount   int     `dynamorm:"attr:requestCount" json:"request_count"`      // Number of requests
	ErrorCount     int     `dynamorm:"attr:errorCount" json:"error_count"`          // Number of errors
	ErrorRate      float64 `dynamorm:"attr:errorRate" json:"error_rate"`            // Error rate percentage
	AverageCostUSD float64 `dynamorm:"attr:averageCostUSD" json:"average_cost_usd"` // Average cost in USD

	// Success/failure tracking
	Success      bool   `dynamorm:"attr:success" json:"success"`
	ErrorMessage string `dynamorm:"attr:errorMessage" json:"error_message,omitempty"`

	// Lambda execution costs (all in microdollars)
	LambdaExecutionCost int64 `dynamorm:"attr:lambdaExecutionCost" json:"lambda_execution_cost"` // Lambda compute cost
	LambdaDurationMs    int64 `dynamorm:"attr:lambdaDurationMs" json:"lambda_duration_ms"`       // Lambda execution time
	LambdaMemoryMB      int64 `dynamorm:"attr:lambdaMemoryMB" json:"lambda_memory_mb"`           // Lambda memory allocation

	// HTTP signature verification costs (CPU intensive)
	SignatureVerificationMs   int64 `dynamorm:"attr:signatureVerificationMs" json:"signature_verification_ms"`     // Time spent verifying signatures
	SignatureVerificationCost int64 `dynamorm:"attr:signatureVerificationCost" json:"signature_verification_cost"` // CPU cost for signature verification

	// Network costs
	HTTPRequestCount  int64 `dynamorm:"attr:httpRequestCount" json:"http_request_count"`   // Number of HTTP requests made
	HTTPRequestCost   int64 `dynamorm:"attr:httpRequestCost" json:"http_request_cost"`     // Cost of HTTP requests ($0.0001 per request)
	DataTransferBytes int64 `dynamorm:"attr:dataTransferBytes" json:"data_transfer_bytes"` // Bytes transferred (inbound/outbound)
	DataTransferCost  int64 `dynamorm:"attr:dataTransferCost" json:"data_transfer_cost"`   // Data transfer costs ($0.09 per GB outbound)

	// DynamoDB costs
	DynamoDBWriteCount int64   `dynamorm:"attr:dynamodbWriteCount" json:"dynamodb_write_count"` // Number of write operations
	DynamoDBReadCount  int64   `dynamorm:"attr:dynamodbReadCount" json:"dynamodb_read_count"`   // Number of read operations
	DynamoDBWriteUnits float64 `dynamorm:"attr:dynamodbWriteUnits" json:"dynamodb_write_units"` // Write capacity consumed
	DynamoDBReadUnits  float64 `dynamorm:"attr:dynamodbReadUnits" json:"dynamodb_read_units"`   // Read capacity consumed
	DynamoDBWriteCost  int64   `dynamorm:"attr:dynamodbWriteCost" json:"dynamodb_write_cost"`   // Write operation costs
	DynamoDBReadCost   int64   `dynamorm:"attr:dynamodbReadCost" json:"dynamodb_read_cost"`     // Read operation costs

	// DNS/WebFinger costs
	DNSLookupCount int64 `dynamorm:"attr:dnsLookupCount" json:"dns_lookup_count"` // Number of DNS lookups
	DNSLookupCost  int64 `dynamorm:"attr:dnsLookupCost" json:"dns_lookup_cost"`   // DNS lookup costs ($0.0004 per query)
	WebFingerCount int64 `dynamorm:"attr:webFingerCount" json:"webfinger_count"`  // WebFinger lookups
	WebFingerCost  int64 `dynamorm:"attr:webFingerCost" json:"webfinger_cost"`    // WebFinger lookup costs

	// SQS costs (for outbound delivery queue)
	SQSMessageCount int64 `dynamorm:"attr:sqsMessageCount" json:"sqs_message_count"` // SQS messages sent
	SQSMessageCost  int64 `dynamorm:"attr:sqsMessageCost" json:"sqs_message_cost"`   // SQS message costs ($0.0000004 per message)

	// Retry penalty costs
	RetryCount int   `dynamorm:"attr:retryCount" json:"retry_count"` // Number of retries performed
	RetryCost  int64 `dynamorm:"attr:retryCost" json:"retry_cost"`   // Additional cost penalties for retries

	// Detailed per-delivery attribution
	BytesSent         int64            `dynamorm:"attr:bytesSent" json:"bytes_sent"`                 // Actual bytes sent per delivery attempt
	RetryAttempts     int              `dynamorm:"attr:retryAttempts" json:"retry_attempts"`         // Number of retry attempts for this specific delivery
	DeliveryAttempts  int              `dynamorm:"attr:deliveryAttempts" json:"delivery_attempts"`   // Total delivery attempts (including first)
	RouteID           string           `dynamorm:"attr:routeID" json:"route_id,omitempty"`           // Route used for delivery
	DestinationServer string           `dynamorm:"attr:destinationServer" json:"destination_server"` // Destination server domain
	RouteBreakdown    map[string]int64 `dynamorm:"attr:routeBreakdown" json:"route_breakdown"`       // Per-route cost breakdown in microcents

	// Per-route delivery metrics
	RouteLatency      map[string]int64   `dynamorm:"attr:routeLatency" json:"route_latency"`            // Per-route response times in ms
	RouteErrors       map[string]int     `dynamorm:"attr:routeErrors" json:"route_errors"`              // Per-route error counts
	RouteAttempts     map[string]int     `dynamorm:"attr:routeAttempts" json:"route_attempts"`          // Per-route total attempts
	RouteSuccessRates map[string]float64 `dynamorm:"attr:routeSuccessRates" json:"route_success_rates"` // Per-route success rates

	// Enhanced retry tracking
	RetryDelaySeconds  []int64  `dynamorm:"attr:retryDelaySeconds" json:"retry_delay_seconds"`   // Delay between retry attempts
	RetryErrorMessages []string `dynamorm:"attr:retryErrorMessages" json:"retry_error_messages"` // Error messages for each retry
	FinalRetrySuccess  bool     `dynamorm:"attr:finalRetrySuccess" json:"final_retry_success"`   // Whether final retry succeeded

	// Performance metrics
	ResponseTimeMs   int64 `dynamorm:"attr:responseTimeMs" json:"response_time_ms"`     // Total response time
	ProcessingTimeMs int64 `dynamorm:"attr:processingTimeMs" json:"processing_time_ms"` // Time spent processing
	QueueWaitTimeMs  int64 `dynamorm:"attr:queueWaitTimeMs" json:"queue_wait_time_ms"`  // Time spent waiting in queue

	// Data volume metrics
	PayloadSize      int64   `dynamorm:"attr:payloadSize" json:"payload_size"`           // Size of activity payload
	CompressedSize   int64   `dynamorm:"attr:compressedSize" json:"compressed_size"`     // Size after compression
	CompressionRatio float64 `dynamorm:"attr:compressionRatio" json:"compression_ratio"` // Compression efficiency

	// Total cost breakdown
	TotalCostMicroCents int64 `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"` // Total cost in microcents

	// Timestamps
	Timestamp time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL for automatic cleanup (30 days for detailed cost records)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys sets the primary keys for the FederationCostTracking model
func (f *FederationCostTracking) UpdateKeys() error {
	timestampStr := f.Timestamp.Format(common.CompactTimeFormat)
	f.PK = fmt.Sprintf("FED_COST#%s#%s", f.Domain, timestampStr)
	f.SK = fmt.Sprintf("ACTIVITY#%s#%s", f.ActivityType, f.ActivityID)
	f.GSI1PK = fmt.Sprintf("FED_COSTS#%s", f.Timestamp.Format(common.CompactDateFormat))
	f.GSI1SK = fmt.Sprintf("TS#%s#%s", timestampStr, f.Domain)
	f.GSI2PK = fmt.Sprintf("FED_TYPE#%s", f.ActivityType)
	f.GSI2SK = fmt.Sprintf("DOMAIN#%s#%s", f.Domain, timestampStr)
	return nil
}

// GetPK returns the partition key (required for BaseModel interface)
func (f *FederationCostTracking) GetPK() string {
	return f.PK
}

// GetSK returns the sort key (required for BaseModel interface)
func (f *FederationCostTracking) GetSK() string {
	return f.SK
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

	// Initialize maps for detailed tracking
	if f.RouteBreakdown == nil {
		f.RouteBreakdown = make(map[string]int64)
	}
	if f.RouteLatency == nil {
		f.RouteLatency = make(map[string]int64)
	}
	if f.RouteErrors == nil {
		f.RouteErrors = make(map[string]int)
	}
	if f.RouteSuccessRates == nil {
		f.RouteSuccessRates = make(map[string]float64)
	}
	if f.RetryDelaySeconds == nil {
		f.RetryDelaySeconds = make([]int64, 0)
	}
	if f.RetryErrorMessages == nil {
		f.RetryErrorMessages = make([]string, 0)
	}

	// Set TTL to 30 days from creation (detailed records)
	f.TTL = now.AddDate(0, 0, 30).Unix()

	if err := f.UpdateKeys(); err != nil {
		return err
	}
	f.CalculateTotalCost()
	return nil
}

// BeforeUpdate is called before updating the record
func (f *FederationCostTracking) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	if err := f.UpdateKeys(); err != nil {
		return err
	}
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

// TableName returns the DynamoDB table backing FederationCostTracking.
func (FederationCostTracking) TableName() string {
	return MainTableName
}

// AddRouteDeliveryAttempt tracks a delivery attempt for a specific route
func (f *FederationCostTracking) AddRouteDeliveryAttempt(routeID string, bytes int64, latencyMs int64, success bool, errorMsg string) {
	// Initialize maps if needed
	if f.RouteBreakdown == nil {
		f.RouteBreakdown = make(map[string]int64)
	}
	if f.RouteLatency == nil {
		f.RouteLatency = make(map[string]int64)
	}
	if f.RouteErrors == nil {
		f.RouteErrors = make(map[string]int)
	}
	if f.RouteAttempts == nil {
		f.RouteAttempts = make(map[string]int)
	}
	if f.RouteSuccessRates == nil {
		f.RouteSuccessRates = make(map[string]float64)
	}

	// Calculate per-route cost based on bytes sent
	routeCost := int64(0)
	if f.DataTransferBytes > 0 {
		routeCost = (bytes * f.DataTransferCost) / f.DataTransferBytes // proportional cost
	}
	f.RouteBreakdown[routeID] += routeCost

	// Update latency (running average)
	if existingLatency, exists := f.RouteLatency[routeID]; exists {
		f.RouteLatency[routeID] = (existingLatency + latencyMs) / 2
	} else {
		f.RouteLatency[routeID] = latencyMs
	}

	// Track errors
	if !success {
		f.RouteErrors[routeID]++
		if errorMsg != "" {
			f.RetryErrorMessages = append(f.RetryErrorMessages, fmt.Sprintf("Route %s: %s", routeID, errorMsg))
		}
	}

	// Track total attempts and calculate success rate
	f.RouteAttempts[routeID]++

	// Calculate success rate: (total attempts - errors) / total attempts
	totalAttempts := f.RouteAttempts[routeID]
	errorCount := f.RouteErrors[routeID]
	successCount := totalAttempts - errorCount

	if totalAttempts > 0 {
		f.RouteSuccessRates[routeID] = float64(successCount) / float64(totalAttempts)
	} else {
		f.RouteSuccessRates[routeID] = 0.0
	}

	// Update overall delivery tracking
	f.DeliveryAttempts++
	f.BytesSent += bytes
	if !success {
		f.RetryAttempts++
	}
	f.FinalRetrySuccess = success
}

// AddRetryDelay tracks the delay before a retry attempt
func (f *FederationCostTracking) AddRetryDelay(delaySeconds int64) {
	if f.RetryDelaySeconds == nil {
		f.RetryDelaySeconds = make([]int64, 0)
	}
	f.RetryDelaySeconds = append(f.RetryDelaySeconds, delaySeconds)
}

// GetAverageRouteLatency returns the average latency across all routes
func (f *FederationCostTracking) GetAverageRouteLatency() int64 {
	if err := common.ValidateSliceNotEmpty("f.RouteLatency", f.RouteLatency); err != nil {
		return 0
	}

	total := int64(0)
	for _, latency := range f.RouteLatency {
		total += latency
	}
	return total / int64(len(f.RouteLatency))
}

// GetTotalRouteErrors returns the total error count across all routes
func (f *FederationCostTracking) GetTotalRouteErrors() int {
	total := 0
	for _, errors := range f.RouteErrors {
		total += errors
	}
	return total
}

// GetMostExpensiveRoute returns the route ID with the highest cost
func (f *FederationCostTracking) GetMostExpensiveRoute() (string, int64) {
	var maxRoute string
	var maxCost int64

	for routeID, cost := range f.RouteBreakdown {
		if cost > maxCost {
			maxCost = cost
			maxRoute = routeID
		}
	}

	return maxRoute, maxCost
}

// GetRetryEfficiency returns metrics about retry effectiveness
func (f *FederationCostTracking) GetRetryEfficiency() map[string]interface{} {
	efficiency := make(map[string]interface{})

	efficiency["total_attempts"] = f.DeliveryAttempts
	efficiency["retry_attempts"] = f.RetryAttempts
	efficiency["final_success"] = f.FinalRetrySuccess

	if f.DeliveryAttempts > 0 {
		efficiency["retry_rate"] = float64(f.RetryAttempts) / float64(f.DeliveryAttempts)
	}

	if len(f.RetryDelaySeconds) > 0 {
		totalDelay := int64(0)
		for _, delay := range f.RetryDelaySeconds {
			totalDelay += delay
		}
		efficiency["average_retry_delay_seconds"] = totalDelay / int64(len(f.RetryDelaySeconds))
	}

	efficiency["error_messages"] = len(f.RetryErrorMessages)

	return efficiency
}

// FederationBudget represents budget limits for federation operations per instance
type FederationBudget struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - federation budgets use FED_BUDGET#{domain}#{period} pattern
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI1 for active budget queries - ACTIVE_BUDGETS, DOMAIN#{domain}#{period}
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// Budget configuration
	Domain string `dynamorm:"attr:domain" json:"domain"`
	Period string `dynamorm:"attr:period" json:"period"` // daily, weekly, monthly

	// Budget limits (in microcents)
	InboundLimitMicroCents  int64 `dynamorm:"attr:inboundLimitMicroCents" json:"inbound_limit_micro_cents"`
	OutboundLimitMicroCents int64 `dynamorm:"attr:outboundLimitMicroCents" json:"outbound_limit_micro_cents"`
	CombinedLimitMicroCents int64 `dynamorm:"attr:combinedLimitMicroCents" json:"combined_limit_micro_cents"`

	// Per-activity type limits
	ActivityTypeLimits map[string]int64 `dynamorm:"attr:activityTypeLimits" json:"activity_type_limits,omitempty"`

	// Current usage (reset per period)
	CurrentInboundCost  int64 `dynamorm:"attr:currentInboundCost" json:"current_inbound_cost"`
	CurrentOutboundCost int64 `dynamorm:"attr:currentOutboundCost" json:"current_outbound_cost"`
	CurrentCombinedCost int64 `dynamorm:"attr:currentCombinedCost" json:"current_combined_cost"`

	// Activity type usage
	ActivityTypeUsage map[string]int64 `dynamorm:"attr:activityTypeUsage" json:"activity_type_usage,omitempty"`

	// Usage tracking
	InboundActivityCount  int64      `dynamorm:"attr:inboundActivityCount" json:"inbound_activity_count"`
	OutboundActivityCount int64      `dynamorm:"attr:outboundActivityCount" json:"outbound_activity_count"`
	LastInboundAt         *time.Time `dynamorm:"attr:lastInboundAt" json:"last_inbound_at,omitempty"`
	LastOutboundAt        *time.Time `dynamorm:"attr:lastOutboundAt" json:"last_outbound_at,omitempty"`

	// Period tracking
	PeriodStart time.Time `dynamorm:"attr:periodStart" json:"period_start"`
	PeriodEnd   time.Time `dynamorm:"attr:periodEnd" json:"period_end"`

	// Alert settings
	AlertThresholdPercent float64    `dynamorm:"attr:alertThresholdPercent" json:"alert_threshold_percent"` // Send alert at this % of limit
	AlertSendingEnabled   bool       `dynamorm:"attr:alertSendingEnabled" json:"alert_sending_enabled"`
	LastAlertSentAt       *time.Time `dynamorm:"attr:lastAlertSentAt" json:"last_alert_sent_at,omitempty"`

	// Enforcement settings
	BlockOnLimitExceeded bool `dynamorm:"attr:blockOnLimitExceeded" json:"block_on_limit_exceeded"`
	RateLimitOnThreshold bool `dynamorm:"attr:rateLimitOnThreshold" json:"rate_limit_on_threshold"`

	// Status
	IsActive bool   `dynamorm:"attr:isActive" json:"is_active"`
	Status   string `dynamorm:"attr:status" json:"status"` // active, suspended, over_limit

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys sets the primary keys for the FederationBudget model
func (f *FederationBudget) UpdateKeys() error {
	f.PK = fmt.Sprintf("FED_BUDGET#%s#%s", f.Domain, f.Period)
	f.SK = SKConfig
	f.GSI1PK = "ACTIVE_BUDGETS"
	f.GSI1SK = fmt.Sprintf("DOMAIN#%s#%s", f.Domain, f.Period)
	return nil
}

// GetPK returns the partition key (required for BaseModel interface)
func (f *FederationBudget) GetPK() string {
	return f.PK
}

// GetSK returns the sort key (required for BaseModel interface)
func (f *FederationBudget) GetSK() string {
	return f.SK
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

	if err := f.UpdateKeys(); err != nil {
		return err
	}
	return nil
}

// BeforeUpdate is called before updating the record
func (f *FederationBudget) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	if err := f.UpdateKeys(); err != nil {
		return err
	}
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

	f.Status = StatusActive
}

// TableName returns the DynamoDB table backing FederationBudget.
func (FederationBudget) TableName() string {
	return MainTableName
}
