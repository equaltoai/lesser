package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// WebSocketCostTracker handles cost tracking for WebSocket operations
type WebSocketCostTracker struct {
	costRepo     *WebSocketCostRepository
	logger       *zap.Logger
	serviceName  string
	functionName string
}

// NewWebSocketCostTracker creates a new WebSocket cost tracker
func NewWebSocketCostTracker(costRepo *WebSocketCostRepository, logger *zap.Logger) *WebSocketCostTracker {
	return &WebSocketCostTracker{
		costRepo:     costRepo,
		logger:       logger,
		serviceName:  getServiceName(),
		functionName: os.Getenv("AWS_LAMBDA_FUNCTION_NAME"),
	}
}

// WebSocketOperationContext holds context for a WebSocket operation
type WebSocketOperationContext struct {
	ConnectionID     string
	UserID           string
	Username         string
	OperationType    string
	StartTime        time.Time
	RequestID        string
	ClientIP         string
	UserAgent        string
	ConnectionSource string
	AuthMethod       string
	ActiveStreams    []string
	StreamTypes      []string
}

// WebSocketOperationResult holds the result of a WebSocket operation
type WebSocketOperationResult struct {
	Success              bool
	ProcessingTimeMs     int64
	ResponseLatencyMs    int64
	MessageCount         int
	MessageSizeBytes     int64
	ConnectionDurationMs int64
	IdleTimeMs           int64
	MemoryUsedMB         float64
	Error                error
}

// TrackWebSocketOperation tracks costs for a WebSocket operation
func (t *WebSocketCostTracker) TrackWebSocketOperation(ctx context.Context, opCtx *WebSocketOperationContext, result *WebSocketOperationResult) error {
	// Calculate processing time if not provided
	if result.ProcessingTimeMs == 0 {
		result.ProcessingTimeMs = time.Since(opCtx.StartTime).Milliseconds()
	}

	// Calculate costs based on operation type
	costBreakdown := t.calculateOperationCosts(opCtx, result)

	// Create cost record
	record := models.NewWebSocketCostRecordBuilder().
		ForOperation(opCtx.OperationType).
		WithConnection(opCtx.ConnectionID, opCtx.UserID, opCtx.Username).
		WithService(t.serviceName, t.functionName, opCtx.RequestID).
		WithClient(opCtx.ClientIP, opCtx.UserAgent, opCtx.ConnectionSource, opCtx.AuthMethod).
		WithStreams(opCtx.ActiveStreams, opCtx.StreamTypes).
		WithCosts(costBreakdown).
		WithPerformance(result.ProcessingTimeMs, result.ResponseLatencyMs, result.MemoryUsedMB).
		WithTag("success", strconv.FormatBool(result.Success)).
		WithProperty("operation_timestamp", opCtx.StartTime).
		Build()

	// Set additional fields
	record.ConnectionDurationMs = result.ConnectionDurationMs
	record.IdleTimeMs = result.IdleTimeMs
	record.MessageCount = result.MessageCount
	record.MessageSizeBytes = result.MessageSizeBytes

	// Add error information if operation failed
	if !result.Success && result.Error != nil {
		record.AddTag("error_type", getErrorType(result.Error))
		record.SetProperty("error_message", result.Error.Error())
	}

	// Store the cost record
	if err := t.costRepo.Create(ctx, record); err != nil {
		t.logger.Error("failed to create WebSocket cost tracking record",
			zap.String("connection_id", opCtx.ConnectionID),
			zap.String("operation_type", opCtx.OperationType),
			zap.Error(err))
		// Don't fail the operation if cost tracking fails
	}

	// Update user budget usage
	if opCtx.UserID != "" && record.TotalCostMicroCents > 0 {
		if err := t.costRepo.UpdateBudgetUsage(ctx, opCtx.UserID, record.TotalCostMicroCents); err != nil {
			t.logger.Warn("failed to update budget usage",
				zap.String("user_id", opCtx.UserID),
				zap.Int64("cost_micro_cents", record.TotalCostMicroCents),
				zap.Error(err))
		}
	}

	t.logger.Debug("tracked WebSocket operation cost",
		zap.String("connection_id", opCtx.ConnectionID),
		zap.String("operation_type", opCtx.OperationType),
		zap.String("user_id", opCtx.UserID),
		zap.Float64("cost_dollars", record.EstimatedCostDollars),
		zap.Int64("processing_time_ms", result.ProcessingTimeMs))

	return nil
}

