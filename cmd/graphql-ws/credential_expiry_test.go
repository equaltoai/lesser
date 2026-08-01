package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	appTheory "github.com/theory-cloud/apptheory/v2/runtime"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

func TestSubscribeRechecksCredentialExpiryPerOperation(t *testing.T) {
	newOperationServer := func(t *testing.T, claims *auth.Claims) (*wsServer, *fakeGraphQLExecutor, <-chan responseEnvelope) {
		t.Helper()
		exec := &fakeGraphQLExecutor{create: func(context.Context, *graphql.RawParams) (*graphql.OperationContext, gqlerror.List) {
			return &graphql.OperationContext{Operation: &ast.OperationDefinition{Operation: ast.Subscription}}, nil
		}}
		server := newServer(nil, nil, exec, zap.NewNop(), &fakeConnRepo{}, nil, &fakeInstanceRepo{state: &models.InstanceState{}})
		server.connections["c1"] = &connectionState{
			username:      "alice",
			claims:        claims,
			initialized:   true,
			subscriptions: map[string]*subscriptionState{},
		}
		messages := make(chan responseEnvelope, 10)
		server.sendJSONMessage = func(_ *appTheory.WebSocketContext, payload any) error {
			raw, err := json.Marshal(payload)
			require.NoError(t, err)
			var envelope responseEnvelope
			require.NoError(t, json.Unmarshal(raw, &envelope))
			messages <- envelope
			return nil
		}
		return server, exec, messages
	}

	t.Run("expired credential is refused with a distinct code", func(t *testing.T) {
		server, exec, messages := newOperationServer(t, &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))},
			Username:         "alice",
		})
		server.handleSubscribe(context.Background(), wsMessage{
			ID:      "expired",
			Type:    "subscribe",
			Payload: json.RawMessage(`{"query":"subscription { timelineUpdates(type: HOME) { id } }"}`),
		}, &appTheory.WebSocketContext{ConnectionID: "c1"})

		message := receiveWSMessage(t, messages)
		require.Equal(t, "error", message.Type)
		require.Equal(t, wsCodeCredentialExpired, graphQLErrorExtensionCode(t, message.Payload))
		require.Zero(t, atomic.LoadInt32(&exec.createCalls), "expired credentials must be refused before operation creation")
	})

	t.Run("unexpired credential proceeds", func(t *testing.T) {
		server, exec, messages := newOperationServer(t, &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			Username:         "alice",
		})
		server.handleSubscribe(context.Background(), wsMessage{
			ID:      "valid",
			Type:    "subscribe",
			Payload: json.RawMessage(`{"query":"subscription { timelineUpdates(type: HOME) { id } }"}`),
		}, &appTheory.WebSocketContext{ConnectionID: "c1"})

		require.Eventually(t, func() bool { return atomic.LoadInt32(&exec.createCalls) == 1 }, time.Second, 10*time.Millisecond)
		require.Equal(t, "complete", receiveWSMessage(t, messages).Type)
	})
}

func TestGetConnectionRehydratesCredentialExpiry(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	repo := &fakeConnRepo{getConnection: &models.WebSocketConnection{
		ConnectionID: "c1",
		UserID:       "alice",
		Username:     "alice",
		Info: models.ConnectionInfo{CustomHeaders: map[string]string{
			"scopes":                    "read",
			wsCredentialExpiresAtHeader: fmt.Sprintf("%d", expiresAt.Unix()),
		}},
	}}
	server := newServer(nil, nil, nil, zap.NewNop(), repo, nil, nil)

	state, err := server.getConnection(context.Background(), "c1")
	require.NoError(t, err)
	require.NotNil(t, state.claims)
	require.NotNil(t, state.claims.ExpiresAt)
	require.Equal(t, expiresAt, state.claims.ExpiresAt.Time)
	require.False(t, state.credentialExpiryUnavailable)
}

func TestLegacyConnectionWithoutCredentialExpiryFailsClosedOnNextOperation(t *testing.T) {
	exec := &fakeGraphQLExecutor{create: func(context.Context, *graphql.RawParams) (*graphql.OperationContext, gqlerror.List) {
		return &graphql.OperationContext{Operation: &ast.OperationDefinition{Operation: ast.Subscription}}, nil
	}}
	repo := &fakeConnRepo{getConnection: &models.WebSocketConnection{
		ConnectionID: "legacy",
		UserID:       "alice",
		Username:     "alice",
		Info: models.ConnectionInfo{
			AuthMethod:    "oauth",
			CustomHeaders: map[string]string{"scopes": "read"},
		},
	}}
	server := newServer(nil, nil, exec, zap.NewNop(), repo, nil, &fakeInstanceRepo{state: &models.InstanceState{}})
	messages := make(chan responseEnvelope, 1)
	server.sendJSONMessage = func(_ *appTheory.WebSocketContext, payload any) error {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		var envelope responseEnvelope
		require.NoError(t, json.Unmarshal(raw, &envelope))
		messages <- envelope
		return nil
	}

	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "legacy-operation",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { timelineUpdates(type: HOME) { id } }"}`),
	}, &appTheory.WebSocketContext{ConnectionID: "legacy"})

	message := receiveWSMessage(t, messages)
	require.Equal(t, "error", message.Type)
	require.Equal(t, wsCodeCredentialExpired, graphQLErrorExtensionCode(t, message.Payload))
	require.Zero(t, atomic.LoadInt32(&exec.createCalls), "legacy authenticated rows without persisted expiry must force re-authentication before operation creation")
}
