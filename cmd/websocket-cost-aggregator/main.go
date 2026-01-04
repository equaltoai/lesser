// Package main implements the websocket-cost-aggregator Lambda function for aggregating WebSocket connection costs.
package main

/*
WebSocket Cost Aggregator

This Lambda function runs on a scheduled basis to:
1. Track costs for idle WebSocket connections
2. Perform cost aggregations for analysis
3. Update user budgets and alert on overages
4. Clean up stale connection records

Uses Lift framework and DynamORM patterns.
*/

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

type websocketCostRepository interface {
	GetTopCostlyUsers(ctx context.Context, startTime, endTime time.Time, limit int) ([]*repositories.WebSocketUserCostRanking, error)
	CheckBudgetLimits(ctx context.Context, userID string) (*repositories.BudgetStatus, error)
	Create(ctx context.Context, record *models.WebSocketCostRecord) error
}

type websocketConnectionRepository interface {
	GetIdleConnections(ctx context.Context, idleThreshold time.Time) ([]models.WebSocketConnection, error)
	GetStaleConnections(ctx context.Context, staleThreshold time.Time) ([]models.WebSocketConnection, error)
	DeleteConnection(ctx context.Context, connectionID string) error
	DeleteAllSubscriptions(ctx context.Context, connectionID string) error
}

type websocketCostTracker interface {
	TrackIdleConnections(ctx context.Context, connections []models.WebSocketConnection) error
	PerformCostAggregation(ctx context.Context, period string, windowStart, windowEnd time.Time) error
}

type snsPublisher interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// WebSocketCostAggregatorHandler handles scheduled WebSocket cost operations
type WebSocketCostAggregatorHandler struct {
	costRepo       websocketCostRepository
	connectionRepo websocketConnectionRepository
	costTracker    websocketCostTracker
	logger         *zap.Logger
	cfg            *common.LambdaContext
	snsClient      snsPublisher
	httpClient     httpDoer
	webhookURL     string
	snsTopicArn    string
}

var (
	lambdaCtx *common.LambdaContext
	handler   *WebSocketCostAggregatorHandler
)

var (
	mustInitializeLambdaFn = common.MustInitializeLambda
	getLambdaClientFn      = dynamorm.GetLambdaClient
	loadAWSConfigFn        = awsconfig.LoadDefaultConfig
	newSNSClientFn         = func(cfg aws.Config) snsPublisher { return sns.NewFromConfig(cfg) }
	timeNowFn              = time.Now
	lambdaStartFn          = lambda.Start
)

func initializeWebSocketCostAggregator() {
	// Initialize Lambda with basic configuration for WebSocket cost aggregation
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName:        "websocket-cost-aggregator",
		LambdaType:         common.LambdaTypeBasic,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableTracing:      true,
		EnableHealthCheck:  false,
		EnableCostTracking: true,
		RequestTimeout:     30 * time.Second,
		RetryMaxAttempts:   3,
	})

	// Initialize DynamORM database connection
	db, err := getLambdaClientFn(context.Background())
	if err != nil {
		lambdaCtx.Logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Initialize repositories
	tableName := lambdaCtx.Config.DynamoTableName
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
	}

	cfg := config.Get()
	connectionsTable := cfg.ConnectionsTable
	if err := common.ValidateRequiredParam("connectionsTable", connectionsTable); err != nil {
		connectionsTable = "lesser-streaming-connections"
	}

	subscriptionsTable := cfg.SubscriptionsTable
	if err := common.ValidateRequiredParam("subscriptionsTable", subscriptionsTable); err != nil {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	costRepo := repositories.NewWebSocketCostRepository(db, tableName, lambdaCtx.Logger, nil)
	connectionRepo := repositories.NewStreamingConnectionRepository(db, connectionsTable, db, subscriptionsTable, lambdaCtx.Logger, nil)
	costTracker := repositories.NewWebSocketCostTracker(costRepo, lambdaCtx.Logger)

	// Initialize AWS SDK config for SNS
	awsCfg, err := loadAWSConfigFn(context.Background())
	if err != nil {
		lambdaCtx.Logger.Error("failed to load AWS config for SNS", zap.Error(err))
	}

	var snsClient snsPublisher
	if awsCfg.Region != "" {
		snsClient = newSNSClientFn(awsCfg)
	}

	// Get alerting configuration from centralized config
	webhookURL := cfg.BudgetAlertWebhookURL
	snsTopicArn := cfg.BudgetAlertSNSTopicArn

	if err := common.ValidateRequiredParam("webhookURL", webhookURL); err != nil {
		if err2 := common.ValidateRequiredParam("snsTopicArn", snsTopicArn); err2 != nil {
			lambdaCtx.Logger.Warn("No alerting configuration found. Set BUDGET_ALERT_WEBHOOK_URL or BUDGET_ALERT_SNS_TOPIC_ARN to enable budget alerts")
		}
	}

	// Create handler instance
	handler = &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: connectionRepo,
		costTracker:    costTracker,
		logger:         lambdaCtx.Logger,
		cfg:            lambdaCtx,
		snsClient:      snsClient,
		webhookURL:     webhookURL,
		snsTopicArn:    snsTopicArn,
	}
}

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeWebSocketCostAggregator()
}

