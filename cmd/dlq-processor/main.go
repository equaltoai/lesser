package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/dlq"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// DLQProcessorHandler handles dead letter queue message processing
type DLQProcessorHandler struct {
	processor *dlq.Processor
	logger    *zap.Logger
}

// NewDLQProcessorHandler creates a new DLQ processor handler
func NewDLQProcessorHandler(processor *dlq.Processor, logger *zap.Logger) *DLQProcessorHandler {
	return &DLQProcessorHandler{
		processor: processor,
		logger:    logger,
	}
}

// HandleSQS processes SQS events containing DLQ messages
func (h *DLQProcessorHandler) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	h.logger.Info("processing DLQ event",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("message_count", len(event.Records)),
	)

	// Initialize AWS clients using the underlying context
	if err := h.processor.InitializeAWSClients(ctx.Request.Context()); err != nil {
		h.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return lift.NewLiftError("AWS_INIT_FAILED", "failed to initialize AWS clients", 500).WithCause(err)
	}

	// Process the DLQ messages
	if err := h.processor.ProcessDLQMessages(ctx.Request.Context(), event); err != nil {
		h.logger.Error("failed to process DLQ messages", zap.Error(err))
		return lift.NewLiftError("DLQ_PROCESSING_FAILED", "failed to process DLQ messages", 500).WithCause(err)
	}

	return nil
}

// HandleEventBridge processes EventBridge events for scheduled operations
func (h *DLQProcessorHandler) HandleEventBridge(ctx *lift.Context, event events.EventBridgeEvent) error {
	h.logger.Info("processing EventBridge event",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("source", event.Source),
		zap.String("detail_type", event.DetailType),
	)

	switch event.DetailType {
	case "DLQ Scheduled Reprocessing":
		return h.handleScheduledReprocessing(ctx)
	case "DLQ Cleanup":
		return h.handleCleanup(ctx)
	case "DLQ Analytics":
		return h.handleAnalytics(ctx)
	default:
		h.logger.Warn("unknown EventBridge event type",
			zap.String("detail_type", event.DetailType),
		)
		return nil
	}
}

// handleScheduledReprocessing handles scheduled reprocessing of failed messages
func (h *DLQProcessorHandler) handleScheduledReprocessing(ctx *lift.Context) error {
	h.logger.Info("starting scheduled reprocessing")

	if err := h.processor.ScheduledReprocessing(ctx.Request.Context()); err != nil {
		h.logger.Error("scheduled reprocessing failed", zap.Error(err))
		return lift.NewLiftError("REPROCESSING_FAILED", "scheduled reprocessing failed", 500).WithCause(err)
	}

	h.logger.Info("completed scheduled reprocessing")
	return nil
}

// handleCleanup handles cleanup of expired DLQ messages
func (h *DLQProcessorHandler) handleCleanup(ctx *lift.Context) error {
	h.logger.Info("starting DLQ cleanup")

	if err := h.processor.CleanupExpiredMessages(ctx.Request.Context()); err != nil {
		h.logger.Error("DLQ cleanup failed", zap.Error(err))
		return lift.NewLiftError("CLEANUP_FAILED", "DLQ cleanup failed", 500).WithCause(err)
	}

	h.logger.Info("completed DLQ cleanup")
	return nil
}

// handleAnalytics handles analytics generation for monitoring
func (h *DLQProcessorHandler) handleAnalytics(ctx *lift.Context) error {
	h.logger.Info("generating DLQ analytics")

	// Generate analytics for all services
	services := []string{
		"notification-processor",
		"activity-processor",
		"media-processor",
		"federation-delivery",
		"search-indexer",
	}

	timeRange := repositories.DLQTimeRange{
		StartTime: time.Now().Add(-24 * time.Hour), // Last 24 hours
		EndTime:   time.Now(),
	}

	for _, service := range services {
		analytics, err := h.processor.GetAnalytics(ctx.Request.Context(), service, timeRange)
		if err != nil {
			h.logger.Error("failed to generate analytics for service",
				zap.String("service", service),
				zap.Error(err),
			)
			continue
		}

		h.logger.Info("generated analytics for service",
			zap.String("service", service),
			zap.Int("total_messages", analytics.TotalMessages),
			zap.Int("resolved_messages", analytics.ResolvedMessages),
			zap.Float64("resolution_rate", analytics.ResolutionRate),
			zap.Float64("total_cost_dollars", analytics.TotalCostDollars),
		)
	}

	h.logger.Info("completed DLQ analytics generation")
	return nil
}

