package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	appTheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

type faultingAtomicGraphQLSubscriptionRepo struct {
	stagedCount int
	durable     []*models.GraphQLStreamSubscription
}

func (r *faultingAtomicGraphQLSubscriptionRepo) PutAll(_ context.Context, records []*models.GraphQLStreamSubscription) error {
	staged := make([]*models.GraphQLStreamSubscription, 0, len(records))
	for _, record := range records {
		staged = append(staged, record)
		r.stagedCount++
		if r.stagedCount == 1 {
			return errors.New("fault after staging first stream")
		}
	}
	r.durable = append(r.durable, staged...)
	return nil
}

func (*faultingAtomicGraphQLSubscriptionRepo) DeleteSubscription(context.Context, string, string) error {
	return nil
}

func (*faultingAtomicGraphQLSubscriptionRepo) DeleteAllForConnection(context.Context, string) error {
	return nil
}

func TestConversationUpdatesPersistFailureLeavesNoHalfOpenPublication(t *testing.T) {
	resolver := &graph.Resolver{Logger: zap.NewNop()}
	exec := executor.New(graph.NewExecutableSchema(graph.NewConfig(resolver)))
	configureGraphQLExecutor(exec, &appconfig.Config{})

	server, messages := newAnonymousOperationTestServer(t, resolver, exec, "conversation-conn")
	server.subscriptionManager = nil
	server.connections["conversation-conn"].username = "alice"
	server.connections["conversation-conn"].claims = &auth.Claims{Username: "alice"}
	repo := &faultingAtomicGraphQLSubscriptionRepo{}
	server.gqlSubRepo = repo

	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "conversation-subscription",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { conversationUpdates { id } }"}`),
	}, &appTheory.WebSocketContext{ConnectionID: "conversation-conn"})

	message := receiveWSMessage(t, messages)
	require.Equal(t, "error", message.Type)
	require.Equal(t, "INTERNAL_ERROR", graphQLErrorExtensionCode(t, message.Payload))
	require.Equal(t, 1, repo.stagedCount, "fault injection must bite after the first staged stream")
	require.Empty(t, repo.durable, "a terminal error must not coexist with a published stream registration")
}