// HandleScheduledEvent handles CloudWatch Events (EventBridge) scheduled events
func (h *WebSocketCostAggregatorHandler) HandleScheduledEvent(ctx *lift.Context, event events.CloudWatchEvent) error {
	h.logger.Info("processing WebSocket cost aggregation",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("source", event.Source),
		zap.Time("event_time", event.Time),
	)

	// Determine what operations to perform based on the event
	operations := []string{"idle_tracking", "cost_aggregation", "budget_updates"}

	// Check if specific operations are requested via detail
	if event.Detail != nil {
		var detail map[string]interface{}
		if err := json.Unmarshal(event.Detail, &detail); err == nil {
			if ops, ok := detail["operations"].([]interface{}); ok {
				operations = make([]string, len(ops))
				for i, op := range ops {
					if opStr, ok := op.(string); ok {
						operations[i] = opStr
					}
				}
			}
		}
	}

	// Process each operation
	for _, operation := range operations {
		switch operation {
		case "idle_tracking":
			if err := h.trackIdleConnections(ctx.Request.Context()); err != nil {
				h.logger.Error("failed to track idle connections",
					zap.String("request_id", ctx.GetRequestID()),
					zap.Error(err))
			}

		case "cost_aggregation":
			if err := h.performCostAggregation(ctx.Request.Context()); err != nil {
				h.logger.Error("failed to perform cost aggregation",
					zap.String("request_id", ctx.GetRequestID()),
					zap.Error(err))
			}

		case "budget_updates":
			if err := h.updateBudgetAlerts(ctx.Request.Context()); err != nil {
				h.logger.Error("failed to update budget alerts",
					zap.String("request_id", ctx.GetRequestID()),
					zap.Error(err))
			}

		case "cleanup":
			if err := h.cleanupStaleConnections(ctx.Request.Context()); err != nil {
				h.logger.Error("failed to cleanup stale connections",
					zap.String("request_id", ctx.GetRequestID()),
					zap.Error(err))
			}

		default:
			h.logger.Warn("unknown operation requested",
				zap.String("operation", operation))
		}
	}

	h.logger.Info("completed WebSocket cost aggregation",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Strings("operations", operations))

	return nil
}

