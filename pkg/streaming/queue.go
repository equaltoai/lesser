// Package streaming provides event queueing infrastructure for real-time streaming
package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// StreamQueueService queues streaming events to DynamoDB for processing by stream-router
type StreamQueueService interface {
	// QueueEventForUser queues an event for a specific user's streams
	QueueEventForUser(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error

	// QueueEventForStream queues an event for all subscribers of a stream
	QueueEventForStream(ctx context.Context, streamName string, eventType string, payload map[string]interface{}) error

	// QueueEventForConversation queues an event for all participants in a conversation
	QueueEventForConversation(ctx context.Context, conversationID string, eventType string, payload map[string]interface{}) error

	// QueueEventForFollowers queues an event for all followers of a user
	QueueEventForFollowers(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error
}

// dynamoStreamQueue implements StreamQueueService using DynamoDB
type dynamoStreamQueue struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewDynamoStreamQueue creates a new DynamoDB-based stream queue service
func NewDynamoStreamQueue(db core.DB, tableName string, logger *zap.Logger) StreamQueueService {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &dynamoStreamQueue{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// queueEvent is a generic helper for queueing streaming events
func (q *dynamoStreamQueue) queueEvent(ctx context.Context, targetType, targetID, eventType, eventIDSuffix, validationParam string, payload map[string]interface{}) error {
	if err := common.ValidateRequiredParam(validationParam, targetID); err != nil {
		return fmt.Errorf("%s cannot be empty", validationParam)
	}

	event := &models.StreamingEvent{
		EventID:    fmt.Sprintf("evt_%d_%s_%s", time.Now().UnixNano(), eventIDSuffix, targetID),
		EventType:  eventType,
		TargetType: targetType,
		TargetID:   targetID,
		Payload:    payload,
		CreatedAt:  time.Now(),
		TTL:        time.Now().Add(24 * time.Hour).Unix(), // Events expire after 24 hours
	}

	// Update keys for GSI indexing
	event.UpdateKeys()

	// Store the event - DynamoDB Streams will trigger the stream-router
	if err := q.db.WithContext(ctx).Model(event).Create(); err != nil {
		q.logger.Error("failed to queue event",
			zap.String("target_type", targetType),
			zap.String("target_id", targetID),
			zap.String("event_type", eventType),
			zap.Error(err))
		return fmt.Errorf("failed to queue event: %w", err)
	}

	q.logger.Debug("queued event",
		zap.String("target_type", targetType),
		zap.String("target_id", targetID),
		zap.String("event_type", eventType),
		zap.String("event_id", event.EventID))

	return nil
}

// QueueEventForUser queues an event for a specific user's streams
func (q *dynamoStreamQueue) QueueEventForUser(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error {
	return q.queueEvent(ctx, "user", userID, eventType, "", "userID", payload)
}

// QueueEventForStream queues an event for all subscribers of a stream
func (q *dynamoStreamQueue) QueueEventForStream(ctx context.Context, streamName string, eventType string, payload map[string]interface{}) error {
	return q.queueEvent(ctx, "stream", streamName, eventType, "", "streamName", payload)
}

// QueueEventForConversation queues an event for all participants in a conversation
func (q *dynamoStreamQueue) QueueEventForConversation(ctx context.Context, conversationID string, eventType string, payload map[string]interface{}) error {
	return q.queueEvent(ctx, "conversation", conversationID, eventType, "conv", "conversationID", payload)
}

// QueueEventForFollowers queues an event for all followers of a user
func (q *dynamoStreamQueue) QueueEventForFollowers(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error {
	return q.queueEvent(ctx, "followers", userID, eventType, "followers", "userID", payload)
}
