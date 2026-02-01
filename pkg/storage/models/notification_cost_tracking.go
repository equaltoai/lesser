package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// NotificationCostTracking tracks costs for notification delivery operations
type NotificationCostTracking struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - notification cost tracking uses NOTIF_COST#{notification_id}#{timestamp} pattern
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// GSI1 for user queries - USER#{username}, COST#{timestamp}
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// GSI2 for delivery method queries - METHOD#{delivery_method}, TIMESTAMP#{timestamp}
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"`

	// GSI3 for daily aggregation - DAILY#{date}, COST#{timestamp}
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK" json:"gsi3_pk"`
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK" json:"gsi3_sk"`

	// Core tracking fields
	ID               string `theorydb:"attr:id" json:"id"`
	NotificationID   string `theorydb:"attr:notificationID" json:"notification_id"`
	UserID           string `theorydb:"attr:userID" json:"user_id"`
	Username         string `theorydb:"attr:username" json:"username"`
	DeliveryMethod   string `theorydb:"attr:deliveryMethod" json:"delivery_method"`     // push, websocket
	NotificationType string `theorydb:"attr:notificationType" json:"notification_type"` // mention, follow, favourite, etc.
	Channel          string `theorydb:"attr:channel" json:"channel"`                    // specific channel within method
	Success          bool   `theorydb:"attr:success" json:"success"`
	ErrorMessage     string `theorydb:"attr:errorMessage" json:"error_message,omitempty"`
	RetryCount       int    `theorydb:"attr:retryCount" json:"retry_count"`

	// Cost breakdown (all in micro-cents for precision)
	PushCostMicroCents      int64 `theorydb:"attr:pushCostMicroCents" json:"push_cost_micro_cents"`
	WebSocketCostMicroCents int64 `theorydb:"attr:webSocketCostMicroCents" json:"websocket_cost_micro_cents"`
	LambdaCostMicroCents    int64 `theorydb:"attr:lambdaCostMicroCents" json:"lambda_cost_micro_cents"`
	DynamoDBCostMicroCents  int64 `theorydb:"attr:dynamoDBCostMicroCents" json:"dynamodb_cost_micro_cents"`
	TotalCostMicroCents     int64 `theorydb:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`

	// Cost breakdown in dollars (calculated from micro-cents)
	PushCostDollars      float64 `theorydb:"attr:pushCostDollars" json:"push_cost_dollars"`
	WebSocketCostDollars float64 `theorydb:"attr:webSocketCostDollars" json:"websocket_cost_dollars"`
	LambdaCostDollars    float64 `theorydb:"attr:lambdaCostDollars" json:"lambda_cost_dollars"`
	DynamoDBCostDollars  float64 `theorydb:"attr:dynamoDBCostDollars" json:"dynamodb_cost_dollars"`
	TotalCostDollars     float64 `theorydb:"attr:totalCostDollars" json:"total_cost_dollars"`

	// Performance metrics
	ProcessingTimeMs int64 `theorydb:"attr:processingTimeMs" json:"processing_time_ms"`
	DeliveryTimeMs   int64 `theorydb:"attr:deliveryTimeMs" json:"delivery_time_ms"`
	TotalTimeMs      int64 `theorydb:"attr:totalTimeMs" json:"total_time_ms"`
	ResponseCode     int   `theorydb:"attr:responseCode" json:"response_code,omitempty"`
	ResponseSize     int64 `theorydb:"attr:responseSize" json:"response_size,omitempty"`

	// Context and metadata
	RequestID          string                 `theorydb:"attr:requestID" json:"request_id"`
	ServiceName        string                 `theorydb:"attr:serviceName" json:"service_name"` // notification-processor
	LambdaFunctionName string                 `theorydb:"attr:lambdaFunctionName" json:"lambda_function_name"`
	LambdaRequestID    string                 `theorydb:"attr:lambdaRequestID" json:"lambda_request_id"`
	Properties         map[string]interface{} `theorydb:"attr:properties" json:"properties,omitempty"` // Additional metadata
	Tags               map[string]string      `theorydb:"attr:tags" json:"tags,omitempty"`             // User-defined tags

	// Timestamps
	Timestamp time.Time `theorydb:"attr:timestamp" json:"timestamp"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing NotificationCostTracking.
func (NotificationCostTracking) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the notification cost tracking before creation
func (n *NotificationCostTracking) BeforeCreate() error {
	if err := common.ValidateRequiredParam("n.ID", n.ID); err != nil {
		n.ID = uuid.New().String()
	}

	now := time.Now()
	n.CreatedAt = now
	n.UpdatedAt = now

	if n.Timestamp.IsZero() {
		n.Timestamp = now
	}

	if err := n.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	n.CalculateDollarAmounts()

	return nil
}

// BeforeUpdate sets up the notification cost tracking before update
func (n *NotificationCostTracking) BeforeUpdate() error {
	n.UpdatedAt = time.Now()
	if err := n.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	n.CalculateDollarAmounts()
	return nil
}

// UpdateKeys updates all the key fields for DynamoDB
func (n *NotificationCostTracking) UpdateKeys() error {
	timestampStr := n.Timestamp.Format(common.CompactTimeFormat)
	dateStr := n.Timestamp.Format(common.CompactDateFormat)

	// Primary keys
	n.PK = fmt.Sprintf("NOTIF_COST#%s", n.NotificationID)
	n.SK = fmt.Sprintf("TS#%s#%s", timestampStr, n.ID)

	// GSI1 - User queries
	n.GSI1PK = fmt.Sprintf(KeyPatternUser, n.Username)
	n.GSI1SK = fmt.Sprintf("COST#%s", timestampStr)

	// GSI2 - Delivery method queries
	n.GSI2PK = fmt.Sprintf("METHOD#%s", n.DeliveryMethod)
	n.GSI2SK = fmt.Sprintf("TIMESTAMP#%s", timestampStr)

	// GSI3 - Daily aggregation
	n.GSI3PK = fmt.Sprintf("DAILY#%s", dateStr)
	n.GSI3SK = fmt.Sprintf("COST#%s", timestampStr)
	return nil
}

// GetPK returns the partition key
func (n *NotificationCostTracking) GetPK() string {
	return n.PK
}

// GetSK returns the sort key
func (n *NotificationCostTracking) GetSK() string {
	return n.SK
}

// CalculateDollarAmounts calculates dollar amounts from micro-cents
func (n *NotificationCostTracking) CalculateDollarAmounts() {
	const microCentsToDollars = 1_000_000.0

	n.PushCostDollars = float64(n.PushCostMicroCents) / microCentsToDollars
	n.WebSocketCostDollars = float64(n.WebSocketCostMicroCents) / microCentsToDollars
	n.LambdaCostDollars = float64(n.LambdaCostMicroCents) / microCentsToDollars
	n.DynamoDBCostDollars = float64(n.DynamoDBCostMicroCents) / microCentsToDollars
	n.TotalCostDollars = float64(n.TotalCostMicroCents) / microCentsToDollars
}

// AddCost adds cost for a specific delivery method
func (n *NotificationCostTracking) AddCost(method string, costMicroCents int64) {
	switch method {
	case "email":
		// Email is not supported by Lesser - ignore
		return
	case "sms":
		// SMS is not supported by Lesser - ignore
		return
	case "push":
		n.PushCostMicroCents += costMicroCents
	case "websocket":
		n.WebSocketCostMicroCents += costMicroCents
	case "lambda":
		n.LambdaCostMicroCents += costMicroCents
	case "dynamodb":
		n.DynamoDBCostMicroCents += costMicroCents
	}

	n.TotalCostMicroCents = n.PushCostMicroCents + n.WebSocketCostMicroCents +
		n.LambdaCostMicroCents + n.DynamoDBCostMicroCents

	n.CalculateDollarAmounts()
}

// SetError marks the tracking as failed with an error
func (n *NotificationCostTracking) SetError(errorMsg string) {
	n.Success = false
	n.ErrorMessage = errorMsg
}

// SetSuccess marks the tracking as successful
func (n *NotificationCostTracking) SetSuccess() {
	n.Success = true
	n.ErrorMessage = ""
}

// NotificationCostAggregation aggregates notification costs by period
type NotificationCostAggregation struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "NOTIF_AGG#{period}#{delivery_method}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "WINDOW#{windowStart}"

	// Aggregation details
	Period         string    `theorydb:"attr:period" json:"period"`                  // daily, hourly, weekly, monthly
	DeliveryMethod string    `theorydb:"attr:deliveryMethod" json:"delivery_method"` // push, websocket, all
	WindowStart    time.Time `theorydb:"attr:windowStart" json:"window_start"`
	WindowEnd      time.Time `theorydb:"attr:windowEnd" json:"window_end"`

	// Aggregate statistics
	TotalNotifications   int64 `theorydb:"attr:totalNotifications" json:"total_notifications"`
	SuccessfulDeliveries int64 `theorydb:"attr:successfulDeliveries" json:"successful_deliveries"`
	FailedDeliveries     int64 `theorydb:"attr:failedDeliveries" json:"failed_deliveries"`
	TotalRetries         int64 `theorydb:"attr:totalRetries" json:"total_retries"`

	// Cost aggregations (micro-cents)
	TotalPushCostMicroCents      int64 `theorydb:"attr:totalPushCostMicroCents" json:"total_push_cost_micro_cents"`
	TotalWebSocketCostMicroCents int64 `theorydb:"attr:totalWebSocketCostMicroCents" json:"total_websocket_cost_micro_cents"`
	TotalLambdaCostMicroCents    int64 `theorydb:"attr:totalLambdaCostMicroCents" json:"total_lambda_cost_micro_cents"`
	TotalDynamoDBCostMicroCents  int64 `theorydb:"attr:totalDynamoDBCostMicroCents" json:"total_dynamodb_cost_micro_cents"`
	TotalCostMicroCents          int64 `theorydb:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`

	// Cost aggregations (dollars)
	TotalPushCostDollars      float64 `theorydb:"attr:totalPushCostDollars" json:"total_push_cost_dollars"`
	TotalWebSocketCostDollars float64 `theorydb:"attr:totalWebSocketCostDollars" json:"total_websocket_cost_dollars"`
	TotalLambdaCostDollars    float64 `theorydb:"attr:totalLambdaCostDollars" json:"total_lambda_cost_dollars"`
	TotalDynamoDBCostDollars  float64 `theorydb:"attr:totalDynamoDBCostDollars" json:"total_dynamodb_cost_dollars"`
	TotalCostDollars          float64 `theorydb:"attr:totalCostDollars" json:"total_cost_dollars"`

	// Performance metrics
	AverageProcessingTimeMs float64 `theorydb:"attr:averageProcessingTimeMs" json:"average_processing_time_ms"`
	AverageDeliveryTimeMs   float64 `theorydb:"attr:averageDeliveryTimeMs" json:"average_delivery_time_ms"`
	AverageTotalTimeMs      float64 `theorydb:"attr:averageTotalTimeMs" json:"average_total_time_ms"`

	// Rate calculations
	SuccessRate     float64 `theorydb:"attr:successRate" json:"success_rate"`          // Percentage of successful deliveries
	RetryRate       float64 `theorydb:"attr:retryRate" json:"retry_rate"`              // Percentage requiring retries
	CostPerDelivery float64 `theorydb:"attr:costPerDelivery" json:"cost_per_delivery"` // Average cost per delivery attempt

	// Breakdown by notification type
	TypeBreakdown    map[string]*NotificationTypeCostStats    `theorydb:"attr:typeBreakdown" json:"type_breakdown,omitempty"`
	ChannelBreakdown map[string]*NotificationChannelCostStats `theorydb:"attr:channelBreakdown" json:"channel_breakdown,omitempty"`
	UserBreakdown    map[string]*NotificationUserCostStats    `theorydb:"attr:userBreakdown" json:"user_breakdown,omitempty"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing NotificationCostAggregation.
