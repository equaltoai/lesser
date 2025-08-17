package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// RelayCost represents cost tracking for relay operations
type RelayCost struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI fields for querying by relay URL
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1sk,omitempty"`

	// GSI fields for querying by time period
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2pk,omitempty"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2sk,omitempty"`

	// Cost tracking fields
	RelayURL      string `json:"relay_url"`
	Domain        string `json:"domain"`
	OperationType string `json:"operation_type"`          // "subscription", "delivery", "processing", "bandwidth"
	Direction     string `json:"direction"`               // "inbound", "outbound"
	ActivityType  string `json:"activity_type,omitempty"` // Create, Announce, Follow, etc.

	// Cost details
	HTTPRequestCount    int64 `json:"http_request_count"`     // Number of HTTP requests
	HTTPRequestCost     int64 `json:"http_request_cost"`      // Cost in microdollars
	DataTransferBytes   int64 `json:"data_transfer_bytes"`    // Bytes transferred
	DataTransferCost    int64 `json:"data_transfer_cost"`     // Cost in microdollars
	LambdaDurationMs    int64 `json:"lambda_duration_ms"`     // Lambda processing time
	LambdaCost          int64 `json:"lambda_cost"`            // Cost in microdollars
	DynamoDBOperations  int64 `json:"dynamodb_operations"`    // DB operation count
	DynamoDBCost        int64 `json:"dynamodb_cost"`          // Cost in microdollars
	SQSMessages         int64 `json:"sqs_messages"`           // SQS message count
	SQSCost             int64 `json:"sqs_cost"`               // Cost in microdollars
	TotalCostMicroCents int64 `json:"total_cost_micro_cents"` // Total cost in microdollars

	// Performance metrics
	ResponseTimeMs int64  `json:"response_time_ms"`
	Success        bool   `json:"success"`
	ErrorMessage   string `json:"error_message,omitempty"`
	RetryCount     int    `json:"retry_count"`

	// Metadata
	RequestID string    `json:"request_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	TTL       int64     `json:"ttl,omitempty" dynamorm:"ttl"` // For automatic cleanup
}

// UpdateKeys updates the composite keys based on the relay cost data
func (rc *RelayCost) UpdateKeys() {
	// Primary key: RELAY_COST#relayURL#operationType
	rc.PK = fmt.Sprintf("RELAY_COST#%s#%s", rc.RelayURL, rc.OperationType)
	rc.SK = fmt.Sprintf("TS#%s#%s", rc.Timestamp.Format(common.CompactTimeFormat), rc.RequestID)

	// GSI1: Query costs by relay URL
	rc.GSI1PK = fmt.Sprintf("RELAY_COSTS#%s", rc.RelayURL)
	rc.GSI1SK = fmt.Sprintf("%s#%s", rc.Timestamp.Format(common.CompactTimeFormat), rc.OperationType)

	// GSI2: Query costs by time period (for aggregation)
	date := rc.Timestamp.Format(common.CompactDateFormat)
	rc.GSI2PK = fmt.Sprintf("RELAY_COSTS_DAILY#%s", date)
	rc.GSI2SK = fmt.Sprintf("%s#%s#%s", rc.RelayURL, rc.OperationType, rc.Timestamp.Format("150405"))
}

// BeforeCreate validates and sets up the model before creation
func (rc *RelayCost) BeforeCreate() error {
	if err := common.ValidateRequiredParam("relay_url", rc.RelayURL); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("operation_type", rc.OperationType); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("direction", rc.Direction); err != nil {
		return err
	}
	if rc.Timestamp.IsZero() {
		rc.Timestamp = time.Now()
	}
	if err := common.ValidateRequiredParam("rc.RequestID", rc.RequestID); err != nil {
		rc.RequestID = fmt.Sprintf("relay-%d", rc.Timestamp.UnixNano())
	}

	// Calculate total cost
	rc.TotalCostMicroCents = rc.HTTPRequestCost + rc.DataTransferCost +
		rc.LambdaCost + rc.DynamoDBCost + rc.SQSCost

	// Set TTL for automatic cleanup (30 days for cost records)
	if rc.TTL == 0 {
		rc.TTL = rc.Timestamp.Add(30 * 24 * time.Hour).Unix()
	}

	rc.UpdateKeys()
	return nil
}

