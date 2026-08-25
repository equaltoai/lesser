package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
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
	_, resp := requireClaims(&apptheory.Context{
		Request: apptheory.Request{Headers: map[string][]string{}},
	})
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	_, resp = requireClaims(&apptheory.Context{
		Request: apptheory.Request{Headers: map[string][]string{"authorization": {"Bearer "}}},
	})
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	_, resp = requireClaims(&apptheory.Context{
		Request: apptheory.Request{Headers: map[string][]string{"authorization": {"Bearer token"}}},
	})
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
	claims, resp := requireClaims(&apptheory.Context{
		Request: apptheory.Request{Headers: map[string][]string{"authorization": {"Bearer token"}}},
	})
	require.Nil(t, resp)
	require.Equal(t, "alice", claims.Username)
}

func TestSecureStreamingRoutesPreserveLegacyUnauthorizedBodies(t *testing.T) {
	originalAuth := authService
	originalLogger := logger
	t.Cleanup(func() {
		authService = originalAuth
		logger = originalLogger
	})
	logger = zap.NewNop()

	routes := []string{
		"/api/v1/streaming/user",
		"/api/v1/streaming/user/notification",
		"/api/v1/streaming/public",
		"/api/v1/streaming/public/local",
		"/api/v1/streaming/public/remote",
		"/api/v1/streaming/hashtag",
		"/api/v1/streaming/hashtag/local",
		"/api/v1/streaming/list",
		"/api/v1/streaming/direct",
	}

	for _, route := range routes {
		t.Run(route+" missing credential", func(t *testing.T) {
			authService = &fakeValidator{}
			resp := buildSSEApp().Serve(context.Background(), apptheory.Request{Method: http.MethodGet, Path: route})
			require.Equal(t, http.StatusUnauthorized, resp.Status)
			require.JSONEq(t, `{"error":"Authentication required"}`, string(resp.Body))
		})

		t.Run(route+" invalid credential", func(t *testing.T) {
			authService = &fakeValidator{err: auth.ErrInvalidToken}
			resp := buildSSEApp().Serve(context.Background(), apptheory.Request{
				Method:  http.MethodGet,
				Path:    route,
				Headers: map[string][]string{"authorization": {"Bearer invalid"}},
			})
			require.Equal(t, http.StatusUnauthorized, resp.Status)
			require.JSONEq(t, `{"error":"Invalid token"}`, string(resp.Body))
		})
	}

	for _, route := range buildSSEApp().Routes() {
		if route.Path == "/api/v1/streaming" || route.Path == "/api/v1/streaming/health" || route.Path == "/api/v1/streaming/oauth/device" {
			continue
		}
		require.Equal(t, apptheory.AuthPosturePublic, route.Posture, route.Path)
	}
}

func TestShouldSkipSSEItem_Round12(t *testing.T) {
	require.False(t, shouldSkipSSEItem(false, streaming.StreamEventLogItem{Event: streamEventTypeUpdate, Data: "{}"}))
	require.False(t, shouldSkipSSEItem(true, streaming.StreamEventLogItem{Event: streamEventTypeDelete, Data: "{}"}))
	require.True(t, shouldSkipSSEItem(true, streaming.StreamEventLogItem{Event: streamEventTypeUpdate, Data: "{}"}))
	require.False(t, shouldSkipSSEItem(true, streaming.StreamEventLogItem{Event: streamEventTypeUpdate, Data: `{"media_attachments":[{}]}`}))
}

func TestEmitSSEItems_Round12(t *testing.T) {
	ch := make(chan apptheory.SSEEvent, 8)
	items := []streaming.StreamEventLogItem{
		{ID: "1", Event: streamEventTypeUpdate, Data: `{"media_attachments":[{}]}`},
		{ID: "2", Event: streamEventTypeDelete, Data: `{"id":"2"}`},
	}
	after := emitSSEItems(ch, items, false, "")
	require.Equal(t, "2", after)

	close(ch)
	var got []apptheory.SSEEvent
	for ev := range ch {
		got = append(got, ev)
	}
	require.Len(t, got, 2)
	require.Equal(t, "2", got[1].Data)
}