// trackIdleConnections tracks costs for connections that have been idle
func (h *WebSocketCostAggregatorHandler) trackIdleConnections(ctx context.Context) error {
	h.logger.Info("tracking idle WebSocket connections")

	// Get idle timeout from environment or use default (30 minutes)
	idleTimeoutMinutes := getIdleTimeoutMinutes()
	idleThreshold := time.Now().Add(-time.Duration(idleTimeoutMinutes) * time.Minute)

	// Get all WebSocket connections that have been idle past the threshold
	idleConnections, err := h.getIdleConnections(ctx, idleThreshold)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get idle connections")
	}

	if err := common.ValidateSliceNotEmpty("idleConnections", idleConnections); err != nil {
		h.logger.Debug("no idle connections found",
			zap.Duration("idle_threshold", time.Since(idleThreshold)))
		return nil
	}

	h.logger.Info("found idle connections to track",
		zap.Int("idle_connections", len(idleConnections)),
		zap.Duration("idle_threshold", time.Since(idleThreshold)))

	// Track costs for idle connections in batches
	batchSize := 50
	totalTracked := 0
	totalCost := int64(0)

	for i := 0; i < len(idleConnections); i += batchSize {
		end := i + batchSize
		if end > len(idleConnections) {
			end = len(idleConnections)
		}
		batch := idleConnections[i:end]

		// Track costs for this batch of idle connections
		batchCost, err := h.trackIdleConnectionsBatch(ctx, batch, idleThreshold)
		if err != nil {
			h.logger.Error("failed to track idle connections batch",
				zap.Int("batch_start", i),
				zap.Int("batch_size", len(batch)),
				zap.Error(err))
			// Continue with next batch
			continue
		}

		totalTracked += len(batch)
		totalCost += batchCost
	}

	h.logger.Info("completed idle connection cost tracking",
		zap.Int("total_tracked", totalTracked),
		zap.Int64("total_cost_microcents", totalCost),
		zap.Float64("total_cost_dollars", float64(totalCost)/1_000_000.0))

	return nil
}

// performCostAggregation performs hourly and daily cost aggregations
func (h *WebSocketCostAggregatorHandler) performCostAggregation(ctx context.Context) error {
	h.logger.Info("performing WebSocket cost aggregation")

	now := timeNowFn()

	// Perform hourly aggregation for the previous hour
	hourStart := now.Add(-1 * time.Hour).Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)

	if err := h.costTracker.PerformCostAggregation(ctx, "hour", hourStart, hourEnd); err != nil {
		h.logger.Error("failed to perform hourly aggregation",
			zap.Time("hour_start", hourStart),
			zap.Error(err))
	}

	// Perform daily aggregation if it's the first hour of the day
	if now.Hour() == 0 {
		dayStart := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		dayEnd := dayStart.Add(24 * time.Hour)

		if err := h.costTracker.PerformCostAggregation(ctx, "day", dayStart, dayEnd); err != nil {
			h.logger.Error("failed to perform daily aggregation",
				zap.Time("day_start", dayStart),
				zap.Error(err))
		}
	}

	h.logger.Info("completed cost aggregation")
	return nil
}

// updateBudgetAlerts checks user budgets and sends alerts for overages
func (h *WebSocketCostAggregatorHandler) updateBudgetAlerts(ctx context.Context) error {
	h.logger.Info("updating budget alerts")

	// Get users who may have budget issues
	// This would typically involve querying for users with high recent costs
	highCostUsers, err := h.costRepo.GetTopCostlyUsers(ctx, time.Now().AddDate(0, 0, -1), time.Now(), 50)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get high cost users")
	}

	alertCount := 0
	for _, user := range highCostUsers {
		// Check budget status for each user
		budgetStatus, err := h.costRepo.CheckBudgetLimits(ctx, user.UserID)
		if err != nil {
			h.logger.Warn("failed to check budget for user",
				zap.String("user_id", user.UserID),
				zap.Error(err))
			continue
		}

		// Send alerts for users who have exceeded thresholds
		if len(budgetStatus.ExceededBudgets) > 0 || len(budgetStatus.WarningBudgets) > 0 {
			if err := h.sendBudgetAlert(ctx, user, budgetStatus); err != nil {
				h.logger.Error("failed to send budget alert",
					zap.String("user_id", user.UserID),
					zap.Error(err))
			} else {
				alertCount++
			}
		}
	}

	h.logger.Info("completed budget alert updates",
		zap.Int("alerts_sent", alertCount))

	return nil
}

