package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeStreamQueue struct {
	queueUser         func(ctx context.Context, userID, eventType string, payload map[string]interface{}) error
	queueStream       func(ctx context.Context, streamName, eventType string, payload map[string]interface{}) error
	queueConversation func(ctx context.Context, conversationID, eventType string, payload map[string]interface{}) error
	queueFollowers    func(ctx context.Context, userID, eventType string, payload map[string]interface{}) error
}

func (q *fakeStreamQueue) QueueEventForUser(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error {
	if q.queueUser == nil {
		return nil
	}
	return q.queueUser(ctx, userID, eventType, payload)
}

func (q *fakeStreamQueue) QueueEventForStream(ctx context.Context, streamName string, eventType string, payload map[string]interface{}) error {
	if q.queueStream == nil {
		return nil
	}
	return q.queueStream(ctx, streamName, eventType, payload)
}

func (q *fakeStreamQueue) QueueEventForConversation(ctx context.Context, conversationID string, eventType string, payload map[string]interface{}) error {
	if q.queueConversation == nil {
		return nil
	}
	return q.queueConversation(ctx, conversationID, eventType, payload)
}

func (q *fakeStreamQueue) QueueEventForFollowers(ctx context.Context, userID string, eventType string, payload map[string]interface{}) error {
	if q.queueFollowers == nil {
		return nil
	}
	return q.queueFollowers(ctx, userID, eventType, payload)
}

func TestQueuePublisher_PublishToUser_ValidatesInputsAndQueues(t *testing.T) {
	var gotUserID string
	var gotEventType string
	var gotPayload map[string]interface{}

	queue := &fakeStreamQueue{
		queueUser: func(_ context.Context, userID, eventType string, payload map[string]interface{}) error {
			gotUserID = userID
			gotEventType = eventType
			gotPayload = payload
			return nil
		},
	}

	p := NewQueuePublisher(queue, zap.NewNop())

	err := p.PublishToUser(context.Background(), "u1", &Event{
		Type:   "status.created",
		Stream: "user:u1",
		Payload: map[string]interface{}{
			"status_id": "s1",
		},
		Timestamp: time.Time{},
	})
	require.NoError(t, err)

	assert.Equal(t, "u1", gotUserID)
	assert.Equal(t, "status.created", gotEventType)
	require.NotNil(t, gotPayload)
	assert.Equal(t, "s1", gotPayload["status_id"])

	meta, ok := gotPayload["__meta"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "user:u1", meta["stream"])
	assert.NotEmpty(t, meta["timestamp"])
}

func TestQueuePublisher_PublishToUser_ErrorsWhenQueueMissing(t *testing.T) {
	p := NewQueuePublisher(nil, zap.NewNop())
	err := p.PublishToUser(context.Background(), "u1", &Event{})
	require.Error(t, err)

	require.NoError(t, p.Close())
}

func TestQueuePublisher_PublishToStream_FallsBackToEventStream(t *testing.T) {
	var gotStream string
	queue := &fakeStreamQueue{
		queueStream: func(_ context.Context, streamName, _ string, _ map[string]interface{}) error {
			gotStream = streamName
			return nil
		},
	}
	p := NewQueuePublisher(queue, zap.NewNop())

	err := p.PublishToStream(context.Background(), "", &Event{Stream: "public"})
	require.NoError(t, err)
	assert.Equal(t, "public", gotStream)
}

func TestQueuePublisher_PublishToConversation_PropagatesQueueErrors(t *testing.T) {
	queue := &fakeStreamQueue{
		queueConversation: func(_ context.Context, _, _ string, _ map[string]interface{}) error {
			return errors.New("nope")
		},
	}
	p := NewQueuePublisher(queue, zap.NewNop())

	err := p.PublishToConversation(context.Background(), "c1", &Event{Type: "x"})
	require.Error(t, err)
}

func TestQueuePublisher_LogQueueError_DefensiveFieldCount(t *testing.T) {
	p := NewQueuePublisher(&fakeStreamQueue{}, zap.NewNop()).(*queuePublisher)

	tooMany := make(map[string]interface{}, maxLogQueueFields+1)
	for i := 0; i < maxLogQueueFields+1; i++ {
		tooMany[time.Unix(0, int64(i)).String()] = i
	}

	p.logQueueError("msg", errors.New("boom"), tooMany)
}

func TestEventTypeOrDefaultAndBuildQueuePayload(t *testing.T) {
	assert.Equal(t, "graphql.event", eventTypeOrDefault(nil))
	assert.Equal(t, "graphql.event", eventTypeOrDefault(&Event{}))
	assert.Equal(t, "x", eventTypeOrDefault(&Event{Type: "x"}))

	payload := buildQueuePayload(&Event{
		Stream:    "public",
		Timestamp: time.Unix(1, 2).UTC(),
		Payload: map[string]interface{}{
			"a": "b",
		},
	})
	assert.Equal(t, "b", payload["a"])

	meta, ok := payload["__meta"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "public", meta["stream"])
	assert.Equal(t, time.Unix(1, 2).UTC().Format(time.RFC3339Nano), meta["timestamp"])
}