func TestEmitSSEItems_SkipsOnlyMedia_Round12(t *testing.T) {
	ch := make(chan apptheory.SSEEvent, 8)
	items := []streaming.StreamEventLogItem{
		{ID: "1", Event: streamEventTypeUpdate, Data: `{}`},
		{ID: "2", Event: streamEventTypeDelete, Data: `"2"`},
	}
	after := emitSSEItems(ch, items, true, "")
	require.Equal(t, "2", after)

	close(ch)
	var got []apptheory.SSEEvent
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

	eventCh := make(chan apptheory.SSEEvent, 1)

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
	resp, err := streamSSE(&apptheory.Context{
		Request: apptheory.Request{Method: http.MethodGet, Path: "/"},
	}, "public", false)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusServiceUnavailable, resp.Status)
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

	resp, err := streamSSE(&apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodGet,
			Path:   "/api/v1/streaming/public",
			Headers: map[string][]string{
				"authorization": {"Bearer token"},
				"last-event-id": {"abc"},
			},
		},
	}, streaming.PublicStream, false)
	require.NoError(t, err)

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event log query")
	}
	require.Equal(t, streaming.PublicStream, gotStreamName)
	require.Equal(t, "abc", gotAfterID)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, []string{"text/event-stream"}, resp.Headers["content-type"])
	_, _ = io.ReadAll(resp.BodyReader)
}

