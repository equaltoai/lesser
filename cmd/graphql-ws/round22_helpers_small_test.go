package main

import (
	"testing"

	"github.com/99designs/gqlgen/graphql/executor"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func TestSendJSON_NilWebSocketContext_ReturnsError(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	require.Error(t, s.sendJSON(nil, responseEnvelope{Type: "ping"}))
}

func TestRememberWebSocketContext_IgnoresEmptyInputs(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)

	s.rememberWebSocketContext("", &apptheory.WebSocketContext{ConnectionID: "c1"})
	s.rememberWebSocketContext("c1", nil)

	require.Empty(t, s.wsContexts)
}

func TestWebSocketContextFromEvent_NilContext_ReturnsError(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)

	wsCtx, connectionID, err := s.webSocketContextFromEvent(nil)
	require.Error(t, err)
	require.Nil(t, wsCtx)
	require.Equal(t, "", connectionID)
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