// calculateOperationCosts calculates costs for a WebSocket operation
func (t *WebSocketCostTracker) calculateOperationCosts(opCtx *WebSocketOperationContext, result *WebSocketOperationResult) *models.WebSocketCostBreakdown {
	var connectionMinutes int64
	var messageCount int64
	var dataMB float64
	var lambdaDurationMs int64

	// Calculate costs based on operation type
	switch opCtx.OperationType {
	case WSEventConnect, WSEventDisconnect:
		// Connection costs are primarily time-based
		if result.ConnectionDurationMs > 0 {
			connectionMinutes = result.ConnectionDurationMs / (60 * 1000) // Convert ms to minutes
		}
		lambdaDurationMs = result.ProcessingTimeMs

	case WSEventMessageIn, WSEventMessageOut:
		// Message costs are per-message plus data transfer
		messageCount = int64(result.MessageCount)
		if result.MessageSizeBytes > 0 {
			dataMB = float64(result.MessageSizeBytes) / (1024 * 1024) // Convert bytes to MB
		}
		lambdaDurationMs = result.ProcessingTimeMs

	case "subscribe", "unsubscribe":
		// Subscription operations have minimal connection cost
		lambdaDurationMs = result.ProcessingTimeMs

	case "idle_time":
		// Idle time tracking for connection minutes
		if result.IdleTimeMs > 0 {
			connectionMinutes = result.IdleTimeMs / (60 * 1000) // Convert ms to minutes
		}

	case "ping", "error":
		// Lightweight operations, minimal cost
		lambdaDurationMs = result.ProcessingTimeMs
	}

	// Calculate using the cost model
	costBreakdown := models.CalculateWebSocketCosts(
		opCtx.OperationType,
		connectionMinutes,
		messageCount,
		dataMB,
		lambdaDurationMs,
	)

	// Add DynamoDB costs (estimated based on operations)
	// Connection operations typically do 2-3 DynamoDB operations
	dynamodbOperations := 2
	if opCtx.OperationType == WSEventMessageIn || opCtx.OperationType == WSEventMessageOut {
		dynamodbOperations = 1 // Messages typically do fewer DB operations
	}

	// DynamoDB on-demand pricing: ~$1.25 per million write requests
	// Approximately 1.25 microcents per write operation
	costBreakdown.DataTransferCost += int64(dynamodbOperations) * 125 / 100 // 1.25 microcents per operation

	// Recalculate total
	costBreakdown.TotalCostMicroCents = costBreakdown.APIGatewayConnectionCost +
		costBreakdown.APIGatewayMessageCost +
		costBreakdown.LambdaExecutionCost +
		costBreakdown.DataTransferCost

	return costBreakdown
}

// CreateOperationContext creates a WebSocket operation context from a Lift context and WebSocket event
func (t *WebSocketCostTracker) CreateOperationContext(ctx *lift.Context, event events.APIGatewayWebsocketProxyRequest, operationType string) *WebSocketOperationContext {
	opCtx := &WebSocketOperationContext{
		ConnectionID:  event.RequestContext.ConnectionID,
		OperationType: operationType,
		StartTime:     time.Now(),
		RequestID:     ctx.GetRequestID(),
		ClientIP:      getClientIP(event),
		UserAgent:     getUserAgent(event),
	}

	// Extract user information from context if available
	if userID := ctx.Get("user_id"); userID != nil {
		if uid, ok := userID.(string); ok {
			opCtx.UserID = uid
		}
	}

	if username := ctx.Get("username"); username != nil {
		if uname, ok := username.(string); ok {
			opCtx.Username = uname
		}
	}

	// Determine connection source and auth method
	opCtx.ConnectionSource = determineConnectionSource(event)
	opCtx.AuthMethod = determineAuthMethod(event)

	return opCtx
}

// CheckBudgetLimits checks if a user can perform WebSocket operations within budget limits
func (t *WebSocketCostTracker) CheckBudgetLimits(ctx context.Context, userID string) (*BudgetStatus, error) {
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		// Allow anonymous connections with default limits
		return &BudgetStatus{
			UserID:          "",
			AllowConnection: true,
			AllowMessages:   true,
			Budgets:         make(map[string]*BudgetPeriodStatus),
		}, nil
	}

	return t.costRepo.CheckBudgetLimits(ctx, userID)
}

// TrackConnectionLifecycle tracks the complete lifecycle of a WebSocket connection
func (t *WebSocketCostTracker) TrackConnectionLifecycle(ctx context.Context, connectionID, userID, username string, duration time.Duration, messagesSent, messagesReceived int, totalDataBytes int64) error {
	durationMs := duration.Milliseconds()

	// Track the connection session as a single operation
	opCtx := &WebSocketOperationContext{
		ConnectionID:  connectionID,
		UserID:        userID,
		Username:      username,
		OperationType: "connection_session",
		StartTime:     time.Now(),
		RequestID:     fmt.Sprintf("lifecycle-%d", time.Now().UnixNano()),
	}

	result := &WebSocketOperationResult{
		Success:              true,
		ProcessingTimeMs:     durationMs,
		ConnectionDurationMs: durationMs,
		MessageSizeBytes:     totalDataBytes,
		MessageCount:         messagesSent + messagesReceived,
	}

	return t.TrackWebSocketOperation(ctx, opCtx, result)
}

