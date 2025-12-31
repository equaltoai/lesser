package search

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakePublisher struct {
	streamCalls []fakePublisherStreamCall
	userCalls   []fakePublisherUserCall
	err         error
}

type fakePublisherStreamCall struct {
	stream string
	event  *streaming.Event
}

type fakePublisherUserCall struct {
	user  string
	event *streaming.Event
}

func (p *fakePublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	p.userCalls = append(p.userCalls, fakePublisherUserCall{user: userID, event: event})
	return p.err
}

func (p *fakePublisher) PublishToStream(_ context.Context, streamName string, event *streaming.Event) error {
	p.streamCalls = append(p.streamCalls, fakePublisherStreamCall{stream: streamName, event: event})
	return p.err
}

func (p *fakePublisher) PublishToConversation(context.Context, string, *streaming.Event) error {
	return p.err
}

func (p *fakePublisher) Close() error { return nil }

func TestService_isLocal(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")

	assert.True(t, svc.isLocal("https://example.com/@alice"))
	assert.True(t, svc.isLocal("http://example.com/@alice"))
	assert.False(t, svc.isLocal("https://remote.example/@alice"))
	assert.False(t, svc.isLocal(""))
}

func TestGetStatusIDFromResult(t *testing.T) {
	t.Run("map_id_string", func(t *testing.T) {
		got := getStatusIDFromResult(map[string]interface{}{"id": "123"})
		assert.Equal(t, "123", got)
	})

	t.Run("map_id_not_string", func(t *testing.T) {
		got := getStatusIDFromResult(map[string]interface{}{"id": 123})
		assert.Equal(t, "", got)
	})

	t.Run("struct_with_ID_field", func(t *testing.T) {
		got := getStatusIDFromResult(struct{ ID string }{ID: "abc"})
		assert.Equal(t, "abc", got)
	})

	t.Run("status_search_result_pointer", func(t *testing.T) {
		got := getStatusIDFromResult(&storage.StatusSearchResult{ID: "s1"})
		assert.Equal(t, "s1", got)
	})

	t.Run("status_search_result_value", func(t *testing.T) {
		got := getStatusIDFromResult(storage.StatusSearchResult{ID: "s2"})
		assert.Equal(t, "s2", got)
	})

	t.Run("unknown_type", func(t *testing.T) {
		got := getStatusIDFromResult(123)
		assert.Equal(t, "", got)
	})
}

func TestService_emitSearchEvent(t *testing.T) {
	t.Run("nil_publisher_noop", func(t *testing.T) {
		svc := NewService(nil, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
		svc.emitSearchEvent(context.Background(), &Query{Query: "cats", Type: "accounts"})
	})

	t.Run("publishes_to_analytics_stream", func(t *testing.T) {
		pub := &fakePublisher{}
		svc := NewService(nil, nil, nil, nil, nil, pub, zap.NewNop(), "example.com")

		now := time.Now()
		svc.emitSearchEvent(context.Background(), &Query{Query: "cats", Type: "accounts"})

		require.Len(t, pub.streamCalls, 1)
		call := pub.streamCalls[0]
		require.Equal(t, "analytics", call.stream)
		require.NotNil(t, call.event)
		assert.Equal(t, "search.performed", call.event.Type)
		assert.Equal(t, "analytics", call.event.Stream)
		assert.Equal(t, "cats", call.event.Payload["query"])
		assert.Equal(t, "accounts", call.event.Payload["type"])
		assert.False(t, call.event.Timestamp.IsZero())
		assert.GreaterOrEqual(t, call.event.Timestamp, now)
	})

	t.Run("publish_error_is_non_fatal", func(t *testing.T) {
		pub := &fakePublisher{err: stderrors.New("boom")}
		svc := NewService(nil, nil, nil, nil, nil, pub, zap.NewNop(), "example.com")

		svc.emitSearchEvent(context.Background(), &Query{Query: "cats", Type: "accounts"})
		require.Len(t, pub.streamCalls, 1)
	})
}

func TestService_emitSuggestionRemovedEvent(t *testing.T) {
	t.Run("nil_publisher_noop", func(t *testing.T) {
		svc := NewService(nil, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
		svc.emitSuggestionRemovedEvent(context.Background(), &RemoveSuggestionCommand{Username: "alice", AccountID: "acct123"})
	})

	t.Run("publishes_to_user_stream", func(t *testing.T) {
		pub := &fakePublisher{}
		svc := NewService(nil, nil, nil, nil, nil, pub, zap.NewNop(), "example.com")

		svc.emitSuggestionRemovedEvent(context.Background(), &RemoveSuggestionCommand{Username: "alice", AccountID: "acct123"})

		require.Len(t, pub.userCalls, 1)
		call := pub.userCalls[0]
		require.Equal(t, "alice", call.user)
		require.NotNil(t, call.event)
		assert.Equal(t, "suggestion.removed", call.event.Type)
		assert.Equal(t, "user:alice", call.event.Stream)
		assert.Equal(t, "acct123", call.event.Payload["account_id"])
		assert.False(t, call.event.Timestamp.IsZero())
	})

	t.Run("publish_error_is_non_fatal", func(t *testing.T) {
		pub := &fakePublisher{err: stderrors.New("boom")}
		svc := NewService(nil, nil, nil, nil, nil, pub, zap.NewNop(), "example.com")

		svc.emitSuggestionRemovedEvent(context.Background(), &RemoveSuggestionCommand{Username: "alice", AccountID: "acct123"})
		require.Len(t, pub.userCalls, 1)
	})
}
