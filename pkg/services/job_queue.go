package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/common"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"go.uber.org/zap"
)

// Priority constants
const (
	priorityNormal = "normal"
	priorityHigh   = "high"
)

// JobQueueServiceInterface defines the interface for job queue operations
type JobQueueServiceInterface interface {
	QueueImportJob(ctx context.Context, msg ImportJobMessage) error
	QueueExportJob(ctx context.Context, msg ExportJobMessage) error
	QueueMediaJob(ctx context.Context, msg MediaJobMessage) error
	QueueScheduledJob(ctx context.Context, msg ScheduledJobMessage) error
	QueueActivityJob(ctx context.Context, msg ActivityJobMessage) error
	QueueDelayedJob(ctx context.Context, queueName string, messageBody interface{}, delaySeconds int32) error
}

type sqsAPI interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	SendMessageBatch(ctx context.Context, params *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

// JobQueueService handles job queueing via SQS
type JobQueueService struct {
	sqsClient sqsAPI
	queueUrls map[string]string
	logger    *zap.Logger
}

// ImportJobMessage represents a message for import processing
type ImportJobMessage struct {
	ImportID  string         `json:"import_id"`
	Username  string         `json:"username"`
	Type      string         `json:"type"`
	Mode      string         `json:"mode"`
	S3Key     string         `json:"s3_key"`
	Timestamp int64          `json:"timestamp"`
	Options   map[string]any `json:"options,omitempty"`
}

// ExportJobMessage represents a message for export processing
type ExportJobMessage struct {
	ExportID     string           `json:"export_id"`
	Username     string           `json:"username"`
	Type         string           `json:"type"`
	Format       string           `json:"format"`
	IncludeMedia bool             `json:"include_media"`
	DateRange    *ExportDateRange `json:"date_range,omitempty"`
	Options      map[string]any   `json:"options,omitempty"`
	Timestamp    int64            `json:"timestamp"`
}

// ExportDateRange for job messages
type ExportDateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MediaJobMessage represents a message for media processing
type MediaJobMessage struct {
	JobID     string `json:"job_id"`
	MediaID   string `json:"media_id"`
	Username  string `json:"username"`
	Timestamp int64  `json:"timestamp"`
}

// ScheduledJobMessage represents a message for scheduled status publishing
type ScheduledJobMessage struct {
	ScheduledStatusID string    `json:"scheduled_status_id"`
	Username          string    `json:"username"`
	ScheduledAt       time.Time `json:"scheduled_at"`
	Timestamp         int64     `json:"timestamp"`
}

// ActivityJobMessage represents a message for federation activity delivery
type ActivityJobMessage struct {
	ActivityID   string                 `json:"activity_id"`
	ActivityData map[string]interface{} `json:"activity_data"`
	ActorID      string                 `json:"actor_id"`
	Recipients   []string               `json:"recipients"`
	Priority     string                 `json:"priority"` // "high", "normal", "low"
	Timestamp    int64                  `json:"timestamp"`
}

// NewJobQueueService creates a new job queue service
func NewJobQueueService(cfg *appConfig.Config, logger *zap.Logger) (*JobQueueService, error) {
	ctx := context.Background()

	// Load AWS configuration
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, ErrAWSConfigLoad
	}

	// Create SQS client
	sqsClient := sqs.NewFromConfig(awsConfig)

	// Initialize queue URLs from configuration
	queueUrls := map[string]string{
		"import-processing":    cfg.ImportQueueURL,
		"export-generation":    cfg.ExportQueueURL,
		"media-processing":     cfg.MediaQueueURL,
		"scheduled-publishing": cfg.ScheduledQueueURL,
		"federation-delivery":  cfg.FederationQueueURL,
	}

	// Validate required queue URLs
	for queueName, queueURL := range queueUrls {
		if err := common.ValidateRequiredParam(queueName+"_url", queueURL); err != nil {
			logger.Warn("queue URL not configured", zap.String("queue", queueName))
		}
	}

	return &JobQueueService{
		sqsClient: sqsClient,
		queueUrls: queueUrls,
		logger:    logger,
	}, nil
}

