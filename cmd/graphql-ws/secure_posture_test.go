package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

func TestSecureWebSocketPosturesRevalidatePersistedConnection(t *testing.T) {
	repo := &fakeConnRepo{getConnection: &models.WebSocketConnection{
		ConnectionID: "connection-1",
		UserID:       "alice-id",
		Username:     "alice",
		Info: models.ConnectionInfo{CustomHeaders: map[string]string{
			"scopes": "read write",
		}},
	}}
	server := newServer(nil, nil, nil, zap.NewNop(), repo, nil, nil)
	var resolved *apptheory.SecurePrincipal
	app := apptheory.NewSecure(apptheory.SecureOptions{
		Tier:              apptheory.TierP2,
		PrincipalResolver: server.resolveWebSocketPrincipal,
		WebSocketSupport:  true,
	})
	app.WebSocket("$default", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		resolved = ctx.SecurePrincipal()
		state, err := server.getConnection(resolvedGraphQLFrameContext(ctx), "connection-1")
		require.NoError(t, err)
		require.Equal(t, "alice", state.username)
		return &apptheory.Response{Status: 200}, nil
	}, apptheory.Optional())

	response := app.ServeWebSocket(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: `{"identity":"mallory","scopes":["admin"]}`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "connection-1",
			RouteKey:     "$default",
		},
	})
	require.Equal(t, 200, response.StatusCode)
	require.NotNil(t, resolved)
	require.Equal(t, "alice", resolved.Identity)
	require.Equal(t, []string{"read", "write"}, resolved.Scopes)
	require.EqualValues(t, 1, repo.getConnCalls)
	require.Equal(t, apptheory.AuthPostureOptional, app.Routes()[0].Posture)
}