// TrackIdleConnections tracks costs for idle WebSocket connections
func (t *WebSocketCostTracker) TrackIdleConnections(ctx context.Context, connections []models.WebSocketConnection) error {
	now := time.Now()

	for _, conn := range connections {
		// Calculate idle time since last activity
		idleTime := now.Sub(conn.LastActivity)

		// Only track if idle for more than 1 minute
		if idleTime < time.Minute {
			continue
		}

		opCtx := &WebSocketOperationContext{
			ConnectionID:  conn.ConnectionID,
			UserID:        conn.UserID,
			Username:      conn.Username,
			OperationType: "idle_time",
			StartTime:     now,
			RequestID:     fmt.Sprintf("idle-%s-%d", conn.ConnectionID, now.Unix()),
			ActiveStreams: conn.Streams,
		}

		result := &WebSocketOperationResult{
			Success:    true,
			IdleTimeMs: idleTime.Milliseconds(),
		}

		if err := t.TrackWebSocketOperation(ctx, opCtx, result); err != nil {
			t.logger.Error("failed to track idle connection cost",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))
			// Continue with other connections
		}
	}

	return nil
}

// GetUserCostSummary retrieves cost summary for a user
func (t *WebSocketCostTracker) GetUserCostSummary(ctx context.Context, userID string, startTime, endTime time.Time) (*WebSocketUserCostSummary, error) {
	return t.costRepo.GetUserCostSummary(ctx, userID, startTime, endTime)
}

// GetHighCostOperations retrieves operations that exceed cost thresholds
func (t *WebSocketCostTracker) GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	return t.costRepo.GetHighCostOperations(ctx, thresholdDollars, startTime, endTime, limit)
}

// PerformCostAggregation aggregates WebSocket costs for analysis
func (t *WebSocketCostTracker) PerformCostAggregation(ctx context.Context, period string, windowStart, windowEnd time.Time) error {
	operationTypes := []string{
		"connect", "disconnect", "message_in", "message_out",
		"subscribe", "unsubscribe", "idle_time", "ping", "error",
	}

	for _, opType := range operationTypes {
		if err := t.costRepo.AggregateWebSocketCosts(ctx, opType, period, windowStart, windowEnd); err != nil {
			t.logger.Error("failed to aggregate WebSocket costs",
				zap.String("operation_type", opType),
				zap.String("period", period),
				zap.Error(err))
			// Continue with other operation types
		}
	}

	return nil
}

// Helper functions

func getServiceName() string {
	cfg := config.Get()
	if cfg.ServiceName != "" {
		return cfg.ServiceName
	}
	if name := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); name != "" {
		return name
	}
	return "websocket-service"
}

