// Package main implements the enhanced federation processor Lambda function
// for handling advanced federation workflows with retry and compression capabilities.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/common"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"go.uber.org/zap"
)

// Handler processes SQS messages for enhanced federation retry
type Handler struct {
	retryProcessor *federation.EnhancedRetryProcessor
	logger         *zap.Logger
}

// NewHandler creates a new enhanced federation processor handler
func NewHandler(lambdaCtx *common.LambdaContext) (*Handler, error) {
	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), lambdaCtx.Config.Region)
	if err != nil {
		return nil, pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to initialize DynamORM")
	}

	// Create SQS client
	sqsClient := sqs.NewFromConfig(lambdaCtx.AWSServices.Config)
	queueURL := lambdaCtx.Config.EnhancedRetryQueueURL

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	deliveryService := federation.NewDeliveryService(federationStorage, lambdaCtx.Config)

	// Create enhanced retry processor
	retryProcessor := federation.NewEnhancedRetryProcessor(deliveryService, sqsClient, queueURL)

	return &Handler{
		retryProcessor: retryProcessor,
		logger:         lambdaCtx.Logger,
	}, nil
}

// HandleSQSEvent processes SQS messages containing enhanced retry requests
func (h *Handler) HandleSQSEvent(ctx context.Context, event events.SQSEvent) error {
	h.logger.Info("Processing enhanced retry SQS event",
		zap.Int("message_count", len(event.Records)))

	for _, record := range event.Records {
		if err := h.processMessage(ctx, record); err != nil {
			h.logger.Error("Failed to process SQS message",
				zap.String("message_id", record.MessageId),
				zap.Error(err))
			// Continue processing other messages
		}
	}

	return nil
}

// processMessage processes a single SQS message
func (h *Handler) processMessage(ctx context.Context, record events.SQSMessage) error {
	// Check message attributes to determine message type
	deliveryType := ""
	if attr, ok := record.MessageAttributes["delivery_type"]; ok && attr.StringValue != nil {
		deliveryType = *attr.StringValue
	}

	if deliveryType != "enhanced_retry" {
		h.logger.Warn("Unknown delivery type, skipping",
			zap.String("delivery_type", deliveryType),
			zap.String("message_id", record.MessageId))
		return nil
	}

	// Parse the enhanced retry message
	var retryMessage federation.EnhancedRetryMessage
	if err := json.Unmarshal([]byte(record.Body), &retryMessage); err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeEventProcessingFailed, pkgErrors.CategoryLambda, "Failed to unmarshal retry message")
	}

	h.logger.Info("Processing enhanced retry message",
		zap.String("delivery_id", retryMessage.DeliveryID),
		zap.String("activity_id", retryMessage.Activity.ID),
		zap.Int("retry_count", retryMessage.RetryCount),
		zap.String("activity_type", retryMessage.ActivityType))

	// Process the retry
	if err := h.retryProcessor.ProcessEnhancedRetry(ctx, &retryMessage); err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to process enhanced retry")
	}

	return nil
}

var (
	lambdaCtx *common.LambdaContext
	handler   *Handler
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	// Initialize Lambda with federation processing configuration
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName:        "enhanced-federation-processor",
		LambdaType:         common.LambdaTypeFederation,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableTracing:      true,
		EnableHealthCheck:  false,
		EnableCostTracking: true,
		RequestTimeout:     60 * time.Second, // Longer timeout for federation processing
		RetryMaxAttempts:   3,
	})

	// Initialize handler
	var err error
	handler, err = NewHandler(lambdaCtx)
	if err != nil {
		lambdaCtx.Logger.Fatal("Failed to create handler", zap.Error(err))
	}
}

func main() {
	lambda.Start(func(ctx context.Context, event events.SQSEvent) (err error) {
		defer func() {
			if r := recover(); r != nil {
				lambdaCtx.Logger.Error("panic in enhanced federation processor handler",
					zap.Any("panic", r),
					zap.Stack("stack"))
				err = fmt.Errorf("panic recovered in enhanced-federation-processor: %v", r)
			}
		}()

		return handler.HandleSQSEvent(ctx, event)
	})
}