func (NotificationCostAggregation) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the aggregation before creation
func (n *NotificationCostAggregation) BeforeCreate() error {
	now := time.Now()
	n.CreatedAt = now
	n.UpdatedAt = now
	if err := n.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	n.CalculateDollarAmounts()
	n.CalculateRates()
	return nil
}

// BeforeUpdate sets up the aggregation before update
func (n *NotificationCostAggregation) BeforeUpdate() error {
	n.UpdatedAt = time.Now()
	if err := n.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	n.CalculateDollarAmounts()
	n.CalculateRates()
	return nil
}

// UpdateKeys updates the primary and sort keys
func (n *NotificationCostAggregation) UpdateKeys() error {
	n.PK = fmt.Sprintf("NOTIF_AGG#%s#%s", n.Period, n.DeliveryMethod)
	n.SK = fmt.Sprintf("WINDOW#%s", n.WindowStart.Format(time.RFC3339))
	return nil
}

// GetPK returns the partition key
func (n *NotificationCostAggregation) GetPK() string {
	return n.PK
}

// GetSK returns the sort key
func (n *NotificationCostAggregation) GetSK() string {
	return n.SK
}

// CalculateDollarAmounts calculates dollar amounts from micro-cents
func (n *NotificationCostAggregation) CalculateDollarAmounts() {
	const microCentsToDollars = 1_000_000.0

	n.TotalPushCostDollars = float64(n.TotalPushCostMicroCents) / microCentsToDollars
	n.TotalWebSocketCostDollars = float64(n.TotalWebSocketCostMicroCents) / microCentsToDollars
	n.TotalLambdaCostDollars = float64(n.TotalLambdaCostMicroCents) / microCentsToDollars
	n.TotalDynamoDBCostDollars = float64(n.TotalDynamoDBCostMicroCents) / microCentsToDollars
	n.TotalCostDollars = float64(n.TotalCostMicroCents) / microCentsToDollars
}

