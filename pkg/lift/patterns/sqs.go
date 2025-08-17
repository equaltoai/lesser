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

// SQSHandler is the interface that services must implement to handle SQS events
type SQSHandler interface {
	HandleSQS(ctx *lift.Context, event events.SQSEvent) error
}

// SQSProcessor wraps a handler with Lift middleware and error handling
type SQSProcessor struct {
	handler   SQSHandler
	logger    *zap.Logger
	queueName string
}

// NewSQSProcessor creates a new SQS processor
func NewSQSProcessor(queueName string, handler SQSHandler, logger *zap.Logger) *SQSProcessor {
	return &SQSProcessor{
		handler:   handler,
		logger:    logger,
		queueName: queueName,
	}
}

// RegisterSQS registers an SQS handler with a Lift app using the native SQS support
func RegisterSQS(app *lift.App, processor *SQSProcessor) {
	_ = app.SQS(processor.queueName, func(ctx *lift.Context) error {
		// Extract SQS event from Lift context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
		}

		// Try direct cast first
		if event, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
			return processor.ProcessEvent(ctx, event)
		}

		// Fall back to JSON marshaling
		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return lift.NewLiftError("EVENT_MARSHAL_ERROR", "failed to marshal raw event", 500).WithCause(err)
		}

		var event events.SQSEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500).WithCause(err)
		}

		return processor.ProcessEvent(ctx, event)
	})
}

// ProcessEvent processes an SQS event with proper logging and error handling
func (sp *SQSProcessor) ProcessEvent(ctx *lift.Context, event events.SQSEvent) error {
	start := time.Now()
	requestID := ctx.GetRequestID()
	if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
		requestID = fmt.Sprintf("%s-%d", sp.queueName, time.Now().UnixNano())
		ctx.Set("requestID", requestID)
	}

	sp.logger.Info("processing SQS batch",
		zap.String("queue", sp.queueName),
		zap.String("request_id", requestID),
		zap.Int("message_count", len(event.Records)),
	)

	// Call the actual handler
	err := sp.handler.HandleSQS(ctx, event)

	duration := time.Since(start)
	if err != nil {
		sp.logger.Error("failed to process SQS batch",
			zap.String("queue", sp.queueName),
			zap.String("request_id", requestID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return err
	}

	sp.logger.Info("successfully processed SQS batch",
		zap.String("queue", sp.queueName),
		zap.String("request_id", requestID),
		zap.Int("message_count", len(event.Records)),
		zap.Duration("duration", duration),
	)

	return nil
}

// CreateSQSApp creates a standard Lift app configured for SQS
func CreateSQSApp(queueName string, handler SQSHandler, logger *zap.Logger) *lift.App {
	app := lift.New()

	// Add standard middleware
	app.Use(RequestIDMiddleware(queueName))
	app.Use(LoggingMiddleware(logger))
	app.Use(RecoveryMiddleware(logger))

	// Create and register the processor
	processor := NewSQSProcessor(queueName, handler, logger)
	RegisterSQS(app, processor)

	return app
}

// StartSQSLambda is a convenience function to start an SQS Lambda
func StartSQSLambda(queueName string, handler SQSHandler, logger *zap.Logger) {
	app := CreateSQSApp(queueName, handler, logger)
	lambda.Start(app.HandleRequest)
}

// SQSBatchError represents errors from processing a batch of SQS messages
type SQSBatchError struct {
	Failed    []string // Message IDs that failed
	Succeeded []string // Message IDs that succeeded
	Errors    []error  // Errors for each failed message
}

func (e *SQSBatchError) Error() string {
	return fmt.Sprintf("partial batch failure: %d of %d messages failed", len(e.Failed), len(e.Failed)+len(e.Succeeded))
}

// ProcessSQSBatch is a helper for processing SQS messages with partial batch failure support
func ProcessSQSBatch(ctx *lift.Context, event events.SQSEvent, processor func(*lift.Context, events.SQSMessage) error) error {
	var batchError SQSBatchError

	for _, msg := range event.Records {
		err := processor(ctx, msg)
		if err != nil {
			batchError.Failed = append(batchError.Failed, msg.MessageId)
			batchError.Errors = append(batchError.Errors, err)
		} else {
			batchError.Succeeded = append(batchError.Succeeded, msg.MessageId)
		}
	}

	if len(batchError.Failed) > 0 {
		return &batchError
	}

	return nil
}
