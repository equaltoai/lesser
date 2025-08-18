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
	"github.com/equaltoai/lesser/pkg/common"
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
	if err := common.ValidateRequiredParam("queueURL", queueURL); err != nil {
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

// queueJobGeneric handles the common pattern for queuing jobs
func (s *AWSQueueService) queueJobGeneric(ctx context.Context, jobID, jobType string) error {
	paramName := jobType + "ID"
	if err := common.ValidateRequiredParam(paramName, jobID); err != nil {
		return err
	}

	message := QueueMessage{
		JobType: jobType,
		JobID:   jobID,
	}

	messageBody, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal %s message: %w", jobType, err)
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
				StringValue: aws.String(jobType),
			},
			"JobID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(jobID),
			},
			"Timestamp": {
				DataType:    aws.String("Number"),
				StringValue: aws.String(fmt.Sprintf("%d", time.Now().Unix())),
			},
		},
		// Add message deduplication for FIFO queues if needed
		MessageGroupId: aws.String(jobType + "-jobs"),
	}

	result, err := s.client.SendMessage(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("failed to send %s message to SQS", jobType),
			zap.String(jobType+"_id", jobID),
			zap.String("queue_url", s.queueURL),
			zap.Error(err))
		return fmt.Errorf("failed to send %s message to SQS: %w", jobType, err)
	}

	s.logger.Info(fmt.Sprintf("%s job queued successfully", jobType),
		zap.String(jobType+"_id", jobID),
		zap.String("message_id", aws.ToString(result.MessageId)),
		zap.String("queue_url", s.queueURL))

	return nil
}

// QueueExportJob queues an export job for asynchronous processing
func (s *AWSQueueService) QueueExportJob(ctx context.Context, exportID string) error {
	return s.queueJobGeneric(ctx, exportID, "export")
}

// QueueImportJob queues an import job for asynchronous processing
func (s *AWSQueueService) QueueImportJob(ctx context.Context, importID string) error {
	return s.queueJobGeneric(ctx, importID, "import")
}