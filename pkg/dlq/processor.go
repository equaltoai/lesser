package dlq

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/common"
)

// Processor handles dead letter queue message processing
type Processor struct {
	db                core.DB
	dlqRepo           *repositories.DLQRepository
	costTrackingRepo  *repositories.CostTrackingRepository
	logger            *zap.Logger
	sqsClient         *sqs.Client
	errorClassifier   *ErrorClassifier
	reprocessorClient *ReprocessorClient
}

// NewProcessor creates a new DLQ processor
func NewProcessor(db core.DB, tableName string, logger *zap.Logger) *Processor {
	return &Processor{
		db:                db,
		dlqRepo:           repositories.NewDLQRepository(db, tableName, logger),
		costTrackingRepo:  repositories.NewCostTrackingRepository(db, tableName, logger),
		logger:            logger,
		errorClassifier:   NewErrorClassifier(),
		reprocessorClient: NewReprocessorClient(logger),
	}
}

// InitializeAWSClients initializes AWS clients
func (p *Processor) InitializeAWSClients(ctx context.Context) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	p.sqsClient = sqs.NewFromConfig(cfg)
	p.reprocessorClient.SetSQSClient(p.sqsClient)

	return nil
}

// ProcessDLQMessages processes messages from dead letter queues
func (p *Processor) ProcessDLQMessages(ctx context.Context, event events.SQSEvent) error {
	p.logger.Info("processing DLQ messages",
		zap.Int("message_count", len(event.Records)),
	)

	var processedCount int
	var errorCount int

	for _, record := range event.Records {
		if err := p.processMessage(ctx, record); err != nil {
			p.logger.Error("failed to process DLQ message",
				zap.String("message_id", record.MessageId),
				zap.Error(err),
			)
			errorCount++
		} else {
			processedCount++
		}
	}

	p.logger.Info("completed DLQ message processing",
		zap.Int("processed", processedCount),
		zap.Int("errors", errorCount),
		zap.Int("total", len(event.Records)),
	)

	// If all messages failed, return error to trigger reprocessing
	if errorCount > 0 && processedCount == 0 {
		return fmt.Errorf("failed to process any DLQ messages")
	}

	return nil
}

// processMessage processes an individual DLQ message
func (p *Processor) processMessage(ctx context.Context, record events.SQSMessage) error {
	start := time.Now()

	// Parse original message from DLQ record
	originalMessage, err := p.parseOriginalMessage(record)
	if err != nil {
		return fmt.Errorf("failed to parse original message: %w", err)
	}

	// Create DLQ message record
	dlqMessage := p.createDLQMessage(record, originalMessage)

	// Store the DLQ message
	if err := p.dlqRepo.CreateDLQMessage(ctx, dlqMessage); err != nil {
		p.logger.Error("failed to store DLQ message",
			zap.String("message_id", dlqMessage.ID),
			zap.Error(err),
		)
		// Continue processing even if storage fails
	}

	// Attempt reprocessing if appropriate
	if dlqMessage.CanReprocess() && !dlqMessage.IsPermanent {
		if err := p.attemptReprocessing(ctx, dlqMessage, originalMessage); err != nil {
			p.logger.Warn("reprocessing failed",
				zap.String("dlq_message_id", dlqMessage.ID),
				zap.Error(err),
			)
			dlqMessage.MarkFailed(err.Error())
		} else {
			dlqMessage.MarkResolved()
		}

		// Update the DLQ message with results
		if err := p.dlqRepo.UpdateDLQMessage(ctx, dlqMessage); err != nil {
			p.logger.Error("failed to update DLQ message after reprocessing",
				zap.String("message_id", dlqMessage.ID),
				zap.Error(err),
			)
		}
	}

	// Track costs
	processingDuration := time.Since(start)
	if err := p.trackCosts(ctx, dlqMessage, processingDuration); err != nil {
		p.logger.Warn("failed to track DLQ processing costs",
			zap.String("message_id", dlqMessage.ID),
			zap.Error(err),
		)
	}

	return nil
}