// RelayMetrics represents aggregated metrics for relay operations
type RelayMetrics struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI fields for querying by relay URL
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1sk,omitempty"`

	// Metrics details
	RelayURL    string    `json:"relay_url"`
	Domain      string    `json:"domain"`
	Period      string    `json:"period"` // "hourly", "daily", "weekly", "monthly"
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`

	// Aggregate counts
	TotalOperations        int64 `json:"total_operations"`
	SuccessfulOperations   int64 `json:"successful_operations"`
	FailedOperations       int64 `json:"failed_operations"`
	TotalHTTPRequests      int64 `json:"total_http_requests"`
	TotalDataTransferBytes int64 `json:"total_data_transfer_bytes"`
	TotalLambdaDurationMs  int64 `json:"total_lambda_duration_ms"`
	TotalDynamoDBOps       int64 `json:"total_dynamodb_ops"`
	TotalSQSMessages       int64 `json:"total_sqs_messages"`

	// Aggregate costs (in microdollars)
	TotalHTTPRequestCost  int64 `json:"total_http_request_cost"`
	TotalDataTransferCost int64 `json:"total_data_transfer_cost"`
	TotalLambdaCost       int64 `json:"total_lambda_cost"`
	TotalDynamoDBCost     int64 `json:"total_dynamodb_cost"`
	TotalSQSCost          int64 `json:"total_sqs_cost"`
	TotalCostMicroCents   int64 `json:"total_cost_micro_cents"`

	// Performance metrics
	AverageResponseTimeMs float64 `json:"average_response_time_ms"`
	SuccessRate           float64 `json:"success_rate"` // 0.0 to 1.0
	AverageRetryCount     float64 `json:"average_retry_count"`

	// Cost efficiency metrics
	CostPerOperation    float64 `json:"cost_per_operation"`     // Dollars
	CostPerSuccessfulOp float64 `json:"cost_per_successful_op"` // Dollars
	CostPerMB           float64 `json:"cost_per_mb"`            // Dollars per MB

	// Breakdown by operation type
	OperationBreakdown map[string]*RelayOperationStats `json:"operation_breakdown,omitempty"`

	// Budget tracking
	BudgetLimitMicroCents int64   `json:"budget_limit_micro_cents,omitempty"`
	BudgetUsedPercent     float64 `json:"budget_used_percent,omitempty"`
	BudgetExceeded        bool    `json:"budget_exceeded,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TTL       int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// RelayOperationStats represents stats for a specific operation type
type RelayOperationStats struct {
	OperationType       string  `json:"operation_type"`
	Count               int64   `json:"count"`
	SuccessCount        int64   `json:"success_count"`
	FailureCount        int64   `json:"failure_count"`
	TotalCostMicroCents int64   `json:"total_cost_micro_cents"`
	AverageResponseTime float64 `json:"average_response_time_ms"`
	SuccessRate         float64 `json:"success_rate"`
}

// UpdateKeys updates the composite keys based on the relay metrics data
func (rm *RelayMetrics) UpdateKeys() {
	// Primary key: RELAY_METRICS#relayURL#period
	rm.PK = fmt.Sprintf("RELAY_METRICS#%s#%s", rm.RelayURL, rm.Period)
	rm.SK = fmt.Sprintf("WINDOW#%s", rm.WindowStart.Format(common.CompactTimeFormat))

	// GSI1: Query metrics by relay URL across all periods
	rm.GSI1PK = fmt.Sprintf("RELAY_METRICS#%s", rm.RelayURL)
	rm.GSI1SK = fmt.Sprintf("%s#%s", rm.Period, rm.WindowStart.Format(common.CompactTimeFormat))
}

// BeforeCreate validates and sets up the model before creation
func (rm *RelayMetrics) BeforeCreate() error {
	if err := common.ValidateRequiredParam("relay_url", rm.RelayURL); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("period", rm.Period); err != nil {
		return err
	}
	if rm.WindowStart.IsZero() {
		return fmt.Errorf("window_start is required")
	}

	now := time.Now()
	if rm.CreatedAt.IsZero() {
		rm.CreatedAt = now
	}
	rm.UpdatedAt = now

	// Calculate derived metrics
	if rm.TotalOperations > 0 {
		rm.SuccessRate = float64(rm.SuccessfulOperations) / float64(rm.TotalOperations)
		rm.CostPerOperation = float64(rm.TotalCostMicroCents) / 1_000_000.0 / float64(rm.TotalOperations)
		if rm.SuccessfulOperations > 0 {
			rm.CostPerSuccessfulOp = float64(rm.TotalCostMicroCents) / 1_000_000.0 / float64(rm.SuccessfulOperations)
		}
	}

	if rm.TotalDataTransferBytes > 0 {
		mb := float64(rm.TotalDataTransferBytes) / 1_000_000.0
		rm.CostPerMB = float64(rm.TotalCostMicroCents) / 1_000_000.0 / mb
	}

	// Calculate budget usage
	if rm.BudgetLimitMicroCents > 0 {
		rm.BudgetUsedPercent = float64(rm.TotalCostMicroCents) / float64(rm.BudgetLimitMicroCents) * 100.0
		rm.BudgetExceeded = rm.TotalCostMicroCents > rm.BudgetLimitMicroCents
	}

	// Set TTL based on period
	ttlDuration := 90 * 24 * time.Hour // 90 days default
	switch rm.Period {
	case "hourly":
		ttlDuration = 7 * 24 * time.Hour // 7 days for hourly
	case PeriodDaily:
		ttlDuration = 30 * 24 * time.Hour // 30 days for daily
	case PeriodWeekly:
		ttlDuration = 90 * 24 * time.Hour // 90 days for weekly
	case PeriodMonthly:
		ttlDuration = 365 * 24 * time.Hour // 1 year for monthly
	}

	if rm.TTL == 0 {
		rm.TTL = rm.CreatedAt.Add(ttlDuration).Unix()
	}

	rm.UpdateKeys()
	return nil
}