// CalculateRates calculates success rate, retry rate, and cost per delivery
func (n *NotificationCostAggregation) CalculateRates() {
	if n.TotalNotifications > 0 {
		n.SuccessRate = (float64(n.SuccessfulDeliveries) / float64(n.TotalNotifications)) * 100
		n.RetryRate = (float64(n.TotalRetries) / float64(n.TotalNotifications)) * 100
		n.CostPerDelivery = n.TotalCostDollars / float64(n.TotalNotifications)
	}
}

// NotificationTypeCostStats represents cost statistics for a specific notification type
type NotificationTypeCostStats struct {
	Type                  string  `json:"type"`
	Count                 int64   `json:"count"`
	SuccessfulDeliveries  int64   `json:"successful_deliveries"`
	FailedDeliveries      int64   `json:"failed_deliveries"`
	TotalCostMicroCents   int64   `json:"total_cost_micro_cents"`
	TotalCostDollars      float64 `json:"total_cost_dollars"`
	AverageCostMicroCents int64   `json:"average_cost_micro_cents"`
	AverageCostDollars    float64 `json:"average_cost_dollars"`
	SuccessRate           float64 `json:"success_rate"`
}

// TableName returns the DynamoDB table backing NotificationTypeCostStats.
func (NotificationTypeCostStats) TableName() string {
	return MainTableName
}

