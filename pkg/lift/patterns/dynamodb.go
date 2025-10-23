// Package patterns provides DynamoDB stream processing patterns and handlers for the Lift framework.
package patterns

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"go.uber.org/zap"
)

// DynamoDBStreamHandler is the interface that services must implement to handle DynamoDB stream events
type DynamoDBStreamHandler interface {
	HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error
}

// DynamoDBStreamProcessor wraps a handler with Lift middleware and error handling
type DynamoDBStreamProcessor struct {
	handler DynamoDBStreamHandler
	logger  *zap.Logger
	name    string
}

// NewDynamoDBStreamProcessor creates a new DynamoDB stream processor
func NewDynamoDBStreamProcessor(name string, handler DynamoDBStreamHandler, logger *zap.Logger) *DynamoDBStreamProcessor {
	return &DynamoDBStreamProcessor{
		handler: handler,
		logger:  logger,
		name:    name,
	}
}

// RegisterDynamoDBStream registers a DynamoDB stream handler with a Lift app
func RegisterDynamoDBStream(app *lift.App, processor *DynamoDBStreamProcessor) {
	// DynamoDB streams don't have native Lift support, so we handle the raw Lambda event
	// This is called directly by lambda.Start(app.HandleRequest)
	_ = app.Handle("POST", "/", func(ctx *lift.Context) error {
		// Extract the raw event from context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no DynamoDB event in request", 400)
		}

		// Try to cast directly first
		if event, ok := ctx.Request.RawEvent.(events.DynamoDBEvent); ok {
			return processor.ProcessEvent(ctx, event)
		}

		// If direct cast fails, try JSON marshaling
		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return lift.NewLiftError("EVENT_MARSHAL_ERROR", "failed to marshal raw event", 500).WithCause(err)
		}

		var event events.DynamoDBEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse DynamoDB event", 500).WithCause(err)
		}

		return processor.ProcessEvent(ctx, event)
	})
}

// ProcessEvent processes a DynamoDB stream event with proper logging and error handling
func (dsp *DynamoDBStreamProcessor) ProcessEvent(ctx *lift.Context, event events.DynamoDBEvent) error {
	return ProcessEventWithTiming(ctx, ProcessEventConfig{
		ProcessorName: dsp.name,
		RequestIDKey:  "requestID",
		RecordCount:   len(event.Records),
		Logger:        dsp.logger,
	}, func(ctx *lift.Context) error {
		return dsp.handler.HandleStream(ctx, event)
	})
}

// CreateDynamoDBStreamApp creates a standard Lift app configured for DynamoDB streams
func CreateDynamoDBStreamApp(name string, handler DynamoDBStreamHandler, logger *zap.Logger) *lift.App {
	app := lift.New()

	// Add standard middleware
	app.Use(RequestIDMiddleware(name))
	app.Use(LoggingMiddleware(logger))
	app.Use(RecoveryMiddleware(logger))

	// Create and register the processor
	processor := NewDynamoDBStreamProcessor(name, handler, logger)
	RegisterDynamoDBStream(app, processor)

	return app
}

// StartDynamoDBStreamLambda is a convenience function to start a DynamoDB stream Lambda
func StartDynamoDBStreamLambda(name string, handler DynamoDBStreamHandler, logger *zap.Logger) {
	processor := NewDynamoDBStreamProcessor(name, handler, logger)

	// Compose middleware stack manually since we're bypassing the Lift router
	middlewares := []lift.Middleware{
		RequestIDMiddleware(name),
		LoggingMiddleware(logger),
		RecoveryMiddleware(logger),
	}

	var finalHandler lift.Handler = lift.HandlerFunc(func(ctx *lift.Context) error {
		event, err := extractDynamoEvent(ctx.Request.RawEvent)
		if err != nil {
			return err
		}
		return processor.ProcessEvent(ctx, event)
	})

	// Apply middleware in reverse order (last added runs closest to the handler)
	for i := len(middlewares) - 1; i >= 0; i-- {
		finalHandler = middlewares[i](finalHandler)
	}

	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		req := lift.NewRequest(&adapters.Request{
			Method:      "POST",
			Path:        "/",
			TriggerType: adapters.TriggerAPIGateway,
			RawEvent:    event,
		})

		liftCtx := lift.NewContext(ctx, req)

		if lc, ok := lambdacontext.FromContext(ctx); ok {
			liftCtx.Set("requestID", lc.AwsRequestID)
		} else {
			liftCtx.Set("requestID", fmt.Sprintf("%s-%d", name, time.Now().UnixNano()))
		}

		return finalHandler.Handle(liftCtx)
	})
}

// extractDynamoEvent converts a raw event into a typed DynamoDB event
func extractDynamoEvent(raw any) (events.DynamoDBEvent, error) {
	switch evt := raw.(type) {
	case events.DynamoDBEvent:
		return evt, nil
	case *events.DynamoDBEvent:
		return *evt, nil
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return events.DynamoDBEvent{}, lift.NewLiftError("EVENT_MARSHAL_ERROR", "failed to marshal DynamoDB event", 500).WithCause(err)
		}

		var parsed events.DynamoDBEvent
		if err := json.Unmarshal(b, &parsed); err != nil {
			return events.DynamoDBEvent{}, lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse DynamoDB event", 500).WithCause(err)
		}
		return parsed, nil
	}
}