// sendBudgetAlert sends a budget alert for a user via webhook and/or SNS
func (h *WebSocketCostAggregatorHandler) sendBudgetAlert(ctx context.Context, user *repositories.WebSocketUserCostRanking, budgetStatus *repositories.BudgetStatus) error {
	// Prepare alert message
	alertMessage := BudgetAlertMessage{
		AlertType:        "BUDGET_ALERT",
		Timestamp:        time.Now().UTC(),
		UserID:           user.UserID,
		Username:         user.Username,
		TotalCostDollars: user.TotalCostDollars,
		ExceededBudgets:  budgetStatus.ExceededBudgets,
		WarningBudgets:   budgetStatus.WarningBudgets,
		Severity:         determineSeverity(budgetStatus),
		Message:          formatAlertMessage(user, budgetStatus),
	}

	// Marshal to JSON
	alertJSON, err := json.Marshal(alertMessage)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to marshal alert message")
	}

	var alertErrors []error

	// Send via webhook if configured
	if h.webhookURL != "" {
		if err := h.sendWebhookAlert(ctx, alertJSON); err != nil {
			h.logger.Error("failed to send webhook alert",
				zap.String("user_id", user.UserID),
				zap.Error(err))
			alertErrors = append(alertErrors, err)
		} else {
			h.logger.Info("webhook alert sent successfully",
				zap.String("user_id", user.UserID))
		}
	}

	// Send via SNS if configured
	if h.snsTopicArn != "" && h.snsClient != nil {
		if err := h.sendSNSAlert(ctx, alertJSON); err != nil {
			h.logger.Error("failed to send SNS alert",
				zap.String("user_id", user.UserID),
				zap.Error(err))
			alertErrors = append(alertErrors, err)
		} else {
			h.logger.Info("SNS alert sent successfully",
				zap.String("user_id", user.UserID))
		}
	}

	// Store alert in DynamoDB for audit trail
	if err := h.storeAlertRecord(ctx, &alertMessage); err != nil {
		h.logger.Warn("failed to store alert record",
			zap.String("user_id", user.UserID),
			zap.Error(err))
	}

	// Return error if all alert methods failed
	if err := common.ValidateSliceNotEmpty("alertErrors", alertErrors); err == nil && len(alertErrors) == countConfiguredAlertMethods(h) {
		return pkgErrors.WrapError(fmt.Errorf("alert errors: %v", alertErrors), pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "All alert methods failed")
	}

	return nil
}

// BudgetAlertMessage represents a budget alert notification
type BudgetAlertMessage struct {
	AlertType        string    `json:"alert_type"`
	Timestamp        time.Time `json:"timestamp"`
	UserID           string    `json:"user_id"`
	Username         string    `json:"username"`
	TotalCostDollars float64   `json:"total_cost_dollars"`
	ExceededBudgets  []string  `json:"exceeded_budgets"`
	WarningBudgets   []string  `json:"warning_budgets"`
	Severity         string    `json:"severity"`
	Message          string    `json:"message"`
}

// sendWebhookAlert sends an alert via webhook
func (h *WebSocketCostAggregatorHandler) sendWebhookAlert(ctx context.Context, alertJSON []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", h.webhookURL, bytes.NewBuffer(alertJSON))
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to create webhook request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alert-Type", "budget-alert")
	req.Header.Set("X-Instance-Domain", config.Get().Domain)

	client := h.httpClient
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Webhook request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return pkgErrors.WrapError(fmt.Errorf("status code: %d", resp.StatusCode), pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Webhook returned non-2xx status")
	}

	return nil
}

