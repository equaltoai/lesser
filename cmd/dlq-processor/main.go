// Package main implements the dlq-processor Lambda function for processing dead letter queue messages.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/dlq"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
)

type dlqProcessor interface {
	InitializeAWSClients(ctx context.Context) error
	ProcessDLQMessages(ctx context.Context, event events.SQSEvent) error
	ScheduledReprocessing(ctx context.Context) error
	CleanupExpiredMessages(ctx context.Context) error
	GetAnalytics(ctx context.Context, service string, timeRange repositories.DLQTimeRange) (*repositories.DLQAnalytics, error)
	GetTrends(ctx context.Context, service string, days int) (*repositories.DLQTrends, error)
	SearchMessages(ctx context.Context, filter *repositories.DLQSearchFilter) ([]*models.DLQMessage, string, error)
}

// DLQProcessorHandler handles dead letter queue message processing.
type DLQProcessorHandler struct {
	processor dlqProcessor
	logger    *zap.Logger
}

// NewDLQProcessorHandler creates a new DLQ processor handler.
func NewDLQProcessorHandler(processor dlqProcessor, logger *zap.Logger) *DLQProcessorHandler {
	return &DLQProcessorHandler{
		processor: processor,
		logger:    logger,
	}
}

func (h *DLQProcessorHandler) HandleSQSMessage(ctx *apptheory.EventContext, msg events.SQSMessage) error {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if h.logger == nil {
		h.logger = zap.NewNop()
	}

	h.logger.Info("processing DLQ message",
		zap.String("request_id", requestID),
		zap.String("message_id", msg.MessageId),
	)

	if err := h.processor.InitializeAWSClients(runCtx); err != nil {
		h.logger.Error("failed to initialize AWS clients",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return err
	}

	// AppTheory routes SQS per-message; convert to a single-record SQSEvent for the dlq processor.
	event := events.SQSEvent{Records: []events.SQSMessage{msg}}
	if err := h.processor.ProcessDLQMessages(runCtx, event); err != nil {
		h.logger.Error("failed to process DLQ message",
			zap.String("request_id", requestID),
			zap.String("message_id", msg.MessageId),
			zap.Error(err),
		)
		return err
	}

	return nil
}

func (h *DLQProcessorHandler) HandleEventBridge(ctx *apptheory.EventContext, event events.EventBridgeEvent) (any, error) {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if h.logger == nil {
		h.logger = zap.NewNop()
	}

	h.logger.Info("processing EventBridge event",
		zap.String("request_id", requestID),
		zap.String("source", event.Source),
		zap.String("detail_type", event.DetailType),
	)

	switch event.DetailType {
	case "DLQ Scheduled Reprocessing", "Scheduled Event":
		return nil, h.handleScheduledReprocessing(runCtx)
	case "DLQ Cleanup":
		return nil, h.handleCleanup(runCtx)
	case "DLQ Analytics":
		return nil, h.handleAnalytics(runCtx)
	default:
		h.logger.Warn("unknown EventBridge event type",
			zap.String("detail_type", event.DetailType),
		)
		return nil, nil
	}
}

func (h *DLQProcessorHandler) handleScheduledReprocessing(ctx context.Context) error {
	h.logger.Info("starting scheduled reprocessing")

	if err := h.processor.ScheduledReprocessing(ctx); err != nil {
		h.logger.Error("scheduled reprocessing failed", zap.Error(err))
		return err
	}

	h.logger.Info("completed scheduled reprocessing")
	return nil
}

func (h *DLQProcessorHandler) handleCleanup(ctx context.Context) error {
	h.logger.Info("starting DLQ cleanup")

	if err := h.processor.CleanupExpiredMessages(ctx); err != nil {
		h.logger.Error("DLQ cleanup failed", zap.Error(err))
		return err
	}

	h.logger.Info("completed DLQ cleanup")
	return nil
}

func (h *DLQProcessorHandler) handleAnalytics(ctx context.Context) error {
	h.logger.Info("generating DLQ analytics")

	services := []string{
		"notification-processor",
		"activity-processor",
		"media-processor",
		"federation-delivery",
		"search-indexer",
	}

	timeRange := repositories.DLQTimeRange{
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	}

	for _, service := range services {
		analytics, err := h.processor.GetAnalytics(ctx, service, timeRange)
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

// Global variables for standardized Lambda initialization.
var (
	lambdaCtx                  *common.LambdaContext
	cfg                        *config.Config
	logger                     *zap.Logger
	repos                      interface{} //nolint:unused // dependency injection pattern - available for processor extensions
	handler                    *DLQProcessorHandler
	runningUnitTestsFn         = common.RunningUnitTests
	mustInitializeLambdaFn     = common.MustInitializeLambda
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	lambdaStartFn              = lambda.Start
)

func init() {
	if runningUnitTestsFn() {
		return
	}

	if err := initializeDLQProcessor(); err != nil {
		logger.Fatal("failed to initialize dlq processor", zap.Error(err))
	}
}

func initializeDLQProcessor() error {
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "dlq-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})

	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos

	db, err := initializeDLQStorage(lambdaCtx)
	if err != nil {
		return err
	}
	processor := dlq.NewProcessor(db, cfg.DynamoTableName, logger)
	handler = NewDLQProcessorHandler(processor, logger)

	logger.Info("DLQ processor initialized successfully")
	return nil
}

func initializeDLQStorage(lambdaCtx *common.LambdaContext) (core.DB, error) {
	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName: "dlq-processor",
		NewDB:       newLambdaOptimizedClientFn,
	})
	if err != nil {
		return nil, err
	}
	return deps.DB, nil
}

func main() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))

	// Inventory-driven DLQ consumers (ConsumeDeadLetterQueue=true).
	dlqQueues := []string{
		"enhanced-federation-queue",
		"export-processor-queue",
		"federation-aggregator-queue",
		"federation-delivery-queue",
		"import-processor-queue",
		"media-processor-queue",
		"notification-processor-queue",
		"push-delivery-queue",
	}

	for _, q := range dlqQueues {
		dlqName := naming.ResourceNameWithApp(appName, fmt.Sprintf("%s-dlq", q), stage)
		app.SQS(dlqName, func(ctx *apptheory.EventContext, msg events.SQSMessage) error {
			if handler == nil {
				return fmt.Errorf("dlq processor handler not initialized")
			}
			return handler.HandleSQSMessage(ctx, msg)
		})
	}

	ruleName := naming.ResourceNameWithApp(appName, "dlq-processor-schedule-0", stage)
	app.EventBridge(apptheory.EventBridgeRule(ruleName), func(ctx *apptheory.EventContext, event events.EventBridgeEvent) (any, error) {
		if handler == nil {
			return nil, fmt.Errorf("dlq processor handler not initialized")
		}
		return handler.HandleEventBridge(ctx, event)
	})

	app.Get("/health", handleHealthCheck)
	app.Get("/analytics/:service", handleAnalyticsHTTP)
	app.Get("/trends/:service", handleTrendsHTTP)
	app.Post("/search", handleSearchHTTP)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func handleHealthCheck(_ *apptheory.Context) (*apptheory.Response, error) {
	return apptheory.JSON(200, map[string]interface{}{
		"status":    "healthy",
		"service":   "dlq-processor",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   os.Getenv("AWS_LAMBDA_FUNCTION_VERSION"),
	})
}

