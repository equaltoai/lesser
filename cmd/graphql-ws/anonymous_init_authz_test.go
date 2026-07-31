package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	appTheory "github.com/theory-cloud/apptheory/v2/runtime"
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

	state, err := server.getConnection(context.Background(), "c1")
	require.NoError(t, err)
	require.Empty(t, state.username)
	require.Nil(t, state.claims)
	require.True(t, state.initialized)
	require.NotNil(t, repo.lastUpdated)
	require.Equal(t, "anonymous", repo.lastUpdated.Info.AuthMethod)
}

func TestConnectionInit_InvalidTokenHasStructuredAuthenticationCode(t *testing.T) {
	server := newServer(&fakeTokenValidator{err: errors.New("expired")}, nil, nil, zap.NewNop(), nil, nil, nil)
	server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := captureWSMessages(t, server)

	_, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"access_token":"expired"}`)},
	)
	require.NoError(t, err)
	require.Len(t, *messages, 1)
	require.Equal(t, "connection_error", (*messages)[0].Type)
	require.Equal(t, "UNAUTHENTICATED", payloadExtensionCode(t, (*messages)[0].Payload))
	require.False(t, server.connections["c1"].initialized)
}

func TestConnectionInit_PresentButEmptyCredentialDoesNotDowngradeToAnonymous(t *testing.T) {
	server := newServer(&fakeTokenValidator{}, nil, nil, zap.NewNop(), nil, nil, nil)
	server.connections["c1"] = &connectionState{subscriptions: map[string]*subscriptionState{}}
	messages := captureWSMessages(t, server)

	_, err := server.handleConnectionInit(
		context.Background(),
		&appTheory.WebSocketContext{ConnectionID: "c1"},
		"c1",
		wsMessage{Type: "connection_init", Payload: json.RawMessage(`{"access_token":""}`)},
	)
	require.NoError(t, err)
	require.Len(t, *messages, 1)
	require.Equal(t, "connection_error", (*messages)[0].Type)
	require.Equal(t, wsCodeUnauthenticated, payloadExtensionCode(t, (*messages)[0].Payload))
	require.False(t, server.connections["c1"].initialized)
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
	exec := executor.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))
	configureGraphQLExecutor(exec, &appconfig.Config{})

	for _, timelineType := range []string{"PUBLIC", "LOCAL", "ACTOR"} {
		t.Run(timelineType, func(t *testing.T) {
			connectionID := "safe-conn-" + timelineType
			subscriptionID := "safe-" + timelineType
			server, messages := newAnonymousOperationTestServer(t, resolver, exec, connectionID)
			wsCtx := &appTheory.WebSocketContext{ConnectionID: connectionID}
			server.handleSubscribe(context.Background(), wsMessage{
				ID:      subscriptionID,
				Type:    "subscribe",
				Payload: json.RawMessage(`{"query":"subscription { timelineUpdates(type: ` + timelineType + `) { id } }"}`),
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
		"HASHTAG timeline":          "timelineUpdates(type: HASHTAG) { id }",
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

func TestAuthenticatedConnectionStillStreamsGatedSubscription(t *testing.T) {
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
	exec := executor.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))
	configureGraphQLExecutor(exec, &appconfig.Config{})

	validator := &fakeTokenValidator{claims: &auth.Claims{Username: "alice", Scopes: []string{"read"}}}
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
	require.Equal(t, "connection_ack", receiveWSMessage(t, messages).Type)

	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "home-subscription",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { timelineUpdates(type: HOME) { id } }"}`),
	}, wsCtx)
	require.Eventually(t, func() bool {
		server.mu.RLock()
		defer server.mu.RUnlock()
		_, ok := server.connections["authenticated-conn"].subscriptions["home-subscription"]
		return ok
	}, time.Second, 10*time.Millisecond)
	require.True(t, server.cancelSubscription(context.Background(), "authenticated-conn", "home-subscription"))
	require.Equal(t, "complete", receiveWSMessage(t, messages).Type)
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
	exec := executor.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))
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

func payloadExtensionCode(t *testing.T, payload any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded struct {
		Extensions map[string]any `json:"extensions"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	code, _ := decoded.Extensions["code"].(string)
	return code
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
