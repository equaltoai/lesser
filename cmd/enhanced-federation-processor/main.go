// Package main implements the enhanced federation processor Lambda function
// for handling advanced federation workflows with retry and compression capabilities.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/common"
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
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Create SQS client
	sqsClient := sqs.NewFromConfig(lambdaCtx.AWSServices.Config)
	queueURL := os.Getenv("ENHANCED_RETRY_QUEUE_URL")

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(db, lambdaCtx.Config.DynamoTableName)
	deliveryService := federation.NewDeliveryService(federationStorage)

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
		return fmt.Errorf("failed to unmarshal retry message: %w", err)
	}

	h.logger.Info("Processing enhanced retry message",
		zap.String("delivery_id", retryMessage.DeliveryID),
		zap.String("activity_id", retryMessage.Activity.ID),
		zap.Int("retry_count", retryMessage.RetryCount),
		zap.String("activity_type", retryMessage.ActivityType))

	// Process the retry
	if err := h.retryProcessor.ProcessEnhancedRetry(ctx, &retryMessage); err != nil {
		return fmt.Errorf("failed to process enhanced retry: %w", err)
	}

	return nil
}

var (
	lambdaCtx *common.LambdaContext
	handler   *Handler
)

func init() {
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
	lambda.Start(handler.HandleSQSEvent)
}