func getClientIP(event events.APIGatewayWebsocketProxyRequest) string {
	if event.RequestContext.Identity.SourceIP != "" {
		return event.RequestContext.Identity.SourceIP
	}
	if event.Headers != nil {
		if ip := event.Headers["x-forwarded-for"]; ip != "" {
			// Take the first IP in the chain
			if parts := strings.Split(ip, ","); len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}
	return StatusUnknown
}

func getUserAgent(event events.APIGatewayWebsocketProxyRequest) string {
	if event.Headers != nil {
		if ua := event.Headers["user-agent"]; ua != "" {
			return ua
		}
		if ua := event.Headers["User-Agent"]; ua != "" {
			return ua
		}
	}
	return StatusUnknown
}

func determineConnectionSource(event events.APIGatewayWebsocketProxyRequest) string {
	userAgent := getUserAgent(event)

	// Simple heuristics to determine source
	lowerUA := strings.ToLower(userAgent)

	if strings.Contains(lowerUA, "mobile") || strings.Contains(lowerUA, "android") || strings.Contains(lowerUA, "iphone") {
		return "mobile"
	}
	if strings.Contains(lowerUA, "postman") || strings.Contains(lowerUA, "curl") || strings.Contains(lowerUA, "wget") {
		return "api"
	}

	return "web"
}

func determineAuthMethod(event events.APIGatewayWebsocketProxyRequest) string {
	// Check query parameters for access token (common in WebSocket auth)
	if event.QueryStringParameters != nil {
		if token := event.QueryStringParameters["access_token"]; token != "" {
			return "oauth"
		}
	}

	// Check headers for authorization
	if event.Headers != nil {
		if auth := event.Headers["Authorization"]; auth != "" {
			if strings.HasPrefix(auth, "Bearer ") {
				return "bearer"
			}
			if strings.HasPrefix(auth, "Basic ") {
				return "basic"
			}
		}
		if auth := event.Headers["authorization"]; auth != "" {
			if strings.HasPrefix(auth, "Bearer ") {
				return "bearer"
			}
		}
	}

	return "anonymous"
}

func getErrorType(err error) string {
	if err == nil {
		return RepliesPolicyNone
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "timeout") {
		return "timeout"
	}
	if strings.Contains(errStr, "connection") {
		return "connection"
	}
	if strings.Contains(errStr, "auth") || strings.Contains(errStr, "unauthorized") {
		return "auth"
	}
	if strings.Contains(errStr, "rate") || strings.Contains(errStr, "limit") {
		return "rate_limit"
	}
	if strings.Contains(errStr, "validation") {
		return "validation"
	}
	if strings.Contains(errStr, "budget") || strings.Contains(errStr, "quota") {
		return "budget"
	}

	return StatusUnknown
}

// WebSocketCostMiddleware creates a Lift middleware for WebSocket cost tracking
func WebSocketCostMiddleware(costTracker *WebSocketCostTracker) func(lift.Handler) lift.Handler {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract WebSocket event and operation type
			event := extractWebSocketEvent(ctx)
			operationType := determineOperationType(event)

			// Create operation context
			opCtx := costTracker.CreateOperationContext(ctx, event, operationType)

			// Check budget limits if needed
			if err := checkBudgetIfRequired(costTracker, ctx, operationType, opCtx); err != nil {
				return err
			}

			// Execute the handler and measure performance
			result, err := executeAndMeasure(next, ctx, operationType)

			// Track the operation before returning
			if trackErr := costTracker.TrackWebSocketOperation(ctx.Request.Context(), opCtx, result); trackErr != nil {
				costTracker.logger.Error("failed to track WebSocket operation cost",
					zap.String("connection_id", opCtx.ConnectionID),
					zap.String("operation_type", operationType),
					zap.Error(trackErr))
			}

			return err
		})
	}
}

// extractWebSocketEvent extracts the WebSocket event from the Lift context
func extractWebSocketEvent(ctx *lift.Context) events.APIGatewayWebsocketProxyRequest {
	var event events.APIGatewayWebsocketProxyRequest

	if ctx.Request.RawEvent != nil {
		if wsEvent, ok := ctx.Request.RawEvent.(events.APIGatewayWebsocketProxyRequest); ok {
			return wsEvent
		}
		// Try to parse from request body
		if len(ctx.Request.Body) > 0 {
			_ = json.Unmarshal(ctx.Request.Body, &event)
		}
	}

	return event
}

// determineOperationType determines the operation type from the route key
func determineOperationType(event events.APIGatewayWebsocketProxyRequest) string {
	switch event.RequestContext.RouteKey {
	case "$connect":
		return "connect"
	case "$disconnect":
		return WSEventDisconnect
	case "$default":
		return WSEventMessageIn
	default:
		return StatusUnknown
	}
}

// checkBudgetIfRequired checks budget limits for non-disconnect operations with a user ID
func checkBudgetIfRequired(costTracker *WebSocketCostTracker, ctx *lift.Context, operationType string, opCtx *WebSocketOperationContext) error {
	if operationType == WSEventDisconnect || opCtx.UserID == "" {
		return nil
	}

	budgetStatus, err := costTracker.CheckBudgetLimits(ctx.Request.Context(), opCtx.UserID)
	if err != nil {
		// Log but don't fail - budget checking is not critical
		costTracker.logger.Warn("failed to check budget limits",
			zap.String("user_id", opCtx.UserID),
			zap.Error(err))
		return nil
	}

	if !budgetStatus.AllowConnection {
		return lift.NewLiftError("BUDGET_EXCEEDED", "WebSocket budget exceeded", 429)
	}

	return nil
}

// executeAndMeasure executes the handler and measures performance metrics
func executeAndMeasure(next lift.Handler, ctx *lift.Context, operationType string) (*WebSocketOperationResult, error) {
	startTime := time.Now()
	err := next.Handle(ctx)
	processingTime := time.Since(startTime)

	result := &WebSocketOperationResult{
		Success:          err == nil,
		ProcessingTimeMs: processingTime.Milliseconds(),
		Error:            err,
		MemoryUsedMB:     128.0, // Default assumption for 128MB function
	}

	// For message operations, extract message details
	if operationType == WSEventMessageIn && ctx.Request.Body != nil {
		result.MessageCount = 1
		result.MessageSizeBytes = int64(len(ctx.Request.Body))
	}

	return result, err
}
