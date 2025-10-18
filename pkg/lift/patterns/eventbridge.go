package patterns

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// EventBridgeHandler is the interface that services must implement to handle EventBridge/CloudWatch events
type EventBridgeHandler interface {
	HandleEvent(ctx *lift.Context, event events.CloudWatchEvent) error
}

// EventBridgeProcessor wraps a handler with Lift middleware and error handling
type EventBridgeProcessor struct {
	handler   EventBridgeHandler
	logger    *zap.Logger
	eventName string
}

// NewEventBridgeProcessor creates a new EventBridge processor
func NewEventBridgeProcessor(eventName string, handler EventBridgeHandler, logger *zap.Logger) *EventBridgeProcessor {
	return &EventBridgeProcessor{
		handler:   handler,
		logger:    logger,
		eventName: eventName,
	}
}

// RegisterEventBridge registers an EventBridge handler with a Lift app
func RegisterEventBridge(app *lift.App, processor *EventBridgeProcessor) {
	// EventBridge events don't have native Lift support, so we handle the raw Lambda event
	_ = app.Handle("POST", "/", func(ctx *lift.Context) error {
		// Extract the raw event from context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no EventBridge event in request", 400)
		}

		// Try to cast directly first
		if event, ok := ctx.Request.RawEvent.(events.CloudWatchEvent); ok {
			return processor.ProcessEvent(ctx, event)
		}

		// If direct cast fails, try JSON marshaling
		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return lift.NewLiftError("EVENT_MARSHAL_ERROR", "failed to marshal raw event", 500).WithCause(err)
		}

		var event events.CloudWatchEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse EventBridge event", 500).WithCause(err)
		}

		return processor.ProcessEvent(ctx, event)
	})
}

// ProcessEvent processes an EventBridge event with proper logging and error handling
func (ep *EventBridgeProcessor) ProcessEvent(ctx *lift.Context, event events.CloudWatchEvent) error {
	start := time.Now()
	requestID := ctx.GetRequestID()
	if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
		requestID = fmt.Sprintf("%s-%d", ep.eventName, time.Now().UnixNano())
		ctx.Set("requestID", requestID)
	}

	ep.logger.Info("processing EventBridge event",
		zap.String("event_name", ep.eventName),
		zap.String("request_id", requestID),
		zap.String("source", event.Source),
		zap.String("detail_type", event.DetailType),
		zap.Time("event_time", event.Time),
	)

	// Call the actual handler
	err := ep.handler.HandleEvent(ctx, event)

	duration := time.Since(start)
	if err != nil {
		ep.logger.Error("failed to process EventBridge event",
			zap.String("event_name", ep.eventName),
			zap.String("request_id", requestID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return err
	}

	ep.logger.Info("successfully processed EventBridge event",
		zap.String("event_name", ep.eventName),
		zap.String("request_id", requestID),
		zap.Duration("duration", duration),
	)

	return nil
}

// CreateEventBridgeApp creates a standard Lift app configured for EventBridge
func CreateEventBridgeApp(eventName string, handler EventBridgeHandler, logger *zap.Logger) *lift.App {
	app := lift.New()

	// Add standard middleware
	app.Use(RequestIDMiddleware(eventName))
	app.Use(LoggingMiddleware(logger))
	app.Use(RecoveryMiddleware(logger))

	// Create and register the processor
	processor := NewEventBridgeProcessor(eventName, handler, logger)
	RegisterEventBridge(app, processor)

	return app
}

// StartEventBridgeLambda is a convenience function to start an EventBridge Lambda
func StartEventBridgeLambda(eventName string, handler EventBridgeHandler, logger *zap.Logger) {
	app := CreateEventBridgeApp(eventName, handler, logger)
	lambda.Start(app.HandleRequest)
}

// ScheduledEventHandler is a specialized interface for scheduled events (cron jobs)
type ScheduledEventHandler interface {
	HandleScheduledEvent(ctx *lift.Context) error
}

// ScheduledEventAdapter adapts a ScheduledEventHandler to an EventBridgeHandler
type ScheduledEventAdapter struct {
	handler ScheduledEventHandler
}

// NewScheduledEventAdapter creates a new adapter
func NewScheduledEventAdapter(handler ScheduledEventHandler) EventBridgeHandler {
	return &ScheduledEventAdapter{handler: handler}
}

// HandleEvent implements EventBridgeHandler by calling the scheduled handler
func (a *ScheduledEventAdapter) HandleEvent(ctx *lift.Context, _ events.CloudWatchEvent) error {
	// For scheduled events, we typically don't need the event details
	// The schedule itself is configured in infrastructure
	return a.handler.HandleScheduledEvent(ctx)
}

// CreateScheduledApp creates a Lift app for scheduled tasks
func CreateScheduledApp(taskName string, handler ScheduledEventHandler, logger *zap.Logger) *lift.App {
	adapter := NewScheduledEventAdapter(handler)
	return CreateEventBridgeApp(taskName, adapter, logger)
}

// StartScheduledLambda starts a Lambda for scheduled tasks
func StartScheduledLambda(taskName string, handler ScheduledEventHandler, logger *zap.Logger) {
	app := CreateScheduledApp(taskName, handler, logger)
	lambda.Start(app.HandleRequest)
}
