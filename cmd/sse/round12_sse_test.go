package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeValidator struct {
	claims *auth.EnhancedClaims
	err    error
	calls  int
}

func (f *fakeValidator) ValidateAccessToken(string) (*auth.EnhancedClaims, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

type fakeEventLog struct {
	enabled bool
	items   []streaming.StreamEventLogItem
	err     error
}

func (f *fakeEventLog) Enabled() bool { return f.enabled }

func (f *fakeEventLog) Query(context.Context, string, string, int32) ([]streaming.StreamEventLogItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

type scriptedEventLog struct {
	enabled bool
	queryFn func(ctx context.Context, streamName, afterID string, limit int32) ([]streaming.StreamEventLogItem, error)
}

func (s *scriptedEventLog) Enabled() bool { return s.enabled }

func (s *scriptedEventLog) Query(ctx context.Context, streamName, afterID string, limit int32) ([]streaming.StreamEventLogItem, error) {
	if s.queryFn == nil {
		return nil, nil
	}
	return s.queryFn(ctx, streamName, afterID, limit)
}

func TestNormalizeDeletePayload_Round12(t *testing.T) {
	require.Equal(t, "x", normalizeDeletePayload(streamEventTypeUpdate, "x"))
	require.Equal(t, "x", normalizeDeletePayload(streamEventTypeDelete, "x"))
	require.Equal(t, "123", normalizeDeletePayload(streamEventTypeDelete, `{"id":"123"}`))
	require.Equal(t, "123", normalizeDeletePayload(streamEventTypeDelete, `"123"`))
}

func TestPayloadHasMedia_Round12(t *testing.T) {
	require.False(t, payloadHasMedia("not-json"))
	require.False(t, payloadHasMedia(`{"media_attachments":[]}`))
	require.True(t, payloadHasMedia(`{"media_attachments":[{}]}`))
}

func TestRequireClaims_Round12(t *testing.T) {
	origAuth := authService
	t.Cleanup(func() { authService = origAuth })

	authService = &fakeValidator{err: errors.New("invalid")}
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Headers: map[string]string{}}))
	_, err := requireClaims(ctx)
	require.Error(t, err)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Headers: map[string]string{"Authorization": "Bearer "}}))
	_, err = requireClaims(ctx)
	require.Error(t, err)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Headers: map[string]string{"Authorization": "Bearer token"}}))
	_, err = requireClaims(ctx)
	require.Error(t, err)

	authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Headers: map[string]string{"Authorization": "Bearer token"}}))
	claims, err := requireClaims(ctx)
	require.NoError(t, err)
	require.Equal(t, "alice", claims.Username)
}

func TestShouldSkipSSEItem_Round12(t *testing.T) {
	require.False(t, shouldSkipSSEItem(false, streaming.StreamEventLogItem{Event: streamEventTypeUpdate, Data: "{}"}))
	require.False(t, shouldSkipSSEItem(true, streaming.StreamEventLogItem{Event: streamEventTypeDelete, Data: "{}"}))
	require.True(t, shouldSkipSSEItem(true, streaming.StreamEventLogItem{Event: streamEventTypeUpdate, Data: "{}"}))
	require.False(t, shouldSkipSSEItem(true, streaming.StreamEventLogItem{Event: streamEventTypeUpdate, Data: `{"media_attachments":[{}]}`}))
}

func TestEmitSSEItems_Round12(t *testing.T) {
	ch := make(chan lift.SSEEvent, 8)
	items := []streaming.StreamEventLogItem{
		{ID: "1", Event: streamEventTypeUpdate, Data: `{"media_attachments":[{}]}`},
		{ID: "2", Event: streamEventTypeDelete, Data: `{"id":"2"}`},
	}
	after := emitSSEItems(ch, items, false, "")
	require.Equal(t, "2", after)

	close(ch)
	var got []lift.SSEEvent
	for ev := range ch {
		got = append(got, ev)
	}
	require.Len(t, got, 2)
	require.Equal(t, "2", got[1].Data)
}

