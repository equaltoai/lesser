package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func TestExtractAccessTokenFromInitPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload json.RawMessage
		want    string
	}{
		{name: "empty", payload: nil, want: ""},
		{name: "invalid_json", payload: json.RawMessage("{"), want: ""},
		{name: "access_token", payload: json.RawMessage(`{"access_token":"a b"}`), want: "a+b"},
		{name: "accessToken", payload: json.RawMessage(`{"accessToken":"t"}`), want: "t"},
		{name: "token", payload: json.RawMessage(`{"token":"t"}`), want: "t"},
		{name: "authToken", payload: json.RawMessage(`{"authToken":"t"}`), want: "t"},
		{name: "authorization_lower", payload: json.RawMessage(`{"authorization":"Bearer token"}`), want: "token"},
		{name: "authorization_upper", payload: json.RawMessage(`{"Authorization":"Bearer token"}`), want: "token"},
		{name: "headers_authorization_lower", payload: json.RawMessage(`{"headers":{"authorization":"Bearer token"}}`), want: "token"},
		{name: "headers_authorization_upper", payload: json.RawMessage(`{"headers":{"Authorization":"Bearer token"}}`), want: "token"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, extractAccessTokenFromInitPayload(tc.payload))
		})
	}
}

type stagedConnRepo struct {
	*fakeConnRepo
	getSequence int32
	conn        *models.WebSocketConnection
}

func (s *stagedConnRepo) GetConnection(_ context.Context, connectionID string) (*models.WebSocketConnection, error) {
	atomic.AddInt32(&s.getConnCalls, 1)

	seq := atomic.AddInt32(&s.getSequence, 1)
	if seq == 1 {
		return nil, errors.New("not found")
	}
	if s.conn != nil {
		return s.conn, nil
	}
	return &models.WebSocketConnection{ConnectionID: connectionID}, nil
}

func TestHandleConnectionInit_UnauthenticatedBranches(t *testing.T) {
	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	t.Run("missing_token_sends_connection_error", func(t *testing.T) {
		var bodies [][]byte
		s := newServer(&fakeTokenValidator{}, nil, nil, zap.NewNop(), nil, nil)
		s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
			b, err := json.Marshal(payload)
			require.NoError(t, err)
			bodies = append(bodies, b)
			return nil
		}

		resp, err := s.handleConnectionInit(context.Background(), wsCtx, "c1", wsMessage{Type: "connection_init", Payload: json.RawMessage(`{}`)})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)

		require.Len(t, bodies, 1)
		var out struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		require.NoError(t, json.Unmarshal(bodies[0], &out))
		require.Equal(t, "connection_error", out.Type)
		require.Equal(t, "unauthorized", out.Payload["code"])
	})

	t.Run("oauth_missing_returns_app_error", func(t *testing.T) {
		s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
		resp, err := s.handleConnectionInit(context.Background(), wsCtx, "c1", wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"access_token":"t"}`)})
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("invalid_token_sends_connection_error", func(t *testing.T) {
		var bodies [][]byte
		validator := &fakeTokenValidator{err: errors.New("bad token")}
		s := newServer(validator, nil, nil, zap.NewNop(), nil, nil)
		s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
			b, err := json.Marshal(payload)
			require.NoError(t, err)
			bodies = append(bodies, b)
			return nil
		}

		resp, err := s.handleConnectionInit(context.Background(), wsCtx, "c1", wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"token":"t"}`)})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)

		require.Len(t, bodies, 1)
		var out struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		require.NoError(t, json.Unmarshal(bodies[0], &out))
		require.Equal(t, "connection_error", out.Type)
		require.Equal(t, "unauthorized", out.Payload["code"])
	})

	t.Run("missing_username_sends_connection_error", func(t *testing.T) {
		var bodies [][]byte
		validator := &fakeTokenValidator{claims: &auth.Claims{}}
		s := newServer(validator, nil, nil, zap.NewNop(), nil, nil)
		s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
			b, err := json.Marshal(payload)
			require.NoError(t, err)
			bodies = append(bodies, b)
			return nil
		}

		resp, err := s.handleConnectionInit(context.Background(), wsCtx, "c1", wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"Authorization":"Bearer t"}`)})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)

		require.Len(t, bodies, 1)
		var out struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		require.NoError(t, json.Unmarshal(bodies[0], &out))
		require.Equal(t, "connection_error", out.Type)
		require.Equal(t, "forbidden", out.Payload["code"])
	})

	t.Run("success_persists_identity_and_acks", func(t *testing.T) {
		var bodies [][]byte

		repo := &stagedConnRepo{
			fakeConnRepo: &fakeConnRepo{},
			conn:         &models.WebSocketConnection{ConnectionID: "c1"},
		}
		validator := &fakeTokenValidator{claims: &auth.Claims{Username: "user", Scopes: []string{"read"}}}
		s := newServer(validator, nil, nil, zap.NewNop(), repo, nil)
		s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
			b, err := json.Marshal(payload)
			require.NoError(t, err)
			bodies = append(bodies, b)
			return nil
		}
		s.connections["c1"] = &connectionState{
			username:      "",
			claims:        nil,
			subscriptions: map[string]*subscriptionState{"sub1": {cancel: func() {}}},
		}

		resp, err := s.handleConnectionInit(context.Background(), wsCtx, "c1", wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"headers":{"authorization":"Bearer t"}}`)})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)

		require.Len(t, bodies, 1)
		var env responseEnvelope
		require.NoError(t, json.Unmarshal(bodies[0], &env))
		require.Equal(t, "connection_ack", env.Type)

		state, err := s.getConnection(context.Background(), "c1")
		require.NoError(t, err)
		require.Equal(t, "user", state.username)
		require.NotNil(t, state.claims)
		require.Contains(t, state.subscriptions, "sub1")

		require.Equal(t, int32(1), atomic.LoadInt32(&repo.writeCalls))
		require.Equal(t, int32(1), atomic.LoadInt32(&repo.updateCalls))
		require.NotNil(t, repo.lastUpdated)
		require.Equal(t, graphqlWSName, repo.lastUpdated.Info.Protocol)
		require.Equal(t, "oauth", repo.lastUpdated.Info.AuthMethod)
		require.Equal(t, "read", repo.lastUpdated.Info.CustomHeaders["scopes"])
	})
}

func TestPersistGraphQLConnectionIdentity_NoRepoDoesNotPanic(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	require.NotPanics(t, func() {
		s.persistGraphQLConnectionIdentity(context.Background(), "c1", "user", &auth.Claims{Username: "user", Scopes: []string{"read"}})
	})
}