// QueueImportJob queues an import job for processing
func (q *JobQueueService) QueueImportJob(ctx context.Context, msg ImportJobMessage) error {
	queueURL := q.queueUrls["import-processing"]
	if err := common.ValidateRequiredParam("import_queue_url", queueURL); err != nil {
		q.logger.Warn("import queue URL not configured, skipping queue operation")
		return nil // Don't fail the request if queue is not configured
	}

	// Set timestamp if not provided
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	// Serialize message to JSON
	messageBody, err := json.Marshal(msg)
	if err != nil {
		return ErrImportJobSerialization
	}

	// Send message to SQS
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"Type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("ImportJob"),
			},
			"Username": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Username),
			},
			"ImportType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Type),
			},
		},
		// Delay processing by 5 seconds to allow database consistency
		DelaySeconds: 5,
	}

	_, err = q.sqsClient.SendMessage(ctx, input)
	if err != nil {
		q.logger.Error("failed to send import job to queue",
			zap.String("import_id", msg.ImportID),
			zap.String("queue_url", queueURL),
			zap.Error(err))
		return ErrImportJobQueue
	}

	q.logger.Info("queued import job",
		zap.String("import_id", msg.ImportID),
		zap.String("username", msg.Username),
		zap.String("type", msg.Type))

	return nil
}

// QueueExportJob queues an export job for processing
func (q *JobQueueService) QueueExportJob(ctx context.Context, msg ExportJobMessage) error {
	queueURL := q.queueUrls["export-generation"]
	if err := common.ValidateRequiredParam("export_queue_url", queueURL); err != nil {
		q.logger.Warn("export queue URL not configured, skipping queue operation")
		return nil // Don't fail the request if queue is not configured
	}

	// Set timestamp if not provided
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	// Serialize message to JSON
	messageBody, err := json.Marshal(msg)
	if err != nil {
		return ErrExportJobSerialization
	}

	// Send message to SQS
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"Type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("ExportJob"),
			},
			"Username": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Username),
			},
			"ExportType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Type),
			},
			"Format": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Format),
			},
		},
		// Delay processing by 5 seconds to allow database consistency
		DelaySeconds: 5,
	}

	_, err = q.sqsClient.SendMessage(ctx, input)
	if err != nil {
		q.logger.Error("failed to send export job to queue",
			zap.String("export_id", msg.ExportID),
			zap.String("queue_url", queueURL),
			zap.Error(err))
		return ErrExportJobQueue
	}

	q.logger.Info("queued export job",
		zap.String("export_id", msg.ExportID),
		zap.String("username", msg.Username),
		zap.String("type", msg.Type),
		zap.String("format", msg.Format))

	return nil
}

// QueueMediaJob queues a media processing job
func (q *JobQueueService) QueueMediaJob(ctx context.Context, msg MediaJobMessage) error {
	queueURL := q.queueUrls["media-processing"]
	if err := common.ValidateRequiredParam("media_queue_url", queueURL); err != nil {
		q.logger.Warn("media queue URL not configured, skipping queue operation")
		return nil // Don't fail the request if queue is not configured
	}

	// Set timestamp if not provided
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	// Serialize message to JSON
	messageBody, err := json.Marshal(msg)
	if err != nil {
		return ErrMediaJobSerialization
	}

	// Send message to SQS
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"Type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("MediaJob"),
			},
			"Username": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Username),
			},
			"MediaID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.MediaID),
			},
		},
		// Delay processing by 2 seconds to allow database consistency
		DelaySeconds: 2,
	}

	_, err = q.sqsClient.SendMessage(ctx, input)
	if err != nil {
		q.logger.Error("failed to send media job to queue",
			zap.String("job_id", msg.JobID),
			zap.String("media_id", msg.MediaID),
			zap.String("queue_url", queueURL),
			zap.Error(err))
		return ErrMediaJobQueue
	}

	q.logger.Info("queued media processing job",
		zap.String("job_id", msg.JobID),
		zap.String("media_id", msg.MediaID),
		zap.String("username", msg.Username))

	return nil
}

