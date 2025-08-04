package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// SQSBatchProcessor processes SQS message batches in Lambda
type SQSBatchProcessor struct {
	db           core.DB
	logger       *zap.Logger
	tracker      CostTracker
	batchWriter  *BatchWriter
	maxBatchSize int
}

// SQSBatchProcessorConfig holds configuration for SQSBatchProcessor
type SQSBatchProcessorConfig struct {
	Logger       *zap.Logger
	Tracker      CostTracker
	MaxBatchSize int // Maximum items to process in a single batch (default: 25)
}

// NewSQSBatchProcessor creates a new SQS batch processor
func NewSQSBatchProcessor(db core.DB, config SQSBatchProcessorConfig) *SQSBatchProcessor {
	maxBatchSize := config.MaxBatchSize
	if maxBatchSize <= 0 || maxBatchSize > MaxBatchWriteSize {
		maxBatchSize = MaxBatchWriteSize
	}

	batchWriter := NewBatchWriter(db, BatchWriterConfig{
		BatchSize: maxBatchSize,
		Logger:    config.Logger,
		Tracker:   config.Tracker,
	})

	return &SQSBatchProcessor{
		db:           db,
		logger:       config.Logger,
		tracker:      config.Tracker,
		batchWriter:  batchWriter,
		maxBatchSize: maxBatchSize,
	}
}

