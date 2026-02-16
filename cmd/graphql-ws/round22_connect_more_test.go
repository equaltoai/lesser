package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHandleConnect_NoToken_PersistsPendingConnectionRecord(t *testing.T) {
	repo := &fakeConnRepo{}
	s := newServer(&fakeTokenValidator{}, nil, nil, zap.NewNop(), repo, nil)
	app := newWebSocketApp(s)

	resp := app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{}, map[string]string{}))
	require.Equal(t, 200, resp.StatusCode)

	require.Equal(t, int32(1), atomic.LoadInt32(&repo.writeCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.updateCalls))
	require.NotNil(t, repo.lastUpdated)
	require.Equal(t, graphqlWSName, repo.lastUpdated.Info.Protocol)
	require.Equal(t, "pending", repo.lastUpdated.Info.AuthMethod)

	state, err := s.getConnection(context.Background(), "c1")
	require.NoError(t, err)
	require.Equal(t, "", state.username)
	require.NotNil(t, s.wsContexts["c1"])
}

func TestHandleConnect_PersistFailure_CleansWebSocketContext(t *testing.T) {
	repo := &fakeConnRepo{writeErr: errors.New("boom")}
	validator := &fakeTokenValidator{claims: &auth.Claims{Username: "user"}}
	s := newServer(validator, nil, nil, zap.NewNop(), repo, nil)
	app := newWebSocketApp(s)

	resp := app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{"access_token": "t"}, map[string]string{}))
	require.Equal(t, 500, resp.StatusCode)

	_, ok := s.wsContexts["c1"]
	require.False(t, ok)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.writeCalls))
}