// sendSNSAlert sends an alert via AWS SNS
func (h *WebSocketCostAggregatorHandler) sendSNSAlert(ctx context.Context, alertJSON []byte) error {
	input := &sns.PublishInput{
		TopicArn: aws.String(h.snsTopicArn),
		Message:  aws.String(string(alertJSON)),
		Subject:  aws.String("Lesser WebSocket Budget Alert"),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"AlertType": {
				DataType:    aws.String("String"),
				StringValue: aws.String("BudgetAlert"),
			},
			"Instance": {
				DataType:    aws.String("String"),
				StringValue: aws.String(config.Get().Domain),
			},
		},
	}

	_, err := h.snsClient.Publish(ctx, input)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to publish SNS message")
	}

	return nil
}

// storeAlertRecord stores the alert in DynamoDB for audit trail
func (h *WebSocketCostAggregatorHandler) storeAlertRecord(ctx context.Context, alert *BudgetAlertMessage) error {
	// Create a WebSocket cost record for the alert
	// Note: We're using the cost tracking table to store alert history for audit purposes
	record := &models.WebSocketCostRecord{
		PK:            "WS_COST#ALERT",
		SK:            fmt.Sprintf("ts#%d#%s", alert.Timestamp.UnixNano(), alert.UserID),
		ID:            fmt.Sprintf("alert_%s_%d", alert.UserID, time.Now().UnixNano()),
		UserID:        alert.UserID,
		Username:      alert.Username,
		ConnectionID:  "BUDGET_ALERT", // Special connection ID for alerts
		OperationType: "BUDGET_ALERT",
		Timestamp:     alert.Timestamp,
		// Store the total cost that triggered the alert in the cost field
		TotalCostMicroCents: int64(alert.TotalCostDollars * 1_000_000), // Convert to microcents for storage
		// Use MessageCount to store severity level (1=INFO, 2=WARNING, 3=CRITICAL)
		MessageCount: severityToLevel(alert.Severity),
		// Store exceeded budget count in StreamCount for tracking
		StreamCount: len(alert.ExceededBudgets),
	}

	// Update GSI keys for user-based queries
	record.GSI2PK = fmt.Sprintf("WS_USER#%s", alert.UserID)
	record.GSI2SK = fmt.Sprintf("%d#BUDGET_ALERT#%s", alert.Timestamp.Unix(), record.ID)

	return h.costRepo.Create(ctx, record)
}

func severityToLevel(severity string) int {
	switch severity {
	case "CRITICAL":
		return 3
	case "WARNING":
		return 2
	case "INFO":
		return 1
	default:
		return 0
	}
}

// Helper functions

func determineSeverity(budgetStatus *repositories.BudgetStatus) string {
	if err := common.ValidateSliceNotEmpty("ExceededBudgets", budgetStatus.ExceededBudgets); err == nil {
		return "CRITICAL"
	}
	if err := common.ValidateSliceNotEmpty("WarningBudgets", budgetStatus.WarningBudgets); err == nil {
		return "WARNING"
	}
	return "INFO"
}

func formatAlertMessage(user *repositories.WebSocketUserCostRanking, budgetStatus *repositories.BudgetStatus) string {
	if err := common.ValidateSliceNotEmpty("ExceededBudgets", budgetStatus.ExceededBudgets); err == nil {
		return fmt.Sprintf("User %s has exceeded budget limits: %v. Total cost: $%.2f",
			user.Username, budgetStatus.ExceededBudgets, user.TotalCostDollars)
	}
	if err := common.ValidateSliceNotEmpty("WarningBudgets", budgetStatus.WarningBudgets); err == nil {
		return fmt.Sprintf("User %s is approaching budget limits: %v. Total cost: $%.2f",
			user.Username, budgetStatus.WarningBudgets, user.TotalCostDollars)
	}
	return fmt.Sprintf("Budget status update for user %s. Total cost: $%.2f",
		user.Username, user.TotalCostDollars)
}

func countConfiguredAlertMethods(h *WebSocketCostAggregatorHandler) int {
	count := 0
	if h.webhookURL != "" {
		count++
	}
	if h.snsTopicArn != "" && h.snsClient != nil {
		count++
	}
	return count
}

