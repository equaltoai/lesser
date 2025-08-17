// Package patterns provides DynamoDB stream processing patterns and handlers for the Lift framework.
package patterns

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
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
	start := time.Now()
	requestID := ctx.GetRequestID()
	if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
		requestID = fmt.Sprintf("%s-%d", dsp.name, time.Now().UnixNano())
		ctx.Set("requestID", requestID)
	}

	dsp.logger.Info("processing DynamoDB stream batch",
		zap.String("processor", dsp.name),
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	// Call the actual handler
	err := dsp.handler.HandleStream(ctx, event)

	duration := time.Since(start)
	if err != nil {
		dsp.logger.Error("failed to process DynamoDB stream batch",
			zap.String("processor", dsp.name),
			zap.String("request_id", requestID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return err
	}

	dsp.logger.Info("successfully processed DynamoDB stream batch",
		zap.String("processor", dsp.name),
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
		zap.Duration("duration", duration),
	)

	return nil
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
	app := CreateDynamoDBStreamApp(name, handler, logger)
	lambda.Start(app.HandleRequest)
}