func TestEmitSSEItems_SkipsOnlyMedia_Round12(t *testing.T) {
	ch := make(chan lift.SSEEvent, 8)
	items := []streaming.StreamEventLogItem{
		{ID: "1", Event: streamEventTypeUpdate, Data: `{}`},
		{ID: "2", Event: streamEventTypeDelete, Data: `"2"`},
	}
	after := emitSSEItems(ch, items, true, "")
	require.Equal(t, "2", after)

	close(ch)
	var got []lift.SSEEvent
	for ev := range ch {
		got = append(got, ev)
	}
	require.Len(t, got, 1)
	require.Equal(t, streamEventTypeDelete, got[0].Event)
	require.Equal(t, "2", got[0].Data)
}

func TestWaitForSSEPoll_Round12(t *testing.T) {
	origAfter := timeAfterFn
	t.Cleanup(func() { timeAfterFn = origAfter })

	eventCh := make(chan lift.SSEEvent, 1)

	// ctx.Done branch.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.True(t, waitForSSEPoll(ctx, eventCh, time.NewTicker(time.Hour)))

	// heartbeat branch.
	hbCh := make(chan time.Time, 1)
	hbCh <- time.Now()
	ticker := &time.Ticker{C: hbCh}
	require.False(t, waitForSSEPoll(context.Background(), eventCh, ticker))
	select {
	case ev := <-eventCh:
		require.Equal(t, "keepalive", ev.Event)
	default:
		t.Fatal("expected keepalive event")
	}

	// time.After branch (stubbed).
	afterCh := make(chan time.Time, 1)
	afterCh <- time.Now()
	timeAfterFn = func(time.Duration) <-chan time.Time { return afterCh }
	require.False(t, waitForSSEPoll(context.Background(), eventCh, time.NewTicker(time.Hour)))
}

func TestStreamSSE_Unavailable_Round12(t *testing.T) {
	origLog := eventLog
	t.Cleanup(func() { eventLog = origLog })

	eventLog = &fakeEventLog{enabled: false}
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/"}))
	require.NoError(t, streamSSE(ctx, "public", false))
	require.Equal(t, http.StatusServiceUnavailable, ctx.Response.StatusCode)
}

func TestStreamSSE_RecordsLastEventID_Round12(t *testing.T) {
	origLog := eventLog
	origAuth := authService
	t.Cleanup(func() {
		eventLog = origLog
		authService = origAuth
	})

	called := make(chan struct{})
	var gotStreamName, gotAfterID string
	eventLog = &scriptedEventLog{
		enabled: true,
		queryFn: func(_ context.Context, streamName, afterID string, _ int32) ([]streaming.StreamEventLogItem, error) {
			gotStreamName = streamName
			gotAfterID = afterID
			close(called)
			return nil, errors.New("stop")
		},
	}
	authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method: http.MethodGet,
		Path:   "/api/v1/streaming/public",
		Headers: map[string]string{
			"Authorization": "Bearer token",
			"Last-Event-ID": "abc",
		},
	}))
	require.NoError(t, streamSSE(ctx, streaming.PublicStream, false))

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event log query")
	}
	require.Equal(t, streaming.PublicStream, gotStreamName)
	require.Equal(t, "abc", gotAfterID)
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	require.Equal(t, "text/event-stream", ctx.Response.Headers["Content-Type"])
}