func handleAnalyticsHTTP(ctx *apptheory.Context) (*apptheory.Response, error) {
	service := ctx.Param("service")
	if err := common.ValidateRequiredParam("service", service); err != nil {
		return apptheory.JSON(400, map[string]any{"error": "service parameter is required"})
	}
	if handler == nil {
		return apptheory.JSON(500, map[string]any{"error": "dlq processor not initialized"})
	}

	timeRange := repositories.DLQTimeRange{
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	}

	analytics, err := handler.processor.GetAnalytics(ctx.Context(), service, timeRange)
	if err != nil {
		return apptheory.JSON(500, map[string]any{"error": "failed to get analytics"})
	}

	return apptheory.JSON(200, analytics)
}

func handleTrendsHTTP(ctx *apptheory.Context) (*apptheory.Response, error) {
	service := ctx.Param("service")
	if err := common.ValidateRequiredParam("service", service); err != nil {
		return apptheory.JSON(400, map[string]any{"error": "service parameter is required"})
	}
	if handler == nil {
		return apptheory.JSON(500, map[string]any{"error": "dlq processor not initialized"})
	}

	trends, err := handler.processor.GetTrends(ctx.Context(), service, 7)
	if err != nil {
		return apptheory.JSON(500, map[string]any{"error": "failed to get trends"})
	}

	return apptheory.JSON(200, trends)
}

// searchFilter represents the search filter request body.
type searchFilter struct {
	Service     string    `json:"service"`
	ErrorType   string    `json:"error_type,omitempty"`
	Status      string    `json:"status,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	IsPermanent *bool     `json:"is_permanent,omitempty"`
	StartTime   time.Time `json:"start_time,omitempty"`
	EndTime     time.Time `json:"end_time,omitempty"`
	SearchText  string    `json:"search_text,omitempty"`
	Limit       int       `json:"limit,omitempty"`
	Cursor      string    `json:"cursor,omitempty"`
}

func handleSearchHTTP(ctx *apptheory.Context) (*apptheory.Response, error) {
	if handler == nil {
		return apptheory.JSON(500, map[string]any{"error": "dlq processor not initialized"})
	}

	filter, err := parseSearchFilter(ctx)
	if err != nil {
		return apptheory.JSON(400, map[string]any{"error": err.Error()})
	}

	messages, nextCursor, err := handler.processor.SearchMessages(ctx.Context(), filter)
	if err != nil {
		return apptheory.JSON(500, map[string]any{"error": "failed to search messages"})
	}

	return apptheory.JSON(200, map[string]interface{}{
		"messages":    messages,
		"next_cursor": nextCursor,
		"count":       len(messages),
	})
}

func parseSearchFilter(ctx *apptheory.Context) (*repositories.DLQSearchFilter, error) {
	var filter searchFilter

	if err := common.ValidateSliceNotEmpty("ctx.Request.Body", ctx.Request.Body); err == nil {
		if err := json.Unmarshal(ctx.Request.Body, &filter); err != nil {
			return nil, fmt.Errorf("invalid search filter")
		}
	}

	if err := common.ValidateRequiredParam("filter.Service", filter.Service); err != nil {
		return nil, fmt.Errorf("service is required for search")
	}

	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	return &repositories.DLQSearchFilter{
		Service:     filter.Service,
		ErrorType:   filter.ErrorType,
		Status:      filter.Status,
		Priority:    filter.Priority,
		IsPermanent: filter.IsPermanent,
		StartTime:   filter.StartTime,
		EndTime:     filter.EndTime,
		SearchText:  filter.SearchText,
		Limit:       filter.Limit,
		Cursor:      filter.Cursor,
	}, nil
}
