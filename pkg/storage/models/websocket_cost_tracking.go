package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

const (
	wsUserKey = "WS_USER#%s"
)

// WebSocketCostRecord represents detailed cost tracking for WebSocket operations
type WebSocketCostRecord struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - using operation type and timestamp for optimal access patterns
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "WS_COST#{operation_type}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "ts#{timestamp}#{id}"

	// GSI1 - Connection-based queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "WS_CONN#{connection_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{timestamp}#{operation_type}#{id}"

	// GSI2 - User-based queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "WS_USER#{user_id}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{timestamp}#{operation_type}#{id}"

	// Core cost tracking data
	ID            string    `dynamorm:"attr:id" json:"id"`
	OperationType string    `dynamorm:"attr:operationType" json:"operation_type"` // connect, disconnect, message_in, message_out, idle_time
	ConnectionID  string    `dynamorm:"attr:connectionID" json:"connection_id"`   // API Gateway connection ID
	UserID        string    `dynamorm:"attr:userID" json:"user_id"`               // User associated with connection
	Username      string    `dynamorm:"attr:username" json:"username"`            // Username for easier queries
	Timestamp     time.Time `dynamorm:"attr:timestamp" json:"timestamp"`

	// Connection details
	ConnectionDurationMs int64 `dynamorm:"attr:connectionDurationMs" json:"connection_duration_ms,omitempty"` // For connection lifecycle tracking
	IdleTimeMs           int64 `dynamorm:"attr:idleTimeMs" json:"idle_time_ms,omitempty"`                     // Time connection was idle
	MessageCount         int   `dynamorm:"attr:messageCount" json:"message_count,omitempty"`                  // Number of messages (for message operations)
	MessageSizeBytes     int64 `dynamorm:"attr:messageSizeBytes" json:"message_size_bytes,omitempty"`         // Size of messages sent/received
	StreamCount          int   `dynamorm:"attr:streamCount" json:"stream_count,omitempty"`                    // Number of streams subscribed

	// AWS costs in microcents for precision
	// API Gateway WebSocket costs: $0.25 per million connection minutes, $1.00 per million messages
	APIGatewayConnectionCost int64 `dynamorm:"attr:apiGatewayConnectionCost" json:"api_gateway_connection_cost"` // Connection time cost in microcents
	APIGatewayMessageCost    int64 `dynamorm:"attr:apiGatewayMessageCost" json:"api_gateway_message_cost"`       // Message sending cost in microcents
	LambdaExecutionCost      int64 `dynamorm:"attr:lambdaExecutionCost" json:"lambda_execution_cost"`            // Lambda execution cost in microcents
	DynamoDBCost             int64 `dynamorm:"attr:dynamoDBCost" json:"dynamodb_cost"`                           // DynamoDB operations cost in microcents
	DataTransferCost         int64 `dynamorm:"attr:dataTransferCost" json:"data_transfer_cost"`                  // Data transfer cost in microcents
	TotalCostMicroCents      int64 `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`           // Total cost in microcents

	// Cost breakdown by category
	ConnectionMinuteCost  int64 `dynamorm:"attr:connectionMinuteCost" json:"connection_minute_cost"`   // Cost per minute of connection
	MessageProcessingCost int64 `dynamorm:"attr:messageProcessingCost" json:"message_processing_cost"` // Cost per message processed
	SubscriptionCost      int64 `dynamorm:"attr:subscriptionCost" json:"subscription_cost"`            // Cost for managing subscriptions

	// Performance metrics
	ProcessingTimeMs  int64   `dynamorm:"attr:processingTimeMs" json:"processing_time_ms"`             // Time to process the operation
	ResponseLatencyMs int64   `dynamorm:"attr:responseLatencyMs" json:"response_latency_ms,omitempty"` // Response latency for messages
	MemoryUsedMB      float64 `dynamorm:"attr:memoryUsedMB" json:"memory_used_mb,omitempty"`           // Lambda memory usage

	// Service information
	ServiceName     string `dynamorm:"attr:serviceName" json:"service_name"`         // streaming or stream-router
	RequestID       string `dynamorm:"attr:requestID" json:"request_id"`             // AWS Request ID
	FunctionName    string `dynamorm:"attr:functionName" json:"function_name"`       // Lambda function name
	FunctionVersion string `dynamorm:"attr:functionVersion" json:"function_version"` // Lambda function version

	// Connection context
	ClientIP         string `dynamorm:"attr:clientIP" json:"client_ip,omitempty"`       // Client IP address
	UserAgent        string `dynamorm:"attr:userAgent" json:"user_agent,omitempty"`     // User agent string
	ConnectionSource string `dynamorm:"attr:connectionSource" json:"connection_source"` // web, mobile, api
	AuthMethod       string `dynamorm:"attr:authMethod" json:"auth_method,omitempty"`   // oauth, bearer, anonymous

	// Stream information
	ActiveStreams []string `dynamorm:"attr:activeStreams" json:"active_streams,omitempty"` // Streams active during operation
	StreamTypes   []string `dynamorm:"attr:streamTypes" json:"stream_types,omitempty"`     // Types of streams (public, user, notification)

	// Additional metadata
	Tags       map[string]string      `dynamorm:"attr:tags" json:"tags,omitempty"`
	Properties map[string]interface{} `dynamorm:"attr:properties" json:"properties,omitempty"`

	// Estimated cost in dollars for easy display
	EstimatedCostDollars float64 `dynamorm:"attr:estimatedCostDollars" json:"estimated_cost_dollars"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL for automatic cleanup (30 days for detailed records)
	ExpiresAt int64 `dynamorm:"ttl,attr:ttl" json:"expires_at"` // Unix timestamp
}

// WebSocketCostBudget represents per-user WebSocket usage budgets
type WebSocketCostBudget struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "WS_BUDGET#{user_id}#{period}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "BUDGET#{period}"

	// GSI1 - User budget queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "WS_USER_BUDGET#{user_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{period}#{status}"

	// Budget configuration
	UserID           string    `dynamorm:"attr:userID" json:"user_id"`
	Username         string    `dynamorm:"attr:username" json:"username"`
	Period           string    `dynamorm:"attr:period" json:"period"`                       // daily, weekly, monthly
	BudgetMicroCents int64     `dynamorm:"attr:budgetMicroCents" json:"budget_micro_cents"` // Budget limit in microcents
	WindowStart      time.Time `dynamorm:"attr:windowStart" json:"window_start"`            // Budget period start
	WindowEnd        time.Time `dynamorm:"attr:windowEnd" json:"window_end"`                // Budget period end

	// Current usage tracking
	UsedMicroCents           int64 `dynamorm:"attr:usedMicroCents" json:"used_micro_cents"`                     // Currently used amount
	RemainingMicroCents      int64 `dynamorm:"attr:remainingMicroCents" json:"remaining_micro_cents"`           // Remaining budget
	ConnectionMinutesUsed    int64 `dynamorm:"attr:connectionMinutesUsed" json:"connection_minutes_used"`       // Total connection time used
	MessagesUsed             int64 `dynamorm:"attr:messagesUsed" json:"messages_used"`                          // Total messages sent/received
	ActiveConnections        int   `dynamorm:"attr:activeConnections" json:"active_connections"`                // Current active connections
	MaxConcurrentConnections int   `dynamorm:"attr:maxConcurrentConnections" json:"max_concurrent_connections"` // Max concurrent connections allowed

	// Budget status
	Status       string  `dynamorm:"attr:status" json:"status"`              // active, warning, exceeded, suspended
	UsagePercent float64 `dynamorm:"attr:usagePercent" json:"usage_percent"` // Percentage of budget used

	// Alerts and limits
	AlertThresholds []int     `dynamorm:"attr:alertThresholds" json:"alert_thresholds"`        // Alert at these usage percentages (e.g., [50, 75, 90])
	AlertsSent      []string  `dynamorm:"attr:alertsSent" json:"alerts_sent,omitempty"`        // Track which alerts have been sent
	SuspendAt       int       `dynamorm:"attr:suspendAt" json:"suspend_at"`                    // Suspend connections at this usage percentage
	LastAlertSent   time.Time `dynamorm:"attr:lastAlertSent" json:"last_alert_sent,omitempty"` // Last time an alert was sent

	// Rate limiting
	ConnectionsPerMinute int `dynamorm:"attr:connectionsPerMinute" json:"connections_per_minute"` // Max new connections per minute
	MessagesPerMinute    int `dynamorm:"attr:messagesPerMinute" json:"messages_per_minute"`       // Max messages per minute

	// Billing tier
	BillingTier string `dynamorm:"attr:billingTier" json:"billing_tier"` // free, basic, premium, enterprise

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL - refresh budgets periodically
	ExpiresAt int64 `dynamorm:"ttl,attr:ttl" json:"expires_at"` // Unix timestamp
}

// WebSocketCostAggregation represents pre-computed WebSocket cost aggregations
type WebSocketCostAggregation struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "WS_AGG#{period}#{operation_type}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "window#{windowStart}"

	// GSI1 - User aggregation queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "WS_USER_AGG#{user_id}#{period}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{timestamp}#{operation_type}"

	// Aggregation details
	Period        string    `dynamorm:"attr:period" json:"period"`                // minute, hour, day, week, month
	OperationType string    `dynamorm:"attr:operationType" json:"operation_type"` // Same as WebSocketCostRecord.OperationType
	UserID        string    `dynamorm:"attr:userID" json:"user_id,omitempty"`     // Specific user or empty for global
	WindowStart   time.Time `dynamorm:"attr:windowStart" json:"window_start"`     // Start of aggregation window
	WindowEnd     time.Time `dynamorm:"attr:windowEnd" json:"window_end"`         // End of aggregation window

	// Connection metrics
	TotalConnections          int64   `dynamorm:"attr:totalConnections" json:"total_connections"`                    // Total connections in period
	UniqueUsers               int64   `dynamorm:"attr:uniqueUsers" json:"unique_users"`                              // Unique users connected
	AverageConnectionDuration float64 `dynamorm:"attr:averageConnectionDuration" json:"average_connection_duration"` // Average connection time in minutes
	MaxConcurrentConnections  int     `dynamorm:"attr:maxConcurrentConnections" json:"max_concurrent_connections"`   // Peak concurrent connections
	TotalConnectionMinutes    int64   `dynamorm:"attr:totalConnectionMinutes" json:"total_connection_minutes"`       // Total connection time

	// Message metrics
	TotalMessagesIn         int64   `dynamorm:"attr:totalMessagesIn" json:"total_messages_in"`                  // Messages received from clients
	TotalMessagesOut        int64   `dynamorm:"attr:totalMessagesOut" json:"total_messages_out"`                // Messages sent to clients
	TotalMessageBytes       int64   `dynamorm:"attr:totalMessageBytes" json:"total_message_bytes"`              // Total message data transferred
	AverageMessageSize      float64 `dynamorm:"attr:averageMessageSize" json:"average_message_size"`            // Average message size
	MessageThroughputPerSec float64 `dynamorm:"attr:messageThroughputPerSec" json:"message_throughput_per_sec"` // Messages per second

	// Stream metrics
	TotalStreamSubscriptions int64            `dynamorm:"attr:totalStreamSubscriptions" json:"total_stream_subscriptions"` // Stream subscriptions created
	UniqueStreamsUsed        int64            `dynamorm:"attr:uniqueStreamsUsed" json:"unique_streams_used"`               // Number of unique streams
	StreamPopularity         map[string]int64 `dynamorm:"attr:streamPopularity" json:"stream_popularity"`                  // Stream name -> subscription count
	StreamTypeBreakdown      map[string]int64 `dynamorm:"attr:streamTypeBreakdown" json:"stream_type_breakdown"`           // Stream type -> count

	// Cost aggregations (in microcents)
	TotalAPIGatewayConnectionCost int64   `dynamorm:"attr:totalAPIGatewayConnectionCost" json:"total_api_gateway_connection_cost"`
	TotalAPIGatewayMessageCost    int64   `dynamorm:"attr:totalAPIGatewayMessageCost" json:"total_api_gateway_message_cost"`
	TotalLambdaExecutionCost      int64   `dynamorm:"attr:totalLambdaExecutionCost" json:"total_lambda_execution_cost"`
	TotalDynamoDBCost             int64   `dynamorm:"attr:totalDynamoDBCost" json:"total_dynamodb_cost"`
	TotalDataTransferCost         int64   `dynamorm:"attr:totalDataTransferCost" json:"total_data_transfer_cost"`
	TotalCostMicroCents           int64   `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`
	TotalCostDollars              float64 `dynamorm:"attr:totalCostDollars" json:"total_cost_dollars"`

	// Performance metrics
	AverageProcessingTime  float64 `dynamorm:"attr:averageProcessingTime" json:"average_processing_time"`   // Average processing time in ms
	AverageResponseLatency float64 `dynamorm:"attr:averageResponseLatency" json:"average_response_latency"` // Average response latency in ms
	AverageMemoryUsage     float64 `dynamorm:"attr:averageMemoryUsage" json:"average_memory_usage"`         // Average memory usage in MB

	// Cost efficiency metrics
	CostPerConnection float64 `dynamorm:"attr:costPerConnection" json:"cost_per_connection"` // Average cost per connection
	CostPerMessage    float64 `dynamorm:"attr:costPerMessage" json:"cost_per_message"`       // Average cost per message
	CostPerMinute     float64 `dynamorm:"attr:costPerMinute" json:"cost_per_minute"`         // Average cost per connection minute
	CostPerUser       float64 `dynamorm:"attr:costPerUser" json:"cost_per_user"`             // Average cost per unique user

	// Error and reliability metrics
	FailedConnections       int64   `dynamorm:"attr:failedConnections" json:"failed_connections"`              // Connections that failed to establish
	DroppedConnections      int64   `dynamorm:"attr:droppedConnections" json:"dropped_connections"`            // Connections dropped unexpectedly
	MessageDeliveryFailures int64   `dynamorm:"attr:messageDeliveryFailures" json:"message_delivery_failures"` // Failed message deliveries
	ErrorRate               float64 `dynamorm:"attr:errorRate" json:"error_rate"`                              // Percentage of operations that failed

	// User behavior metrics
	UserEngagementScore  float64            `dynamorm:"attr:userEngagementScore" json:"user_engagement_score"`   // Engagement score based on activity
	TopUsers             []string           `dynamorm:"attr:topUsers" json:"top_users"`                          // Most active users by cost/usage
	UserBehaviorPatterns map[string]float64 `dynamorm:"attr:userBehaviorPatterns" json:"user_behavior_patterns"` // Usage patterns by behavior type

	// Cost breakdown by user tier
	CostByTier map[string]*WebSocketTierCostStats `dynamorm:"attr:costByTier" json:"cost_by_tier,omitempty"`

	// Percentiles for cost and performance distribution
	CostPercentiles               map[string]float64 `dynamorm:"attr:costPercentiles" json:"cost_percentiles,omitempty"`                              // p50, p90, p95, p99
	LatencyPercentiles            map[string]float64 `dynamorm:"attr:latencyPercentiles" json:"latency_percentiles,omitempty"`                        // p50, p90, p95, p99
	ConnectionDurationPercentiles map[string]float64 `dynamorm:"attr:connectionDurationPercentiles" json:"connection_duration_percentiles,omitempty"` // p50, p90, p95, p99

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL (longer for aggregated data)
	ExpiresAt int64 `dynamorm:"ttl,attr:ttl" json:"expires_at"`
}

// WebSocketTierCostStats represents cost statistics for a billing tier
type WebSocketTierCostStats struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	TierName            string  `dynamorm:"attr:tierName" json:"tier_name"`
	UserCount           int64   `dynamorm:"attr:userCount" json:"user_count"`
	TotalCostMicroCents int64   `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`
	TotalCostDollars    float64 `dynamorm:"attr:totalCostDollars" json:"total_cost_dollars"`
	AverageCostPerUser  float64 `dynamorm:"attr:averageCostPerUser" json:"average_cost_per_user"`
	ConnectionMinutes   int64   `dynamorm:"attr:connectionMinutes" json:"connection_minutes"`
	MessageCount        int64   `dynamorm:"attr:messageCount" json:"message_count"`
}

// TableName returns the DynamoDB table backing WebSocketCostRecord.
func (WebSocketCostRecord) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing WebSocketCostBudget.
func (WebSocketCostBudget) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing WebSocketCostAggregation.
func (WebSocketCostAggregation) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing WebSocketTierCostStats.
func (WebSocketTierCostStats) TableName() string {
	return MainTableName
}

// GetPK returns the partition key for BaseModel interface
func (w *WebSocketCostRecord) GetPK() string {
	return w.PK
}

// GetSK returns the sort key for BaseModel interface
func (w *WebSocketCostRecord) GetSK() string {
	return w.SK
}

// UpdateKeys sets up all keys (PK, SK, GSI keys) for the WebSocketCostRecord
func (w *WebSocketCostRecord) UpdateKeys() error {
	// Generate ID if not provided
	if err := common.ValidateRequiredParam("w.ID", w.ID); err != nil {
		w.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if w.Timestamp.IsZero() {
		w.Timestamp = time.Now()
	}

	// Set up primary key
	w.PK = fmt.Sprintf("WS_COST#%s", w.OperationType)
	timestamp := w.Timestamp.Format("20060102150405")
	w.SK = fmt.Sprintf("ts#%s#%s", timestamp, w.ID)

	// Set up GSI keys
	w.setupGSIKeys()

	return nil
}

// BeforeCreate sets up the WebSocketCostRecord model before creation
func (w *WebSocketCostRecord) BeforeCreate() error {
	now := time.Now()
	w.CreatedAt = now
	w.UpdatedAt = now

	// Set timestamp if not provided
	if w.Timestamp.IsZero() {
		w.Timestamp = now
	}

	// Calculate estimated cost in dollars
	w.EstimatedCostDollars = float64(w.TotalCostMicroCents) / 1_000_000.0

	// Set TTL (30 days for detailed records)
	w.ExpiresAt = now.Add(30 * 24 * time.Hour).Unix()

	// Update all keys
	if err := w.UpdateKeys(); err != nil {
		return err
	}

	return w.Validate()
}

// BeforeUpdate sets up the WebSocketCostRecord model before update
func (w *WebSocketCostRecord) BeforeUpdate() error {
	w.UpdatedAt = time.Now()

	// Recalculate estimated cost
	w.EstimatedCostDollars = float64(w.TotalCostMicroCents) / 1_000_000.0

	// Update GSI keys in case indexed fields changed
	w.setupGSIKeys()

	return w.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (w *WebSocketCostRecord) setupGSIKeys() {
	timestampStr := w.Timestamp.Format(time.RFC3339)

	// GSI1 - Connection queries
	if w.ConnectionID != "" {
		w.GSI1PK = fmt.Sprintf("WS_CONN#%s", w.ConnectionID)
		w.GSI1SK = fmt.Sprintf("%s#%s#%s", timestampStr, w.OperationType, w.ID)
	}

	// GSI2 - User queries
	if w.UserID != "" {
		w.GSI2PK = fmt.Sprintf(wsUserKey, w.UserID)
		w.GSI2SK = fmt.Sprintf("%s#%s#%s", timestampStr, w.OperationType, w.ID)
	}
}

// Validate performs validation on the WebSocketCostRecord
func (w *WebSocketCostRecord) Validate() error {
	if err := common.ValidateRequiredParam("ID", w.ID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("OperationType", w.OperationType); err != nil {
		return err
	}
	if !isValidWebSocketOperationType(w.OperationType) {
		return fmt.Errorf("%w: %s", ErrInvalidWebSocketOperationType, w.OperationType)
	}
	if err := common.ValidateRequiredParam("ConnectionID", w.ConnectionID); err != nil {
		return err
	}

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (w *WebSocketCostBudget) GetPK() string {
	return w.PK
}

// GetSK returns the sort key for BaseModel interface
func (w *WebSocketCostBudget) GetSK() string {
	return w.SK
}

// UpdateKeys sets up all keys (PK, SK, GSI keys) for the WebSocketCostBudget
func (w *WebSocketCostBudget) UpdateKeys() error {
	// Set up primary key
	w.PK = fmt.Sprintf("WS_BUDGET#%s#%s", w.UserID, w.Period)
	w.SK = fmt.Sprintf("BUDGET#%s", w.Period)

	// Set up GSI keys
	w.GSI1PK = fmt.Sprintf("WS_USER_BUDGET#%s", w.UserID)
	w.GSI1SK = fmt.Sprintf("%s#%s", w.Period, w.Status)

	return nil
}

// BeforeCreate for WebSocketCostBudget
func (w *WebSocketCostBudget) BeforeCreate() error {
	now := time.Now()
	w.CreatedAt = now
	w.UpdatedAt = now

	// Calculate remaining budget
	w.RemainingMicroCents = w.BudgetMicroCents - w.UsedMicroCents
	if w.RemainingMicroCents < 0 {
		w.RemainingMicroCents = 0
	}

	// Calculate usage percentage
	if w.BudgetMicroCents > 0 {
		w.UsagePercent = (float64(w.UsedMicroCents) / float64(w.BudgetMicroCents)) * 100
	}

	// Determine status based on usage
	w.updateStatus()

	// Set TTL based on period
	ttlDuration := 24 * time.Hour // Default to 1 day
	switch w.Period {
	case "daily":
		ttlDuration = 2 * 24 * time.Hour // Keep for 2 days
	case "weekly":
		ttlDuration = 14 * 24 * time.Hour // Keep for 2 weeks
	case "monthly":
		ttlDuration = 62 * 24 * time.Hour // Keep for ~2 months
	}
	w.ExpiresAt = now.Add(ttlDuration).Unix()

	// Update all keys
	if err := w.UpdateKeys(); err != nil {
		return err
	}

	return w.Validate()
}

// BeforeUpdate for WebSocketCostBudget
func (w *WebSocketCostBudget) BeforeUpdate() error {
	w.UpdatedAt = time.Now()

	// Recalculate remaining budget
	w.RemainingMicroCents = w.BudgetMicroCents - w.UsedMicroCents
	if w.RemainingMicroCents < 0 {
		w.RemainingMicroCents = 0
	}

	// Recalculate usage percentage
	if w.BudgetMicroCents > 0 {
		w.UsagePercent = (float64(w.UsedMicroCents) / float64(w.BudgetMicroCents)) * 100
	}

	// Update status
	w.updateStatus()

	// Update keys (including GSI keys)
	if err := w.UpdateKeys(); err != nil {
		return err
	}

	return w.Validate()
}

// updateStatus updates the budget status based on current usage
func (w *WebSocketCostBudget) updateStatus() {
	if w.UsagePercent >= 100 {
		w.Status = "exceeded"
	} else if w.SuspendAt > 0 && w.UsagePercent >= float64(w.SuspendAt) {
		w.Status = "suspended"
	} else if w.UsagePercent >= 90 {
		w.Status = "warning"
	} else {
		w.Status = "active"
	}
}

// Validate for WebSocketCostBudget
func (w *WebSocketCostBudget) Validate() error {
	if err := common.ValidateRequiredParam("UserID", w.UserID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("period", w.Period); err != nil {
		return err
	}
	if !isValidWebSocketPeriod(w.Period) {
		return fmt.Errorf("%w: %s", ErrInvalidWebSocketPeriod, w.Period)
	}
	if w.BudgetMicroCents < 0 {
		return ErrBudgetMicroCentsNegative
	}
	if w.WindowStart.IsZero() {
		return ErrWebSocketWindowStartRequired
	}
	if w.WindowEnd.IsZero() {
		return ErrWebSocketWindowEndRequired
	}
	if w.WindowEnd.Before(w.WindowStart) {
		return ErrWebSocketWindowEndBeforeStart
	}

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (w *WebSocketCostAggregation) GetPK() string {
	return w.PK
}

// GetSK returns the sort key for BaseModel interface
func (w *WebSocketCostAggregation) GetSK() string {
	return w.SK
}

// UpdateKeys sets up all keys (PK, SK, GSI keys) for the WebSocketCostAggregation
func (w *WebSocketCostAggregation) UpdateKeys() error {
	// Set up primary key
	w.PK = fmt.Sprintf("WS_AGG#%s#%s", w.Period, w.OperationType)
	w.SK = fmt.Sprintf("window#%s", w.WindowStart.Format(time.RFC3339))

	// Set up GSI keys for user-specific aggregations
	if w.UserID != "" {
		w.GSI1PK = fmt.Sprintf("WS_USER_AGG#%s#%s", w.UserID, w.Period)
		w.GSI1SK = fmt.Sprintf("%s#%s", w.WindowStart.Format(time.RFC3339), w.OperationType)
	}

	return nil
}

// BeforeCreate for WebSocketCostAggregation
func (w *WebSocketCostAggregation) BeforeCreate() error {
	now := time.Now()
	w.CreatedAt = now
	w.UpdatedAt = now

	// Calculate total cost in dollars
	w.TotalCostDollars = float64(w.TotalCostMicroCents) / 1_000_000.0

	// Calculate efficiency metrics
	if w.TotalConnections > 0 {
		w.CostPerConnection = w.TotalCostDollars / float64(w.TotalConnections)
	}

	totalMessages := w.TotalMessagesIn + w.TotalMessagesOut
	if totalMessages > 0 {
		w.CostPerMessage = w.TotalCostDollars / float64(totalMessages)
		w.AverageMessageSize = float64(w.TotalMessageBytes) / float64(totalMessages)
	}

	if w.TotalConnectionMinutes > 0 {
		w.CostPerMinute = w.TotalCostDollars / float64(w.TotalConnectionMinutes)
	}

	if w.UniqueUsers > 0 {
		w.CostPerUser = w.TotalCostDollars / float64(w.UniqueUsers)
	}

	// Calculate error rate
	totalOperations := w.TotalConnections + totalMessages
	if totalOperations > 0 {
		failedOperations := w.FailedConnections + w.DroppedConnections + w.MessageDeliveryFailures
		w.ErrorRate = (float64(failedOperations) / float64(totalOperations)) * 100
	}

	// Set TTL (keep aggregated data longer)
	ttlDays := 90
	if w.Period == "month" {
		ttlDays = 365 // Keep monthly data for a year
	}
	w.ExpiresAt = now.Add(time.Duration(ttlDays) * 24 * time.Hour).Unix()

	// Update all keys
	if err := w.UpdateKeys(); err != nil {
		return err
	}

	return w.Validate()
}

// BeforeUpdate for WebSocketCostAggregation
func (w *WebSocketCostAggregation) BeforeUpdate() error {
	w.UpdatedAt = time.Now()

	// Recalculate totals and metrics (same as BeforeCreate)
	w.TotalCostDollars = float64(w.TotalCostMicroCents) / 1_000_000.0

	if w.TotalConnections > 0 {
		w.CostPerConnection = w.TotalCostDollars / float64(w.TotalConnections)
	}

	totalMessages := w.TotalMessagesIn + w.TotalMessagesOut
	if totalMessages > 0 {
		w.CostPerMessage = w.TotalCostDollars / float64(totalMessages)
		w.AverageMessageSize = float64(w.TotalMessageBytes) / float64(totalMessages)
	}

	if w.TotalConnectionMinutes > 0 {
		w.CostPerMinute = w.TotalCostDollars / float64(w.TotalConnectionMinutes)
	}

	if w.UniqueUsers > 0 {
		w.CostPerUser = w.TotalCostDollars / float64(w.UniqueUsers)
	}

	// Recalculate error rate
	totalOperations := w.TotalConnections + totalMessages
	if totalOperations > 0 {
		failedOperations := w.FailedConnections + w.DroppedConnections + w.MessageDeliveryFailures
		w.ErrorRate = (float64(failedOperations) / float64(totalOperations)) * 100
	}

	return w.Validate()
}

// Validate for WebSocketCostAggregation
func (w *WebSocketCostAggregation) Validate() error {
	if err := common.ValidateRequiredParam("OperationType", w.OperationType); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("period", w.Period); err != nil {
		return err
	}
	if w.WindowStart.IsZero() {
		return ErrWebSocketWindowStartRequired
	}
	if w.WindowEnd.IsZero() {
		return ErrWebSocketWindowEndRequired
	}
	if w.WindowEnd.Before(w.WindowStart) {
		return ErrWebSocketWindowEndBeforeStart
	}

	return nil
}

// AddTag adds a tag to the cost tracking
func (w *WebSocketCostRecord) AddTag(key, value string) {
	if w.Tags == nil {
		w.Tags = make(map[string]string)
	}
	w.Tags[key] = value
}

// SetProperty sets a custom property
func (w *WebSocketCostRecord) SetProperty(key string, value interface{}) {
	if w.Properties == nil {
		w.Properties = make(map[string]interface{})
	}
	w.Properties[key] = value
}

// GetProperty gets a custom property
func (w *WebSocketCostRecord) GetProperty(key string) (interface{}, bool) {
	if w.Properties == nil {
		return nil, false
	}
	value, exists := w.Properties[key]
	return value, exists
}

// isValidWebSocketOperationType checks if the WebSocket operation type is valid
func isValidWebSocketOperationType(opType string) bool {
	validTypes := map[string]bool{
		"connect":     true,
		"disconnect":  true,
		"message_in":  true, // Message received from client
		"message_out": true, // Message sent to client
		"subscribe":   true, // Stream subscription
		"unsubscribe": true, // Stream unsubscription
		"idle_time":   true, // Connection idle time tracking
		"ping":        true, // Ping/pong messages
		"error":       true, // Error handling
	}
	return validTypes[opType]
}

// WebSocket Cost Constants (in microcents for precision)
const (
	// API Gateway WebSocket pricing (as of 2024)
	// Connection minutes: $0.25 per million minutes = 0.00000025 per minute = 0.25 microcents per minute
	APIGatewayConnectionCostPerMinute = 25 // microcents per 100 minutes (for precision)

	// Messages: $1.00 per million messages = 0.000001 per message = 1 microcent per message
	APIGatewayMessageCostPerMessage = 1 // microcent per message

	// Lambda costs (example: $0.0000166667 per GB-second)
	// For 512MB Lambda: $0.0000083334 per invocation-second = 8.3334 microcents per second
	LambdaCostPerSecond512MB = 8334 // microcents per 1000 seconds (for precision)

	// Data transfer: $0.09 per GB = 90 microcents per MB
	DataTransferCostPerMB = 90 // microcents per MB
)

// CalculateWebSocketCosts calculates costs for WebSocket operations
func CalculateWebSocketCosts(operationType string, connectionMinutes, messageCount int64, dataMB float64, lambdaDurationMs int64) *WebSocketCostBreakdown {
	breakdown := &WebSocketCostBreakdown{
		OperationType: operationType,
	}

	// API Gateway connection cost (charged per minute)
	if connectionMinutes > 0 {
		breakdown.APIGatewayConnectionCost = (connectionMinutes * APIGatewayConnectionCostPerMinute) / 100
	}

	// API Gateway message cost (charged per message)
	if messageCount > 0 {
		breakdown.APIGatewayMessageCost = messageCount * APIGatewayMessageCostPerMessage
	}

	// Lambda execution cost (512MB function)
	if lambdaDurationMs > 0 {
		durationSeconds := float64(lambdaDurationMs) / 1000.0
		breakdown.LambdaExecutionCost = int64((durationSeconds * float64(LambdaCostPerSecond512MB)) / 1000.0)
	}

	// Data transfer cost
	if dataMB > 0 {
		breakdown.DataTransferCost = int64(dataMB * float64(DataTransferCostPerMB))
	}

	// Total cost
	breakdown.TotalCostMicroCents = breakdown.APIGatewayConnectionCost +
		breakdown.APIGatewayMessageCost +
		breakdown.LambdaExecutionCost +
		breakdown.DataTransferCost

	return breakdown
}

// WebSocketCostBreakdown represents a cost calculation result
type WebSocketCostBreakdown struct {
	OperationType            string
	APIGatewayConnectionCost int64
	APIGatewayMessageCost    int64
	LambdaExecutionCost      int64
	DataTransferCost         int64
	TotalCostMicroCents      int64
}

// TableName returns the DynamoDB table backing WebSocketCostBreakdown.
func (WebSocketCostBreakdown) TableName() string {
	return MainTableName
}

// WebSocketCostRecordBuilder helps create WebSocket cost tracking records
type WebSocketCostRecordBuilder struct {
	record *WebSocketCostRecord
}

// TableName returns the DynamoDB table backing WebSocketCostRecordBuilder.
func (WebSocketCostRecordBuilder) TableName() string {
	return MainTableName
}

// NewWebSocketCostRecordBuilder creates a new WebSocket cost tracking builder
func NewWebSocketCostRecordBuilder() *WebSocketCostRecordBuilder {
	return &WebSocketCostRecordBuilder{
		record: &WebSocketCostRecord{
			Tags:       make(map[string]string),
			Properties: make(map[string]interface{}),
		},
	}
}

// ForOperation sets the operation type
func (b *WebSocketCostRecordBuilder) ForOperation(operationType string) *WebSocketCostRecordBuilder {
	b.record.OperationType = operationType
	return b
}

// WithConnection sets the connection details
func (b *WebSocketCostRecordBuilder) WithConnection(connectionID, userID, username string) *WebSocketCostRecordBuilder {
	b.record.ConnectionID = connectionID
	b.record.UserID = userID
	b.record.Username = username
	return b
}

// WithDuration sets the connection duration
func (b *WebSocketCostRecordBuilder) WithDuration(durationMs int64) *WebSocketCostRecordBuilder {
	b.record.ConnectionDurationMs = durationMs
	return b
}

// WithMessages sets the message details
func (b *WebSocketCostRecordBuilder) WithMessages(count int, sizeBytes int64) *WebSocketCostRecordBuilder {
	b.record.MessageCount = count
	b.record.MessageSizeBytes = sizeBytes
	return b
}

// WithCosts sets the cost breakdown
func (b *WebSocketCostRecordBuilder) WithCosts(breakdown *WebSocketCostBreakdown) *WebSocketCostRecordBuilder {
	b.record.APIGatewayConnectionCost = breakdown.APIGatewayConnectionCost
	b.record.APIGatewayMessageCost = breakdown.APIGatewayMessageCost
	b.record.LambdaExecutionCost = breakdown.LambdaExecutionCost
	b.record.DataTransferCost = breakdown.DataTransferCost
	b.record.TotalCostMicroCents = breakdown.TotalCostMicroCents
	return b
}

// WithService sets the service information
func (b *WebSocketCostRecordBuilder) WithService(serviceName, functionName, requestID string) *WebSocketCostRecordBuilder {
	b.record.ServiceName = serviceName
	b.record.FunctionName = functionName
	b.record.RequestID = requestID
	return b
}

// WithStreams sets the stream information
func (b *WebSocketCostRecordBuilder) WithStreams(activeStreams, streamTypes []string) *WebSocketCostRecordBuilder {
	b.record.ActiveStreams = activeStreams
	b.record.StreamTypes = streamTypes
	b.record.StreamCount = len(activeStreams)
	return b
}

// WithPerformance sets the performance metrics
func (b *WebSocketCostRecordBuilder) WithPerformance(processingTimeMs, responseLatencyMs int64, memoryMB float64) *WebSocketCostRecordBuilder {
	b.record.ProcessingTimeMs = processingTimeMs
	b.record.ResponseLatencyMs = responseLatencyMs
	b.record.MemoryUsedMB = memoryMB
	return b
}

// WithClient sets the client information
func (b *WebSocketCostRecordBuilder) WithClient(clientIP, userAgent, source, authMethod string) *WebSocketCostRecordBuilder {
	b.record.ClientIP = clientIP
	b.record.UserAgent = userAgent
	b.record.ConnectionSource = source
	b.record.AuthMethod = authMethod
	return b
}

// WithTag adds a tag
func (b *WebSocketCostRecordBuilder) WithTag(key, value string) *WebSocketCostRecordBuilder {
	b.record.AddTag(key, value)
	return b
}

// WithProperty adds a property
func (b *WebSocketCostRecordBuilder) WithProperty(key string, value interface{}) *WebSocketCostRecordBuilder {
	b.record.SetProperty(key, value)
	return b
}

// Build creates the WebSocket cost tracking record
func (b *WebSocketCostRecordBuilder) Build() *WebSocketCostRecord {
	return b.record
}

// Helper function to check if a period is valid for WebSocket tracking
func isValidWebSocketPeriod(period string) bool {
	validPeriods := map[string]bool{
		"minute":  true,
		"hour":    true,
		"day":     true,
		"week":    true,
		"monthly": true,
		"daily":   true,
		"weekly":  true,
	}
	return validPeriods[period]
}
