package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

func TestSecureWebSocketPosturesRevalidatePersistedConnection(t *testing.T) {
	repo := &fakeConnectionRepo{getConnectionResp: &models.WebSocketConnection{
		ConnectionID: "connection-1",
		UserID:       "alice-id",
		Username:     "alice",
	}}
	streamHandler := &StreamingHandler{connectionRepo: repo}
	var resolved *apptheory.SecurePrincipal
	app := apptheory.NewSecure(apptheory.SecureOptions{
		Tier:              apptheory.TierP2,
		PrincipalResolver: streamHandler.resolveWebSocketPrincipal,
		WebSocketSupport:  true,
	})
	app.WebSocket("$default", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		resolved = ctx.SecurePrincipal()
		return &apptheory.Response{Status: 200}, nil
	}, apptheory.Optional())

	response := app.ServeWebSocket(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: `{"identity":"mallory"}`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "connection-1",
			RouteKey:     "$default",
		},
	})
	require.Equal(t, 200, response.StatusCode)
	require.NotNil(t, resolved)
	require.Equal(t, "alice", resolved.Identity)
	require.Equal(t, apptheory.AuthPostureOptional, app.Routes()[0].Posture)
}
