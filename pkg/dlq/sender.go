package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// DLQSender handles sending failed messages to dead letter queues
//
//nolint:revive // DLQ prefix clarifies this is Dead Letter Queue sender
type DLQSender struct {
	sqsClient *sqs.Client
	logger    *zap.Logger
	queueURLs map[string]string // Cache for queue URLs
}

// NewDLQSender creates a new DLQ sender
func NewDLQSender(logger *zap.Logger) *DLQSender {
	return &DLQSender{
		logger:    logger,
		queueURLs: make(map[string]string),
	}
}

// InitializeAWSClients initializes AWS clients
func (s *DLQSender) InitializeAWSClients(ctx context.Context) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	s.sqsClient = sqs.NewFromConfig(cfg)
	return nil
}

// SendFailedMessage sends a failed message to the appropriate DLQ
func (s *DLQSender) SendFailedMessage(ctx context.Context, service string, originalMessage events.SQSMessage, processingError error) error {
	// Create DLQ message
	dlqMessage := s.createDLQMessage(service, originalMessage, processingError)

	// Get DLQ name for service
	dlqQueueName := s.getDLQQueueName(service)

	// Get queue URL
	queueURL, err := s.getQueueURL(ctx, dlqQueueName)
	if err != nil {
		s.logger.Error("failed to get DLQ queue URL",
			zap.String("service", service),
			zap.String("queue_name", dlqQueueName),
			zap.Error(err),
		)
		// Don't fail the original processing if DLQ sending fails
		return nil
	}

	// Send to DLQ
	if err := s.sendToDLQ(ctx, queueURL, dlqMessage); err != nil {
		s.logger.Error("failed to send message to DLQ",
			zap.String("service", service),
			zap.String("message_id", originalMessage.MessageId),
			zap.Error(err),
		)
		// Don't fail the original processing if DLQ sending fails
		return nil
	}

	s.logger.Info("sent failed message to DLQ",
		zap.String("service", service),
		zap.String("message_id", originalMessage.MessageId),
		zap.String("dlq_queue", dlqQueueName),
	)

	return nil
}

