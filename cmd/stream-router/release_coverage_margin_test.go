package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestReleaseCoverageMargin_ConversationUpdateRejectsIncompleteRecords(t *testing.T) {
	t.Parallel()

	validClient := &fakeStreamerClient{}
	validSubs := &fakeGraphQLSubRepo{}
	tests := []struct {
		name   string
		h      *StreamRouterHandler
		record events.DynamoDBEventRecord
	}{
		{
			name: "nil handler",
		},
		{
			name:   "missing subscription repository",
			h:      &StreamRouterHandler{logger: zap.NewNop(), graphqlClient: validClient},
			record: newUserConversationStateRecord("alice", "conv-1", "ACCEPTED"),
		},
		{
			name: "unmarshal failure",
			h: &StreamRouterHandler{
				logger:        zap.NewNop(),
				graphqlClient: validClient,
				gqlSubRepo:    validSubs,
			},
			record: events.DynamoDBEventRecord{},
		},
		{
			name: "malformed partition key",
			h: &StreamRouterHandler{
				logger:        zap.NewNop(),
				graphqlClient: validClient,
				gqlSubRepo:    validSubs,
			},
			record: conversationStateRecord(map[string]events.DynamoDBAttributeValue{
				"PK":             events.NewStringAttribute("MALFORMED"),
				"conversationID": events.NewStringAttribute("conv-1"),
			}),
		},
		{
			name: "empty username",
			h: &StreamRouterHandler{
				logger:        zap.NewNop(),
				graphqlClient: validClient,
				gqlSubRepo:    validSubs,
			},
			record: conversationStateRecord(map[string]events.DynamoDBAttributeValue{
				"PK":             events.NewStringAttribute("USER_CONVERSATION_STATE# "),
				"conversationID": events.NewStringAttribute("conv-1"),
			}),
		},
		{
			name: "missing conversation identifier",
			h: &StreamRouterHandler{
				logger:        zap.NewNop(),
				graphqlClient: validClient,
				gqlSubRepo:    validSubs,
			},
			record: conversationStateRecord(map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("USER_CONVERSATION_STATE#alice"),
			}),
		},
		{
			name: "subscription lookup failure",
			h: &StreamRouterHandler{
				logger:        zap.NewNop(),
				graphqlClient: validClient,
				gqlSubRepo:    &fakeGraphQLSubRepo{listErr: errors.New("subscriptions unavailable")},
			},
			record: newUserConversationStateRecord("alice", "conv-1", "ACCEPTED"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.h.processUserConversationStateEvent(context.Background(), "req-1", tt.record))
		})
	}
	require.Empty(t, validClient.postCalls)
}

func TestReleaseCoverageMargin_ConversationUpdateRemovesStaleConnections(t *testing.T) {
	t.Parallel()

	streamName := streaming.DMInboxStreamName("alice")
	client := &fakeStreamerClient{postErrByID: map[string]error{
		"stale": errors.New("GoneException: 410 Gone"),
		"live":  errors.New("temporary upstream error"),
	}}
	subRepo := &fakeGraphQLSubRepo{subsByStream: map[string][]models.GraphQLStreamSubscription{
		streamName: {
			{ConnectionID: "stale", SubscriptionID: "sub-stale", Stream: streamName},
			{ConnectionID: "live", SubscriptionID: "sub-live", Stream: streamName},
		},
	}}
	streamRepo := &fakeStreamRepo{}
	h := &StreamRouterHandler{
		logger:        zap.NewNop(),
		graphqlClient: client,
		gqlSubRepo:    subRepo,
		streamingRepo: streamRepo,
	}

	require.NoError(t, h.processUserConversationStateEvent(
		context.Background(),
		"req-1",
		newUserConversationStateRecord("alice", "conv-1", "ACCEPTED"),
	))
	require.ElementsMatch(t, []string{"stale", "live"}, client.postCalls)
	require.Equal(t, []string{"stale"}, subRepo.deleteAllCalls)
	require.Equal(t, []string{"stale"}, streamRepo.deleteConnectionCalls)
}

func conversationStateRecord(image map[string]events.DynamoDBAttributeValue) events.DynamoDBEventRecord {
	return events.DynamoDBEventRecord{
		EventID:   "evt-conversation-margin",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: image,
		},
	}
}
