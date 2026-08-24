package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	testingpkg "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appTheory "github.com/theory-cloud/apptheory/v4/runtime"
	tablemocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

func TestConnectionInit_EmptyPayloadAcknowledgesAnonymousPrincipal(t *testing.T) {
	repo := &fakeConnRepo{}
	server := newServer(nil, nil, nil, zap.NewNop(), repo, nil, nil)
	server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}

	messages := captureWSMessages(t, server)
	response, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{}`)},
	)
	require.NoError(t, err)
	require.Equal(t, 200, response.Status)
	require.Len(t, *messages, 1)
	require.Equal(t, "connection_ack", (*messages)[0].Type)
	require.False(t, connectionACKAuthenticated(t, (*messages)[0].Payload))

	state, err := server.getConnection(context.Background(), "c1")
	require.NoError(t, err)
	require.Empty(t, state.username)
	require.Nil(t, state.claims)
	require.True(t, state.initialized)
	require.NotNil(t, repo.lastUpdated)
	require.Equal(t, "anonymous", repo.lastUpdated.Info.AuthMethod)
}

func TestConnectionInit_InvalidTokenClosesAsForbidden(t *testing.T) {
	server := newServer(&fakeTokenValidator{err: errors.New("expired")}, nil, nil, zap.NewNop(), nil, nil, nil)
	server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := captureWSMessages(t, server)
	var closeCode int
	server.closeConnection = func(_ context.Context, _ *appTheory.WebSocketContext, _ string, code int, _ string) error {
		closeCode = code
		return nil
	}

	_, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"access_token":"expired"}`)},
	)
	require.NoError(t, err)
	require.Empty(t, *messages, "connection_init refusal is a socket close, not an operation error")
	require.Equal(t, wsCloseForbidden, closeCode)
	require.NotContains(t, server.connections, "c1")
}

func TestConnectionInit_PresentButEmptyCredentialDoesNotDowngradeToAnonymous(t *testing.T) {
	server := newServer(&fakeTokenValidator{}, nil, nil, zap.NewNop(), nil, nil, nil)
	server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := captureWSMessages(t, server)
	var closeCode int
	server.closeConnection = func(_ context.Context, _ *appTheory.WebSocketContext, _ string, code int, _ string) error {
		closeCode = code
		return nil
	}

	_, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"access_token":""}`)},
	)
	require.NoError(t, err)
	require.Empty(t, *messages)
	require.Equal(t, wsCloseForbidden, closeCode)
	require.NotContains(t, server.connections, "c1")
}

func TestConnectionInit_CredentialLikeUnknownShapesFailClosed(t *testing.T) {
	tests := map[string]json.RawMessage{
		"headers must be an object": json.RawMessage(`{"headers":"Bearer token"}`),
		"unknown nested envelope":   json.RawMessage(`{"payload":{"Authorization":"Bearer token"}}`),
		"nested token mixed case":   json.RawMessage(`{"metadata":{"ToKeN":"secret"}}`),
	}

	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			server := newServer(&fakeTokenValidator{}, nil, nil, zap.NewNop(), nil, nil, nil)
			server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
			messages := captureWSMessages(t, server)
			var closeCode int
			server.closeConnection = func(_ context.Context, _ *appTheory.WebSocketContext, _ string, code int, _ string) error {
				closeCode = code
				return nil
			}

			_, err := server.handleConnectionInit(
				context.Background(),
				&appTheory.WebSocketContext{ConnectionID: "c1"},
				"c1",
				wsMessage{Type: "connection_init", Payload: payload},
			)
			require.NoError(t, err)
			require.Empty(t, *messages)
			require.Equal(t, wsCloseInvalidMessage, closeCode)
			require.NotContains(t, server.connections, "c1")
		})
	}
}

func TestConnectionInit_BenignMetadataAcknowledgesAnonymousPrincipal(t *testing.T) {
	repo := &fakeConnRepo{}
	server := newServer(nil, nil, nil, zap.NewNop(), repo, nil, nil)
	server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := captureWSMessages(t, server)
	closeCalled := false
	server.closeConnection = func(_ context.Context, _ *appTheory.WebSocketContext, _ string, _ int, _ string) error {
		closeCalled = true
		return nil
	}

	_, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"client":"contentus","version":"4"}`)},
	)
	require.NoError(t, err)
	require.False(t, closeCalled)
	require.Len(t, *messages, 1)
	require.Equal(t, "connection_ack", (*messages)[0].Type)
	require.False(t, connectionACKAuthenticated(t, (*messages)[0].Payload))
	require.True(t, server.connections["c1"].initialized)
}