// parseOriginalMessage extracts the original failed message from the DLQ record
func (p *Processor) parseOriginalMessage(record events.SQSMessage) (*OriginalMessage, error) {
	originalMessage := &OriginalMessage{
		MessageID:     record.MessageId,
		Body:          record.Body,
		Attributes:    make(map[string]string),
		ReceiptHandle: record.ReceiptHandle,
	}

	// Copy attributes
	for key, value := range record.MessageAttributes {
		if value.StringValue != nil {
			originalMessage.Attributes[key] = *value.StringValue
		}
	}

	// Try to extract source queue information from attributes
	if sourceQueue, exists := originalMessage.Attributes["DeadLetterQueue.SourceQueue"]; exists {
		originalMessage.SourceQueue = sourceQueue
	}

	if originalMessageID, exists := originalMessage.Attributes["DeadLetterQueue.OriginalMessageId"]; exists {
		originalMessage.OriginalMessageID = originalMessageID
	}

	return originalMessage, nil
}

// createDLQMessage creates a DLQ message record from the SQS record
func (p *Processor) createDLQMessage(record events.SQSMessage, originalMessage *OriginalMessage) *models.DLQMessage {
	// Extract service name from queue ARN or attributes
	service := p.extractServiceName(record)

	// Classify the error
	errorInfo := p.errorClassifier.ClassifyError(originalMessage.Body, service)

	// Determine queue names
	queueName := p.extractQueueName(record.EventSourceARN)
	sourceQueue := originalMessage.SourceQueue
	if err := common.ValidateRequiredParam("sourceQueue", sourceQueue); err != nil {
		sourceQueue = strings.ReplaceAll(queueName, "-dlq", "") // Remove DLQ suffix
	}

	// Extract function context
	functionName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	logGroup := os.Getenv("AWS_LAMBDA_LOG_GROUP_NAME")
	logStream := os.Getenv("AWS_LAMBDA_LOG_STREAM_NAME")

	// Build DLQ message
	builder := models.NewDLQMessageBuilder().
		ForService(service).
		WithOriginalMessage(originalMessage.MessageID, originalMessage.Body).
		WithQueue(queueName, sourceQueue).
		WithError(errorInfo.ErrorType, errorInfo.ErrorMessage, errorInfo.StackTrace).
		WithFailureReason(errorInfo.FailureReason).
		WithPriority(errorInfo.Priority).
		WithContext(functionName, logGroup, logStream, "").
		WithAttributes(originalMessage.Attributes)

	// Set permanent flag if error is classified as permanent
	if errorInfo.IsPermanent {
		builder.MarkAsPermanent()
	}

	// Add metadata
	metadata := map[string]interface{}{
		"queue_arn":                 record.EventSourceARN,
		"receipt_handle":            record.ReceiptHandle,
		"approximate_receive_count": record.Attributes["ApproximateReceiveCount"],
	}
	builder.WithMetadata(metadata)

	// Add tags based on classification
	tags := []string{
		"service:" + service,
		"error_type:" + errorInfo.ErrorType,
		"priority:" + errorInfo.Priority,
	}
	if errorInfo.IsPermanent {
		tags = append(tags, "permanent")
	} else {
		tags = append(tags, "transient")
	}
	builder.WithTags(tags...)

	return builder.Build()
}

// extractServiceName extracts the service name from the SQS record
func (p *Processor) extractServiceName(record events.SQSMessage) string {
	// Try to extract from queue ARN
	queueName := p.extractQueueName(record.EventSourceARN)

	// Remove common suffixes to get service name
	serviceName := strings.ReplaceAll(queueName, "-dlq", "")
	serviceName = strings.ReplaceAll(serviceName, "-queue", "")

	// Common service mappings
	serviceMapping := map[string]string{
		"notification-processor-dlq": "notification-processor",
		"activity-processor-dlq":     "activity-processor",
		"media-processor-dlq":        "media-processor",
		"federation-delivery-dlq":    "federation-delivery",
		"search-indexer-dlq":         "search-indexer",
	}

	if mappedService, exists := serviceMapping[queueName]; exists {
		return mappedService
	}

	return serviceName
}

// extractQueueName extracts the queue name from the ARN
func (p *Processor) extractQueueName(queueARN string) string {
	// ARN format: arn:aws:sqs:region:account:queue-name
	parts := strings.Split(queueARN, ":")
	if len(parts) >= 6 {
		return parts[5]
	}
	return "unknown"
}