// NotificationChannelCostStats represents cost statistics for a specific delivery channel
type NotificationChannelCostStats struct {
	Channel               string  `json:"channel"`
	Count                 int64   `json:"count"`
	SuccessfulDeliveries  int64   `json:"successful_deliveries"`
	FailedDeliveries      int64   `json:"failed_deliveries"`
	TotalCostMicroCents   int64   `json:"total_cost_micro_cents"`
	TotalCostDollars      float64 `json:"total_cost_dollars"`
	AverageCostMicroCents int64   `json:"average_cost_micro_cents"`
	AverageCostDollars    float64 `json:"average_cost_dollars"`
	SuccessRate           float64 `json:"success_rate"`
	AverageDeliveryTimeMs float64 `json:"average_delivery_time_ms"`
}

// TableName returns the DynamoDB table backing NotificationChannelCostStats.
func (NotificationChannelCostStats) TableName() string {
	return MainTableName
}

// NotificationUserCostStats represents cost statistics for a specific user
type NotificationUserCostStats struct {
	Username              string  `json:"username"`
	Count                 int64   `json:"count"`
	SuccessfulDeliveries  int64   `json:"successful_deliveries"`
	FailedDeliveries      int64   `json:"failed_deliveries"`
	TotalCostMicroCents   int64   `json:"total_cost_micro_cents"`
	TotalCostDollars      float64 `json:"total_cost_dollars"`
	AverageCostMicroCents int64   `json:"average_cost_micro_cents"`
	AverageCostDollars    float64 `json:"average_cost_dollars"`
	SuccessRate           float64 `json:"success_rate"`
}