func TestHandlers_Round12(t *testing.T) {
	origLog := eventLog
	origAuth := authService
	origAuthorize := authorizeListStreamFn
	t.Cleanup(func() {
		eventLog = origLog
		authService = origAuth
		authorizeListStreamFn = origAuthorize
	})

	authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
	authorizeListStreamFn = func(context.Context, string, string) error { return nil }
	queries := make(chan struct{}, 16)
	eventLog = &scriptedEventLog{
		enabled: true,
		queryFn: func(_ context.Context, _ string, _ string, _ int32) ([]streaming.StreamEventLogItem, error) {
			queries <- struct{}{}
			return nil, errors.New("stop")
		},
	}

	// Root + health.
	resp, err := handleStreamingRoot(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusNotFound, resp.Status)

	resp, err = handleHealth(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/health"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status)

	// User streams.
	resp, err = handleUserStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/user",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []string{"text/event-stream"}, resp.Headers["content-type"])
	_, _ = io.ReadAll(resp.BodyReader)

	resp, err = handleUserNotificationStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/user/notification",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	_, _ = io.ReadAll(resp.BodyReader)

	// Public handler closure.
	public := handlePublicStream(streaming.PublicStream)
	resp, err = public(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/public",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
			Query:   map[string][]string{"only_media": {"true"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	_, _ = io.ReadAll(resp.BodyReader)

	// Hashtag handler closure.
	hashtag := handleHashtagStream(false)
	resp, err = hashtag(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/hashtag",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
			Query:   map[string][]string{},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.Status)

	resp, err = hashtag(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/hashtag",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
			Query:   map[string][]string{"tag": {"cats"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []string{"text/event-stream"}, resp.Headers["content-type"])
	_, _ = io.ReadAll(resp.BodyReader)

	// List stream.
	resp, err = handleListStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/list",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
			Query:   map[string][]string{},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.Status)

	resp, err = handleListStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/list",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
			Query:   map[string][]string{"list": {"1"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []string{"text/event-stream"}, resp.Headers["content-type"])
	_, _ = io.ReadAll(resp.BodyReader)

	// Direct stream.
	resp, err = handleDirectStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/direct",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []string{"text/event-stream"}, resp.Headers["content-type"])
	_, _ = io.ReadAll(resp.BodyReader)

	for i := 0; i < 6; i++ {
		select {
		case <-queries:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event log query %d", i+1)
		}
	}
}

func TestHandleListStream_AuthorizationFailure_Round12(t *testing.T) {
	origAuth := authService
	origAuthorize := authorizeListStreamFn
	t.Cleanup(func() {
		authService = origAuth
		authorizeListStreamFn = origAuthorize
	})

	authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
	authorizeListStreamFn = func(context.Context, string, string) error { return apperrors.NotFound("list") }

	resp, err := handleListStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/list",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
			Query:   map[string][]string{"list": {"1"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusNotFound, resp.Status)
}

func TestHandlers_Unauthorized_Round12(t *testing.T) {
	origAuth := authService
	t.Cleanup(func() { authService = origAuth })

	authService = &fakeValidator{err: errors.New("invalid")}

	resp, err := handleUserStream(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/user"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	resp, err = handleUserNotificationStream(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/user/notification"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	public := handlePublicStream(streaming.PublicStream)
	resp, err = public(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/public"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	hashtag := handleHashtagStream(false)
	resp, err = hashtag(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/hashtag"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	resp, err = handleListStream(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/list"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)

	resp, err = handleDirectStream(&apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/direct"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 401, resp.Status)
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
	resp, err := handler(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/hashtag/local",
			Headers: map[string][]string{"authorization": {"Bearer token"}},
			Query:   map[string][]string{"tag": {"cats"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	_, _ = io.ReadAll(resp.BodyReader)

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for local hashtag query")
	}
	require.Equal(t, "hashtag:local:cats", gotStreamName)
}

func TestHandleOAuthDeviceStream_Round23(t *testing.T) {
	origCfg := cfg
	origGet := getOAuthDeviceSessionFn
	origAfter := timeAfterFn
	t.Cleanup(func() {
		cfg = origCfg
		getOAuthDeviceSessionFn = origGet
		timeAfterFn = origAfter
	})

	cfg = &config.Config{AllowDeviceFlow: false}
	resp, err := handleOAuthDeviceStream(&apptheory.Context{
		Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/streaming/oauth/device"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.Status)

	cfg.AllowDeviceFlow = true
	resp, err = handleOAuthDeviceStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/oauth/device",
			Headers: map[string][]string{},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.Status)

	timeAfterFn = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}

	calls := 0
	getOAuthDeviceSessionFn = func(context.Context, string) (*storage.OAuthDeviceSession, error) {
		calls++
		if calls == 1 {
			return &storage.OAuthDeviceSession{
				Status:    "pending",
				ExpiresAt: time.Now().Add(1 * time.Minute),
			}, nil
		}
		return &storage.OAuthDeviceSession{
			Status:           "approved",
			ApprovedUsername: "alice",
			ExpiresAt:        time.Now().Add(1 * time.Minute),
		}, nil
	}

	resp, err = handleOAuthDeviceStream(&apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/streaming/oauth/device",
			Headers: map[string][]string{"x-lesser-device-code": {"dev-1"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, []string{"text/event-stream"}, resp.Headers["content-type"])

	body, readErr := io.ReadAll(resp.BodyReader)
	require.NoError(t, readErr)
	require.Contains(t, string(body), `"status":"pending"`)
	require.Contains(t, string(body), `"status":"approved"`)
	require.Contains(t, string(body), `"username":"alice"`)
}

func TestProduceOAuthDeviceStreamEvents_Round23(t *testing.T) {
	origGet := getOAuthDeviceSessionFn
	t.Cleanup(func() { getOAuthDeviceSessionFn = origGet })

	t.Run("empty device code hash emits invalid", func(t *testing.T) {
		ch := make(chan apptheory.SSEEvent, 2)
		produceOAuthDeviceStreamEvents(context.Background(), ch, "")

		var got []apptheory.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Contains(t, got[0].Data.(string), `"status":"invalid"`)
	})

	t.Run("denied emits denied", func(t *testing.T) {
		getOAuthDeviceSessionFn = func(context.Context, string) (*storage.OAuthDeviceSession, error) {
			return &storage.OAuthDeviceSession{
				Status:    "denied",
				ExpiresAt: time.Now().Add(1 * time.Minute),
			}, nil
		}

		ch := make(chan apptheory.SSEEvent, 2)
		produceOAuthDeviceStreamEvents(context.Background(), ch, "hash")

		var got []apptheory.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Contains(t, got[0].Data.(string), `"status":"denied"`)
	})

	t.Run("session lookup error emits invalid", func(t *testing.T) {
		getOAuthDeviceSessionFn = func(context.Context, string) (*storage.OAuthDeviceSession, error) {
			return nil, errors.New("boom")
		}

		ch := make(chan apptheory.SSEEvent, 2)
		produceOAuthDeviceStreamEvents(context.Background(), ch, "hash")

		var got []apptheory.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Contains(t, got[0].Data.(string), `"status":"invalid"`)
	})

	t.Run("expired emits expired", func(t *testing.T) {
		getOAuthDeviceSessionFn = func(context.Context, string) (*storage.OAuthDeviceSession, error) {
			return &storage.OAuthDeviceSession{
				Status:    "pending",
				ExpiresAt: time.Now().Add(-1 * time.Minute),
			}, nil
		}

		ch := make(chan apptheory.SSEEvent, 2)
		produceOAuthDeviceStreamEvents(context.Background(), ch, "hash")

		var got []apptheory.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Contains(t, got[0].Data.(string), `"status":"expired"`)
	})

	t.Run("consumed emits consumed", func(t *testing.T) {
		getOAuthDeviceSessionFn = func(context.Context, string) (*storage.OAuthDeviceSession, error) {
			return &storage.OAuthDeviceSession{
				Status:    "consumed",
				ExpiresAt: time.Now().Add(1 * time.Minute),
			}, nil
		}

		ch := make(chan apptheory.SSEEvent, 2)
		produceOAuthDeviceStreamEvents(context.Background(), ch, "hash")

		var got []apptheory.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Contains(t, got[0].Data.(string), `"status":"consumed"`)
	})

	t.Run("unknown status emits invalid", func(t *testing.T) {
		getOAuthDeviceSessionFn = func(context.Context, string) (*storage.OAuthDeviceSession, error) {
			return &storage.OAuthDeviceSession{
				Status:    "wat",
				ExpiresAt: time.Now().Add(1 * time.Minute),
			}, nil
		}

		ch := make(chan apptheory.SSEEvent, 2)
		produceOAuthDeviceStreamEvents(context.Background(), ch, "hash")

		var got []apptheory.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Contains(t, got[0].Data.(string), `"status":"invalid"`)
	})
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

		ch := make(chan apptheory.SSEEvent, 4)
		produceSSEEvents(context.Background(), ch, "stream", false, "")

		var got []apptheory.SSEEvent
		for ev := range ch {
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		require.Equal(t, "error", got[0].Event)
		require.Contains(t, got[0].Data.(string), "internal_error")
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

		ch := make(chan apptheory.SSEEvent, 4)
		produceSSEEvents(ctx, ch, "stream", true, "")

		var got []apptheory.SSEEvent
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
				Region:            "us-east-1",
				DynamoTableName:   "test-table",
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
		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.APIGatewayV2HTTPRequest{
			Version:  "2.0",
			RouteKey: "GET /api/v1/streaming/health",
			RawPath:  "/api/v1/streaming/health",
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				RequestID: "req",
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: "GET",
					Path:   "/api/v1/streaming/health",
				},
			},
		}
		raw, err := json.Marshal(event)
		require.NoError(t, err)
		resp, err := fn(context.Background(), raw)
		require.NoError(t, err)
		lambdaResp, ok := resp.(events.APIGatewayV2HTTPResponse)
		require.True(t, ok)
		require.Equal(t, 200, lambdaResp.StatusCode)
	}

	cfg.DebugMode = true
	runSSE()
	require.True(t, called)

	main()
}