func TestConnectionInit_AuthorizationHeaderIsCaseInsensitive(t *testing.T) {
	validator := &fakeTokenValidator{claims: &auth.Claims{Username: "alice"}}
	server := newServer(validator, nil, nil, zap.NewNop(), &fakeConnRepo{}, nil, nil)
	server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := captureWSMessages(t, server)

	_, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"AUTHORIZATION":"Bearer valid"}`)},
	)
	require.NoError(t, err)
	require.Equal(t, "valid", validator.token)
	require.Len(t, *messages, 1)
	require.Equal(t, "connection_ack", (*messages)[0].Type)
	require.True(t, connectionACKAuthenticated(t, (*messages)[0].Payload))
}

func TestConnectionInit_DuplicatePreservesInitializedConnection(t *testing.T) {
	tests := []struct {
		name           string
		initialPayload json.RawMessage
		secondPayload  json.RawMessage
		wantAuthCalls  int32
	}{
		{
			name:           "authenticated valid second payload",
			initialPayload: json.RawMessage(`{"access_token":"valid"}`),
			secondPayload:  json.RawMessage(`{"access_token":"valid"}`),
			wantAuthCalls:  1,
		},
		{
			name:           "authenticated malformed second payload",
			initialPayload: json.RawMessage(`{"access_token":"valid"}`),
			secondPayload:  json.RawMessage(`{`),
			wantAuthCalls:  1,
		},
		{
			name:           "anonymous valid second payload",
			initialPayload: json.RawMessage(`{}`),
			secondPayload:  json.RawMessage(`{"access_token":"valid"}`),
			wantAuthCalls:  0,
		},
		{
			name:           "anonymous malformed second payload",
			initialPayload: json.RawMessage(`{}`),
			secondPayload:  json.RawMessage(`{`),
			wantAuthCalls:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeConnRepo{}
			validator := &fakeTokenValidator{claims: &auth.Claims{Username: "alice"}}
			server := newServer(validator, nil, nil, zap.NewNop(), repo, nil, nil)
			server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
			messages := captureWSMessages(t, server)
			wsCtx := &appTheory.WebSocketContext{ConnectionID: "c1"}

			_, err := server.handleConnectionInit(
				context.Background(),
				wsCtx,
				"c1",
				wsMessage{Type: "connection_init", Payload: tc.initialPayload},
			)
			require.NoError(t, err)
			require.Len(t, *messages, 1)
			require.Equal(t, "connection_ack", (*messages)[0].Type)

			cancelled := false
			liveSubscription := &subscriptionState{cancel: func() { cancelled = true }}
			server.connections["c1"].subscriptions["live-subscription"] = liveSubscription
			stateBefore := server.connections["c1"]
			recordBefore := repo.lastUpdated
			writesBefore := atomic.LoadInt32(&repo.writeCalls)
			updatesBefore := atomic.LoadInt32(&repo.updateCalls)

			var closeCode int
			var closeReason string
			server.closeConnection = func(_ context.Context, _ *appTheory.WebSocketContext, _ string, code int, reason string) error {
				closeCode = code
				closeReason = reason
				return nil
			}

			_, err = server.handleConnectionInit(
				context.Background(),
				wsCtx,
				"c1",
				wsMessage{Type: "connection_init", Payload: tc.secondPayload},
			)
			require.NoError(t, err)
			require.Equal(t, 4429, closeCode)
			require.Equal(t, "Too many initialisation requests", closeReason)
			require.Len(t, *messages, 1, "duplicate connection_init must not send a second ACK")
			require.Same(t, stateBefore, server.connections["c1"])
			require.Same(t, liveSubscription, server.connections["c1"].subscriptions["live-subscription"])
			require.False(t, cancelled, "duplicate connection_init must not cancel live subscriptions")
			require.Same(t, recordBefore, repo.lastUpdated)
			require.Equal(t, writesBefore, atomic.LoadInt32(&repo.writeCalls))
			require.Equal(t, updatesBefore, atomic.LoadInt32(&repo.updateCalls))
			require.Zero(t, atomic.LoadInt32(&repo.deleteSubsCalls))
			require.Zero(t, atomic.LoadInt32(&repo.deleteConnCalls))
			require.Equal(t, tc.wantAuthCalls, atomic.LoadInt32(&validator.calls), "duplicate payload must not be parsed or validated")
		})
	}
}

type persistedConnectionRepo struct {
	*fakeConnRepo
	connection *models.WebSocketConnection
}

func (r *persistedConnectionRepo) GetConnection(_ context.Context, _ string) (*models.WebSocketConnection, error) {
	atomic.AddInt32(&r.getConnCalls, 1)
	return r.connection, nil
}

func TestConnectionInit_DuplicateAnonymousConnectionRehydratesFromRepository(t *testing.T) {
	repo := &persistedConnectionRepo{
		fakeConnRepo: &fakeConnRepo{},
		connection: &models.WebSocketConnection{
			ConnectionID: "c1",
			Username:     "",
			Info: models.ConnectionInfo{
				AuthMethod: "anonymous",
			},
		},
	}
	server := newServer(nil, nil, nil, zap.NewNop(), repo, nil, nil)
	messages := captureWSMessages(t, server)
	var closeCode int
	var closeReason string
	server.closeConnection = func(_ context.Context, _ *appTheory.WebSocketContext, _ string, code int, reason string) error {
		closeCode = code
		closeReason = reason
		return nil
	}

	require.NotContains(t, server.connections, "c1", "the test must start with a cold in-memory cache")
	_, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{`)},
	)
	require.NoError(t, err)
	require.Equal(t, wsCloseTooManyInitialisationRequests, closeCode)
	require.Equal(t, "Too many initialisation requests", closeReason)
	require.Empty(t, *messages, "the duplicate must be rejected before its malformed payload is parsed")
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.getConnCalls))
	require.Zero(t, atomic.LoadInt32(&repo.writeCalls))
	require.Zero(t, atomic.LoadInt32(&repo.updateCalls))
	require.Zero(t, atomic.LoadInt32(&repo.deleteSubsCalls))
	require.Zero(t, atomic.LoadInt32(&repo.deleteConnCalls))

	state := server.connections["c1"]
	require.NotNil(t, state)
	require.True(t, state.initialized)
	require.Empty(t, state.username)
	require.Nil(t, state.claims)
}

