package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// Maximum number of fields allowed in logging to avoid allocation overflow/panic.
const maxLogQueueFields = 1000

// NewQueuePublisher creates a publisher that enqueues streaming events to the DynamoDB-backed stream queue.
func NewQueuePublisher(queue StreamQueueService, logger *zap.Logger) Publisher {
	return &queuePublisher{
		queue:  queue,
		logger: logger,
	}
}

type queuePublisher struct {
	queue  StreamQueueService
	logger *zap.Logger
}

func (p *queuePublisher) PublishToUser(ctx context.Context, userID string, event *Event) error {
	if p.queue == nil {
		return fmt.Errorf("stream queue not configured")
	}
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		return err
	}

	payload := buildQueuePayload(event)
	if err := p.queue.QueueEventForUser(ctx, userID, eventTypeOrDefault(event), payload); err != nil {
		p.logQueueError("queue user event", err, map[string]interface{}{
			"user_id":    userID,
			"event_type": eventTypeOrDefault(event),
		})
		return err
	}

	return nil
}

func (p *queuePublisher) PublishToStream(ctx context.Context, streamName string, event *Event) error {
	if p.queue == nil {
		return fmt.Errorf("stream queue not configured")
	}
	if streamName == "" && event != nil {
		streamName = event.Stream
	}
	if err := common.ValidateRequiredParam("streamName", streamName); err != nil {
		return err
	}

	payload := buildQueuePayload(event)
	if err := p.queue.QueueEventForStream(ctx, streamName, eventTypeOrDefault(event), payload); err != nil {
		p.logQueueError("queue stream event", err, map[string]interface{}{
			"stream":     streamName,
			"event_type": eventTypeOrDefault(event),
		})
		return err
	}

	return nil
}

func (p *queuePublisher) PublishToConversation(ctx context.Context, conversationID string, event *Event) error {
	if p.queue == nil {
		return fmt.Errorf("stream queue not configured")
	}
	if err := common.ValidateRequiredParam("conversationID", conversationID); err != nil {
		return err
	}

	payload := buildQueuePayload(event)
	if err := p.queue.QueueEventForConversation(ctx, conversationID, eventTypeOrDefault(event), payload); err != nil {
		p.logQueueError("queue conversation event", err, map[string]interface{}{
			"conversation_id": conversationID,
			"event_type":      eventTypeOrDefault(event),
		})
		return err
	}

	return nil
}

func (p *queuePublisher) Close() error {
	return nil
}

func (p *queuePublisher) logQueueError(message string, err error, fields map[string]interface{}) {
	if p.logger == nil {
		return
	}

	fieldCount := len(fields)
	if fieldCount > maxLogQueueFields {
		p.logger.Error("logQueueError: too many fields, possible malformed input", zap.Int("field_count", fieldCount), zap.Error(err))
		return
	}

	zapFields := make([]zap.Field, 0, fieldCount)
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	zapFields = append(zapFields, zap.Error(err))

	p.logger.Error(message, zapFields...)
}

func eventTypeOrDefault(event *Event) string {
	if event != nil && event.Type != "" {
		return event.Type
	}
	return "graphql.event"
}

func buildQueuePayload(event *Event) map[string]interface{} {
	payload := make(map[string]interface{})
	if event != nil && event.Payload != nil {
		for k, v := range event.Payload {
			payload[k] = v
		}
	}

	if event != nil {
		meta := make(map[string]interface{})
		if event.Stream != "" {
			meta["stream"] = event.Stream
		}
		if !event.Timestamp.IsZero() {
			meta["timestamp"] = event.Timestamp.UTC().Format(time.RFC3339Nano)
		} else {
			meta["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		payload["__meta"] = meta
	}

	return payload
}