func TestHandlers_Round12(t *testing.T) {
	origLog := eventLog
	origAuth := authService
	t.Cleanup(func() {
		eventLog = origLog
		authService = origAuth
	})

	authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
	queries := make(chan struct{}, 16)
	eventLog = &scriptedEventLog{
		enabled: true,
		queryFn: func(_ context.Context, _ string, _ string, _ int32) ([]streaming.StreamEventLogItem, error) {
			queries <- struct{}{}
			return nil, errors.New("stop")
		},
	}

	// Root + health.
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming"}))
	require.NoError(t, handleStreamingRoot(ctx))
	require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming/health"}))
	require.NoError(t, handleHealth(ctx))

	// User streams.
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/streaming/user",
		Headers: map[string]string{"Authorization": "Bearer token"},
	}))
	require.NoError(t, handleUserStream(ctx))
	require.Equal(t, "text/event-stream", ctx.Response.Headers["Content-Type"])

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/streaming/user/notification",
		Headers: map[string]string{"Authorization": "Bearer token"},
	}))
	require.NoError(t, handleUserNotificationStream(ctx))

	// Public handler closure.
	public := handlePublicStream(streaming.PublicStream)
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/api/v1/streaming/public",
		Headers:     map[string]string{"Authorization": "Bearer token"},
		QueryParams: map[string]string{"only_media": "true"},
	}))
	require.NoError(t, public.Handle(ctx))

	// Hashtag handler closure.
	hashtag := handleHashtagStream(false)
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/api/v1/streaming/hashtag",
		Headers:     map[string]string{"Authorization": "Bearer token"},
		QueryParams: map[string]string{},
	}))
	require.NoError(t, hashtag.Handle(ctx))
	require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/api/v1/streaming/hashtag",
		Headers:     map[string]string{"Authorization": "Bearer token"},
		QueryParams: map[string]string{"tag": "cats"},
	}))
	require.NoError(t, hashtag.Handle(ctx))
	require.Equal(t, "text/event-stream", ctx.Response.Headers["Content-Type"])

	// List stream.
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/api/v1/streaming/list",
		Headers:     map[string]string{"Authorization": "Bearer token"},
		QueryParams: map[string]string{},
	}))
	require.NoError(t, handleListStream(ctx))
	require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/api/v1/streaming/list",
		Headers:     map[string]string{"Authorization": "Bearer token"},
		QueryParams: map[string]string{"list": "1"},
	}))
	require.NoError(t, handleListStream(ctx))
	require.Equal(t, "text/event-stream", ctx.Response.Headers["Content-Type"])

	// Direct stream.
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/streaming/direct",
		Headers: map[string]string{"Authorization": "Bearer token"},
	}))
	require.NoError(t, handleDirectStream(ctx))
	require.Equal(t, "text/event-stream", ctx.Response.Headers["Content-Type"])

	for i := 0; i < 6; i++ {
		select {
		case <-queries:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event log query %d", i+1)
		}
	}
}

func TestHandlers_Unauthorized_Round12(t *testing.T) {
	origAuth := authService
	t.Cleanup(func() { authService = origAuth })

	authService = &fakeValidator{err: errors.New("invalid")}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming/user"}))
	err := handleUserStream(ctx)
	require.Error(t, err)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming/user/notification"}))
	err = handleUserNotificationStream(ctx)
	require.Error(t, err)

	public := handlePublicStream(streaming.PublicStream)
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming/public"}))
	err = public.Handle(ctx)
	require.Error(t, err)

	hashtag := handleHashtagStream(false)
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming/hashtag"}))
	err = hashtag.Handle(ctx)
	require.Error(t, err)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming/list"}))
	err = handleListStream(ctx)
	require.Error(t, err)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/api/v1/streaming/direct"}))
	err = handleDirectStream(ctx)
	require.Error(t, err)
}

func TestHandleHashtagStream_LocalOnly_Round12(t *testing.T) {
	origLog := eventLog
	origAuth := authService
	t.Cleanup(func() {
		eventLog = origLog
		authService = origAuth
	})

	authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
	called := make(chan struct{})
	var gotStreamName string
	eventLog = &scriptedEventLog{
		enabled: true,
		queryFn: func(_ context.Context, streamName, _ string, _ int32) ([]streaming.StreamEventLogItem, error) {
			gotStreamName = streamName
			close(called)
			return nil, errors.New("stop")
		},
	}

	handler := handleHashtagStream(true)
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/api/v1/streaming/hashtag/local",
		Headers:     map[string]string{"Authorization": "Bearer token"},
		QueryParams: map[string]string{"tag": "cats"},
	}))
	require.NoError(t, handler.Handle(ctx))

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for local hashtag query")
	}
	require.Equal(t, "hashtag:local:cats", gotStreamName)
}