func TestAnonymousSubscriptionOperationAuthorization(t *testing.T) {
	manager := graph.NewSubscriptionManager(
		inmemory.NewStreamingConnectionRepository(),
		streaming.NewMockPublisher(),
		zap.NewNop(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	resolver := &graph.Resolver{Logger: zap.NewNop(), SubscriptionManager: manager}
	exec := executor.New(graph.NewExecutableSchema(graph.NewConfig(resolver)))
	configureGraphQLExecutor(exec, &appconfig.Config{})

	anonymousSafe := map[string]string{
		"PUBLIC":  "timelineUpdates(type: PUBLIC) { id }",
		"LOCAL":   "timelineUpdates(type: LOCAL) { id }",
		"ACTOR":   `timelineUpdates(type: ACTOR, actorUsername: "alice") { id }`,
		"HASHTAG": `timelineUpdates(type: HASHTAG, hashtag: "golang") { id }`,
	}
	for name, field := range anonymousSafe {
		t.Run(name, func(t *testing.T) {
			connectionID := "safe-conn-" + name
			subscriptionID := "safe-" + name
			server, messages := newAnonymousOperationTestServer(t, resolver, exec, connectionID)
			wsCtx := &appTheory.WebSocketContext{ConnectionID: connectionID}
			payload, err := json.Marshal(subscribePayload{Query: "subscription { " + field + " }"})
			require.NoError(t, err)
			server.handleSubscribe(context.Background(), wsMessage{
				ID:      subscriptionID,
				Type:    "subscribe",
				Payload: payload,
			}, wsCtx)

			require.Eventually(t, func() bool {
				server.mu.RLock()
				defer server.mu.RUnlock()
				_, ok := server.connections[connectionID].subscriptions[subscriptionID]
				return ok
			}, time.Second, 10*time.Millisecond)
			select {
			case message := <-messages:
				t.Fatalf("anonymous-safe subscription returned early message: %#v", message)
			case <-time.After(25 * time.Millisecond):
			}
			require.True(t, server.cancelSubscription(context.Background(), connectionID, subscriptionID))
			require.Equal(t, "complete", receiveWSMessage(t, messages).Type)
		})
	}

	gated := map[string]string{
		"HOME timeline":             "timelineUpdates(type: HOME) { id }",
		"LIST timeline":             `timelineUpdates(type: LIST, listId: "list-1") { id }`,
		"DIRECT timeline":           "timelineUpdates(type: DIRECT) { id }",
		"activity stream":           "activityStream { __typename }",
		"notification stream":       "notificationStream { __typename }",
		"conversation updates":      "conversationUpdates { __typename }",
		"list updates":              `listUpdates(listId: "list-1") { __typename }`,
		"relationship updates":      "relationshipUpdates { __typename }",
		"cost updates":              "costUpdates { __typename }",
		"moderation events":         "moderationEvents { __typename }",
		"trust updates":             `trustUpdates(actorId: "actor-1") { __typename }`,
		"AI analysis updates":       "aiAnalysisUpdates { __typename }",
		"quote activity":            `quoteActivity(noteId: "note-1") { __typename }`,
		"hashtag activity":          `hashtagActivity(hashtags: ["go"]) { __typename }`,
		"metrics updates":           "metricsUpdates { __typename }",
		"moderation alerts":         "moderationAlerts { __typename }",
		"cost alerts":               "costAlerts(thresholdUSD: 1) { __typename }",
		"budget alerts":             "budgetAlerts { __typename }",
		"federation health updates": "federationHealthUpdates { __typename }",
		"moderation queue updates":  "moderationQueueUpdate { __typename }",
		"threat intelligence":       "threatIntelligence { __typename }",
		"performance alerts":        "performanceAlert(severity: CRITICAL) { __typename }",
		"infrastructure events":     "infrastructureEvent { __typename }",
		"agent activity":            `agentActivity(username: "agent-1") { __typename }`,
	}
	for name, field := range gated {
		t.Run(name, func(t *testing.T) {
			connectionID := "gated-conn-" + name
			server, messages := newAnonymousOperationTestServer(t, resolver, exec, connectionID)
			wsCtx := &appTheory.WebSocketContext{ConnectionID: connectionID}
			payload, err := json.Marshal(subscribePayload{Query: "subscription { " + field + " }"})
			require.NoError(t, err)
			server.handleSubscribe(context.Background(), wsMessage{
				ID:      "gated-" + name,
				Type:    "subscribe",
				Payload: payload,
			}, wsCtx)

			message := receiveWSMessage(t, messages)
			require.Equal(t, "error", message.Type)
			require.Equal(t, "UNAUTHENTICATED", graphQLErrorExtensionCode(t, message.Payload))

			server.mu.RLock()
			_, active := server.connections[connectionID].subscriptions["gated-"+name]
			server.mu.RUnlock()
			require.False(t, active, "terminal operation errors must not leave half-open subscriptions")
			select {
			case extra := <-messages:
				t.Fatalf("terminal operation error must not be followed by %#v", extra)
			case <-time.After(25 * time.Millisecond):
			}
		})
	}
}

// newListAuthorizedRegistry returns a services.Registry whose list service can
// authorize the "list-1" list owned by "alice". The authenticated LIST timeline
// subscription exercises the resolver's list ownership check, so the test must
// provide a real list repository rather than leaving Registry nil (which makes
// the resolver return a non-terminal 404 and races the subscription lifecycle
// assertions).
func newListAuthorizedRegistry(t *testing.T) *services.Registry {
	t.Helper()

	mockDB := new(tablemocks.MockDB)
	mockQuery := new(tablemocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe().Run(func(args mock.Arguments) {
		if dest, ok := args.Get(0).(*models.List); ok {
			*dest = models.List{
				ID:            "list-1",
				Username:      "alice",
				Title:         "List One",
				RepliesPolicy: repositories.RepliesPolicyList,
			}
		}
	})

	listRepo := repositories.NewListRepository(mockDB, "test-table", zap.NewNop(), nil)
	storage := testingpkg.NewMockRepositoryStorage(testingpkg.WithListRepository(listRepo))

	registry, err := services.NewRegistry(
		services.WithStorage(storage),
		services.WithPublisher(streaming.NewMockPublisher()),
		services.WithLogger(zap.NewNop()),
	)
	require.NoError(t, err)
	return registry
}

func TestAuthenticatedConnectionStreamsTimelineAuthorizationSet(t *testing.T) {
	manager := graph.NewSubscriptionManager(
		inmemory.NewStreamingConnectionRepository(),
		streaming.NewMockPublisher(),
		zap.NewNop(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	resolver := &graph.Resolver{Logger: zap.NewNop(), SubscriptionManager: manager, Registry: newListAuthorizedRegistry(t)}
	exec := executor.New(graph.NewExecutableSchema(graph.NewConfig(resolver)))
	configureGraphQLExecutor(exec, &appconfig.Config{})

	validator := &fakeTokenValidator{claims: &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		Username:         "alice",
		Scopes:           []string{"read"},
	}}
	server := newServer(validator, resolver, exec, zap.NewNop(), &fakeConnRepo{}, nil, &fakeInstanceRepo{state: &models.InstanceState{}})
	server.connections["authenticated-conn"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := make(chan responseEnvelope, 10)
	server.sendJSONMessage = func(_ *appTheory.WebSocketContext, payload any) error {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		var envelope responseEnvelope
		require.NoError(t, json.Unmarshal(raw, &envelope))
		messages <- envelope
		return nil
	}
	wsCtx := &appTheory.WebSocketContext{ConnectionID: "authenticated-conn"}

	_, err := server.handleConnectionInit(context.Background(), wsCtx, "authenticated-conn", wsMessage{
		Type:    "connection_init",
		Payload: json.RawMessage(`{"access_token":"valid"}`),
	})
	require.NoError(t, err)
	ack := receiveWSMessage(t, messages)
	require.Equal(t, "connection_ack", ack.Type)
	require.True(t, connectionACKAuthenticated(t, ack.Payload))

	authenticated := map[string]string{
		"HOME":    "timelineUpdates(type: HOME) { id }",
		"DIRECT":  "timelineUpdates(type: DIRECT) { id }",
		"ACTOR":   `timelineUpdates(type: ACTOR, actorUsername: "alice") { id }`,
		"HASHTAG": `timelineUpdates(type: HASHTAG, hashtag: "golang") { id }`,
		"LIST":    `timelineUpdates(type: LIST, listId: "list-1") { id }`,
	}
	for name, field := range authenticated {
		t.Run(name, func(t *testing.T) {
			subscriptionID := strings.ToLower(name) + "-subscription"
			payload, marshalErr := json.Marshal(subscribePayload{Query: "subscription { " + field + " }"})
			require.NoError(t, marshalErr)
			server.handleSubscribe(context.Background(), wsMessage{
				ID:      subscriptionID,
				Type:    "subscribe",
				Payload: payload,
			}, wsCtx)
			require.Eventually(t, func() bool {
				server.mu.RLock()
				defer server.mu.RUnlock()
				_, ok := server.connections["authenticated-conn"].subscriptions[subscriptionID]
				return ok
			}, time.Second, 10*time.Millisecond)
			require.True(t, server.cancelSubscription(context.Background(), "authenticated-conn", subscriptionID))
			require.Equal(t, "complete", receiveWSMessage(t, messages).Type)
		})
	}
}

func TestAuthenticatedNonAdminAuthorizationFailureIsTerminalError(t *testing.T) {
	dispatched := false
	exec := &fakeGraphQLExecutor{dispatch: func(ctx context.Context, _ *graphql.OperationContext) (graphql.ResponseHandler, context.Context) {
		return func(context.Context) *graphql.Response {
			if dispatched {
				return nil
			}
			dispatched = true
			return &graphql.Response{Errors: gqlerror.List{&gqlerror.Error{
				Message:    "admin privileges required",
				Extensions: map[string]any{"code": "FORBIDDEN"},
			}}}
		}, ctx
	}}
	subscriptionCanceled := false
	ctx, cancelContext := context.WithCancel(context.Background())
	cancel := func() {
		subscriptionCanceled = true
		cancelContext()
	}
	server := newServer(nil, nil, exec, zap.NewNop(), &fakeConnRepo{}, nil, nil)
	server.connections["member-conn"] = &connectionState{
		username:      "alice",
		claims:        &auth.Claims{Username: "alice", Scopes: []string{"read"}},
		initialized:   true,
		subscriptions: map[string]*subscriptionState{"admin-only": {cancel: cancel}},
	}
	messages := make(chan responseEnvelope, 10)
	server.sendJSONMessage = func(_ *appTheory.WebSocketContext, payload any) error {
		server.mu.RLock()
		_, active := server.connections["member-conn"].subscriptions["admin-only"]
		server.mu.RUnlock()
		require.False(t, active, "terminal operation errors must be cleaned up before they are observable")
		require.True(t, subscriptionCanceled, "terminal operation errors must cancel their operation context before they are observable")

		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		var envelope responseEnvelope
		require.NoError(t, json.Unmarshal(raw, &envelope))
		messages <- envelope
		return nil
	}

	server.executeSubscription(ctx, "member-conn", "admin-only", &graphql.OperationContext{}, cancel, &appTheory.WebSocketContext{ConnectionID: "member-conn"})

	message := receiveWSMessage(t, messages)
	require.Equal(t, "error", message.Type)
	require.Equal(t, "FORBIDDEN", graphQLErrorExtensionCode(t, message.Payload))
	select {
	case extra := <-messages:
		t.Fatalf("terminal authorization error must not be followed by %#v", extra)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestTerminalAuthorizationErrorsCoverEveryHTTPAuthCode(t *testing.T) {
	terminalCodes := []string{
		wsCodeUnauthenticated,
		string(apperrors.CodeUnauthorized),
		string(apperrors.CodeAuthFailed),
		string(apperrors.CodeTokenExpired),
		string(apperrors.CodeTokenInvalid),
		string(apperrors.CodeTokenRevoked),
		string(apperrors.CodeForbidden),
		string(apperrors.CodeInsufficientScope),
		string(apperrors.CodeAccountSuspended),
	}
	for _, code := range terminalCodes {
		t.Run(code, func(t *testing.T) {
			require.True(t, hasTerminalAuthorizationError(gqlerror.List{&gqlerror.Error{
				Extensions: map[string]any{"code": code},
			}}))
		})
	}

	require.False(t, hasTerminalAuthorizationError(gqlerror.List{&gqlerror.Error{
		Extensions: map[string]any{"code": string(apperrors.CodeInternal)},
	}}))
	require.False(t, hasTerminalAuthorizationError(gqlerror.List{&gqlerror.Error{
		Extensions: map[string]any{"code": apperrors.CodeForbidden},
	}}), "the websocket presenter contract stores codes as strings")
}

func TestSubscribeBeforeConnectionAckReturnsOneTerminalError(t *testing.T) {
	server := newServer(nil, nil, nil, zap.NewNop(), &fakeConnRepo{}, nil, &fakeInstanceRepo{state: &models.InstanceState{}})
	server.connections["pending"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := captureWSMessages(t, server)

	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "too-early",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { timelineUpdates(type: PUBLIC) { id } }"}`),
	}, &appTheory.WebSocketContext{ConnectionID: "pending"})

	require.Len(t, *messages, 1)
	require.Equal(t, "error", (*messages)[0].Type)
	require.Equal(t, wsCodeUnauthenticated, graphQLErrorExtensionCode(t, (*messages)[0].Payload))
}

func TestAnonymousConversationServerlessFastPathRefusesBeforePersistence(t *testing.T) {
	manager := graph.NewSubscriptionManager(
		inmemory.NewStreamingConnectionRepository(),
		streaming.NewMockPublisher(),
		zap.NewNop(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	resolver := &graph.Resolver{Logger: zap.NewNop(), SubscriptionManager: manager}
	exec := executor.New(graph.NewExecutableSchema(graph.NewConfig(resolver)))
	configureGraphQLExecutor(exec, &appconfig.Config{})
	server, messages := newAnonymousOperationTestServer(t, resolver, exec, "conversation-conn")
	server.gqlSubRepo = &repositories.GraphQLStreamSubscriptionRepository{}

	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "conversation-subscription",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { conversationUpdates { id } }"}`),
	}, &appTheory.WebSocketContext{ConnectionID: "conversation-conn"})

	message := receiveWSMessage(t, messages)
	require.Equal(t, "error", message.Type)
	require.Equal(t, wsCodeUnauthenticated, graphQLErrorExtensionCode(t, message.Payload))
	server.mu.RLock()
	_, active := server.connections["conversation-conn"].subscriptions["conversation-subscription"]
	server.mu.RUnlock()
	require.False(t, active)
}

func newAnonymousOperationTestServer(t *testing.T, resolver *graph.Resolver, exec *executor.Executor, connectionID string) (*wsServer, <-chan responseEnvelope) {
	t.Helper()
	server := newServer(nil, resolver, exec, zap.NewNop(), &fakeConnRepo{}, nil, &fakeInstanceRepo{state: &models.InstanceState{}})
	server.connections[connectionID] = &connectionState{initialized: true, subscriptions: map[string]*subscriptionState{}}
	messages := make(chan responseEnvelope, 10)
	server.sendJSONMessage = func(_ *appTheory.WebSocketContext, payload any) error {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		var envelope responseEnvelope
		require.NoError(t, json.Unmarshal(raw, &envelope))
		messages <- envelope
		return nil
	}
	return server, messages
}

func receiveWSMessage(t *testing.T, messages <-chan responseEnvelope) responseEnvelope {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket message")
		return responseEnvelope{}
	}
}

func captureWSMessages(t *testing.T, server *wsServer) *[]responseEnvelope {
	t.Helper()
	messages := make([]responseEnvelope, 0)
	server.sendJSONMessage = func(_ *appTheory.WebSocketContext, payload any) error {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		var envelope responseEnvelope
		require.NoError(t, json.Unmarshal(raw, &envelope))
		messages = append(messages, envelope)
		return nil
	}
	return &messages
}

func connectionACKAuthenticated(t *testing.T, payload any) bool {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded struct {
		Extensions struct {
			Authenticated bool `json:"authenticated"`
		} `json:"extensions"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded.Extensions.Authenticated
}

func graphQLErrorExtensionCode(t *testing.T, payload any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded []struct {
		Extensions map[string]any `json:"extensions"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotEmpty(t, decoded)
	code, _ := decoded[0].Extensions["code"].(string)
	return code
}
