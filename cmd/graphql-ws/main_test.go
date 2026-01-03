package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	liftadapters "github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

func newWebSocketLiftContext(t *testing.T, endpoint string, connectionID string, body string) *lift.Context {
	t.Helper()

	req := lift.NewRequest(&liftadapters.Request{
		TriggerType: liftadapters.TriggerWebSocket,
		Metadata: map[string]any{
			"connectionId":       connectionID,
			"managementEndpoint": endpoint,
			"routeKey":           "$default",
			"region":             "us-east-1",
		},
		Headers:     map[string]string{},
		QueryParams: map[string]string{},
		Body:        []byte(body),
	})
	ctx := lift.NewContext(context.Background(), req)
	ctx.SetRequestID("req")
	return ctx
}

func TestTokenHelpers(t *testing.T) {
	require.Equal(t, "", cleanToken("  "))
	require.Equal(t, "a+b", cleanToken("a b"))
	require.Equal(t, "a+b", cleanToken("a%20b"))

	require.Equal(t, "", normalizeAuthToken(" "))
	require.Equal(t, "token", normalizeAuthToken("Bearer token"))
	require.Equal(t, "token", normalizeAuthToken("bearer token"))

	require.Equal(t, "", previewToken(""))
	require.Equal(t, "short", previewToken("short"))
	require.Equal(t, "12345...67890", previewToken("1234567890abc67890"))
}

func TestSubscriptionHelpers(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	require.NotNil(t, s)

	err := s.registerConnection(context.Background(), "c1", "user", &auth.Claims{Username: "user"})
	require.Error(t, err) // repo not configured, but in-memory state should be updated

	calls := 0
	cancel1 := func() { calls++ }
	cancel2 := func() { calls++ }

	require.True(t, s.addSubscription("c1", "sub", cancel1))
	require.True(t, s.addSubscription("c1", "sub", cancel2))
	require.Equal(t, 1, calls, "replacing subscription cancels prior one")

	require.True(t, s.cancelSubscription(context.Background(), "c1", "sub"))
	require.Equal(t, 2, calls)
	require.False(t, s.cancelSubscription(context.Background(), "c1", "missing"))
}

func TestConvertErrors(t *testing.T) {
	errs := gqlerror.List{nil, gqlerror.Errorf("boom")}
	out := convertErrors(errs)
	require.Len(t, out, 1)
}

func TestLiftLoggerAdapter_NoPanic(t *testing.T) {
	adapter := &liftLoggerAdapter{logger: zap.NewNop()}
	adapter.Debug("d", map[string]any{"a": 1})
	adapter.Info("i", map[string]any{"b": 2})
	adapter.Warn("w", map[string]any{"c": 3})
	adapter.Error("e", map[string]any{"d": 4})

	withFields := adapter.WithFields(map[string]any{"x": "y"})
	require.NotNil(t, withFields)
	withField := adapter.WithField("k", "v")
	require.NotNil(t, withField)
}

func TestHandleDefaultLift_SendsExpectedMessages(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_SESSION_TOKEN", "dummy")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)

	require.NoError(t, s.handleDefaultLift(newWebSocketLiftContext(t, srv.URL, "c1", `{"type":"connection_init"}`)))
	require.NoError(t, s.handleDefaultLift(newWebSocketLiftContext(t, srv.URL, "c1", `{"type":"ping"}`)))
	require.NoError(t, s.handleDefaultLift(newWebSocketLiftContext(t, srv.URL, "c1", `{"id":"sub1","type":"complete"}`)))

	require.Len(t, bodies, 3)
	var env responseEnvelope
	require.NoError(t, json.Unmarshal(bodies[0], &env))
	require.Equal(t, "connection_ack", env.Type)
	require.NoError(t, json.Unmarshal(bodies[1], &env))
	require.Equal(t, "pong", env.Type)
	require.NoError(t, json.Unmarshal(bodies[2], &env))
	require.Equal(t, "complete", env.Type)

	// Unsupported type returns a 400 error without sending.
	err := s.handleDefaultLift(newWebSocketLiftContext(t, srv.URL, "c1", `{"type":"nope"}`))
	require.Error(t, err)

	// Invalid JSON returns 400 error without sending.
	err = s.handleDefaultLift(newWebSocketLiftContext(t, srv.URL, "c1", `{"type":`))
	require.Error(t, err)
}
