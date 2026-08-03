package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPublicActorAndHashtagSubscribersReceiveOnlyPublicStatuses(t *testing.T) {
	const (
		anonymousActorConnection   = "anonymous-actor"
		anonymousHashtagConnection = "anonymous-hashtag"
		authorConnection           = "author-self"
	)

	streamRepo := &fakeStreamRepo{
		subsByStream: map[string][]models.WebSocketSubscription{
			streaming.PublicActorStreamName("alice"): {{ConnectionID: anonymousActorConnection}},
			streaming.HashtagStreamName("golang"):    {{ConnectionID: anonymousHashtagConnection}},
			streaming.UserStreamName("alice"):        {{ConnectionID: authorConnection}},
		},
		getConnErrByID: make(map[string]error),
	}
	client := &fakeStreamerClient{}
	handler := &StreamRouterHandler{
		logger:        zap.NewNop(),
		apiClient:     client,
		streamingRepo: streamRepo,
		accountRepo:   fakeFollowerRepo{},
		domain:        "example.com",
	}

	publicRecord := newStatusInsertRecord(eventNameInsert)
	require.NoError(t, handler.processStatusEvent(context.Background(), "public", publicRecord))
	require.Equal(t, 1, connectionCallCount(client.postCalls, anonymousActorConnection))
	require.Equal(t, 1, connectionCallCount(client.postCalls, anonymousHashtagConnection))
	require.Equal(t, 1, connectionCallCount(client.postCalls, authorConnection))

	followersOnlyRecord := statusRecordWithVisibility(publicRecord, models.VisibilityPrivate)
	require.NoError(t, handler.processStatusEvent(context.Background(), "followers-only", followersOnlyRecord))
	directRecord := statusRecordWithVisibility(publicRecord, models.VisibilityDirect)
	require.NoError(t, handler.processStatusEvent(context.Background(), "direct", directRecord))
	require.NoError(t, handler.processNotificationEvent(context.Background(), "notification", newNotificationInsertRecord("alice")))

	// The anonymous streams receive no followers-only status, direct status, or
	// notification fanout. Those events remain confined to private user streams.
	require.Equal(t, 1, connectionCallCount(client.postCalls, anonymousActorConnection))
	require.Equal(t, 1, connectionCallCount(client.postCalls, anonymousHashtagConnection))
	require.Greater(t, connectionCallCount(client.postCalls, authorConnection), 1)
}

func TestBuildWebSocketStatusStreamsKeepsAnonymousAndPrivateKeysDisjoint(t *testing.T) {
	handler := &StreamRouterHandler{logger: zap.NewNop(), domain: "example.com"}
	base := &models.Status{
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Hashtags:       []string{"#GoLang", "golang"},
	}

	publicStatus := *base
	publicStatus.Visibility = models.VisibilityPublic
	publicStreams := handler.buildWebSocketStatusStreams(&publicStatus)
	require.ElementsMatch(t, []string{
		streaming.PublicStream,
		streaming.PublicLocalStream,
		streaming.PublicActorStreamName("alice"),
		streaming.HashtagStreamName("golang"),
		streaming.UserStreamName("alice"),
	}, publicStreams)
	require.NotEqual(t, streaming.PublicActorStreamName("alice"), streaming.UserStreamName("alice"))

	for _, visibility := range []string{models.VisibilityUnlisted, models.VisibilityPrivate, models.VisibilityDirect} {
		status := *base
		status.Visibility = visibility
		require.Equal(t, []string{streaming.UserStreamName("alice")}, handler.buildWebSocketStatusStreams(&status))
	}

	remoteStatus := publicStatus
	remoteStatus.AuthorID = "https://remote.example/users/alice"
	remoteStatus.AuthorUsername = "alice@remote.example"
	remoteStreams := handler.buildWebSocketStatusStreams(&remoteStatus)
	require.Contains(t, remoteStreams, streaming.PublicRemoteStream)
	require.NotContains(t, remoteStreams, streaming.PublicLocalStream)
	require.NotContains(t, remoteStreams, streaming.PublicActorStreamName("alice"))
}

func statusRecordWithVisibility(record events.DynamoDBEventRecord, visibility string) events.DynamoDBEventRecord {
	record.Change.NewImage["visibility"] = events.NewStringAttribute(visibility)
	return record
}

func connectionCallCount(calls []string, connectionID string) int {
	count := 0
	for _, call := range calls {
		if call == connectionID {
			count++
		}
	}
	return count
}