// SendBatchFailedMessages sends multiple failed messages to DLQ
func (s *DLQSender) SendBatchFailedMessages(ctx context.Context, service string, failures []ProcessingFailure) error {
	if err := common.ValidateSliceNotEmpty("failures", failures); err != nil {
		return nil // Empty slice is not an error for this function
	}

	dlqQueueName := s.getDLQQueueName(service)
	queueURL, err := s.getQueueURL(ctx, dlqQueueName)
	if err != nil {
		s.logger.Error("failed to get DLQ queue URL for batch",
			zap.String("service", service),
			zap.String("queue_name", dlqQueueName),
			zap.Error(err),
		)
		return nil
	}

	// Send messages in batch (up to 10 at a time due to SQS limits)
	const batchSize = 10
	for i := 0; i < len(failures); i += batchSize {
		end := i + batchSize
		if end > len(failures) {
			end = len(failures)
		}

		batch := failures[i:end]
		if err := s.sendBatchToDLQ(ctx, queueURL, service, batch); err != nil {
			s.logger.Error("failed to send batch to DLQ",
				zap.String("service", service),
				zap.Int("batch_size", len(batch)),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("sent batch failures to DLQ",
		zap.String("service", service),
		zap.Int("failure_count", len(failures)),
	)

	return nil
}

// createDLQMessage creates a DLQ message from the failed processing
func (s *DLQSender) createDLQMessage(service string, originalMessage events.SQSMessage, processingError error) *DLQFailureMessage {
	// Extract function context
	functionName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	logGroup := os.Getenv("AWS_LAMBDA_LOG_GROUP_NAME")
	logStream := os.Getenv("AWS_LAMBDA_LOG_STREAM_NAME")

	// Classify error
	errorClassifier := NewErrorClassifier()
	errorInfo := errorClassifier.ClassifyError(processingError.Error(), service)

	// Convert message attributes
	attributes := make(map[string]string)
	for key, value := range originalMessage.MessageAttributes {
		if value.StringValue != nil {
			attributes[key] = *value.StringValue
		}
	}

	return &DLQFailureMessage{
		OriginalMessageID: originalMessage.MessageId,
		Service:           service,
		QueueName:         extractQueueNameFromARN(originalMessage.EventSourceARN),
		MessageBody:       originalMessage.Body,
		MessageAttributes: attributes,
		ErrorInfo:         errorInfo,
		ProcessingContext: ProcessingContext{
			FunctionName: functionName,
			LogGroup:     logGroup,
			LogStream:    logStream,
			Timestamp:    time.Now(),
		},
		RetryCount: s.getRetryCount(originalMessage),
	}
}

// getRetryCount extracts retry count from message attributes or SQS attributes
func (s *DLQSender) getRetryCount(message events.SQSMessage) int {
	// Check SQS receive count
	if receiveCount, exists := message.Attributes["ApproximateReceiveCount"]; exists {
		// Parse receive count as retry count
		if count := parseIntSafe(receiveCount); count > 0 {
			return count - 1 // First receive is not a retry
		}
	}

	// Check custom retry attribute
	if retryAttr, exists := message.MessageAttributes["RetryCount"]; exists {
		if retryAttr.StringValue != nil {
			return parseIntSafe(*retryAttr.StringValue)
		}
	}

	return 0
}

// getDLQQueueName returns the DLQ queue name for a service
func (s *DLQSender) getDLQQueueName(service string) string {
	// Standard naming convention: service-name-dlq
	return fmt.Sprintf("%s-dlq", service)
}

// getQueueURL gets the URL for a queue name (with caching)
func (s *DLQSender) getQueueURL(ctx context.Context, queueName string) (string, error) {
	// Check cache first
	if url, exists := s.queueURLs[queueName]; exists {
		return url, nil
	}

	// Get queue URL from SQS
	result, err := s.sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get queue URL: %w", err)
	}

	// Cache the URL
	s.queueURLs[queueName] = *result.QueueUrl
	return *result.QueueUrl, nil
}

// sendToDLQ sends a single message to the DLQ
func (s *DLQSender) sendToDLQ(ctx context.Context, queueURL string, dlqMessage *DLQFailureMessage) error {
	// Serialize message
	messageBody, err := json.Marshal(dlqMessage)
	if err != nil {
		return fmt.Errorf("failed to serialize DLQ message: %w", err)
	}

	// Create message attributes
	messageAttributes := make(map[string]types.MessageAttributeValue)

	messageAttributes["Service"] = types.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(dlqMessage.Service),
	}

	messageAttributes["ErrorType"] = types.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(dlqMessage.ErrorInfo.ErrorType),
	}

	messageAttributes["IsPermanent"] = types.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(fmt.Sprintf("%t", dlqMessage.ErrorInfo.IsPermanent)),
	}

	messageAttributes["Priority"] = types.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(dlqMessage.ErrorInfo.Priority),
	}

	// Send message
	_, err = s.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(string(messageBody)),
		MessageAttributes: messageAttributes,
	})

	if err != nil {
		return fmt.Errorf("failed to send message to DLQ: %w", err)
	}

	return nil
}

// sendBatchToDLQ sends multiple messages to DLQ in a batch
func (s *DLQSender) sendBatchToDLQ(ctx context.Context, queueURL string, service string, failures []ProcessingFailure) error {
	entries := make([]types.SendMessageBatchRequestEntry, 0, len(failures))

	for i, failure := range failures {
		dlqMessage := s.createDLQMessage(service, failure.OriginalMessage, failure.Error)

		messageBody, err := json.Marshal(dlqMessage)
		if err != nil {
			s.logger.Error("failed to serialize DLQ message in batch",
				zap.String("message_id", failure.OriginalMessage.MessageId),
				zap.Error(err),
			)
			continue
		}

		// Create message attributes
		messageAttributes := make(map[string]types.MessageAttributeValue)
		messageAttributes["Service"] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(service),
		}
		messageAttributes["ErrorType"] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(dlqMessage.ErrorInfo.ErrorType),
		}

		entries = append(entries, types.SendMessageBatchRequestEntry{
			Id:                aws.String(fmt.Sprintf("dlq-%d", i)),
			MessageBody:       aws.String(string(messageBody)),
			MessageAttributes: messageAttributes,
		})
	}

	if err := common.ValidateSliceNotEmpty("entries", entries); err != nil {
		return nil // Empty entries is not an error
	}

	// Send batch
	result, err := s.sqsClient.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
		QueueUrl: aws.String(queueURL),
		Entries:  entries,
	})

	if err != nil {
		return fmt.Errorf("failed to send batch to DLQ: %w", err)
	}

	// Log any failed entries
	if err := common.ValidateSliceNotEmpty("result.Failed", result.Failed); err == nil {
		s.logger.Warn("some messages failed to send to DLQ",
			zap.Int("failed_count", len(result.Failed)),
			zap.Int("successful_count", len(result.Successful)),
		)
	}

	return nil
}