func TestProduceSSEEvents_Round12(t *testing.T) {
	origLog := eventLog
	t.Cleanup(func() { eventLog = origLog })

	t.Run("query error emits error event", func(t *testing.T) {
		eventLog = &scriptedEventLog{
			enabled: true,
			queryFn: func(_ context.Context, _ string, _ string, _ int32) ([]streaming.StreamEventLogItem, error) {
				return nil, errors.New("boom")
			},
		}

		ch := make(chan lift.SSEEvent, 4)
		produceSSEEvents(context.Background(), ch, "stream", false, "")

		var got []lift.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Equal(t, "error", got[0].Event)
		require.Contains(t, got[0].Data, "internal_error")
	})

	t.Run("items emit and loop continues", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		calls := 0
		eventLog = &scriptedEventLog{
			enabled: true,
			queryFn: func(_ context.Context, _ string, _ string, _ int32) ([]streaming.StreamEventLogItem, error) {
				calls++
				if calls == 1 {
					return []streaming.StreamEventLogItem{
						{ID: "1", Event: streamEventTypeUpdate, Data: `{}`},
						{ID: "2", Event: streamEventTypeDelete, Data: `{"id":"2"}`},
					}, nil
				}
				cancel()
				return nil, nil
			},
		}

		ch := make(chan lift.SSEEvent, 4)
		produceSSEEvents(ctx, ch, "stream", true, "")

		var got []lift.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Equal(t, streamEventTypeDelete, got[0].Event)
		require.Equal(t, "2", got[0].Data)
	})
}

func TestSSEStreamStateExpired_Round12(t *testing.T) {
	state := sseStreamState{start: time.Now().Add(-streamMaxDuration - time.Second)}
	require.True(t, state.expired())
}

func TestInitializeAndRunSSE_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origDefaults := initializeWithDefaultsFn
	origNewClient := newLambdaOptimizedClientFn
	origNewRepos := newRepositoryFactoryFn
	origAuth := newAuthServiceFn
	origEventLog := newStreamEventLogFn
	origStart := lambdaStartFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origDefaults
		newLambdaOptimizedClientFn = origNewClient
		newRepositoryFactoryFn = origNewRepos
		newAuthServiceFn = origAuth
		newStreamEventLogFn = origEventLog
		lambdaStartFn = origStart
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Region:          "us-east-1",
				DynamoTableName: "test-table",
				StreamEventsTable: "stream-events",
			},
			Logger: zap.NewNop(),
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return new(dynamormmocks.MockDB), nil
	}
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (*factory.RepositoryFactory, error) {
		return nil, nil
	}
	newAuthServiceFn = func(*config.Config, core.RepositoryStorage) (accessTokenValidator, error) {
		return &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}, nil
	}
	newStreamEventLogFn = func(dynamormCore.DB, time.Duration) streamEventLog {
		return &fakeEventLog{enabled: false}
	}

	initializeSSE()
	require.NotNil(t, lambdaCtx)
	require.NotNil(t, cfg)
	require.NotNil(t, logger)
	require.NotNil(t, authService)
	require.NotNil(t, eventLog)

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		fn, ok := handler.(func(context.Context, any) (any, error))
		require.True(t, ok)

		event := map[string]any{
			"version":  "2.0",
			"routeKey": "GET /api/v1/streaming/health",
			"rawPath":  "/api/v1/streaming/health",
			"requestContext": map[string]any{
				"requestId": "req",
				"http": map[string]any{
					"method": "GET",
					"path":   "/api/v1/streaming/health",
				},
			},
		}
		resp, err := fn(context.Background(), event)
		require.NoError(t, err)
		liftResp, ok := resp.(*lift.Response)
		require.True(t, ok)
		require.Equal(t, 200, liftResp.StatusCode)
	}

	cfg.DebugMode = true
	runSSE()
	require.True(t, called)

	main()
}