// QueueScheduledJob queues a scheduled status publishing job
func (q *JobQueueService) QueueScheduledJob(ctx context.Context, msg ScheduledJobMessage) error {
	queueURL := q.queueUrls["scheduled-publishing"]
	if err := common.ValidateRequiredParam("scheduled_queue_url", queueURL); err != nil {
		q.logger.Warn("scheduled publishing queue URL not configured, skipping queue operation")
		return nil // Don't fail the request if queue is not configured
	}

	// Set timestamp if not provided
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	// Calculate delay until scheduled time
	now := time.Now()
	delaySeconds := int32(msg.ScheduledAt.Sub(now).Seconds())

	// If scheduled time is in the past or very soon, set minimal delay
	if delaySeconds < 5 {
		delaySeconds = 5
	}

	// SQS maximum delay is 15 minutes (900 seconds)
	// For longer delays, we'll use the maximum and handle rescheduling in the processor
	if delaySeconds > 900 {
		delaySeconds = 900
	}

	// Serialize message to JSON
	messageBody, err := json.Marshal(msg)
	if err != nil {
		return ErrScheduledJobSerialization
	}

	// Send message to SQS
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"Type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("ScheduledJob"),
			},
			"Username": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Username),
			},
			"ScheduledStatusID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.ScheduledStatusID),
			},
		},
		DelaySeconds: delaySeconds,
	}

	_, err = q.sqsClient.SendMessage(ctx, input)
	if err != nil {
		q.logger.Error("failed to send scheduled job to queue",
			zap.String("scheduled_status_id", msg.ScheduledStatusID),
			zap.String("queue_url", queueURL),
			zap.Time("scheduled_at", msg.ScheduledAt),
			zap.Int32("delay_seconds", delaySeconds),
			zap.Error(err))
		return ErrScheduledJobQueue
	}

	q.logger.Info("queued scheduled publishing job",
		zap.String("scheduled_status_id", msg.ScheduledStatusID),
		zap.String("username", msg.Username),
		zap.Time("scheduled_at", msg.ScheduledAt),
		zap.Int32("delay_seconds", delaySeconds))

	return nil
}

// QueueActivityJob queues a federation activity delivery job
func (q *JobQueueService) QueueActivityJob(ctx context.Context, msg ActivityJobMessage) error {
	queueURL := q.queueUrls["federation-delivery"]
	if err := common.ValidateRequiredParam("federation_delivery_queue_url", queueURL); err != nil {
		q.logger.Warn("federation delivery queue URL not configured, skipping queue operation")
		return nil // Don't fail the request if queue is not configured
	}

	// Set timestamp if not provided
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	// Set default priority if not provided
	if err := common.ValidateRequiredParam("priority", msg.Priority); err != nil {
		msg.Priority = priorityNormal
	}

	// Serialize message to JSON
	messageBody, err := json.Marshal(msg)
	if err != nil {
		return ErrActivityJobSerialization
	}

	// Calculate delay based on priority
	var delaySeconds int32
	switch msg.Priority {
	case priorityHigh:
		delaySeconds = 0 // Immediate processing
	case priorityNormal:
		delaySeconds = 5 // Small delay for batch efficiency
	case "low":
		delaySeconds = 30 // Longer delay for non-urgent activities
	default:
		delaySeconds = 5
	}

	// Send message to SQS
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"Type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("ActivityJob"),
			},
			"ActorID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.ActorID),
			},
			"Priority": {
				DataType:    aws.String("String"),
				StringValue: aws.String(msg.Priority),
			},
		},
		DelaySeconds: delaySeconds,
	}

	_, err = q.sqsClient.SendMessage(ctx, input)
	if err != nil {
		q.logger.Error("failed to send activity job to queue",
			zap.String("activity_id", msg.ActivityID),
			zap.String("queue_url", queueURL),
			zap.String("priority", msg.Priority),
			zap.Int32("delay_seconds", delaySeconds),
			zap.Error(err))
		return ErrActivityJobQueue
	}

	q.logger.Info("queued federation activity job",
		zap.String("activity_id", msg.ActivityID),
		zap.String("actor_id", msg.ActorID),
		zap.String("priority", msg.Priority),
		zap.Int("recipients_count", len(msg.Recipients)),
		zap.Int32("delay_seconds", delaySeconds))

	return nil
}

