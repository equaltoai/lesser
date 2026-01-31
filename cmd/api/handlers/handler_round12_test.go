package lift

import (
	"context"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
)

type streamQueueCall struct {
	streamName string
	eventType  string
	payload    map[string]interface{}
}

type streamQueueStub struct {
	queueCalls []streamQueueCall
	queueErr   error
}

func (s *streamQueueStub) QueueEventForUser(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func (s *streamQueueStub) QueueEventForStream(_ context.Context, streamName string, eventType string, payload map[string]interface{}) error {
	s.queueCalls = append(s.queueCalls, streamQueueCall{
		streamName: streamName,
		eventType:  eventType,
		payload:    payload,
	})
	return s.queueErr
}

func (s *streamQueueStub) QueueEventForConversation(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func (s *streamQueueStub) QueueEventForFollowers(context.Context, string, string, map[string]interface{}) error {
	return nil
}

type streamQueuePublisherStub struct {
	streamQueueStub
}

func (s *streamQueuePublisherStub) PublishToUser(context.Context, string, *streaming.Event) error {
	return nil
}

func (s *streamQueuePublisherStub) PublishToStream(context.Context, string, *streaming.Event) error {
	return nil
}

func (s *streamQueuePublisherStub) PublishToConversation(context.Context, string, *streaming.Event) error {
	return nil
}

func (s *streamQueuePublisherStub) Close() error {
	return nil
}

func TestStreamingEventEmitterRound12(t *testing.T) {
	t.Run("queues all events", func(t *testing.T) {
		queue := &streamQueueStub{}
		emitter := &streamingEventEmitter{streamQueue: queue}

		err := emitter.EmitEvents(context.Background(), []*common.StreamingEvent{
			{Type: "status.created", Timestamp: time.Now(), Metadata: map[string]interface{}{"id": "s1"}},
			{Type: "status.updated", Timestamp: time.Now(), Metadata: nil},
		})
		require.NoError(t, err)
		require.Len(t, queue.queueCalls, 2)
		require.Equal(t, "user", queue.queueCalls[0].streamName)
		require.Equal(t, "status.created", queue.queueCalls[0].eventType)
		require.Equal(t, "s1", queue.queueCalls[0].payload["id"])
		require.Equal(t, "status.updated", queue.queueCalls[1].eventType)
	})

	t.Run("propagates queue errors", func(t *testing.T) {
		queue := &streamQueueStub{queueErr: stdErrors.New("boom")}
		emitter := &streamingEventEmitter{streamQueue: queue}

		err := emitter.EmitEvents(context.Background(), []*common.StreamingEvent{
			{Type: "status.created", Timestamp: time.Now(), Metadata: map[string]interface{}{"id": "s1"}},
		})
		require.Error(t, err)
		require.Len(t, queue.queueCalls, 1)
	})
}

func TestNewHandlerRound12(t *testing.T) {
	cfg := round11TestConfig()
	_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
	logger := round10TestLogger(t)

	t.Run("nil stream queue", func(t *testing.T) {
		h := NewHandler(cfg, repos, logger, nil, nil)
		require.NotNil(t, h)
		require.NotNil(t, h.converter)
		require.NotNil(t, h.loaders)
		require.NotNil(t, h.registry)
		require.Nil(t, h.streamQueue)
	})

	t.Run("stream queue without publisher", func(t *testing.T) {
		queue := &streamQueueStub{}
		h := NewHandler(cfg, repos, logger, nil, queue)
		require.NotNil(t, h)
		require.Equal(t, queue, h.streamQueue)
		require.NotNil(t, h.registry)
	})

	t.Run("stream queue implements publisher", func(t *testing.T) {
		queue := &streamQueuePublisherStub{}
		h := NewHandler(cfg, repos, logger, nil, queue)
		require.NotNil(t, h)
		require.Equal(t, queue, h.streamQueue)
		require.NotNil(t, h.registry)
	})
}

func TestAuthenticateWithScopeRound12(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
		require.Nil(t, claims)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
	})

	t.Run("invalid required scope returns internal error", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithScope(ctx, "bad")
		require.Nil(t, claims)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeInternal))
	})

	t.Run("invalid token scopes return invalid token", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"bad"})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
		require.Nil(t, claims)
		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("insufficient scope returns error", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
		require.Nil(t, claims)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeInsufficientScope))
	})

	t.Run("success returns claims", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
		require.NoError(t, err)
		require.Equal(t, "alice", claims.Username)
	})
}

func TestGetOptionalAuthenticatedUserRound12(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("no token returns empty", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "", h.getOptionalAuthenticatedUser(ctx))
	})

	t.Run("invalid token returns empty", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "", h.getOptionalAuthenticatedUser(ctx))
	})

	t.Run("valid token returns username", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "alice", h.getOptionalAuthenticatedUser(ctx))
	})
}