// cleanupStaleConnections removes connection records for connections that have been disconnected
func (h *WebSocketCostAggregatorHandler) cleanupStaleConnections(ctx context.Context) error {
	h.logger.Info("cleaning up stale WebSocket connections")

	// Get stale timeout from environment or use default (24 hours)
	staleTimeoutHours := getStaleTimeoutHours()
	staleThreshold := time.Now().Add(-time.Duration(staleTimeoutHours) * time.Hour)

	// Find connections that are older than the stale threshold
	staleConnections, err := h.getStaleConnections(ctx, staleThreshold)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get stale connections")
	}

	if err := common.ValidateSliceNotEmpty("staleConnections", staleConnections); err != nil {
		h.logger.Debug("no stale connections found to clean up",
			zap.Duration("stale_threshold", time.Since(staleThreshold)))
		return nil
	}

	h.logger.Info("found stale connections to clean up",
		zap.Int("stale_connections", len(staleConnections)),
		zap.Duration("stale_threshold", time.Since(staleThreshold)))

	// Clean up connections in batches
	batchSize := 25 // Smaller batch for cleanup operations
	totalCleaned := 0
	totalErrors := 0
	reclaimedCost := int64(0)

	for i := 0; i < len(staleConnections); i += batchSize {
		end := i + batchSize
		if end > len(staleConnections) {
			end = len(staleConnections)
		}
		batch := staleConnections[i:end]

		// Clean up this batch of stale connections
		batchResults := h.cleanupStaleConnectionsBatch(ctx, batch)
		totalCleaned += batchResults.Cleaned
		totalErrors += batchResults.Errors
		reclaimedCost += batchResults.ReclaimedCostMicroCents
	}

	h.logger.Info("completed stale connection cleanup",
		zap.Int("total_cleaned", totalCleaned),
		zap.Int("total_errors", totalErrors),
		zap.Int64("reclaimed_cost_microcents", reclaimedCost),
		zap.Float64("reclaimed_cost_dollars", float64(reclaimedCost)/1_000_000.0))

	// Send alert if cleanup reclaimed significant cost
	if reclaimedCost > 100_000 { // > $0.10 reclaimed
		h.sendCleanupAlert(ctx, totalCleaned, reclaimedCost)
	}

	return nil
}

// getIdleConnections retrieves WebSocket connections that have been idle past the threshold
func (h *WebSocketCostAggregatorHandler) getIdleConnections(ctx context.Context, idleThreshold time.Time) ([]models.WebSocketConnection, error) {
	// Use the repository method to get idle connections
	return h.connectionRepo.GetIdleConnections(ctx, idleThreshold)
}

// trackIdleConnectionsBatch tracks costs for a batch of idle connections
func (h *WebSocketCostAggregatorHandler) trackIdleConnectionsBatch(ctx context.Context, connections []models.WebSocketConnection, idleThreshold time.Time) (int64, error) {
	if err := h.costTracker.TrackIdleConnections(ctx, connections); err != nil {
		return 0, pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to track idle connections")
	}

	// Calculate total idle cost for reporting
	totalIdleCost := int64(0)
	now := time.Now()

	for _, conn := range connections {
		// Calculate idle time since threshold
		idleTime := now.Sub(conn.LastActivity)
		if conn.LastActivity.Before(idleThreshold) {
			idleTime = now.Sub(idleThreshold)
		}

		// Estimate idle cost (connection minutes * cost per minute)
		idleMinutes := int64(idleTime.Minutes())
		// Using API Gateway connection cost: ~0.25 microcents per minute
		idleCost := (idleMinutes * 25) / 100 // microcents
		totalIdleCost += idleCost
	}

	return totalIdleCost, nil
}

// getStaleConnections retrieves WebSocket connections that are considered stale
func (h *WebSocketCostAggregatorHandler) getStaleConnections(ctx context.Context, staleThreshold time.Time) ([]models.WebSocketConnection, error) {
	// Use the repository method to get stale connections
	return h.connectionRepo.GetStaleConnections(ctx, staleThreshold)
}