// QueueDelayedJob queues a job with a specific delay
func (q *JobQueueService) QueueDelayedJob(ctx context.Context, queueName string, messageBody interface{}, delaySeconds int32) error {
	queueURL := q.queueUrls[queueName]
	if err := common.ValidateRequiredParam(queueName+"_url", queueURL); err != nil {
		return ErrQueueURLNotConfigured
	}

	// Serialize message to JSON
	bodyBytes, err := json.Marshal(messageBody)
	if err != nil {
		return ErrMessageSerialization
	}

	// Send message to SQS
	input := &sqs.SendMessageInput{
		QueueUrl:     aws.String(queueURL),
		MessageBody:  aws.String(string(bodyBytes)),
		DelaySeconds: delaySeconds,
	}

	_, err = q.sqsClient.SendMessage(ctx, input)
	if err != nil {
		q.logger.Error("failed to send delayed job to queue",
			zap.String("queue", queueName),
			zap.Int32("delay_seconds", delaySeconds),
			zap.Error(err))
		return ErrDelayedJobQueue
	}

	return nil
}

// SendBatchMessages sends multiple messages to a queue in a single batch operation
func (q *JobQueueService) SendBatchMessages(ctx context.Context, queueName string, messages []interface{}) error {
	queueURL := q.queueUrls[queueName]
	if err := common.ValidateRequiredParam(queueName+"_url", queueURL); err != nil {
		return ErrQueueURLNotConfigured
	}

	// Convert messages to SQS batch entries
	entries := make([]types.SendMessageBatchRequestEntry, 0, len(messages))
	for i, msg := range messages {
		bodyBytes, err := json.Marshal(msg)
		if err != nil {
			return ErrMessageSerialization
		}

		entries = append(entries, types.SendMessageBatchRequestEntry{
			Id:          aws.String(fmt.Sprintf("msg-%d", i)),
			MessageBody: aws.String(string(bodyBytes)),
		})

		// SQS batch limit is 10 messages
		if len(entries) >= 10 {
			if err := q.sendBatch(ctx, queueURL, entries); err != nil {
				return err
			}
			entries = entries[:0] // Clear the slice
		}
	}

	// Send remaining messages
	if len(entries) > 0 {
		return q.sendBatch(ctx, queueURL, entries)
	}

	return nil
}

// sendBatch sends a batch of messages to SQS
func (q *JobQueueService) sendBatch(ctx context.Context, queueURL string, entries []types.SendMessageBatchRequestEntry) error {
	input := &sqs.SendMessageBatchInput{
		QueueUrl: aws.String(queueURL),
		Entries:  entries,
	}

	result, err := q.sqsClient.SendMessageBatch(ctx, input)
	if err != nil {
		return ErrBatchMessageSend
	}

	// Log any failed messages
	if len(result.Failed) > 0 {
		for _, failed := range result.Failed {
			q.logger.Error("failed to send message in batch",
				zap.String("message_id", aws.ToString(failed.Id)),
				zap.String("code", aws.ToString(failed.Code)),
				zap.String("message", aws.ToString(failed.Message)))
		}
		return ErrBatchOperation
	}

	return nil
}

// GetQueueAttributes gets attributes for a queue (useful for monitoring)
func (q *JobQueueService) GetQueueAttributes(ctx context.Context, queueName string) (map[string]string, error) {
	queueURL := q.queueUrls[queueName]
	if err := common.ValidateRequiredParam(queueName+"_url", queueURL); err != nil {
		return nil, ErrQueueURLNotConfigured
	}

	input := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameApproximateNumberOfMessages,
			types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			types.QueueAttributeNameApproximateNumberOfMessagesDelayed,
		},
	}

	result, err := q.sqsClient.GetQueueAttributes(ctx, input)
	if err != nil {
		return nil, ErrQueueAttributeQuery
	}

	return result.Attributes, nil
}
