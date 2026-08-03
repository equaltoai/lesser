package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
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
	// Remote status projection stores the bare actor-path username, so a remote
	// alice collides with local alice unless AuthorID locality remains in the
	// authorization guard.
	remoteStatus.AuthorUsername = "alice"
	require.Equal(t, publicStatus.AuthorUsername, remoteStatus.AuthorUsername)
	remoteStreams := handler.buildWebSocketStatusStreams(&remoteStatus)
	require.Contains(t, remoteStreams, streaming.PublicRemoteStream)
	require.NotContains(t, remoteStreams, streaming.PublicLocalStream)
	require.NotContains(t, remoteStreams, streaming.PublicActorStreamName("alice"))
}

func TestPublicActorDeletionFanoutRequiresPublicLocalAuthor(t *testing.T) {
	const actorConnection = "anonymous-actor"

	streamRepo := &fakeStreamRepo{
		subsByStream: map[string][]models.WebSocketSubscription{
			streaming.PublicActorStreamName("alice"): {{ConnectionID: actorConnection}},
		},
		getConnErrByID: make(map[string]error),
	}
	client := &fakeStreamerClient{}
	handler := &StreamRouterHandler{
		logger:        zap.NewNop(),
		apiClient:     client,
		streamingRepo: streamRepo,
		accountRepo:   fakeFollowerRepo{},
		statusRepo: fakeStatusRepo{statusByID: map[string]*models.Status{
			"local-public":   {StatusID: "local-public"},
			"local-private":  {StatusID: "local-private"},
			"remote-public":  {StatusID: "remote-public"},
			"missing-author": {StatusID: "missing-author"},
		}},
		domain: "example.com",
	}

	deletedAt := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	localPublic := newTombstoneRecord("local-public", "https://example.com/users/alice", true, deletedAt)
	require.NoError(t, handler.processTombstoneEvent(context.Background(), "local-public", localPublic))
	require.Equal(t, 1, connectionCallCount(client.postCalls, actorConnection))

	// Both remote and local projections use the bare username "alice". The
	// remote actor ID must keep its deletion off public:actor:alice.
	remotePublic := newTombstoneRecord("remote-public", "https://remote.example/users/alice", true, deletedAt)
	require.NoError(t, handler.processTombstoneEvent(context.Background(), "remote-public", remotePublic))
	require.Equal(t, 1, connectionCallCount(client.postCalls, actorConnection))

	localPrivate := newTombstoneRecord("local-private", "https://example.com/users/alice", false, deletedAt)
	require.NoError(t, handler.processTombstoneEvent(context.Background(), "local-private", localPrivate))
	require.Equal(t, 1, connectionCallCount(client.postCalls, actorConnection))

	// DeletedBy identifies the deleter, not necessarily the author. A public
	// tombstone without attribution must not guess an actor stream from it.
	missingAuthor := newTombstoneRecordWithActors(
		"missing-author",
		"",
		"https://example.com/users/alice",
		true,
		deletedAt,
	)
	require.NoError(t, handler.processTombstoneEvent(context.Background(), "missing-author", missingAuthor))
	require.Equal(t, 1, connectionCallCount(client.postCalls, actorConnection))
}

func newTombstoneRecord(id, authorID string, isPublic bool, deletedAt time.Time) events.DynamoDBEventRecord {
	return newTombstoneRecordWithActors(id, authorID, authorID, isPublic, deletedAt)
}

func newTombstoneRecordWithActors(id, attributedTo, deletedBy string, isPublic bool, deletedAt time.Time) events.DynamoDBEventRecord {
	return events.DynamoDBEventRecord{
		EventID:   "tombstone-" + id,
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"id":           events.NewStringAttribute(id),
			"type":         events.NewStringAttribute("Tombstone"),
			"formerType":   events.NewStringAttribute("Note"),
			"deleted":      events.NewStringAttribute(deletedAt.Format(time.RFC3339)),
			"deletedBy":    events.NewStringAttribute(deletedBy),
			"attributedTo": events.NewStringAttribute(attributedTo),
			"isPublic":     events.NewBooleanAttribute(isPublic),
		}},
	}
}

func TestPublicActorDeletionDoesNotWriteUnconsumableSSEEvent(t *testing.T) {
	t.Setenv("STREAM_EVENTS_TABLE_NAME", "stream-events")

	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db).Twice()
	db.On("Model", mock.Anything).Return(query).Twice()
	query.On("Create").Return(nil).Twice()

	handler := &StreamRouterHandler{
		logger:         zap.NewNop(),
		apiClient:      &fakeStreamerClient{},
		streamingRepo:  &fakeStreamRepo{},
		statusRepo:     fakeStatusRepo{statusByID: map[string]*models.Status{"local-public": {StatusID: "local-public"}}},
		streamEventLog: streaming.NewStreamEventLog(db, time.Hour),
		domain:         "example.com",
	}
	tombstone := &models.Tombstone{
		ID:           "local-public",
		FormerType:   activitypub.NoteType,
		AttributedTo: "https://example.com/users/alice",
		DeletedBy:    "https://example.com/users/alice",
		IsPublic:     true,
	}

	require.NoError(t, handler.broadcastDeletionToStreams(context.Background(), "sse-delete", tombstone, StreamMessage{}))
	db.AssertExpectations(t)
	query.AssertExpectations(t)
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
