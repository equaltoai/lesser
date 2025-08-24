// Package batch provides example Lambda handlers demonstrating SQS batch processing with DynamORM.
package batch

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// Example Lambda handlers demonstrating SQS batch processing

var (
	db        core.DB
	logger    *zap.Logger
	processor *SQSBatchProcessor
)

// init initializes the Lambda environment (runs once per container)
func init() {
	var err error

	// Initialize logger
	logger, err = zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	// Initialize DynamoDB connection using Lambda-optimized client
	// NOTE: In actual usage, import dynamorm from your main package and call:
	// db, err = dynamorm.GetLambdaClient(context.Background())
	// For this example, you need to initialize db using the DynamORM package directly
	// panic("Example file - please initialize 'db' using DynamORM in your actual implementation")

	// Initialize SQS batch processor
	processor = NewSQSBatchProcessor(db, SQSBatchProcessorConfig{
		Logger:       logger,
		Tracker:      nil, // Cost tracker can be provided if needed
		MaxBatchSize: 25,  // DynamoDB batch write limit
	})

	logger.Info("lambda_initialized",
		zap.String("processor_type", "sqs_batch"),
	)
}

// HandleGenericBatch handles generic batch operations from SQS
func HandleGenericBatch(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	logger.Info("batch_processing_started",
		zap.Int("message_count", len(event.Records)),
	)

	response, err := processor.ProcessBatch(ctx, event)
	if err != nil {
		logger.Error("batch_processing_failed", zap.Error(err))
		return response, err
	}

	logger.Info("batch_processing_completed",
		zap.Int("failed_messages", len(response.BatchItemFailures)),
	)

	return response, nil
}

// HandleTimelineBatch handles timeline entry batches from SQS
func HandleTimelineBatch(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	logger.Info("timeline_batch_processing_started",
		zap.Int("message_count", len(event.Records)),
	)

	response, err := processor.ProcessTimelineEntries(ctx, event)
	if err != nil {
		logger.Error("timeline_batch_processing_failed", zap.Error(err))
		return response, err
	}

	logger.Info("timeline_batch_processing_completed",
		zap.Int("failed_messages", len(response.BatchItemFailures)),
	)

	return response, nil
}

// HandleNotificationBatch handles notification batches from SQS
func HandleNotificationBatch(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	logger.Info("notification_batch_processing_started",
		zap.Int("message_count", len(event.Records)),
	)

	response, err := processor.ProcessNotifications(ctx, event)
	if err != nil {
		logger.Error("notification_batch_processing_failed", zap.Error(err))
		return response, err
	}

	logger.Info("notification_batch_processing_completed",
		zap.Int("failed_messages", len(response.BatchItemFailures)),
	)

	return response, nil
}

// Example of how to use the handlers in actual Lambda functions:

// TimelineBatchLambda - Lambda function for processing timeline batches
func TimelineBatchLambda() {
	lambda.Start(HandleTimelineBatch)
}

// NotificationBatchLambda - Lambda function for processing notification batches
func NotificationBatchLambda() {
	lambda.Start(HandleNotificationBatch)
}

// GenericBatchLambda - Lambda function for processing generic batches
func GenericBatchLambda() {
	lambda.Start(HandleGenericBatch)
}

// Example helper functions for creating SQS messages

// SendTimelineBatchToSQS demonstrates how to send timeline batches to SQS
// This would typically be called from other services when posting new statuses
func SendTimelineBatchToSQS(sqsURL string, followerIDs []string, statusID, authorID string, createdAt string) error {
	// Create the batch message
	message := struct {
		FollowerIDs []string `json:"follower_ids"`
		StatusID    string   `json:"status_id"`
		AuthorID    string   `json:"author_id"`
		CreatedAt   string   `json:"created_at"`
	}{
		FollowerIDs: followerIDs,
		StatusID:    statusID,
		AuthorID:    authorID,
		CreatedAt:   createdAt,
	}

	messageBody, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// Here you would use AWS SQS SDK to send the message
	// This is just a placeholder showing the message structure
	logger.Info("timeline_batch_message_created",
		zap.String("sqs_url", sqsURL),
		zap.Int("follower_count", len(followerIDs)),
		zap.String("status_id", statusID),
		zap.String("message_body", string(messageBody)),
	)

	return nil
}

// SendNotificationBatchToSQS demonstrates how to send notification batches to SQS
func SendNotificationBatchToSQS(sqsURL string, userIDs []string, statusID, authorID, notifType, targetType string) error {
	message := struct {
		UserIDs    []string `json:"user_ids"`
		StatusID   string   `json:"status_id"`
		AuthorID   string   `json:"author_id"`
		Type       string   `json:"type"`
		TargetType string   `json:"target_type"`
	}{
		UserIDs:    userIDs,
		StatusID:   statusID,
		AuthorID:   authorID,
		Type:       notifType,
		TargetType: targetType,
	}

	messageBody, err := json.Marshal(message)
	if err != nil {
		return err
	}

	logger.Info("notification_batch_message_created",
		zap.String("sqs_url", sqsURL),
		zap.Int("user_count", len(userIDs)),
		zap.String("type", notifType),
		zap.String("message_body", string(messageBody)),
	)

	return nil
}

// OptimalBatchSize calculates the optimal batch size based on payload size
func OptimalBatchSize(items []any) int {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return 0
	}

	// Estimate item size (this is simplified)
	sampleItem, _ := json.Marshal(items[0])
	estimatedItemSize := len(sampleItem)

	// DynamoDB has a 400KB item limit and 16MB request limit
	// Be conservative with batch sizes for large items
	if estimatedItemSize > 10000 { // 10KB per item
		return 10 // Smaller batches for large items
	} else if estimatedItemSize > 1000 { // 1KB per item
		return 20
	}

	return 25 // Maximum DynamoDB batch size
}

// HandleFailedBatch retries failed items with exponential backoff
func HandleFailedBatch(failedItems []events.SQSBatchItemFailure, originalEvent events.SQSEvent) {
	// Log failed items for monitoring
	for _, failure := range failedItems {
		// Find the original message
		for _, record := range originalEvent.Records {
			if record.MessageId == failure.ItemIdentifier {
				logger.Error("batch_item_failed",
					zap.String("message_id", failure.ItemIdentifier),
					zap.String("message_body", record.Body),
				)
				break
			}
		}
	}

	// Failed items will automatically be retried by SQS based on the queue's
	// redrive policy and maxReceiveCount settings
}