// BatchMessage represents a message payload containing items to batch process
type BatchMessage struct {
	Operation string `json:"operation"` // "create", "update", "delete"
	Items     []any  `json:"items"`
	TableName string `json:"table_name,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ProcessBatch processes an SQS event batch and returns response with failures
func (p *SQSBatchProcessor) ProcessBatch(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	if len(event.Records) == 0 {
		return events.SQSEventResponse{}, nil
	}

	startTime := time.Now()
	totalMessages := len(event.Records)
	
	if p.logger != nil {
		p.logger.Info("sqs_batch_processing_started",
			zap.Int("message_count", totalMessages),
		)
	}

	var batchItemFailures []events.SQSBatchItemFailure
	processedCount := 0
	
	// Process each SQS message in the batch
	for _, record := range event.Records {
		if err := p.processMessage(ctx, record); err != nil {
			// Add to batch item failures for SQS retry
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
			
			if p.logger != nil {
				p.logger.Error("sqs_message_processing_failed",
					zap.String("message_id", record.MessageId),
					zap.Error(err),
				)
			}
		} else {
			processedCount++
		}
	}

	duration := time.Since(startTime)
	failedCount := len(batchItemFailures)

	if p.logger != nil {
		p.logger.Info("sqs_batch_processing_completed",
			zap.Int("total_messages", totalMessages),
			zap.Int("processed_messages", processedCount),
			zap.Int("failed_messages", failedCount),
			zap.Duration("duration", duration),
		)
	}

	return events.SQSEventResponse{
		BatchItemFailures: batchItemFailures,
	}, nil
}

// processMessage processes a single SQS message
func (p *SQSBatchProcessor) processMessage(ctx context.Context, record events.SQSMessage) error {
	var batchMsg BatchMessage
	if err := json.Unmarshal([]byte(record.Body), &batchMsg); err != nil {
		return fmt.Errorf("failed to unmarshal batch message: %w", err)
	}

	if len(batchMsg.Items) == 0 {
		return nil // Nothing to process
	}

	// Log message processing start
	if p.logger != nil {
		p.logger.Debug("processing_batch_message",
			zap.String("message_id", record.MessageId),
			zap.String("operation", batchMsg.Operation),
			zap.Int("item_count", len(batchMsg.Items)),
			zap.String("table_name", batchMsg.TableName),
		)
	}

	// Process based on operation type
	switch batchMsg.Operation {
	case "create", "upsert":
		return p.processBatchWrite(ctx, batchMsg.Items, record.MessageId)
	case "delete":
		return p.processBatchDelete(ctx, batchMsg.Items, record.MessageId)
	default:
		return fmt.Errorf("unsupported operation: %s", batchMsg.Operation)
	}
}

// processBatchWrite handles batch write operations
func (p *SQSBatchProcessor) processBatchWrite(ctx context.Context, items []any, messageID string) error {
	startTime := time.Now()
	
	// Split into sub-batches if needed to respect DynamoDB limits
	totalItems := len(items)
	processed := 0
	
	for i := 0; i < totalItems; i += p.maxBatchSize {
		end := i + p.maxBatchSize
		if end > totalItems {
			end = totalItems
		}
		
		batch := items[i:end]
		result, err := p.batchWriter.WriteItems(ctx, batch)
		if err != nil {
			return fmt.Errorf("batch write failed at index %d: %w", i, err)
		}
		
		if result.FailedItems > 0 {
			// If any items failed, we should retry the entire message
			if p.logger != nil {
				p.logger.Warn("batch_write_partial_failure",
					zap.String("message_id", messageID),
					zap.Int("total_items", len(batch)),
					zap.Int("failed_items", result.FailedItems),
				)
			}
			return fmt.Errorf("batch write had %d failed items", result.FailedItems)
		}
		
		processed += result.ProcessedItems
	}

	duration := time.Since(startTime)
	
	if p.logger != nil {
		p.logger.Info("batch_write_completed",
			zap.String("message_id", messageID),
			zap.Int("total_items", totalItems),
			zap.Int("processed_items", processed),
			zap.Duration("duration", duration),
		)
	}

	return nil
}

// processBatchDelete handles batch delete operations
func (p *SQSBatchProcessor) processBatchDelete(ctx context.Context, items []any, messageID string) error {
	startTime := time.Now()
	
	// For delete operations, items should be key objects
	// Process in batches respecting DynamoDB limits
	totalItems := len(items)
	processed := 0
	
	for i := 0; i < totalItems; i += p.maxBatchSize {
		end := i + p.maxBatchSize
		if end > totalItems {
			end = totalItems
		}
		
		batch := items[i:end]
		
		// Use DynamORM's batch delete functionality
		// Note: This assumes the items are properly formatted key objects
		query := p.db.Model(batch[0])
		if err := query.BatchDelete(batch); err != nil {
			return fmt.Errorf("batch delete failed at index %d: %w", i, err)
		}
		
		processed += len(batch)
		
		// Track cost if tracker is available
		if p.tracker != nil {
			p.tracker.TrackDynamoWrite(len(batch))
		}
	}

	duration := time.Since(startTime)
	
	if p.logger != nil {
		p.logger.Info("batch_delete_completed",
			zap.String("message_id", messageID),
			zap.Int("total_items", totalItems),
			zap.Int("processed_items", processed),
			zap.Duration("duration", duration),
		)
	}

	return nil
}

// ProcessTimelineEntries processes timeline entries from SQS messages
func (p *SQSBatchProcessor) ProcessTimelineEntries(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	if len(event.Records) == 0 {
		return events.SQSEventResponse{}, nil
	}

	var batchItemFailures []events.SQSBatchItemFailure
	
	for _, record := range event.Records {
		var timelineMsg struct {
			FollowerIDs []string  `json:"follower_ids"`
			StatusID    string    `json:"status_id"`
			AuthorID    string    `json:"author_id"`
			CreatedAt   time.Time `json:"created_at"`
		}
		
		if err := json.Unmarshal([]byte(record.Body), &timelineMsg); err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
			continue
		}
		
		// Create timeline entries
		entries := make([]any, 0, len(timelineMsg.FollowerIDs))
		for _, followerID := range timelineMsg.FollowerIDs {
			entry := map[string]any{
				"PK":        fmt.Sprintf("USER#%s", followerID),
				"SK":        fmt.Sprintf("TIMELINE#%s#%s", timelineMsg.CreatedAt.Format("20060102150405"), timelineMsg.StatusID),
				"StatusID":  timelineMsg.StatusID,
				"AuthorID":  timelineMsg.AuthorID,
				"CreatedAt": timelineMsg.CreatedAt,
				"Type":      "home",
			}
			entries = append(entries, entry)
		}
		
		if len(entries) > 0 {
			if _, err := p.batchWriter.WriteItems(ctx, entries); err != nil {
				batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: record.MessageId,
				})
				
				if p.logger != nil {
					p.logger.Error("timeline_entries_write_failed",
						zap.String("message_id", record.MessageId),
						zap.Error(err),
					)
				}
			}
		}
	}

	return events.SQSEventResponse{
		BatchItemFailures: batchItemFailures,
	}, nil
}

// ProcessNotifications processes notification batches from SQS messages
func (p *SQSBatchProcessor) ProcessNotifications(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	if len(event.Records) == 0 {
		return events.SQSEventResponse{}, nil
	}

	var batchItemFailures []events.SQSBatchItemFailure
	
	for _, record := range event.Records {
		var notifMsg struct {
			UserIDs      []string `json:"user_ids"`
			StatusID     string   `json:"status_id"`
			AuthorID     string   `json:"author_id"`
			Type         string   `json:"type"` // "mention", "like", "repost", etc.
			TargetType   string   `json:"target_type"`
		}
		
		if err := json.Unmarshal([]byte(record.Body), &notifMsg); err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
			continue
		}
		
		now := time.Now()
		notifications := make([]any, 0, len(notifMsg.UserIDs))
		
		for _, userID := range notifMsg.UserIDs {
			notification := map[string]any{
				"PK":         fmt.Sprintf("USER#%s", userID),
				"SK":         fmt.Sprintf("NOTIF#%s#%s", now.Format("20060102150405"), notifMsg.StatusID),
				"ID":         fmt.Sprintf("%s_%s", notifMsg.StatusID, userID),
				"Type":       notifMsg.Type,
				"ActorID":    notifMsg.AuthorID,
				"TargetID":   notifMsg.StatusID,
				"TargetType": notifMsg.TargetType,
				"CreatedAt":  now,
				"IsRead":     false,
				"ExpiresAt":  now.Add(30 * 24 * time.Hour), // 30 days TTL
			}
			notifications = append(notifications, notification)
		}
		
		if len(notifications) > 0 {
			if _, err := p.batchWriter.WriteItems(ctx, notifications); err != nil {
				batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: record.MessageId,
				})
				
				if p.logger != nil {
					p.logger.Error("notifications_write_failed",
						zap.String("message_id", record.MessageId),
						zap.Error(err),
					)
				}
			}
		}
	}

	return events.SQSEventResponse{
		BatchItemFailures: batchItemFailures,
	}, nil
}

// Helper functions for creating batch messages

// CreateTimelineMessage creates a timeline batch message
func CreateTimelineMessage(followerIDs []string, statusID, authorID string, createdAt time.Time) *BatchMessage {
	entries := make([]any, 0, len(followerIDs))
	for _, followerID := range followerIDs {
		entry := map[string]any{
			"PK":        fmt.Sprintf("USER#%s", followerID),
			"SK":        fmt.Sprintf("TIMELINE#%s#%s", createdAt.Format("20060102150405"), statusID),
			"StatusID":  statusID,
			"AuthorID":  authorID,
			"CreatedAt": createdAt,
			"Type":      "home",
		}
		entries = append(entries, entry)
	}
	
	return &BatchMessage{
		Operation: "create",
		Items:     entries,
		TableName: "timeline",
		Metadata: map[string]any{
			"status_id": statusID,
			"author_id": authorID,
		},
	}
}

// CreateNotificationMessage creates a notification batch message
func CreateNotificationMessage(userIDs []string, statusID, authorID, notifType, targetType string) *BatchMessage {
	now := time.Now()
	notifications := make([]any, 0, len(userIDs))
	
	for _, userID := range userIDs {
		notification := map[string]any{
			"PK":         fmt.Sprintf("USER#%s", userID),
			"SK":         fmt.Sprintf("NOTIF#%s#%s", now.Format("20060102150405"), statusID),
			"ID":         fmt.Sprintf("%s_%s", statusID, userID),
			"Type":       notifType,
			"ActorID":    authorID,
			"TargetID":   statusID,
			"TargetType": targetType,
			"CreatedAt":  now,
			"IsRead":     false,
			"ExpiresAt":  now.Add(30 * 24 * time.Hour),
		}
		notifications = append(notifications, notification)
	}
	
	return &BatchMessage{
		Operation: "create",
		Items:     notifications,
		TableName: "notifications",
		Metadata: map[string]any{
			"status_id": statusID,
			"author_id": authorID,
			"type":      notifType,
		},
	}
}

// CreateBatchDeleteMessage creates a batch delete message
func CreateBatchDeleteMessage(keys []any, tableName string) *BatchMessage {
	return &BatchMessage{
		Operation: "delete",
		Items:     keys,
		TableName: tableName,
	}
}