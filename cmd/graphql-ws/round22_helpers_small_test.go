package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

func TestSendJSON_NilWebSocketContext_ReturnsError(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
	require.Error(t, s.sendJSON(nil, responseEnvelope{Type: "ping"}))
}

func TestSendJSON_UsesWebSocketContextFallback(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
	s.sendJSONMessage = nil

	err := s.sendJSON(&apptheory.WebSocketContext{}, responseEnvelope{Type: "ping"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection id is empty")
}

func TestRememberWebSocketContext_IgnoresEmptyInputs(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)

	s.rememberWebSocketContext("", &apptheory.WebSocketContext{ConnectionID: "c1"})
	s.rememberWebSocketContext("c1", nil)

	require.Empty(t, s.wsContexts)
}

func TestWebSocketContextFromEvent_NilContext_ReturnsError(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)

	wsCtx, connectionID, err := s.webSocketContextFromEvent(nil)
	require.Error(t, err)
	require.Nil(t, wsCtx)
	require.Equal(t, "", connectionID)
}

func TestWebSocketContextFromEvent_NonWebSocketContext_ReturnsError(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)

	wsCtx, connectionID, err := s.webSocketContextFromEvent(&apptheory.Context{})
	require.Error(t, err)
	require.Nil(t, wsCtx)
	require.Equal(t, "", connectionID)
}

func TestBuildRequestContext_AddsClaimsAndConnectionID(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)
	claims := &auth.Claims{Username: "alice"}

	ctxWithClaims := s.buildRequestContext(nil, &connectionState{claims: claims}, "")
	require.Equal(t, claims, ctxWithClaims.Value(common.ContextKeyClaims))

	baseCtx := context.Background()
	ctxWithConnectionID := s.buildRequestContext(baseCtx, nil, "conn-1")
	require.NotEqual(t, baseCtx, ctxWithConnectionID)
}

func TestHandleComplete_CancelsTrackedSubscription(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, nil)

	cancelled := 0
	s.connections["c1"] = &connectionState{
		subscriptions: map[string]*subscriptionState{
			"sub-1": {cancel: func() { cancelled++ }},
		},
	}

	resp, err := s.handleComplete(context.Background(), &apptheory.WebSocketContext{ConnectionID: "c1"}, "c1", wsMessage{ID: "sub-1"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, cancelled)
}

func TestHandleSubscribe_EarlyBranches(t *testing.T) {
	var bodies []responseEnvelope
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil, &fakeInstanceRepo{state: &models.InstanceState{Locked: true}})
	s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)

		var env responseEnvelope
		require.NoError(t, json.Unmarshal(raw, &env))
		bodies = append(bodies, env)
		return nil
	}

	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	s.handleSubscribe(context.Background(), wsMessage{}, wsCtx)
	require.Len(t, bodies, 1)
	require.Equal(t, "error", bodies[0].Type)
	require.Equal(t, protocolErrorID, bodies[0].ID)

	bodies = nil
	s.handleSubscribe(context.Background(), wsMessage{ID: "sub-1"}, wsCtx)
	require.Len(t, bodies, 1)
	require.Equal(t, "error", bodies[0].Type)
	require.Equal(t, "sub-1", bodies[0].ID)
}

func TestConfigureGraphQLExecutor_ExercisesConfigBranches(t *testing.T) {
	exec := executor.New(nil)
	cfg := &appconfig.Config{
		GraphQLParserTokenLimit: 123,
		GraphQLMaxDepth:         5,
		GraphQLMaxComplexity:    10,
		DebugMode:               true,
	}

	require.NotPanics(t, func() {
		configureGraphQLExecutor(exec, cfg)
	})
}