// BeforeUpdate validates and sets up the model before update
func (rm *RelayMetrics) BeforeUpdate() error {
	rm.UpdatedAt = time.Now()

	// Recalculate derived metrics
	if rm.TotalOperations > 0 {
		rm.SuccessRate = float64(rm.SuccessfulOperations) / float64(rm.TotalOperations)
		rm.CostPerOperation = float64(rm.TotalCostMicroCents) / 1_000_000.0 / float64(rm.TotalOperations)
		if rm.SuccessfulOperations > 0 {
			rm.CostPerSuccessfulOp = float64(rm.TotalCostMicroCents) / 1_000_000.0 / float64(rm.SuccessfulOperations)
		}
	}

	if rm.TotalDataTransferBytes > 0 {
		mb := float64(rm.TotalDataTransferBytes) / 1_000_000.0
		rm.CostPerMB = float64(rm.TotalCostMicroCents) / 1_000_000.0 / mb
	}

	// Update budget usage
	if rm.BudgetLimitMicroCents > 0 {
		rm.BudgetUsedPercent = float64(rm.TotalCostMicroCents) / float64(rm.BudgetLimitMicroCents) * 100.0
		rm.BudgetExceeded = rm.TotalCostMicroCents > rm.BudgetLimitMicroCents
	}

	rm.UpdateKeys()
	return nil
}

// RelayBudget represents budget limits for relay operations
type RelayBudget struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// Budget details
	RelayURL        string `json:"relay_url"`
	Domain          string `json:"domain"`
	Period          string `json:"period"`            // "daily", "weekly", "monthly"
	LimitMicroCents int64  `json:"limit_micro_cents"` // Budget limit in microdollars

	// Alert thresholds
	WarningThresholdPercent  float64 `json:"warning_threshold_percent"`  // e.g., 75.0 for 75%
	CriticalThresholdPercent float64 `json:"critical_threshold_percent"` // e.g., 90.0 for 90%

	// Current usage
	CurrentUsageMicroCents int64     `json:"current_usage_micro_cents"`
	CurrentUsagePercent    float64   `json:"current_usage_percent"`
	LastResetAt            time.Time `json:"last_reset_at"`

	// Alert status
	WarningAlertSent  bool `json:"warning_alert_sent"`
	CriticalAlertSent bool `json:"critical_alert_sent"`
	BudgetExceeded    bool `json:"budget_exceeded"`

	// Actions on budget exceeded
	PauseRelay      bool `json:"pause_relay"`      // Pause relay when budget exceeded
	NotifyAdmin     bool `json:"notify_admin"`     // Send admin notification
	ReduceFrequency bool `json:"reduce_frequency"` // Reduce relay forwarding frequency

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TTL       int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the composite keys based on the relay budget data
func (rb *RelayBudget) UpdateKeys() {
	// Primary key: RELAY_BUDGET#relayURL#period
	rb.PK = fmt.Sprintf("RELAY_BUDGET#%s#%s", rb.RelayURL, rb.Period)
	rb.SK = SKConfig
}

// BeforeCreate validates and sets up the model before creation
func (rb *RelayBudget) BeforeCreate() error {
	if err := common.ValidateRequiredParam("relay_url", rb.RelayURL); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("period", rb.Period); err != nil {
		return err
	}
	if rb.LimitMicroCents <= 0 {
		return fmt.Errorf("limit_micro_cents must be positive")
	}

	now := time.Now()
	if rb.CreatedAt.IsZero() {
		rb.CreatedAt = now
	}
	rb.UpdatedAt = now

	if rb.LastResetAt.IsZero() {
		rb.LastResetAt = now
	}

	// Set default thresholds if not provided
	if rb.WarningThresholdPercent == 0 {
		rb.WarningThresholdPercent = 75.0
	}
	if rb.CriticalThresholdPercent == 0 {
		rb.CriticalThresholdPercent = 90.0
	}

	// Calculate current usage percent
	if rb.LimitMicroCents > 0 {
		rb.CurrentUsagePercent = float64(rb.CurrentUsageMicroCents) / float64(rb.LimitMicroCents) * 100.0
		rb.BudgetExceeded = rb.CurrentUsageMicroCents > rb.LimitMicroCents
	}

	// Set TTL for cleanup (keep budgets longer than metrics)
	if rb.TTL == 0 {
		rb.TTL = rb.CreatedAt.Add(365 * 24 * time.Hour).Unix() // 1 year
	}

	rb.UpdateKeys()
	return nil
}

// BeforeUpdate validates and sets up the model before update
func (rb *RelayBudget) BeforeUpdate() error {
	rb.UpdatedAt = time.Now()

	// Recalculate usage percent
	if rb.LimitMicroCents > 0 {
		rb.CurrentUsagePercent = float64(rb.CurrentUsageMicroCents) / float64(rb.LimitMicroCents) * 100.0
		rb.BudgetExceeded = rb.CurrentUsageMicroCents > rb.LimitMicroCents
	}

	rb.UpdateKeys()
	return nil
}
