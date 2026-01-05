package scheduled

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakePublisher struct {
	userCalls []fakePublisherUserCall
	err       error
}

type fakePublisherUserCall struct {
	user  string
	event *streaming.Event
}

func (p *fakePublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	p.userCalls = append(p.userCalls, fakePublisherUserCall{user: userID, event: event})
	return p.err
}

func (p *fakePublisher) PublishToStream(context.Context, string, *streaming.Event) error {
	return p.err
}

func (p *fakePublisher) PublishToConversation(context.Context, string, *streaming.Event) error {
	return p.err
}

func (p *fakePublisher) Close() error { return nil }

func TestService_validateScheduledTime(t *testing.T) {
	svc := &Service{}

	t.Run("too_soon", func(t *testing.T) {
		err := svc.validateScheduledTime(time.Now().Add(1 * time.Minute))
		require.ErrorIs(t, err, svcErrors.ErrScheduledTimeInPast)
	})

	t.Run("valid", func(t *testing.T) {
		err := svc.validateScheduledTime(time.Now().Add(10 * time.Minute))
		require.NoError(t, err)
	})

	t.Run("too_far", func(t *testing.T) {
		err := svc.validateScheduledTime(time.Now().Add(370 * 24 * time.Hour))
		require.ErrorIs(t, err, svcErrors.ErrValidationFailed)
	})
}

func TestService_emitScheduledStatusEvents(t *testing.T) {
	now := time.Now()
	publishedAt := now.Add(1 * time.Hour)
	status := &storage.ScheduledStatus{
		ID:          "sched1",
		Username:    "alice",
		ScheduledAt: now.Add(2 * time.Hour),
		PublishedAt: &publishedAt,
	}

	t.Run("created_returns_event_and_publishes", func(t *testing.T) {
		pub := &fakePublisher{}
		svc := NewService(nil, nil, nil, pub, zap.NewNop(), "example.com")

		events := svc.emitScheduledStatusCreatedEvents(context.Background(), status)

		require.Len(t, events, 1)
		require.Len(t, pub.userCalls, 1)
		call := pub.userCalls[0]
		require.Equal(t, "alice", call.user)
		require.NotNil(t, call.event)
		assert.Equal(t, "scheduled_status.created", call.event.Type)
		assert.Equal(t, "user:alice", call.event.Stream)
		assert.Equal(t, "sched1", call.event.Payload["id"])
	})

	t.Run("updated_returns_event_and_publishes", func(t *testing.T) {
		pub := &fakePublisher{}
		svc := NewService(nil, nil, nil, pub, zap.NewNop(), "example.com")

		events := svc.emitScheduledStatusUpdatedEvents(context.Background(), status)

		require.Len(t, events, 1)
		require.Len(t, pub.userCalls, 1)
		call := pub.userCalls[0]
		require.Equal(t, "alice", call.user)
		require.NotNil(t, call.event)
		assert.Equal(t, "scheduled_status.updated", call.event.Type)
		assert.Equal(t, "user:alice", call.event.Stream)
		assert.Equal(t, "sched1", call.event.Payload["id"])
	})

	t.Run("deleted_publishes", func(t *testing.T) {
		pub := &fakePublisher{}
		svc := NewService(nil, nil, nil, pub, zap.NewNop(), "example.com")

		svc.emitScheduledStatusDeletedEvents(context.Background(), status)
		require.Len(t, pub.userCalls, 1)
		assert.Equal(t, "scheduled_status.deleted", pub.userCalls[0].event.Type)
	})

	t.Run("published_publishes", func(t *testing.T) {
		pub := &fakePublisher{}
		svc := NewService(nil, nil, nil, pub, zap.NewNop(), "example.com")

		svc.emitScheduledStatusPublishedEvents(context.Background(), status)
		require.Len(t, pub.userCalls, 1)
		assert.Equal(t, "scheduled_status.published", pub.userCalls[0].event.Type)
	})

	t.Run("publisher_error_is_non_fatal", func(t *testing.T) {
		pub := &fakePublisher{err: stderrors.New("boom")}
		svc := NewService(nil, nil, nil, pub, zap.NewNop(), "example.com")

		svc.emitScheduledStatusDeletedEvents(context.Background(), status)
		require.Len(t, pub.userCalls, 1)
	})

	t.Run("nil_publisher_noop", func(t *testing.T) {
		svc := NewService(nil, nil, nil, nil, zap.NewNop(), "example.com")

		assert.Nil(t, svc.emitScheduledStatusCreatedEvents(context.Background(), status))
		assert.Nil(t, svc.emitScheduledStatusUpdatedEvents(context.Background(), status))

		svc.emitScheduledStatusDeletedEvents(context.Background(), status)
		svc.emitScheduledStatusPublishedEvents(context.Background(), status)
	})
}