// CleanupBatchResult represents the result of cleaning up a batch of stale connections
type CleanupBatchResult struct {
	Cleaned                 int
	Errors                  int
	ReclaimedCostMicroCents int64
}

// cleanupStaleConnectionsBatch cleans up a batch of stale connections
func (h *WebSocketCostAggregatorHandler) cleanupStaleConnectionsBatch(ctx context.Context, connections []models.WebSocketConnection) *CleanupBatchResult {
	result := &CleanupBatchResult{}
	now := time.Now()

	for _, conn := range connections {
		// Calculate how long the connection has been stale
		staleTime := now.Sub(conn.LastActivity)

		// Estimate reclaimed cost (connection time that won't accumulate further cost)
		staleMinutes := int64(staleTime.Minutes())
		reclaimedCost := (staleMinutes * 25) / 100 // microcents (API Gateway connection cost)

		// Delete the connection record
		if err := h.connectionRepo.DeleteConnection(ctx, conn.ConnectionID); err != nil {
			h.logger.Error("failed to delete stale connection",
				zap.String("connection_id", conn.ConnectionID),
				zap.String("user_id", conn.UserID),
				zap.Duration("stale_time", staleTime),
				zap.Error(err))
			result.Errors++
			continue
		}

		// Delete all subscriptions for this connection
		if err := h.connectionRepo.DeleteAllSubscriptions(ctx, conn.ConnectionID); err != nil {
			h.logger.Warn("failed to delete stale connection subscriptions",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))
			// Don't count this as an error since the main connection was deleted
		}

		// Record cleanup action for audit
		h.recordCleanupAction(ctx, conn, staleTime, reclaimedCost)

		result.Cleaned++
		result.ReclaimedCostMicroCents += reclaimedCost

		h.logger.Debug("cleaned up stale connection",
			zap.String("connection_id", conn.ConnectionID),
			zap.String("user_id", conn.UserID),
			zap.Duration("stale_time", staleTime),
			zap.Int64("reclaimed_cost_microcents", reclaimedCost))
	}

	return result
}

// recordCleanupAction records a cleanup action for audit purposes
func (h *WebSocketCostAggregatorHandler) recordCleanupAction(ctx context.Context, conn models.WebSocketConnection, staleTime time.Duration, reclaimedCost int64) {
	// Create a cost record to track the cleanup action
	record := &models.WebSocketCostRecord{
		PK:            "WS_COST#cleanup",
		SK:            fmt.Sprintf("ts#%d#%s", time.Now().UnixNano(), conn.ConnectionID),
		ID:            fmt.Sprintf("cleanup_%s_%d", conn.ConnectionID, time.Now().UnixNano()),
		UserID:        conn.UserID,
		Username:      conn.Username,
		ConnectionID:  conn.ConnectionID,
		OperationType: "cleanup",
		Timestamp:     time.Now(),
		// Store stale time in ConnectionDurationMs for tracking
		ConnectionDurationMs: staleTime.Milliseconds(),
		// Use negative cost to represent reclaimed/prevented cost
		TotalCostMicroCents: -reclaimedCost,
		// Mark as cleanup operation
		Tags: map[string]string{
			"action":      "stale_cleanup",
			"automated":   "true",
			"stale_hours": fmt.Sprintf("%.1f", staleTime.Hours()),
		},
	}

	// Update GSI keys
	record.GSI1PK = fmt.Sprintf("WS_CONN#%s", conn.ConnectionID)
	record.GSI1SK = fmt.Sprintf("%s#cleanup#%s", time.Now().Format(time.RFC3339), record.ID)

	if conn.UserID != "" {
		record.GSI2PK = fmt.Sprintf("WS_USER#%s", conn.UserID)
		record.GSI2SK = fmt.Sprintf("%s#cleanup#%s", time.Now().Format(time.RFC3339), record.ID)
	}

	if err := h.costRepo.Create(ctx, record); err != nil {
		h.logger.Warn("failed to record cleanup action",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(err))
	}
}