// TableName returns the DynamoDB table backing NotificationUserCostStats.
func (NotificationUserCostStats) TableName() string {
	return MainTableName
}

// NotificationBudget represents a budget limit for notification sending per user
type NotificationBudget struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "NOTIF_BUDGET#{username}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "PERIOD#{period}"

	// Budget details
	Username            string  `theorydb:"attr:username" json:"username"`
	Period              string  `theorydb:"attr:period" json:"period"`                             // daily, weekly, monthly
	LimitMicroCents     int64   `theorydb:"attr:limitMicroCents" json:"limit_micro_cents"`         // Budget limit in micro-cents
	LimitDollars        float64 `theorydb:"attr:limitDollars" json:"limit_dollars"`                // Budget limit in dollars
	SpentMicroCents     int64   `theorydb:"attr:spentMicroCents" json:"spent_micro_cents"`         // Amount spent in current period
	SpentDollars        float64 `theorydb:"attr:spentDollars" json:"spent_dollars"`                // Amount spent in dollars
	RemainingMicroCents int64   `theorydb:"attr:remainingMicroCents" json:"remaining_micro_cents"` // Remaining budget
	RemainingDollars    float64 `theorydb:"attr:remainingDollars" json:"remaining_dollars"`        // Remaining budget in dollars

	// Period tracking
	PeriodStart   time.Time `theorydb:"attr:periodStart" json:"period_start"`
	PeriodEnd     time.Time `theorydb:"attr:periodEnd" json:"period_end"`
	NextResetTime time.Time `theorydb:"attr:nextResetTime" json:"next_reset_time"`

	// Notification limits
	MaxNotificationsPerPeriod   int64 `theorydb:"attr:maxNotificationsPerPeriod" json:"max_notifications_per_period,omitempty"`
	NotificationsSentThisPeriod int64 `theorydb:"attr:notificationsSentThisPeriod" json:"notifications_sent_this_period"`

	// Enforcement settings
	Enabled                bool     `theorydb:"attr:enabled" json:"enabled"`                                           // Whether budget is actively enforced
	SendWarningAt          float64  `theorydb:"attr:sendWarningAt" json:"send_warning_at"`                             // Send warning at X% of budget
	BlockDeliveryAt        float64  `theorydb:"attr:blockDeliveryAt" json:"block_delivery_at"`                         // Block delivery at X% of budget
	AllowedDeliveryMethods []string `theorydb:"attr:allowedDeliveryMethods" json:"allowed_delivery_methods,omitempty"` // Restrict to specific methods

	// Status
	BudgetExceeded   bool      `theorydb:"attr:budgetExceeded" json:"budget_exceeded"`
	WarningsSent     int       `theorydb:"attr:warningsSent" json:"warnings_sent"`
	LastWarningTime  time.Time `theorydb:"attr:lastWarningTime" json:"last_warning_time,omitempty"`
	LastExceededTime time.Time `theorydb:"attr:lastExceededTime" json:"last_exceeded_time,omitempty"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing NotificationBudget.
func (NotificationBudget) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the budget before creation
func (n *NotificationBudget) BeforeCreate() error {
	now := time.Now()
	n.CreatedAt = now
	n.UpdatedAt = now
	if err := n.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	n.CalculateDollarAmounts()
	n.CalculateRemaining()
	return nil
}

// BeforeUpdate sets up the budget before update
func (n *NotificationBudget) BeforeUpdate() error {
	n.UpdatedAt = time.Now()
	if err := n.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	n.CalculateDollarAmounts()
	n.CalculateRemaining()
	return nil
}

// UpdateKeys updates the primary and sort keys
func (n *NotificationBudget) UpdateKeys() error {
	n.PK = fmt.Sprintf("NOTIF_BUDGET#%s", n.Username)
	n.SK = fmt.Sprintf("PERIOD#%s", n.Period)
	return nil
}

// GetPK returns the partition key
func (n *NotificationBudget) GetPK() string {
	return n.PK
}

// GetSK returns the sort key
func (n *NotificationBudget) GetSK() string {
	return n.SK
}

// CalculateDollarAmounts calculates dollar amounts from micro-cents
func (n *NotificationBudget) CalculateDollarAmounts() {
	const microCentsToDollars = 1_000_000.0

	n.LimitDollars = float64(n.LimitMicroCents) / microCentsToDollars
	n.SpentDollars = float64(n.SpentMicroCents) / microCentsToDollars
}

// CalculateRemaining calculates remaining budget
func (n *NotificationBudget) CalculateRemaining() {
	n.RemainingMicroCents = n.LimitMicroCents - n.SpentMicroCents
	if n.RemainingMicroCents < 0 {
		n.RemainingMicroCents = 0
	}

	const microCentsToDollars = 1_000_000.0
	n.RemainingDollars = float64(n.RemainingMicroCents) / microCentsToDollars
}

// AddSpending adds spending to the budget and checks limits
func (n *NotificationBudget) AddSpending(costMicroCents int64) bool {
	n.SpentMicroCents += costMicroCents
	n.NotificationsSentThisPeriod++

	n.CalculateDollarAmounts()
	n.CalculateRemaining()

	// Check if budget is exceeded
	if n.SpentMicroCents >= n.LimitMicroCents {
		n.BudgetExceeded = true
		n.LastExceededTime = time.Now()
		return false // Budget exceeded
	}

	return true // Within budget
}

// ShouldSendWarning checks if a warning should be sent
func (n *NotificationBudget) ShouldSendWarning() bool {
	if !n.Enabled || n.SendWarningAt <= 0 {
		return false
	}

	percentUsed := (float64(n.SpentMicroCents) / float64(n.LimitMicroCents)) * 100
	return percentUsed >= n.SendWarningAt && time.Since(n.LastWarningTime) > time.Hour
}

// ShouldBlockDelivery checks if delivery should be blocked
func (n *NotificationBudget) ShouldBlockDelivery() bool {
	if !n.Enabled {
		return false
	}

	if n.BudgetExceeded {
		return true
	}

	if n.BlockDeliveryAt > 0 {
		percentUsed := (float64(n.SpentMicroCents) / float64(n.LimitMicroCents)) * 100
		if percentUsed >= n.BlockDeliveryAt {
			return true
		}
	}

	// Check notification count limit
	if n.MaxNotificationsPerPeriod > 0 && n.NotificationsSentThisPeriod >= n.MaxNotificationsPerPeriod {
		return true
	}

	return false
}

// IsMethodAllowed checks if a delivery method is allowed
func (n *NotificationBudget) IsMethodAllowed(method string) bool {
	if err := common.ValidateSliceNotEmpty("n.AllowedDeliveryMethods", n.AllowedDeliveryMethods); err != nil {
		return true // No restrictions
	}

	for _, allowed := range n.AllowedDeliveryMethods {
		if allowed == method {
			return true
		}
	}
	return false
}

// ResetPeriod resets the budget for a new period
func (n *NotificationBudget) ResetPeriod(newPeriodStart, newPeriodEnd time.Time) {
	n.SpentMicroCents = 0
	n.SpentDollars = 0
	n.NotificationsSentThisPeriod = 0
	n.BudgetExceeded = false
	n.WarningsSent = 0
	n.PeriodStart = newPeriodStart
	n.PeriodEnd = newPeriodEnd
	n.CalculateRemaining()

	// Calculate next reset time
	switch n.Period {
	case PeriodDaily:
		n.NextResetTime = newPeriodEnd.AddDate(0, 0, 1)
	case PeriodWeekly:
		n.NextResetTime = newPeriodEnd.AddDate(0, 0, 7)
	case PeriodMonthly:
		n.NextResetTime = newPeriodEnd.AddDate(0, 1, 0)
	}
}

// NotificationCostTrackingBuilder helps build notification cost tracking records
type NotificationCostTrackingBuilder struct {
	tracking *NotificationCostTracking
}

// TableName returns the DynamoDB table backing NotificationCostTrackingBuilder.
func (NotificationCostTrackingBuilder) TableName() string {
	return MainTableName
}

// NewNotificationCostTrackingBuilder creates a new builder
func NewNotificationCostTrackingBuilder() *NotificationCostTrackingBuilder {
	return &NotificationCostTrackingBuilder{
		tracking: &NotificationCostTracking{
			Properties: make(map[string]interface{}),
			Tags:       make(map[string]string),
		},
	}
}

// WithNotification sets notification details
func (b *NotificationCostTrackingBuilder) WithNotification(notificationID, userID, username, notificationType string) *NotificationCostTrackingBuilder {
	b.tracking.NotificationID = notificationID
	b.tracking.UserID = userID
	b.tracking.Username = username
	b.tracking.NotificationType = notificationType
	return b
}

// WithDelivery sets delivery details
func (b *NotificationCostTrackingBuilder) WithDelivery(method, channel string, success bool, retryCount int) *NotificationCostTrackingBuilder {
	b.tracking.DeliveryMethod = method
	b.tracking.Channel = channel
	b.tracking.Success = success
	b.tracking.RetryCount = retryCount
	return b
}

// WithCosts sets cost details
func (b *NotificationCostTrackingBuilder) WithCosts(pushCost, websocketCost, lambdaCost, dynamodbCost int64) *NotificationCostTrackingBuilder {
	b.tracking.PushCostMicroCents = pushCost
	b.tracking.WebSocketCostMicroCents = websocketCost
	b.tracking.LambdaCostMicroCents = lambdaCost
	b.tracking.DynamoDBCostMicroCents = dynamodbCost
	b.tracking.TotalCostMicroCents = pushCost + websocketCost + lambdaCost + dynamodbCost
	return b
}

// WithPerformance sets performance metrics
func (b *NotificationCostTrackingBuilder) WithPerformance(processingTimeMs, deliveryTimeMs int64, responseCode int, responseSize int64) *NotificationCostTrackingBuilder {
	b.tracking.ProcessingTimeMs = processingTimeMs
	b.tracking.DeliveryTimeMs = deliveryTimeMs
	b.tracking.TotalTimeMs = processingTimeMs + deliveryTimeMs
	b.tracking.ResponseCode = responseCode
	b.tracking.ResponseSize = responseSize
	return b
}

// WithContext sets context information
func (b *NotificationCostTrackingBuilder) WithContext(requestID, serviceName, functionName, lambdaRequestID string) *NotificationCostTrackingBuilder {
	b.tracking.RequestID = requestID
	b.tracking.ServiceName = serviceName
	b.tracking.LambdaFunctionName = functionName
	b.tracking.LambdaRequestID = lambdaRequestID
	return b
}

// WithError sets error information
func (b *NotificationCostTrackingBuilder) WithError(errorMsg string) *NotificationCostTrackingBuilder {
	b.tracking.ErrorMessage = errorMsg
	b.tracking.Success = false
	return b
}

// WithProperty adds a custom property
func (b *NotificationCostTrackingBuilder) WithProperty(key string, value interface{}) *NotificationCostTrackingBuilder {
	b.tracking.Properties[key] = value
	return b
}

// WithTag adds a custom tag
func (b *NotificationCostTrackingBuilder) WithTag(key, value string) *NotificationCostTrackingBuilder {
	b.tracking.Tags[key] = value
	return b
}

// WithTimestamp sets the timestamp
func (b *NotificationCostTrackingBuilder) WithTimestamp(timestamp time.Time) *NotificationCostTrackingBuilder {
	b.tracking.Timestamp = timestamp
	return b
}

// Build returns the completed notification cost tracking record
func (b *NotificationCostTrackingBuilder) Build() *NotificationCostTracking {
	if b.tracking.Timestamp.IsZero() {
		b.tracking.Timestamp = time.Now()
	}
	return b.tracking
}

// Predefined cost constants (in micro-cents)
const (
	// Push notification costs (estimates)
	PushCostPerMessage = 5000 // $0.00005 per notification = 5 micro-cents

	// WebSocket costs (API Gateway + Lambda)
	WebSocketCostPerMessage = 1000 // $0.00001 per message = 1 micro-cent

	// Lambda costs (per invocation + duration)
	LambdaCostPerInvocation = 20   // $0.0000002 per invocation = 0.02 micro-cents (rounded to 0.2)
	LambdaCostPerGBSecond   = 1667 // $0.0000166667 per GB-second = 1.67 micro-cents

	// DynamoDB costs (on-demand pricing)
	DynamoDBReadCostPerRCU  = 25  // $0.25 per million RCUs = 0.25 micro-cents per RCU
	DynamoDBWriteCostPerWCU = 125 // $1.25 per million WCUs = 1.25 micro-cents per WCU
)

// CalculatePushCost calculates the cost of sending push notifications
func CalculatePushCost(messageCount int64) int64 {
	return messageCount * PushCostPerMessage
}

// CalculateWebSocketCost calculates the cost of sending WebSocket messages
func CalculateWebSocketCost(messageCount int64) int64 {
	return messageCount * WebSocketCostPerMessage
}

// CalculateEmailCost returns 0 since email is not supported by Lesser
func CalculateEmailCost(_ int64) int64 {
	// Email delivery is not supported by Lesser
	return 0
}

// CalculateSMSCost returns 0 since SMS is not supported by Lesser
func CalculateSMSCost(_ int64) int64 {
	// SMS delivery is not supported by Lesser
	return 0
}

// CalculateLambdaCost calculates the cost of Lambda execution
func CalculateLambdaCost(invocations int64, gbSeconds float64) int64 {
	invocationCost := invocations * LambdaCostPerInvocation
	durationCost := int64(gbSeconds * float64(LambdaCostPerGBSecond))
	return invocationCost + durationCost
}

// CalculateDynamoDBCost calculates the cost of DynamoDB operations
func CalculateDynamoDBCost(readCapacityUnits, writeCapacityUnits float64) int64 {
	readCost := int64(readCapacityUnits * float64(DynamoDBReadCostPerRCU))
	writeCost := int64(writeCapacityUnits * float64(DynamoDBWriteCostPerWCU))
	return readCost + writeCost
}
