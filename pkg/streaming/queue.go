// Package streaming provides event queueing infrastructure for real-time streaming
package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
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

// QueueEventForUser queues an event for a specific user's streams
func (q *dynamoStreamQueue) QueueEventForUser(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}

	event := &models.StreamingEvent{
		EventID:   fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), userID),
		EventType: eventType,
		TargetType: "user",
		TargetID:  userID,
		Payload:   payload,
		CreatedAt: time.Now(),
		TTL:       time.Now().Add(24 * time.Hour).Unix(), // Events expire after 24 hours
	}

	// Update keys for GSI indexing
	event.UpdateKeys()

	// Store the event - DynamoDB Streams will trigger the stream-router
	if err := q.db.WithContext(ctx).Model(event).Create(); err != nil {
		q.logger.Error("failed to queue user event",
			zap.String("user_id", userID),
			zap.String("event_type", eventType),
			zap.Error(err))
		return fmt.Errorf("failed to queue event: %w", err)
	}

	q.logger.Debug("queued event for user",
		zap.String("user_id", userID),
		zap.String("event_type", eventType),
		zap.String("event_id", event.EventID))

	return nil
}

// QueueEventForStream queues an event for all subscribers of a stream
func (q *dynamoStreamQueue) QueueEventForStream(ctx context.Context, streamName string, eventType string, payload map[string]interface{}) error {
	if streamName == "" {
		return fmt.Errorf("streamName cannot be empty")
	}

	event := &models.StreamingEvent{
		EventID:   fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), streamName),
		EventType: eventType,
		TargetType: "stream",
		TargetID:  streamName,
		Payload:   payload,
		CreatedAt: time.Now(),
		TTL:       time.Now().Add(24 * time.Hour).Unix(),
	}

	event.UpdateKeys()

	if err := q.db.WithContext(ctx).Model(event).Create(); err != nil {
		q.logger.Error("failed to queue stream event",
			zap.String("stream", streamName),
			zap.String("event_type", eventType),
			zap.Error(err))
		return fmt.Errorf("failed to queue event: %w", err)
	}

	q.logger.Debug("queued event for stream",
		zap.String("stream", streamName),
		zap.String("event_type", eventType),
		zap.String("event_id", event.EventID))

	return nil
}

// QueueEventForConversation queues an event for all participants in a conversation
func (q *dynamoStreamQueue) QueueEventForConversation(ctx context.Context, conversationID string, eventType string, payload map[string]interface{}) error {
	if conversationID == "" {
		return fmt.Errorf("conversationID cannot be empty")
	}

	event := &models.StreamingEvent{
		EventID:   fmt.Sprintf("evt_%d_conv_%s", time.Now().UnixNano(), conversationID),
		EventType: eventType,
		TargetType: "conversation",
		TargetID:  conversationID,
		Payload:   payload,
		CreatedAt: time.Now(),
		TTL:       time.Now().Add(24 * time.Hour).Unix(),
	}

	event.UpdateKeys()

	if err := q.db.WithContext(ctx).Model(event).Create(); err != nil {
		q.logger.Error("failed to queue conversation event",
			zap.String("conversation_id", conversationID),
			zap.String("event_type", eventType),
			zap.Error(err))
		return fmt.Errorf("failed to queue event: %w", err)
	}

	q.logger.Debug("queued event for conversation",
		zap.String("conversation_id", conversationID),
		zap.String("event_type", eventType),
		zap.String("event_id", event.EventID))

	return nil
}

// QueueEventForFollowers queues an event for all followers of a user
func (q *dynamoStreamQueue) QueueEventForFollowers(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}

	event := &models.StreamingEvent{
		EventID:   fmt.Sprintf("evt_%d_followers_%s", time.Now().UnixNano(), userID),
		EventType: eventType,
		TargetType: "followers",
		TargetID:  userID, // The user whose followers should receive this
		Payload:   payload,
		CreatedAt: time.Now(),
		TTL:       time.Now().Add(24 * time.Hour).Unix(),
	}

	event.UpdateKeys()

	if err := q.db.WithContext(ctx).Model(event).Create(); err != nil {
		q.logger.Error("failed to queue followers event",
			zap.String("user_id", userID),
			zap.String("event_type", eventType),
			zap.Error(err))
		return fmt.Errorf("failed to queue event: %w", err)
	}

	q.logger.Debug("queued event for followers",
		zap.String("user_id", userID),
		zap.String("event_type", eventType),
		zap.String("event_id", event.EventID))

	return nil
}