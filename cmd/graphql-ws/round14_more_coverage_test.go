package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

func TestGraphQLWSHelpers_Round14(t *testing.T) {
	t.Run("sendJSON rejects nil websocket context", func(t *testing.T) {
		s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
		require.Error(t, s.sendJSON(nil, map[string]any{"ok": true}))
	})

	t.Run("sendJSON uses injected sender when available", func(t *testing.T) {
		s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
		calls := 0
		s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
			calls++
			_, err := json.Marshal(payload)
			return err
		}
		require.NoError(t, s.sendJSON(&apptheory.WebSocketContext{ConnectionID: "c1"}, responseEnvelope{Type: "pong"}))
		require.Equal(t, 1, calls)
	})

	t.Run("queryValue and headerValue tolerate nil context", func(t *testing.T) {
		require.Equal(t, "", queryValue(nil, "k"))
		require.Equal(t, "", headerValue(nil, "k"))
	})

	t.Run("headerValue normalizes key casing and whitespace", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Headers: map[string][]string{
					"authorization": {"Bearer token"},
				},
			},
		}
		require.Equal(t, "Bearer token", headerValue(ctx, " Authorization "))
		require.Equal(t, "", headerValue(ctx, "missing"))
	})

	t.Run("appError defaults code and message", func(t *testing.T) {
		err := appError("   ", " ")
		appErr, ok := err.(*apptheory.AppTheoryError)
		require.True(t, ok)
		require.Equal(t, "app.internal", appErr.Code)
		require.Equal(t, "internal error", appErr.Message)
	})

	t.Run("sendGraphQLErrors emits unknown error when list empty", func(t *testing.T) {
		wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}
		var payload []byte

		s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
		s.sendJSONMessage = func(_ *apptheory.WebSocketContext, env any) error {
			b, err := json.Marshal(env)
			require.NoError(t, err)
			payload = b
			return nil
		}

		s.sendGraphQLErrors(wsCtx, "id1", gqlerror.List{nil})
		require.Contains(t, string(payload), "unknown error")
	})
}

func TestHandleConnect_CleansUpOnPersistError_Round14(t *testing.T) {
	setDummyAWSEnv(t)

	writeErr := errors.New("write failed")
	connRepo := &fakeConnRepo{writeErr: writeErr}
	server := newServer(nil, nil, nil, zap.NewNop(), connRepo, nil, nil)
	app := newWebSocketApp(server)

	resp := app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{"access_token": "t"}, map[string]string{}))
	require.Equal(t, 200, resp.StatusCode)
	require.NotNil(t, server.wsContexts["c1"], "connect should retain websocket context for message sending")
}

func TestHandleDefaultAndDisconnect_NilContexts_Round14(t *testing.T) {
	server := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)

	resp, err := server.handleDisconnect(nil)
	require.NoError(t, err)
	require.Equal(t, 200, resp.Status)

	resp, err = server.handleDefault(nil)
	require.Error(t, err)
	require.Nil(t, resp)
}