// sendCleanupAlert sends an alert about significant cleanup activity
func (h *WebSocketCostAggregatorHandler) sendCleanupAlert(ctx context.Context, cleanedCount int, reclaimedCost int64) {
	alertMessage := CleanupAlertMessage{
		AlertType:               "CLEANUP_ALERT",
		Timestamp:               time.Now().UTC(),
		CleanedConnections:      cleanedCount,
		ReclaimedCostMicroCents: reclaimedCost,
		ReclaimedCostDollars:    float64(reclaimedCost) / 1_000_000.0,
		Message: fmt.Sprintf("Cleaned up %d stale WebSocket connections, reclaiming $%.4f in potential costs",
			cleanedCount, float64(reclaimedCost)/1_000_000.0),
	}

	// Marshal to JSON
	alertJSON, err := json.Marshal(alertMessage)
	if err != nil {
		h.logger.Error("failed to marshal cleanup alert", zap.Error(err))
		return
	}

	// Send via configured channels
	if h.webhookURL != "" {
		if err := h.sendWebhookAlert(ctx, alertJSON); err != nil {
			h.logger.Error("failed to send cleanup webhook alert", zap.Error(err))
		}
	}

	if h.snsTopicArn != "" && h.snsClient != nil {
		if err := h.sendSNSAlert(ctx, alertJSON); err != nil {
			h.logger.Error("failed to send cleanup SNS alert", zap.Error(err))
		}
	}
}

// CleanupAlertMessage represents a cleanup alert notification
type CleanupAlertMessage struct {
	AlertType               string    `json:"alert_type"`
	Timestamp               time.Time `json:"timestamp"`
	CleanedConnections      int       `json:"cleaned_connections"`
	ReclaimedCostMicroCents int64     `json:"reclaimed_cost_micro_cents"`
	ReclaimedCostDollars    float64   `json:"reclaimed_cost_dollars"`
	Message                 string    `json:"message"`
}

// getIdleTimeoutMinutes returns the idle timeout in minutes from config
func getIdleTimeoutMinutes() int {
	return config.Get().IdleTimeoutMinutes
}

// getStaleTimeoutHours returns the stale timeout in hours from config
func getStaleTimeoutHours() int {
	return config.Get().StaleTimeoutHours
}

func main() {
	// Create a new Lift application
	app := lift.New()

	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Add standard middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("ws-cost-agg-%d", time.Now().UnixNano())
			ctx.SetRequestID(requestID)
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID")

			lambdaCtx.Logger.Info("processing WebSocket cost aggregation",
				zap.Any("request_id", requestID))

			err := next.Handle(ctx)
			duration := time.Since(start)

			if err != nil {
				lambdaCtx.Logger.Error("failed to process WebSocket cost aggregation",
					zap.Any("request_id", requestID),
					zap.Error(err),
					zap.Duration("duration", duration))
			} else {
				lambdaCtx.Logger.Info("successfully processed WebSocket cost aggregation",
					zap.Any("request_id", requestID),
					zap.Duration("duration", duration))
			}

			return err
		})
	})

	// Add recovery middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					requestID := ctx.Get("requestID")
					lambdaCtx.Logger.Error("panic recovered in WebSocket cost aggregator",
						zap.Any("request_id", requestID),
						zap.Any("panic", r))
				}
			}()
			return next.Handle(ctx)
		})
	})

	_ = app.EventBridge("lesser-websocket-cost-aggregator-schedule-*", func(ctx *lift.Context) error {
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no EventBridge event in request", 400)
		}

		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return lift.NewLiftError("EVENT_MARSHAL_ERROR", "failed to marshal raw event", 500).WithCause(err)
		}

		var event events.CloudWatchEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse EventBridge event", 500).WithCause(err)
		}

		return handler.HandleScheduledEvent(ctx, event)
	})

	// Start the Lambda handler with Lift
	lambdaStartFn(app.HandleRequest)
}
