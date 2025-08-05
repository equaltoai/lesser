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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// WebSocketCostAggregatorHandler handles scheduled WebSocket cost operations
type WebSocketCostAggregatorHandler struct {
	costRepo       *repositories.WebSocketCostRepository
	connectionRepo *repositories.StreamingConnectionRepository
	costTracker    *repositories.WebSocketCostTracker
	logger         *zap.Logger
	cfg            *config.Config
}

var (
	logger  *zap.Logger
	cfg     *config.Config
	handler *WebSocketCostAggregatorHandler
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM database connection
	db, err := dynamorm.GetLambdaClient(context.Background())
	if err != nil {
		logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Initialize repositories
	tableName := cfg.DynamoTableName
	if tableName == "" {
		tableName = "lesser-main"
	}

	connectionsTable := os.Getenv("CONNECTIONS_TABLE")
	if connectionsTable == "" {
		connectionsTable = "lesser-streaming-connections"
	}

	subscriptionsTable := os.Getenv("SUBSCRIPTIONS_TABLE")
	if subscriptionsTable == "" {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	costRepo := repositories.NewWebSocketCostRepository(db, tableName, logger)
	connectionRepo := repositories.NewStreamingConnectionRepository(db, connectionsTable, db, subscriptionsTable, logger)
	costTracker := repositories.NewWebSocketCostTracker(costRepo, logger)

	// Create handler instance
	handler = &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: connectionRepo,
		costTracker:    costTracker,
		logger:         logger,
		cfg:            cfg,
	}
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

	// Get all active connections (this would need to be implemented in the connection repository)
	// For now, we'll simulate getting connections that have been idle for more than 5 minutes
	// idleThreshold := time.Now().Add(-5 * time.Minute) // TODO: Use when implementing actual query

	// In a real implementation, you'd query for connections with LastActivity < idleThreshold
	// For this example, we'll create a placeholder
	idleConnections := []models.WebSocketConnection{
		// This would be populated from actual database query
	}

	if len(idleConnections) == 0 {
		h.logger.Debug("no idle connections found")
		return nil
	}

	// Track costs for idle connections
	if err := h.costTracker.TrackIdleConnections(ctx, idleConnections); err != nil {
		return fmt.Errorf("failed to track idle connections: %w", err)
	}

	h.logger.Info("tracked idle connection costs",
		zap.Int("idle_connections", len(idleConnections)))

	return nil
}

// performCostAggregation performs hourly and daily cost aggregations
func (h *WebSocketCostAggregatorHandler) performCostAggregation(ctx context.Context) error {
	h.logger.Info("performing WebSocket cost aggregation")

	now := time.Now()

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
		return fmt.Errorf("failed to get high cost users: %w", err)
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

// sendBudgetAlert sends a budget alert for a user (placeholder implementation)
func (h *WebSocketCostAggregatorHandler) sendBudgetAlert(ctx context.Context, user *repositories.WebSocketUserCostRanking, budgetStatus *repositories.BudgetStatus) error {
	// In a real implementation, this would send an email, push notification, or webhook
	h.logger.Info("budget alert triggered",
		zap.String("user_id", user.UserID),
		zap.String("username", user.Username),
		zap.Float64("total_cost_dollars", user.TotalCostDollars),
		zap.Strings("exceeded_budgets", budgetStatus.ExceededBudgets),
		zap.Strings("warning_budgets", budgetStatus.WarningBudgets))

	// For now, just log the alert
	// TODO: Implement actual alerting mechanism (email, SNS, webhook, etc.)

	return nil
}

// cleanupStaleConnections removes connection records for connections that have been disconnected
func (h *WebSocketCostAggregatorHandler) cleanupStaleConnections(ctx context.Context) error {
	h.logger.Info("cleaning up stale WebSocket connections")

	// In a real implementation, you'd query for connections older than a certain threshold
	// and verify they're no longer active with API Gateway, then clean them up

	// For now, this is a placeholder
	staleThreshold := time.Now().Add(-24 * time.Hour)
	
	h.logger.Info("cleanup completed",
		zap.Time("stale_threshold", staleThreshold))

	return nil
}

func main() {
	// Create a new Lift application
	app := lift.New()

	// Add standard middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("ws-cost-agg-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID")
			
			logger.Info("processing WebSocket cost aggregation",
				zap.Any("request_id", requestID))

			err := next.Handle(ctx)
			duration := time.Since(start)

			if err != nil {
				logger.Error("failed to process WebSocket cost aggregation",
					zap.Any("request_id", requestID),
					zap.Error(err),
					zap.Duration("duration", duration))
			} else {
				logger.Info("successfully processed WebSocket cost aggregation",
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
					logger.Error("panic recovered in WebSocket cost aggregator",
						zap.Any("request_id", requestID),
						zap.Any("panic", r))
				}
			}()
			return next.Handle(ctx)
		})
	})

	// Set up the scheduled event handler
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Parse the event as CloudWatch Event
			if ctx.Request.RawEvent == nil {
				return lift.NewLiftError("MISSING_EVENT", "no event in request", 400)
			}

			var event events.CloudWatchEvent
			if cwEvent, ok := ctx.Request.RawEvent.(events.CloudWatchEvent); ok {
				event = cwEvent
			} else {
				// Try to parse from request body
				if ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
					if err := json.Unmarshal(ctx.Request.Body, &event); err != nil {
						return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse CloudWatch event", 500)
					}
				} else {
					return lift.NewLiftError("EVENT_MISSING", "CloudWatch event not found", 400)
				}
			}

			return handler.HandleScheduledEvent(ctx, event)
		})
	})

	// Start the Lambda handler with Lift
	lambda.Start(app.HandleRequest)
}