package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.uber.org/zap"
)

// AWSQueueService implements QueueService using AWS SQS
type AWSQueueService struct {
	client   *sqs.Client
	queueURL string
	logger   *zap.Logger
}

// QueueMessage represents a message structure for import/export jobs
type QueueMessage struct {
	JobType string `json:"job_type"` // "export" or "import"
	JobID   string `json:"job_id"`
	UserID  string `json:"user_id,omitempty"`
}

// NewAWSQueueService creates a new AWS SQS-based queue service
func NewAWSQueueService(ctx context.Context, logger *zap.Logger) (*AWSQueueService, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	// Check for queue URL first - if not set, we can't function
	queueURL := os.Getenv("IMPORT_EXPORT_QUEUE_URL")
	if queueURL == "" {
		return nil, fmt.Errorf("IMPORT_EXPORT_QUEUE_URL environment variable is required")
	}

	// Load AWS configuration with retry and timeout
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(os.Getenv("AWS_REGION")),
		config.WithRetryMaxAttempts(3),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := sqs.NewFromConfig(cfg)

	// Test connectivity by getting queue attributes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameApproximateNumberOfMessages,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SQS queue %s: %w", queueURL, err)
	}

	return &AWSQueueService{
		client:   client,
		queueURL: queueURL,
		logger:   logger,
	}, nil
}

// QueueExportJob queues an export job for asynchronous processing
func (s *AWSQueueService) QueueExportJob(ctx context.Context, exportID string) error {
	if exportID == "" {
		return fmt.Errorf("export ID cannot be empty")
	}

	message := QueueMessage{
		JobType: "export",
		JobID:   exportID,
	}

	messageBody, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal export message: %w", err)
	}

	// Add timeout to the context if not already present
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(s.queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"JobType": {
				DataType:    aws.String("String"),
				StringValue: aws.String("export"),
			},
			"JobID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(exportID),
			},
			"Timestamp": {
				DataType:    aws.String("Number"),
				StringValue: aws.String(fmt.Sprintf("%d", time.Now().Unix())),
			},
		},
		// Add message deduplication for FIFO queues if needed
		MessageGroupId: aws.String("export-jobs"),
	}

	result, err := s.client.SendMessage(ctx, input)
	if err != nil {
		s.logger.Error("failed to send export message to SQS",
			zap.String("export_id", exportID),
			zap.String("queue_url", s.queueURL),
			zap.Error(err))
		return fmt.Errorf("failed to send export message to SQS: %w", err)
	}

	s.logger.Info("export job queued successfully",
		zap.String("export_id", exportID),
		zap.String("message_id", aws.ToString(result.MessageId)),
		zap.String("queue_url", s.queueURL))

	return nil
}

// QueueImportJob queues an import job for asynchronous processing
func (s *AWSQueueService) QueueImportJob(ctx context.Context, importID string) error {
	if importID == "" {
		return fmt.Errorf("import ID cannot be empty")
	}

	message := QueueMessage{
		JobType: "import",
		JobID:   importID,
	}

	messageBody, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal import message: %w", err)
	}

	// Add timeout to the context if not already present
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(s.queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"JobType": {
				DataType:    aws.String("String"),
				StringValue: aws.String("import"),
			},
			"JobID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(importID),
			},
			"Timestamp": {
				DataType:    aws.String("Number"),
				StringValue: aws.String(fmt.Sprintf("%d", time.Now().Unix())),
			},
		},
		// Add message deduplication for FIFO queues if needed
		MessageGroupId: aws.String("import-jobs"),
	}

	result, err := s.client.SendMessage(ctx, input)
	if err != nil {
		s.logger.Error("failed to send import message to SQS",
			zap.String("import_id", importID),
			zap.String("queue_url", s.queueURL),
			zap.Error(err))
		return fmt.Errorf("failed to send import message to SQS: %w", err)
	}

	s.logger.Info("import job queued successfully",
		zap.String("import_id", importID),
		zap.String("message_id", aws.ToString(result.MessageId)),
		zap.String("queue_url", s.queueURL))

	return nil
}