// attemptReprocessing attempts to reprocess the failed message
func (p *Processor) attemptReprocessing(ctx context.Context, dlqMessage *models.DLQMessage, originalMessage *OriginalMessage) error {
	p.logger.Info("attempting to reprocess message",
		zap.String("dlq_message_id", dlqMessage.ID),
		zap.String("service", dlqMessage.Service),
		zap.String("error_type", dlqMessage.ErrorType),
	)

	// Mark as reprocessing
	dlqMessage.MarkForReprocessing()

	// Attempt reprocessing based on service
	switch dlqMessage.Service {
	case "notification-processor":
		return p.reprocessorClient.ReprocessNotification(ctx, originalMessage)
	case "activity-processor":
		return p.reprocessorClient.ReprocessActivity(ctx, originalMessage)
	case "media-processor":
		return p.reprocessorClient.ReprocessMedia(ctx, originalMessage)
	case "federation-delivery":
		return p.reprocessorClient.ReprocessFederation(ctx, originalMessage)
	case "search-indexer":
		return p.reprocessorClient.ReprocessSearch(ctx, originalMessage)
	default:
		return p.reprocessorClient.ReprocessGeneric(ctx, dlqMessage.SourceQueue, originalMessage)
	}
}

// trackCosts tracks the cost of processing DLQ messages
func (p *Processor) trackCosts(ctx context.Context, dlqMessage *models.DLQMessage, processingDuration time.Duration) error {
	// Calculate processing costs
	lambdaCostMicroCents := int64(20) // Base Lambda cost
	storageCostMicroCents := int64(5) // DynamoDB operation cost

	// Additional costs based on reprocessing
	var reprocessingCostMicroCents int64
	if dlqMessage.ReprocessingCount > 0 {
		reprocessingCostMicroCents = int64(dlqMessage.ReprocessingCount * 10) // Cost per retry
	}

	totalCostMicroCents := lambdaCostMicroCents + storageCostMicroCents + reprocessingCostMicroCents

	// Update the DLQ message costs
	dlqMessage.UpdateCosts(lambdaCostMicroCents+storageCostMicroCents, reprocessingCostMicroCents)

	// Create cost tracking record
	costRecord := &models.DynamoDBCostRecord{
		Table:                "lesser-main",
		OperationType:        "DLQProcessing",
		ServiceName:          "dlq-processor",
		Timestamp:            time.Now(),
		TotalCostMicroCents:  totalCostMicroCents,
		EstimatedCostDollars: float64(totalCostMicroCents) / 1_000_000.0,
		Properties: map[string]interface{}{
			"dlq_message_id":         dlqMessage.ID,
			"service":                dlqMessage.Service,
			"error_type":             dlqMessage.ErrorType,
			"reprocessing_count":     dlqMessage.ReprocessingCount,
			"status":                 dlqMessage.Status,
			"processing_duration_ms": processingDuration.Milliseconds(),
		},
		Tags: map[string]string{
			"service":    dlqMessage.Service,
			"error_type": dlqMessage.ErrorType,
			"priority":   dlqMessage.Priority,
		},
	}

	return p.costTrackingRepo.Create(ctx, costRecord)
}

// ScheduledReprocessing handles scheduled reprocessing of failed messages
func (p *Processor) ScheduledReprocessing(ctx context.Context) error {
	p.logger.Info("starting scheduled reprocessing")

	// Get services to process
	services := []string{
		"notification-processor",
		"activity-processor",
		"media-processor",
		"federation-delivery",
		"search-indexer",
	}

	var totalProcessed int
	var totalErrors int

	for _, service := range services {
		processed, errors, err := p.reprocessServiceMessages(ctx, service)
		if err != nil {
			p.logger.Error("failed to reprocess messages for service",
				zap.String("service", service),
				zap.Error(err),
			)
			totalErrors++
		} else {
			totalProcessed += processed
			totalErrors += errors
		}
	}

	p.logger.Info("completed scheduled reprocessing",
		zap.Int("total_processed", totalProcessed),
		zap.Int("total_errors", totalErrors),
	)

	return nil
}

