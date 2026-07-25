package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

func newWebSocketEvent(routeKey, connectionID, body string, query map[string]string, headers map[string]string) events.APIGatewayWebsocketProxyRequest {
	if query == nil {
		query = map[string]string{}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	return events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: connectionID,
			RouteKey:     routeKey,
			DomainName:   "example.com",
			Stage:        "dev",
			RequestID:    "req",
			EventType:    "MESSAGE",
		},
		HTTPMethod:            "POST",
		Path:                  "/graphql-ws",
		Headers:               headers,
		QueryStringParameters: query,
		Body:                  body,
		IsBase64Encoded:       false,
	}
}

func newWebSocketApp(s *wsServer) *apptheory.App {
	app := apptheory.New()
	app.WebSocket("$connect", s.handleConnect)
	app.WebSocket("$disconnect", s.handleDisconnect)
	app.WebSocket("$default", s.handleDefault)
	return app
}

func newWebSocketAppTheoryServerContext(t *testing.T, routeKey, connectionID, body string) events.APIGatewayProxyResponse {
	t.Helper()

	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
	app := newWebSocketApp(s)
	return app.ServeWebSocket(context.Background(), newWebSocketEvent(routeKey, connectionID, body, nil, nil))
}

func previewToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 10 {
		return token
	}
	return token[:5] + "..." + token[len(token)-5:]
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
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
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

func TestHandleDefault_SendsExpectedMessages(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_SESSION_TOKEN", "dummy")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	var bodies [][]byte
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
	// Pretend $connect already authenticated so connection_init can ACK.
	s.connections["c1"] = &connectionState{username: "user", claims: &auth.Claims{Username: "user"}, subscriptions: map[string]*subscriptionState{}}
	s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		bodies = append(bodies, b)
		return nil
	}

	app := newWebSocketApp(s)
	resp := app.ServeWebSocket(context.Background(), newWebSocketEvent("$default", "c1", `{"type":"connection_init"}`, nil, nil))
	require.Equal(t, 200, resp.StatusCode)
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$default", "c1", `{"type":"ping"}`, nil, nil))
	require.Equal(t, 200, resp.StatusCode)
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$default", "c1", `{"id":"sub1","type":"complete"}`, nil, nil))
	require.Equal(t, 200, resp.StatusCode)

	require.Len(t, bodies, 3)
	var env responseEnvelope
	require.NoError(t, json.Unmarshal(bodies[0], &env))
	require.Equal(t, "connection_ack", env.Type)
	require.NoError(t, json.Unmarshal(bodies[1], &env))
	require.Equal(t, "pong", env.Type)
	require.NoError(t, json.Unmarshal(bodies[2], &env))
	require.Equal(t, "complete", env.Type)

	// Unsupported type returns a 400 error without sending.
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$default", "c1", `{"type":"nope"}`, nil, nil))
	require.Equal(t, 400, resp.StatusCode)
	require.Len(t, bodies, 3)

	// Invalid JSON returns 400 error without sending.
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$default", "c1", `{"type":`, nil, nil))
	require.Equal(t, 400, resp.StatusCode)
	require.Len(t, bodies, 3)
}

func TestHandleDefault_SubscribeWithoutInstanceRepo_SendsErrorAndComplete(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_SESSION_TOKEN", "dummy")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	var bodies [][]byte
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
	s.connections["c1"] = &connectionState{
		username:      "user",
		claims:        &auth.Claims{Username: "user"},
		subscriptions: map[string]*subscriptionState{},
	}
	s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		bodies = append(bodies, b)
		return nil
	}

	app := newWebSocketApp(s)
	resp := app.ServeWebSocket(context.Background(), newWebSocketEvent("$default", "c1", `{"id":"sub1","type":"subscribe","payload":{"query":"subscription { ping }"}}`, nil, nil))
	require.Equal(t, 200, resp.StatusCode)
	require.Len(t, bodies, 2)

	var env responseEnvelope
	require.NoError(t, json.Unmarshal(bodies[0], &env))
	require.Equal(t, "error", env.Type)
	require.Equal(t, "sub1", env.ID)

	require.NoError(t, json.Unmarshal(bodies[1], &env))
	require.Equal(t, "complete", env.Type)
	require.Equal(t, "sub1", env.ID)
}