// Helper functions

// extractQueueNameFromARN extracts queue name from ARN
func extractQueueNameFromARN(arn string) string {
	// ARN format: arn:aws:sqs:region:account:queue-name
	parts := strings.Split(arn, ":")
	if len(parts) >= 6 {
		return parts[5]
	}
	return "unknown"
}

// parseIntSafe safely parses an integer string
func parseIntSafe(s string) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return 0
}

// Data structures

// DLQFailureMessage represents a message being sent to DLQ
//
//nolint:revive // DLQ prefix clarifies this is Dead Letter Queue message
type DLQFailureMessage struct {
	OriginalMessageID string            `json:"original_message_id"`
	Service           string            `json:"service"`
	QueueName         string            `json:"queue_name"`
	MessageBody       string            `json:"message_body"`
	MessageAttributes map[string]string `json:"message_attributes"`
	ErrorInfo         *ErrorInfo        `json:"error_info"`
	ProcessingContext ProcessingContext `json:"processing_context"`
	RetryCount        int               `json:"retry_count"`
	Timestamp         time.Time         `json:"timestamp"`
}

// ProcessingContext contains context about the failed processing
type ProcessingContext struct {
	FunctionName string    `json:"function_name"`
	LogGroup     string    `json:"log_group"`
	LogStream    string    `json:"log_stream"`
	RequestID    string    `json:"request_id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// ProcessingFailure represents a failed message processing attempt
type ProcessingFailure struct {
	OriginalMessage events.SQSMessage `json:"original_message"`
	Error           error             `json:"error"`
	Timestamp       time.Time         `json:"timestamp"`
}

// Helper methods for integration with existing processors

// WrapSQSHandler wraps an existing SQS handler to automatically send failures to DLQ
func WrapSQSHandler(service string, handler func(context.Context, events.SQSEvent) error, logger *zap.Logger) func(context.Context, events.SQSEvent) error {
	sender := NewDLQSender(logger)

	return func(ctx context.Context, event events.SQSEvent) error {
		// Initialize DLQ sender
		if err := sender.InitializeAWSClients(ctx); err != nil {
			logger.Error("failed to initialize DLQ sender", zap.Error(err))
			// Continue processing even if DLQ initialization fails
		}

		// Track failures
		var failures []ProcessingFailure

		// Try to process normally first
		err := handler(ctx, event)
		if err != nil {
			// If the whole batch failed, record all messages as failed
			for _, record := range event.Records {
				failures = append(failures, ProcessingFailure{
					OriginalMessage: record,
					Error:           err,
					Timestamp:       time.Now(),
				})
			}
		}

		// Send failures to DLQ
		if err := common.ValidateSliceNotEmpty("failures", failures); err == nil {
			if dlqErr := sender.SendBatchFailedMessages(ctx, service, failures); dlqErr != nil {
				logger.Error("failed to send failures to DLQ",
					zap.String("service", service),
					zap.Int("failure_count", len(failures)),
					zap.Error(dlqErr),
				)
			}
		}

		return err
	}
}

// SendIndividualFailures sends individual message failures to DLQ
// This is useful when processing messages individually within a batch
func SendIndividualFailures(ctx context.Context, service string, failures []ProcessingFailure, logger *zap.Logger) {
	if err := common.ValidateSliceNotEmpty("failures", failures); err != nil {
		return
	}

	sender := NewDLQSender(logger)
	if err := sender.InitializeAWSClients(ctx); err != nil {
		logger.Error("failed to initialize DLQ sender for individual failures", zap.Error(err))
		return
	}

	if err := sender.SendBatchFailedMessages(ctx, service, failures); err != nil {
		logger.Error("failed to send individual failures to DLQ",
			zap.String("service", service),
			zap.Int("failure_count", len(failures)),
			zap.Error(err),
		)
	}
}