// reprocessServiceMessages reprocesses messages for a specific service
func (p *Processor) reprocessServiceMessages(ctx context.Context, service string) (int, int, error) {
	// Get messages ready for reprocessing
	messages, err := p.dlqRepo.GetDLQMessagesForReprocessing(ctx, service, "new", 50)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get messages for reprocessing: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("messages", messages); err != nil {
		return 0, 0, nil
	}

	p.logger.Info("reprocessing messages for service",
		zap.String("service", service),
		zap.Int("message_count", len(messages)),
	)

	var processed int
	var errors int

	for _, message := range messages {
		if !message.CanReprocess() {
			continue
		}

		// Create original message for reprocessing
		originalMessage := &OriginalMessage{
			MessageID:         message.OriginalMessageID,
			Body:              message.MessageBody,
			Attributes:        message.MessageAttributes,
			SourceQueue:       message.SourceQueue,
			OriginalMessageID: message.OriginalMessageID,
		}

		// Attempt reprocessing
		if err := p.attemptReprocessing(ctx, message, originalMessage); err != nil {
			message.MarkFailed(err.Error())
			if message.ShouldAbandon() {
				message.MarkAbandoned()
			}
			errors++
		} else {
			message.MarkResolved()
			processed++
		}

		// Update message status
		if err := p.dlqRepo.UpdateDLQMessage(ctx, message); err != nil {
			p.logger.Error("failed to update message after reprocessing",
				zap.String("message_id", message.ID),
				zap.Error(err),
			)
		}
	}

	return processed, errors, nil
}

// GetAnalytics returns DLQ analytics for monitoring
func (p *Processor) GetAnalytics(ctx context.Context, service string, timeRange repositories.DLQTimeRange) (*repositories.DLQAnalytics, error) {
	return p.dlqRepo.GetDLQAnalytics(ctx, service, timeRange)
}

// GetTrends returns DLQ trends for monitoring
func (p *Processor) GetTrends(ctx context.Context, service string, days int) (*repositories.DLQTrends, error) {
	return p.dlqRepo.GetDLQTrends(ctx, service, days)
}

// SearchMessages searches DLQ messages with filters
func (p *Processor) SearchMessages(ctx context.Context, filter *repositories.DLQSearchFilter) ([]*models.DLQMessage, string, error) {
	return p.dlqRepo.SearchDLQMessages(ctx, filter)
}

// CleanupExpiredMessages removes expired DLQ messages
func (p *Processor) CleanupExpiredMessages(ctx context.Context) error {
	before := time.Now().Add(-90 * 24 * time.Hour) // 90 days ago
	deletedCount, err := p.dlqRepo.CleanupExpiredMessages(ctx, before)
	if err != nil {
		return err
	}

	p.logger.Info("cleaned up expired DLQ messages",
		zap.Int("deleted_count", deletedCount),
	)

	return nil
}

// OriginalMessage represents the original failed message
type OriginalMessage struct {
	MessageID         string            `json:"message_id"`
	OriginalMessageID string            `json:"original_message_id"`
	Body              string            `json:"body"`
	Attributes        map[string]string `json:"attributes"`
	SourceQueue       string            `json:"source_queue"`
	ReceiptHandle     string            `json:"receipt_handle"`
}

// ProcessingResult represents the result of processing a DLQ message
type ProcessingResult struct {
	MessageID         string    `json:"message_id"`
	Success           bool      `json:"success"`
	Error             string    `json:"error,omitempty"`
	ReprocessingCount int       `json:"reprocessing_count"`
	ProcessingTimeMs  int64     `json:"processing_time_ms"`
	CostMicroCents    int64     `json:"cost_micro_cents"`
	Timestamp         time.Time `json:"timestamp"`
}

// ProcessingStats represents statistics for a processing batch
type ProcessingStats struct {
	TotalMessages       int     `json:"total_messages"`
	ProcessedMessages   int     `json:"processed_messages"`
	FailedMessages      int     `json:"failed_messages"`
	ReprocessedMessages int     `json:"reprocessed_messages"`
	ResolvedMessages    int     `json:"resolved_messages"`
	AbandonedMessages   int     `json:"abandoned_messages"`
	TotalCostMicroCents int64   `json:"total_cost_micro_cents"`
	TotalCostDollars    float64 `json:"total_cost_dollars"`
	ProcessingTimeMs    int64   `json:"processing_time_ms"`
}