// Global variables for Lambda lifecycle management
var (
	logger  *zap.Logger
	cfg     *config.Config
	handler *DLQProcessorHandler
	db      core.DB
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize DLQ processor
	processor := dlq.NewProcessor(db, cfg.DynamoTableName, logger)

	// Initialize handler
	handler = NewDLQProcessorHandler(processor, logger)

	logger.Info("DLQ processor initialized successfully")
}

func main() {
	// Create Lift app
	app := lift.New()

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("dlq-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID").(string)

			logger.Info("processing request",
				zap.String("request_id", requestID),
			)

			err := next.Handle(ctx)

			duration := time.Since(start)
			if err != nil {
				logger.Error("request failed",
					zap.String("request_id", requestID),
					zap.Error(err),
					zap.Duration("duration", duration),
				)
			} else {
				logger.Info("request completed successfully",
					zap.String("request_id", requestID),
					zap.Duration("duration", duration),
				)
			}

			return err
		})
	})

	// Add error handling middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)
			if err != nil {
				logger.Error("handler error",
					zap.String("request_id", ctx.Get("requestID").(string)),
					zap.Error(err),
				)

				// Track error metrics
				if liftErr, ok := err.(*lift.LiftError); ok {
					logger.Error("lift error details",
						zap.String("error_code", liftErr.Code),
						zap.String("error_message", liftErr.Message),
						zap.Int("status_code", liftErr.StatusCode),
					)
				}
			}
			return err
		})
	})

	// Add cost tracking middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			
			err := next.Handle(ctx)
			
			// Track processing costs
			duration := time.Since(start)
			processingCostMicroCents := int64(20) // Base Lambda cost
			
			// Additional cost based on processing time
			if duration > time.Second {
				processingCostMicroCents += int64(duration.Seconds() * 10) // Cost per second
			}

			logger.Info("request cost tracking",
				zap.String("request_id", ctx.Get("requestID").(string)),
				zap.Duration("duration", duration),
				zap.Int64("cost_micro_cents", processingCostMicroCents),
				zap.Float64("cost_dollars", float64(processingCostMicroCents)/1_000_000.0),
			)

			return err
		})
	})

	// SQS handler for DLQ message processing
	app.SQS("dlq-processing", func(ctx *lift.Context) error {
		// Extract SQS event from Lift context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
		}

		// Parse the raw event as SQS event
		var event events.SQSEvent
		if sqsEvent, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
			event = sqsEvent
		} else {
			// Try to parse from request body
			if ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
				if err := json.Unmarshal(ctx.Request.Body, &event); err != nil {
					return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500)
				}
			} else {
				return lift.NewLiftError("EVENT_MISSING", "SQS event not found", 400)
			}
		}

		return handler.HandleSQS(ctx, event)
	})

	// EventBridge handler for scheduled operations
	app.EventBridge("dlq-scheduled-operations", func(ctx *lift.Context) error {
		// Extract EventBridge event from Lift context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no EventBridge event in request", 400)
		}

		// Parse the raw event as EventBridge event
		var event events.EventBridgeEvent
		if ebEvent, ok := ctx.Request.RawEvent.(events.EventBridgeEvent); ok {
			event = ebEvent
		} else {
			// Try to parse from request body
			if ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
				if err := json.Unmarshal(ctx.Request.Body, &event); err != nil {
					return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse EventBridge event", 500)
				}
			} else {
				return lift.NewLiftError("EVENT_MISSING", "EventBridge event not found", 400)
			}
		}

		return handler.HandleEventBridge(ctx, event)
	})

	// HTTP handlers for admin/monitoring (optional)
	app.GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]interface{}{
			"status":    "healthy",
			"service":   "dlq-processor",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   os.Getenv("AWS_LAMBDA_FUNCTION_VERSION"),
		})
	})

	app.GET("/analytics/:service", func(ctx *lift.Context) error {
		service := ctx.Param("service")
		if service == "" {
			return lift.ValidationError("service parameter is required")
		}

		// Default to last 24 hours
		timeRange := repositories.DLQTimeRange{
			StartTime: time.Now().Add(-24 * time.Hour),
			EndTime:   time.Now(),
		}

		analytics, err := handler.processor.GetAnalytics(ctx.Request.Context(), service, timeRange)
		if err != nil {
			return lift.NewLiftError("ANALYTICS_ERROR", "failed to get analytics", 500).WithCause(err)
		}

		return ctx.JSON(analytics)
	})

	app.GET("/trends/:service", func(ctx *lift.Context) error {
		service := ctx.Param("service")
		if service == "" {
			return lift.ValidationError("service parameter is required")
		}

		// Default to last 7 days
		days := 7

		trends, err := handler.processor.GetTrends(ctx.Request.Context(), service, days)
		if err != nil {
			return lift.NewLiftError("TRENDS_ERROR", "failed to get trends", 500).WithCause(err)
		}

		return ctx.JSON(trends)
	})

	// Search endpoint for admin tools
	app.POST("/search", func(ctx *lift.Context) error {
		var searchFilter struct {
			Service      string    `json:"service"`
			ErrorType    string    `json:"error_type,omitempty"`
			Status       string    `json:"status,omitempty"`
			Priority     string    `json:"priority,omitempty"`
			IsPermanent  *bool     `json:"is_permanent,omitempty"`
			StartTime    time.Time `json:"start_time,omitempty"`
			EndTime      time.Time `json:"end_time,omitempty"`
			SearchText   string    `json:"search_text,omitempty"`
			Limit        int       `json:"limit,omitempty"`
			Cursor       string    `json:"cursor,omitempty"`
		}

		if ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &searchFilter); err != nil {
				return lift.ValidationError("invalid search filter")
			}
		}

		if searchFilter.Service == "" {
			return lift.ValidationError("service is required for search")
		}

		// Set default limit
		if searchFilter.Limit <= 0 {
			searchFilter.Limit = 50
		}

		// Convert to the proper type expected by SearchMessages
		repoFilter := &repositories.DLQSearchFilter{
			Service:     searchFilter.Service,
			ErrorType:   searchFilter.ErrorType,
			Status:      searchFilter.Status,
			Priority:    searchFilter.Priority,
			IsPermanent: searchFilter.IsPermanent,
			StartTime:   searchFilter.StartTime,
			EndTime:     searchFilter.EndTime,
			SearchText:  searchFilter.SearchText,
			Limit:       searchFilter.Limit,
			Cursor:      searchFilter.Cursor,
		}
		
		messages, nextCursor, err := handler.processor.SearchMessages(ctx.Request.Context(), repoFilter)
		if err != nil {
			return lift.NewLiftError("SEARCH_ERROR", "failed to search messages", 500).WithCause(err)
		}

		return ctx.JSON(map[string]interface{}{
			"messages":    messages,
			"next_cursor": nextCursor,
			"count":       len(messages),
		})
	})

	// Start the Lambda handler
	lambda.Start(app.HandleRequest)
}

// Helper functions for admin operations

// triggerReprocessing manually triggers reprocessing for a service
func triggerReprocessing(ctx context.Context, service string) error {
	processor := dlq.NewProcessor(db, cfg.DynamoTableName, logger)
	
	if err := processor.InitializeAWSClients(ctx); err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	return processor.ScheduledReprocessing(ctx)
}

// generateReport generates a comprehensive DLQ report
func generateReport(ctx context.Context, services []string, timeRange repositories.DLQTimeRange) (map[string]interface{}, error) {
	processor := dlq.NewProcessor(db, cfg.DynamoTableName, logger)
	report := make(map[string]interface{})

	for _, service := range services {
		analytics, err := processor.GetAnalytics(ctx, service, timeRange)
		if err != nil {
			logger.Error("failed to get analytics for service",
				zap.String("service", service),
				zap.Error(err),
			)
			continue
		}

		trends, err := processor.GetTrends(ctx, service, 7)
		if err != nil {
			logger.Error("failed to get trends for service",
				zap.String("service", service),
				zap.Error(err),
			)
		}

		report[service] = map[string]interface{}{
			"analytics": analytics,
			"trends":    trends,
		}
	}

	return report, nil
}