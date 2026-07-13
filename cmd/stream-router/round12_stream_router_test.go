package main

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/activitypub"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/apptheory/pkg/streamer"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type fakeDB struct{}

func (fakeDB) Model(any) dynamormCore.Query                { return nil }
func (fakeDB) Migrate() error                              { return nil }
func (fakeDB) AutoMigrate(...any) error                    { return nil }
func (fakeDB) Close() error                                { return nil }
func (fakeDB) WithContext(context.Context) dynamormCore.DB { return fakeDB{} }

type fakeStreamRepo struct {
	connectionsByID   map[string]*models.WebSocketConnection
	connectionsByUser map[string][]models.WebSocketConnection
	subsByStream      map[string][]models.WebSocketSubscription

	getConnErrByID          map[string]error
	getConnectionsByUserErr error
	getSubscriptionsErr     error

	deleteSubscriptionCalls []string
	deleteConnectionCalls   []string
	deleteSubscriptionErr   error
	deleteConnectionErr     error
}

func (r *fakeStreamRepo) GetConnection(_ context.Context, connectionID string) (*models.WebSocketConnection, error) {
	if err := r.getConnErrByID[connectionID]; err != nil {
		return nil, err
	}
	if r.connectionsByID == nil {
		return nil, nil
	}
	return r.connectionsByID[connectionID], nil
}

func (r *fakeStreamRepo) GetConnectionsByUser(_ context.Context, userID string) ([]models.WebSocketConnection, error) {
	if r.getConnectionsByUserErr != nil {
		return nil, r.getConnectionsByUserErr
	}
	if r.connectionsByUser == nil {
		return []models.WebSocketConnection{}, nil
	}
	return r.connectionsByUser[userID], nil
}

func (r *fakeStreamRepo) GetSubscriptionsForStream(_ context.Context, stream string) ([]models.WebSocketSubscription, error) {
	if r.getSubscriptionsErr != nil {
		return nil, r.getSubscriptionsErr
	}
	if r.subsByStream == nil {
		return []models.WebSocketSubscription{}, nil
	}
	return r.subsByStream[stream], nil
}

func (r *fakeStreamRepo) DeleteSubscription(_ context.Context, connectionID, stream string) error {
	r.deleteSubscriptionCalls = append(r.deleteSubscriptionCalls, stream+"|"+connectionID)
	return r.deleteSubscriptionErr
}

func (r *fakeStreamRepo) DeleteConnection(_ context.Context, connectionID string) error {
	r.deleteConnectionCalls = append(r.deleteConnectionCalls, connectionID)
	return r.deleteConnectionErr
}

type fakeStreamerClient struct {
	postCalls    []string
	postErrByID  map[string]error
	lastPayloads map[string][]byte
}

func (c *fakeStreamerClient) PostToConnection(_ context.Context, connectionID string, data []byte) error {
	c.postCalls = append(c.postCalls, connectionID)
	if c.lastPayloads == nil {
		c.lastPayloads = make(map[string][]byte)
	}
	c.lastPayloads[connectionID] = append([]byte(nil), data...)
	if c.postErrByID == nil {
		return nil
	}
	return c.postErrByID[connectionID]
}

func (c *fakeStreamerClient) DeleteConnection(context.Context, string) error {
	return nil
}

func (c *fakeStreamerClient) GetConnection(context.Context, string) (streamer.Connection, error) {
	return streamer.Connection{}, nil
}

type fakeGraphQLSubRepo struct {
	subsByStream map[string][]models.GraphQLStreamSubscription

	deleteAllCalls []string
}

func (r *fakeGraphQLSubRepo) ListByStream(_ context.Context, stream string) ([]models.GraphQLStreamSubscription, error) {
	if r.subsByStream == nil {
		return []models.GraphQLStreamSubscription{}, nil
	}
	return r.subsByStream[stream], nil
}

func (r *fakeGraphQLSubRepo) DeleteAllForConnection(_ context.Context, connectionID string) error {
	r.deleteAllCalls = append(r.deleteAllCalls, connectionID)
	return nil
}

type fakeFollowerRepo struct {
	actors []*activitypub.Actor
	cursor string
	err    error
}

func (r fakeFollowerRepo) GetFollowers(_ context.Context, _ string, _ int, _ string) ([]*activitypub.Actor, string, error) {
	return r.actors, r.cursor, r.err
}

type fakeStatusRepo struct {
	statusByID map[string]*models.Status
	err        error
}

func (r fakeStatusRepo) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.statusByID == nil {
		return nil, nil
	}
	return r.statusByID[statusID], nil
}

func newUserModifyRecordWithoutID() events.DynamoDBEventRecord {
	return events.DynamoDBEventRecord{
		EventID:   "evt-user-1",
		EventName: eventNameModify,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("USER#1"),
			},
		},
	}
}

func newNotificationInsertRecord(username string) events.DynamoDBEventRecord {
	now := time.Now().UTC()
	notificationMap := map[string]events.DynamoDBAttributeValue{
		"id":         events.NewStringAttribute("notif-1"),
		"type":       events.NewStringAttribute("mention"),
		"username":   events.NewStringAttribute(username),
		"account_id": events.NewStringAttribute("https://example.com/users/bob"),
		"status_id":  events.NewStringAttribute("https://example.com/statuses/1"),
		"read":       events.NewBooleanAttribute(false),
		"created_at": events.NewStringAttribute(now.Format(time.RFC3339)),
	}

	return events.DynamoDBEventRecord{
		EventID:   "evt-notif-1",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":           events.NewStringAttribute("NOTIFICATION#1"),
				"SK":           events.NewStringAttribute("NOTIFICATION#1"),
				"Notification": events.NewMapAttribute(notificationMap),
				"CreatedAt":    events.NewStringAttribute(now.Format(time.RFC3339)),
			},
		},
	}
}

func newUserConversationStateRecord(username, conversationID, requestState string) events.DynamoDBEventRecord {
	return events.DynamoDBEventRecord{
		EventID:   "evt-user-conversation-state-1",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":             events.NewStringAttribute("USER_CONVERSATION_STATE#" + username),
				"SK":             events.NewStringAttribute("CONVERSATION#" + conversationID),
				"gsi1PK":         events.NewStringAttribute("CONVERSATION#" + conversationID),
				"gsi1SK":         events.NewStringAttribute("2026-02-19T00:00:00Z#" + conversationID),
				"gsi3PK":         events.NewStringAttribute("CONVERSATION#" + conversationID),
				"gsi3SK":         events.NewStringAttribute("USER#" + username),
				"viewerID":       events.NewStringAttribute(username),
				"conversationID": events.NewStringAttribute(conversationID),
				"counterpartID":  events.NewStringAttribute("bob"),
				"folder":         events.NewStringAttribute("INBOX"),
				"requestState":   events.NewStringAttribute(requestState),
				"sortAt":         events.NewStringAttribute("2026-02-19T00:00:00Z"),
				"createdAt":      events.NewStringAttribute("2026-02-19T00:00:00Z"),
				"updatedAt":      events.NewStringAttribute("2026-02-19T00:00:00Z"),
			},
		},
	}
}

func TestAppendStreamEvent_Round12(t *testing.T) {
	t.Run("ignores disabled and incomplete inputs", func(t *testing.T) {
		h := &StreamRouterHandler{logger: zap.NewNop()}

		h.appendStreamEvent(context.Background(), "req", "home", "update", "{}")

		t.Setenv("STREAM_EVENTS_TABLE_NAME", "stream-events")
		h.streamEventLog = streaming.NewStreamEventLog(nil, time.Minute)
		h.appendStreamEvent(context.Background(), "req", "", "update", "{}")
		h.appendStreamEvent(context.Background(), "req", "home", "", "{}")
	})

	t.Run("attempts append when configured", func(t *testing.T) {
		t.Setenv("STREAM_EVENTS_TABLE_NAME", "stream-events")

		db := new(dynamormmocks.MockDB)
		query := new(dynamormmocks.MockQuery)
		db.On("WithContext", mock.Anything).Return(db).Once()
		db.On("Model", mock.Anything).Return(query).Once()
		query.On("Create").Return(stdErrors.New("boom")).Once()

		h := &StreamRouterHandler{
			logger:         zap.NewNop(),
			streamEventLog: streaming.NewStreamEventLog(db, time.Minute),
		}

		h.appendStreamEvent(context.Background(), "req", "home", "update", `{"id":"1"}`)
		db.AssertExpectations(t)
		query.AssertExpectations(t)
	})
}

func TestHandleStreamRouterStreamRecord_Round12(t *testing.T) {
	origHandler := handler
	t.Cleanup(func() { handler = origHandler })

	handler = nil
	err := handleStreamRouterStreamRecord(nil, events.DynamoDBEventRecord{EventID: "evt-1", EventName: eventNameInsert})
	require.Error(t, err)

	handler = &StreamRouterHandler{logger: zap.NewNop()}
	err = handleStreamRouterStreamRecord(nil, events.DynamoDBEventRecord{EventID: "evt-2", EventName: "REMOVE"})
	require.NoError(t, err)
}

func newStatusInsertRecord(eventName string) events.DynamoDBEventRecord {
	now := time.Now().UTC()
	noteMap := map[string]events.DynamoDBAttributeValue{
		"BaseObject": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"id":        events.NewStringAttribute("https://example.com/statuses/1"),
			"type":      events.NewStringAttribute("Note"),
			"published": events.NewStringAttribute(now.Format(time.RFC3339)),
			"summary":   events.NewStringAttribute("cw"),
			"sensitive": events.NewBooleanAttribute(false),
		}),
		"content":      events.NewStringAttribute("<p>Hello</p>"),
		"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
		"attachment": events.NewListAttribute([]events.DynamoDBAttributeValue{
			events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
				"type":      events.NewStringAttribute("Image"),
				"mediaType": events.NewStringAttribute("image/jpeg"),
				"url":       events.NewStringAttribute("https://cdn.example.com/media/1.jpg"),
				"name":      events.NewStringAttribute("alt"),
				"width":     events.NewNumberAttribute("640"),
				"height":    events.NewNumberAttribute("480"),
			}),
		}),
	}

	return events.DynamoDBEventRecord{
		EventID:   "evt-status-1",
		EventName: eventName,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":             events.NewStringAttribute("STATUS#1"),
				"SK":             events.NewStringAttribute("STATUS#1"),
				"statusID":       events.NewStringAttribute("1"),
				"note":           events.NewMapAttribute(noteMap),
				"content":        events.NewStringAttribute("<p>Hello</p>"),
				"visibility":     events.NewStringAttribute("public"),
				"language":       events.NewStringAttribute("en"),
				"sensitive":      events.NewBooleanAttribute(false),
				"authorUsername": events.NewStringAttribute("alice"),
				"authorID":       events.NewStringAttribute("https://example.com/users/alice"),
				"inReplyToID":    events.NewStringAttribute("parent"),
				"hashtags":       events.NewListAttribute([]events.DynamoDBAttributeValue{events.NewStringAttribute("golang")}),
				"mentions":       events.NewListAttribute([]events.DynamoDBAttributeValue{events.NewStringAttribute("bob")}),
				"publishedAt":    events.NewStringAttribute(now.Format(time.RFC3339)),
				"updatedAt":      events.NewStringAttribute(now.Format(time.RFC3339)),
				"reblogOfID":     events.NewStringAttribute(""),
			},
		},
	}
}

func TestConnectionRepositoryAdapter_Round12(t *testing.T) {
	repo := &fakeStreamRepo{
		connectionsByUser: map[string][]models.WebSocketConnection{
			"u1": {
				{ConnectionID: "c1", UserID: "u1", Username: "alice", Streams: []string{"public"}, LastActivity: time.Now()},
				{ConnectionID: "c2", UserID: "u1", Username: "alice", Streams: []string{"user:alice"}, LastActivity: time.Now()},
			},
		},
		subsByStream: map[string][]models.WebSocketSubscription{
			"public": {
				{ConnectionID: "c1"},
				{ConnectionID: "c2"},
			},
		},
		connectionsByID: map[string]*models.WebSocketConnection{
			"c1": {ConnectionID: "c1", UserID: "u1", Username: "alice", Streams: []string{"public"}, LastActivity: time.Now()},
			"c2": {ConnectionID: "c2", UserID: "u1", Username: "alice", Streams: []string{"user:alice"}, LastActivity: time.Now()},
		},
	}

	adapter := &connectionRepositoryAdapter{streamingRepo: repo, logger: zap.NewNop()}

	userConns, err := adapter.GetUserConnections(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, userConns, 2)
	require.Equal(t, "c1", userConns[0].ConnectionID)

	streamConns, err := adapter.GetStreamConnections(context.Background(), "public")
	require.NoError(t, err)
	require.Len(t, streamConns, 2)

	t.Run("missing connection is skipped", func(t *testing.T) {
		repo.getConnErrByID = map[string]error{
			"c2": errors.ItemNotFoundWithID("streaming connection", "c2"),
		}
		conns, err := adapter.GetStreamConnections(context.Background(), "public")
		require.NoError(t, err)
		require.Len(t, conns, 1)
	})

	t.Run("nil connection is skipped", func(t *testing.T) {
		repo.getConnErrByID = nil
		delete(repo.connectionsByID, "c2")
		conns, err := adapter.GetStreamConnections(context.Background(), "public")
		require.NoError(t, err)
		require.Len(t, conns, 1)
	})

	t.Run("generic get connection error is skipped", func(t *testing.T) {
		repo.getConnErrByID = map[string]error{
			"c2": stdErrors.New("boom"),
		}
		conns, err := adapter.GetStreamConnections(context.Background(), "public")
		require.NoError(t, err)
		require.Len(t, conns, 1)
	})

	convConns, err := adapter.GetConversationConnections(context.Background(), "abc")
	require.NoError(t, err)
	require.Len(t, convConns, 0)

	repo.getConnectionsByUserErr = stdErrors.New("boom")
	_, err = adapter.GetUserConnections(context.Background(), "u1")
	require.Error(t, err)

	repo.getConnectionsByUserErr = nil
	repo.getSubscriptionsErr = stdErrors.New("subs boom")
	_, err = adapter.GetStreamConnections(context.Background(), "public")
	require.Error(t, err)
}

func TestStreamRouterHandler_HandleDynamoDBRecord_Round12(t *testing.T) {
	h := &StreamRouterHandler{logger: zap.NewNop(), domain: "example.com"}

	t.Run("processRecord returns errors", func(t *testing.T) {
		err := h.processRecord(context.Background(), "req-1", newUserModifyRecordWithoutID())
		require.Error(t, err)
	})

	t.Run("HandleDynamoDBRecord returns processing errors for retry", func(t *testing.T) {
		require.Error(t, h.HandleDynamoDBRecord(nil, newUserModifyRecordWithoutID()))
	})
}

func TestStreamRouterHandler_StatusAndNotification_Round12(t *testing.T) {
	streamRepo := &fakeStreamRepo{
		subsByStream: map[string][]models.WebSocketSubscription{
			streaming.PublicStream: {
				{ConnectionID: "ok"},
			},
			streaming.UserStreamName("alice"): {
				{ConnectionID: "gone"},
				{ConnectionID: "ok"},
			},
		},
		deleteSubscriptionCalls: nil,
		getConnErrByID:          make(map[string]error),
	}
	client := &fakeStreamerClient{
		postErrByID: map[string]error{
			"gone": stdErrors.New("GoneException: 410 Gone"),
		},
	}

	accountRepo := fakeFollowerRepo{
		actors: []*activitypub.Actor{
			{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
			{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/eve"}, PreferredUsername: "eve"},
			{PreferredUsername: "carol"},
		},
		cursor: "next",
	}

	h := &StreamRouterHandler{
		logger:        zap.NewNop(),
		apiClient:     client,
		streamingRepo: streamRepo,
		accountRepo:   accountRepo,
		domain:        "example.com",
	}
	ctx := context.Background()
	requestID := "req-2"

	require.NoError(t, h.processRecord(ctx, requestID, newStatusInsertRecord(eventNameInsert)))
	require.NoError(t, h.processRecord(ctx, requestID, newStatusInsertRecord(eventNameModify)))

	// Notification: missing username yields a typed error
	require.Error(t, h.processRecord(ctx, requestID, newNotificationInsertRecord("")))

	// Notification: broadcast failure is logged but does not fail processing
	streamRepo.getSubscriptionsErr = stdErrors.New("repo down")
	require.NoError(t, h.processRecord(ctx, requestID, newNotificationInsertRecord("alice")))

	require.GreaterOrEqual(t, len(client.postCalls), 1)
	require.NotEmpty(t, streamRepo.deleteSubscriptionCalls)
	require.NotEmpty(t, streamRepo.deleteConnectionCalls)
}

func TestStreamRouterHandler_BroadcastToStream_Round12(t *testing.T) {
	ctx := context.Background()
	requestID := "req-3"

	t.Run("empty subscriptions returns nil", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger:        zap.NewNop(),
			apiClient:     &fakeStreamerClient{},
			streamingRepo: &fakeStreamRepo{},
		}
		require.NoError(t, h.broadcastToStream(ctx, requestID, "stream", "update", []byte(`{"ok":true}`)))
	})

	t.Run("all failures returns SendToAllConnectionsFailed", func(t *testing.T) {
		repo := &fakeStreamRepo{
			subsByStream: map[string][]models.WebSocketSubscription{
				"stream": {
					{ConnectionID: "c1"},
					{ConnectionID: "c2"},
				},
			},
		}
		client := &fakeStreamerClient{
			postErrByID: map[string]error{
				"c1": stdErrors.New("boom"),
				"c2": stdErrors.New("boom"),
			},
		}
		h := &StreamRouterHandler{logger: zap.NewNop(), apiClient: client, streamingRepo: repo}
		require.Error(t, h.broadcastToStream(ctx, requestID, "stream", "update", []byte(`{"ok":true}`)))
	})

	t.Run("subscription query error returns error", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger:        zap.NewNop(),
			apiClient:     &fakeStreamerClient{},
			streamingRepo: &fakeStreamRepo{getSubscriptionsErr: stdErrors.New("db down")},
		}
		require.Error(t, h.broadcastToStream(ctx, requestID, "stream", "update", []byte(`{"ok":true}`)))
	})
}

func TestBroadcastToFollowers_Round12(t *testing.T) {
	t.Run("invalid account id fails fast", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger: zap.NewNop(),
			domain: "example.com",
		}
		require.Error(t, h.broadcastToFollowers(context.Background(), "req-followers-invalid", "not-a-url", []byte(`{}`)))
	})

	t.Run("follower query error is surfaced", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger:      zap.NewNop(),
			accountRepo: fakeFollowerRepo{err: stdErrors.New("followers failed")},
			domain:      "example.com",
		}
		require.Error(t, h.broadcastToFollowers(context.Background(), "req-followers-err", "https://example.com/users/alice", []byte(`{}`)))
	})

	t.Run("no followers returns nil", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger:      zap.NewNop(),
			accountRepo: fakeFollowerRepo{},
			domain:      "example.com",
		}
		require.NoError(t, h.broadcastToFollowers(context.Background(), "req-followers-none", "https://example.com/users/alice", []byte(`{}`)))
	})

	t.Run("successful broadcasts return nil", func(t *testing.T) {
		client := &fakeStreamerClient{}
		streamRepo := &fakeStreamRepo{
			subsByStream: map[string][]models.WebSocketSubscription{
				streaming.UserStreamName("bob"): {{ConnectionID: "c1"}},
			},
		}
		h := &StreamRouterHandler{
			logger:        zap.NewNop(),
			domain:        "example.com",
			apiClient:     client,
			streamingRepo: streamRepo,
			accountRepo: fakeFollowerRepo{
				actors: []*activitypub.Actor{
					{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
				},
			},
		}

		require.NoError(t, h.broadcastToFollowers(context.Background(), "req-followers-ok", "https://example.com/users/alice", []byte(`{"ok":true}`)))
		require.Contains(t, client.postCalls, "c1")
	})

	t.Run("all broadcast failures returns error", func(t *testing.T) {
		client := &fakeStreamerClient{
			postErrByID: map[string]error{
				"c1": stdErrors.New("boom"),
			},
		}
		streamRepo := &fakeStreamRepo{
			subsByStream: map[string][]models.WebSocketSubscription{
				streaming.UserStreamName("bob"): {{ConnectionID: "c1"}},
			},
		}
		h := &StreamRouterHandler{
			logger:        zap.NewNop(),
			domain:        "example.com",
			apiClient:     client,
			streamingRepo: streamRepo,
			accountRepo: fakeFollowerRepo{
				actors: []*activitypub.Actor{
					{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
				},
			},
		}

		require.Error(t, h.broadcastToFollowers(context.Background(), "req-followers-fail", "https://example.com/users/alice", []byte(`{"ok":true}`)))
	})
}

func TestStreamRouterHandler_LocalRecipients_Round12(t *testing.T) {
	h := &StreamRouterHandler{domain: "example.com"}
	status := &models.Status{
		ToRecipients: []string{
			"acct:alice@example.com",
			"acct:remote@remote.example",
			"https://example.com/users/bob",
			"https://remote.example/users/eve",
		},
		CcRecipients: []string{"acct:alice@example.com"},
	}

	usernames := h.localRecipients(status)
	require.ElementsMatch(t, []string{"alice", "bob"}, usernames)
	require.Equal(t, "carol", h.extractLocalUsername("acct:carol@example.com"))
	require.Equal(t, "", h.extractLocalUsername("acct:carol@remote.example"))
	require.Equal(t, "dave", h.extractLocalUsername("https://example.com/users/dave"))
	require.Equal(t, "", h.extractLocalUsername("https://remote.example/users/dave"))
	require.Equal(t, "", h.extractLocalUsername("not-a-url"))
}

func TestMapAttachmentType_Round12(t *testing.T) {
	require.Equal(t, mediaTypeImage, mapAttachmentType("image"))
	require.Equal(t, mediaTypeVideo, mapAttachmentType("video"))
	require.Equal(t, mediaTypeAudio, mapAttachmentType("audio"))
	require.Equal(t, mediaTypeUnknown, mapAttachmentType("document"))
	require.Equal(t, mediaTypeImage, mapAttachmentType("some-image-type"))
	require.Equal(t, mediaTypeUnknown, mapAttachmentType("weird"))
}

func TestStreamRouterErrors_Constructors_Round12(t *testing.T) {
	errs := []error{
		ConnectionNotFound(),
		WebSocketEndpointNotSet(),
		HandlerNotInitialized(),
		AllRecordsFailedProcessing(),
		BroadcastToAllFollowersFailed(),
		SendToAllConnectionsFailed(),
		NotificationMissingUsername(),
		AccountMissingID(),
		UsernameCannotBeEmpty(),
		CouldNotExtractUsername(),
		UnknownEventName(),
		FailedToGetSubscriptionsForStream(stdErrors.New("x")),
		FailedToQueryConnection(stdErrors.New("x")),
		FailedToMarshalStatus(stdErrors.New("x")),
		FailedToMarshalNotification(stdErrors.New("x")),
		FailedToMarshalAccount(stdErrors.New("x")),
		FailedToMarshalMessage(stdErrors.New("x")),
		FailedToCreateAccountPayload(stdErrors.New("x")),
		FailedToGetFollowers(stdErrors.New("x")),
		FailedToGetSubscriptions(stdErrors.New("x")),
		FailedToGetStatusForHashtagExtraction(stdErrors.New("x")),
		HashtagProcessingFailed(stdErrors.New("x")),
	}

	for _, err := range errs {
		require.Error(t, err)
	}
}

func TestStreamRouterHandler_ProcessAccountEvent_Success_Round12(t *testing.T) {
	h := &StreamRouterHandler{
		logger:      zap.NewNop(),
		domain:      "example.com",
		accountRepo: fakeFollowerRepo{},
	}
	ctx := context.Background()
	requestID := "req-4"
	record := events.DynamoDBEventRecord{
		EventID:   "evt-account-1",
		EventName: eventNameModify,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"ID": events.NewStringAttribute("https://example.com/users/alice"),
			},
		},
	}

	require.NoError(t, h.processAccountEvent(ctx, requestID, record))
}

type fakePublisher struct {
	errByStream map[string]error
	calls       []string
}

func (p *fakePublisher) PublishToUser(context.Context, string, *streaming.Event) error {
	return nil
}

func (p *fakePublisher) PublishToStream(_ context.Context, streamName string, _ *streaming.Event) error {
	p.calls = append(p.calls, streamName)
	if p.errByStream == nil {
		return nil
	}
	return p.errByStream[streamName]
}

func (p *fakePublisher) PublishToConversation(context.Context, string, *streaming.Event) error {
	return nil
}

func (p *fakePublisher) Close() error { return nil }

func TestStreamRouterHandler_ProcessTombstoneEvent_Round12(t *testing.T) {
	now := time.Now().UTC()
	streamRepo := &fakeStreamRepo{
		subsByStream: map[string][]models.WebSocketSubscription{
			streaming.PublicStream: {{ConnectionID: "c1"}},
		},
	}
	client := &fakeStreamerClient{}
	publisher := &fakePublisher{
		errByStream: map[string]error{
			streaming.HashtagStreamName("golang"): stdErrors.New("publish failed"),
		},
	}

	h := &StreamRouterHandler{
		logger:        zap.NewNop(),
		apiClient:     client,
		streamingRepo: streamRepo,
		accountRepo: fakeFollowerRepo{
			actors: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
			},
		},
		statusRepo: fakeStatusRepo{
			statusByID: map[string]*models.Status{
				"1": {StatusID: "1", AuthorID: "https://example.com/users/alice", Content: "<p>Hello #golang</p>"},
			},
		},
		publisher: publisher,
		domain:    "example.com",
	}

	ctx := context.Background()
	requestID := "req-5"
	record := events.DynamoDBEventRecord{
		EventID:   "evt-tomb-1",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"ID":         events.NewStringAttribute("1"),
				"Type":       events.NewStringAttribute("Tombstone"),
				"FormerType": events.NewStringAttribute("Note"),
				"Deleted":    events.NewStringAttribute(now.Format(time.RFC3339)),
				"DeletedBy":  events.NewStringAttribute("https://example.com/users/alice"),
			},
		},
	}

	require.NoError(t, h.processTombstoneEvent(ctx, requestID, record))
	require.NotEmpty(t, publisher.calls)
}

func TestStreamRouterHandler_ProcessTombstoneEvent_ErrorBranches_Round12(t *testing.T) {
	h := &StreamRouterHandler{
		logger:      zap.NewNop(),
		domain:      "example.com",
		accountRepo: fakeFollowerRepo{err: stdErrors.New("followers failed")},
	}
	ctx := context.Background()
	requestID := "req-err-tomb"

	// Non-insert events are ignored.
	modify := events.DynamoDBEventRecord{
		EventID:   "evt-tomb-mod",
		EventName: eventNameModify,
	}
	require.NoError(t, h.processTombstoneEvent(ctx, requestID, modify))

	// Unmarshal failures are logged but do not fail the batch.
	bad := events.DynamoDBEventRecord{
		EventID:   "evt-tomb-bad",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"ID":         events.NewStringAttribute("1"),
				"DeletedBy":  events.NewStringAttribute("https://example.com/users/alice"),
				"FormerType": events.NewStringAttribute("Note"),
				"Deleted": events.NewListAttribute([]events.DynamoDBAttributeValue{
					events.NewStringAttribute("not-a-time"),
				}),
			},
		},
	}
	require.NoError(t, h.processTombstoneEvent(ctx, requestID, bad))

	// Follower removal errors are logged but not returned.
	erringFollowers := events.DynamoDBEventRecord{
		EventID:   "evt-tomb-followers",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"ID":         events.NewStringAttribute("1"),
				"FormerType": events.NewStringAttribute("Follow"),
				"Deleted":    events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
				"DeletedBy":  events.NewStringAttribute("https://example.com/users/alice"),
			},
		},
	}
	require.NoError(t, h.processTombstoneEvent(ctx, requestID, erringFollowers))
}

func TestStreamRouterHandler_RemoveFromHashtagStreams_Round12(t *testing.T) {
	ctx := context.Background()
	requestID := "req-hashtags"

	t.Run("status not found is ignored", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger:     zap.NewNop(),
			statusRepo: fakeStatusRepo{err: stdErrors.New("not found")},
		}
		require.NoError(t, h.removeFromHashtagStreams(ctx, requestID, "1"))
	})

	t.Run("status retrieval failure is returned", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger:     zap.NewNop(),
			statusRepo: fakeStatusRepo{err: stdErrors.New("boom")},
		}
		require.Error(t, h.removeFromHashtagStreams(ctx, requestID, "1"))
	})

	t.Run("no hashtags returns nil", func(t *testing.T) {
		h := &StreamRouterHandler{
			logger: zap.NewNop(),
			statusRepo: fakeStatusRepo{
				statusByID: map[string]*models.Status{"1": {StatusID: "1", AuthorID: "https://example.com/users/alice", Content: "hello"}},
			},
		}
		require.NoError(t, h.removeFromHashtagStreams(ctx, requestID, "1"))
	})

	t.Run("successful publish returns nil", func(t *testing.T) {
		pub := &fakePublisher{}
		h := &StreamRouterHandler{
			logger: zap.NewNop(),
			statusRepo: fakeStatusRepo{
				statusByID: map[string]*models.Status{"1": {StatusID: "1", AuthorID: "https://remote.example/users/alice", Content: "hello #golang"}},
			},
			publisher: pub,
		}
		require.NoError(t, h.removeFromHashtagStreams(ctx, requestID, "1"))
		require.Contains(t, pub.calls, streaming.HashtagStreamName("golang"))
	})
}

func TestStreamRouterHandler_processRecord_MoreBranches_Round12(t *testing.T) {
	h := &StreamRouterHandler{
		logger:      zap.NewNop(),
		domain:      "example.com",
		accountRepo: fakeFollowerRepo{},
	}
	ctx := context.Background()
	requestID := "req-records"

	// Missing PK => unidentifiable record is skipped.
	require.NoError(t, h.processRecord(ctx, requestID, events.DynamoDBEventRecord{
		EventID:   "evt-missing-pk",
		EventName: eventNameInsert,
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}},
	}))

	// Unknown entity type is ignored.
	require.NoError(t, h.processRecord(ctx, requestID, events.DynamoDBEventRecord{
		EventID:   "evt-unknown",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("UNKNOWN#1")},
		},
	}))

	// Tombstone entity routes to tombstone handler.
	require.NoError(t, h.processRecord(ctx, requestID, events.DynamoDBEventRecord{
		EventID:   "evt-tomb-route",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":         events.NewStringAttribute("TOMBSTONE#1"),
				"ID":         events.NewStringAttribute("1"),
				"FormerType": events.NewStringAttribute("Follow"),
				"Deleted":    events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
				"DeletedBy":  events.NewStringAttribute("https://example.com/users/alice"),
			},
		},
	}))
}

func TestStreamRouterHandler_PublishAccountEventToInternalBus_Round12(t *testing.T) {
	h := &StreamRouterHandler{logger: zap.NewNop()}
	requestID := "req-internal"

	require.NoError(t, h.publishAccountEventToInternalBus(requestID, "https://example.com/users/alice", eventNameInsert, []string{"account:alice"}))
	require.NoError(t, h.publishAccountEventToInternalBus(requestID, "https://example.com/users/alice", eventNameRemove, []string{"account:alice"}))
	require.Error(t, h.publishAccountEventToInternalBus(requestID, "https://example.com/users/alice", "WAT", nil))
}

func TestStreamRouterHandler_PublishStatusEventToInternalBus_Round12(t *testing.T) {
	h := &StreamRouterHandler{logger: zap.NewNop()}
	requestID := "req-status-internal"
	record := events.DynamoDBEventRecord{EventName: "WAT"}

	require.Error(t, h.publishStatusEventToInternalBus(requestID, record, &models.Status{StatusID: "1"}, nil))
}

func TestStreamRouterHandler_MiscHelpers_Round12(t *testing.T) {
	h := &StreamRouterHandler{domain: "example.com"}

	require.False(t, h.isLocalActorID(""))
	require.False(t, h.isLocalActorID("not-a-url"))
	require.True(t, h.isLocalActorID("https://example.com/users/alice"))

	require.Equal(t, "", localHashtagStreamName(""))
	require.Equal(t, "hashtag:local:golang", localHashtagStreamName("golang"))
}

func TestGenerateAttachmentID_Round12(t *testing.T) {
	require.Equal(t, "1", generateAttachmentID("https://cdn.example.com/media/1.jpg"))
	require.Equal(t, "file", generateAttachmentID("https://cdn.example.com/media/file"))
}

func TestStreamRouterHandler_RemoveSubscription_Round12(t *testing.T) {
	t.Run("delete connection error is tolerated", func(t *testing.T) {
		repo := &fakeStreamRepo{deleteConnectionErr: stdErrors.New("delete failed")}
		h := &StreamRouterHandler{
			logger:        zap.NewNop(),
			streamingRepo: repo,
		}
		h.removeSubscription(context.Background(), "req-remove-sub", "stream", "conn")
		require.NotEmpty(t, repo.deleteSubscriptionCalls)
		require.NotEmpty(t, repo.deleteConnectionCalls)
	})

	t.Run("delete subscription error returns early", func(t *testing.T) {
		repo := &fakeStreamRepo{deleteSubscriptionErr: stdErrors.New("delete failed")}
		h := &StreamRouterHandler{
			logger:        zap.NewNop(),
			streamingRepo: repo,
		}
		h.removeSubscription(context.Background(), "req-remove-sub", "stream", "conn")
		require.NotEmpty(t, repo.deleteSubscriptionCalls)
		require.Empty(t, repo.deleteConnectionCalls)
	})
}

func TestStreamRouterHandler_ProcessNotificationEvent_EarlyReturns_Round12(t *testing.T) {
	h := &StreamRouterHandler{logger: zap.NewNop()}
	ctx := context.Background()
	requestID := "req-notif"

	// Non-insert notifications are ignored.
	require.NoError(t, h.processNotificationEvent(ctx, requestID, events.DynamoDBEventRecord{
		EventID:   "evt-notif-mod",
		EventName: eventNameModify,
	}))

	// Non-notification PKs are ignored.
	require.NoError(t, h.processNotificationEvent(ctx, requestID, events.DynamoDBEventRecord{
		EventID:   "evt-notif-skip",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("STATUS#1"),
			},
		},
	}))

	// Missing Notification field yields nil (unmarshal produces nil pointer).
	require.NoError(t, h.processNotificationEvent(ctx, requestID, events.DynamoDBEventRecord{
		EventID:   "evt-notif-missing",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("NOTIFICATION#1"),
				"SK": events.NewStringAttribute("NOTIFICATION#1"),
			},
		},
	}))
}

func TestStreamRouterHandler_GetFollowersForUser_EdgeCases_Round12(t *testing.T) {
	h := &StreamRouterHandler{
		logger: zap.NewNop(),
		domain: "example.com",
		accountRepo: fakeFollowerRepo{
			actors: []*activitypub.Actor{
				nil,
				{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/eve"}, PreferredUsername: "eve"},
				{PreferredUsername: "carol"},
				{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
			},
		},
	}
	ctx := context.Background()
	requestID := "req-followers"

	_, _, err := h.getFollowersForUser(ctx, requestID, "", 10)
	require.Error(t, err)

	followers, _, err := h.getFollowersForUser(ctx, requestID, "alice", 1000)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"carol", "bob"}, followers)
}

func TestStreamRouterHandler_BuildSSEStatusStreams_Direct_Round12(t *testing.T) {
	h := &StreamRouterHandler{logger: zap.NewNop(), domain: "example.com"}
	ctx := context.Background()
	requestID := "req-sse"
	status := &models.Status{
		AuthorUsername: "alice",
		AuthorID:       "https://example.com/users/alice",
		Visibility:     models.VisibilityDirect,
		ToRecipients:   []string{"acct:bob@example.com", "acct:eve@remote.example"},
	}

	streams := h.buildSSEStatusStreams(ctx, requestID, status)
	require.Contains(t, streams, streaming.UserStreamName("alice"))
	require.Contains(t, streams, streaming.DirectStreamName("bob"))
	require.Contains(t, streams, streaming.UserStreamName("bob"))
}

func TestStreamRouterHandler_ProcessAccountEvent_EarlyReturn_Round12(t *testing.T) {
	h := &StreamRouterHandler{logger: zap.NewNop(), domain: "example.com", accountRepo: fakeFollowerRepo{}}
	ctx := context.Background()
	require.NoError(t, h.processAccountEvent(ctx, "req-account", events.DynamoDBEventRecord{EventName: eventNameInsert}))
}

func TestStreamRouterHandler_ProcessConversationParticipantEvent_BroadcastsGraphQL_Round12(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		requestState string
		streamName   string
	}{
		{
			name:         "pending_routes_to_requests_stream",
			requestState: "PENDING",
			streamName:   streaming.DMRequestsStreamName("alice"),
		},
		{
			name:         "accepted_routes_to_inbox_stream",
			requestState: "ACCEPTED",
			streamName:   streaming.DMInboxStreamName("alice"),
		},
		{
			name:         "declined_routes_to_requests_stream",
			requestState: "DECLINED",
			streamName:   streaming.DMRequestsStreamName("alice"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeStreamerClient{}
			subRepo := &fakeGraphQLSubRepo{
				subsByStream: map[string][]models.GraphQLStreamSubscription{
					tc.streamName: {
						{
							ConnectionID:   "conn-1",
							SubscriptionID: "sub-1",
							Stream:         tc.streamName,
							Field:          "conversationUpdates",
							UserID:         "alice",
						},
					},
				},
			}

			h := &StreamRouterHandler{
				logger:        zap.NewNop(),
				graphqlClient: client,
				gqlSubRepo:    subRepo,
			}

			record := newUserConversationStateRecord("alice", "conv-1", tc.requestState)
			require.NoError(t, h.processUserConversationStateEvent(context.Background(), "req-1", record))

			require.Equal(t, []string{"conn-1"}, client.postCalls)

			var envelope map[string]any
			require.NoError(t, json.Unmarshal(client.lastPayloads["conn-1"], &envelope))
			require.Equal(t, "sub-1", envelope["id"])
			require.Equal(t, "next", envelope["type"])

			payload, ok := envelope["payload"].(map[string]any)
			require.True(t, ok)
			data, ok := payload["data"].(map[string]any)
			require.True(t, ok)
			updates, ok := data["conversationUpdates"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "conv-1", updates["id"])
		})
	}
}

func TestNewStreamRouterHandler_StreamClientFailure_Round12(t *testing.T) {
	origLambda := lambdaCtx
	origNewStreamer := newStreamerClient
	t.Cleanup(func() {
		lambdaCtx = origLambda
		newStreamerClient = origNewStreamer
	})

	newStreamerClient = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
		return nil, stdErrors.New("client down")
	}

	lambdaCtx = &common.LambdaContext{
		Logger:      zap.NewNop(),
		Config:      &appconfig.Config{DynamoTableName: "tbl", WebSocketEndpoint: "https://ws.example.com", Domain: "example.com"},
		AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		DynamoDB:    fakeDB{},
	}

	_, err := NewStreamRouterHandler()
	require.Error(t, err)
}

func TestNewStreamRouterHandler_ManualInitFailure_Round12(t *testing.T) {
	origLambda := lambdaCtx
	origNewClient := newLambdaOptimizedClient
	t.Cleanup(func() {
		lambdaCtx = origLambda
		newLambdaOptimizedClient = origNewClient
	})

	newLambdaOptimizedClient = func(context.Context, string) (dynamormCore.DB, error) {
		return nil, stdErrors.New("dynamo down")
	}

	lambdaCtx = &common.LambdaContext{
		Logger:      zap.NewNop(),
		Config:      &appconfig.Config{Region: "us-east-1", DynamoTableName: "tbl", WebSocketEndpoint: "https://ws.example.com", Domain: "example.com"},
		AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		DynamoDB:    nil,
	}

	_, err := NewStreamRouterHandler()
	require.Error(t, err)
}

func TestStreamRouterMain_Round12(t *testing.T) {
	origLambdaCtx := lambdaCtx
	origHandler := handler
	origStart := startLambda
	t.Cleanup(func() {
		lambdaCtx = origLambdaCtx
		handler = origHandler
		startLambda = origStart
	})

	lambdaCtx = &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &appconfig.Config{
			DynamoTableName: "test-table",
		},
	}
	handler = &StreamRouterHandler{logger: zap.NewNop(), domain: "example.com"}

	called := false
	startLambda = func(h any) {
		called = true

		fn, ok := h.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{
			Records: []events.DynamoDBEventRecord{
				{
					EventID:        "1",
					EventName:      eventNameRemove,
					EventSource:    "aws:dynamodb",
					EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/test-table/stream/2024-01-01T00:00:00.000",
					Change:         events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}},
				},
			},
		}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := fn(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := respAny.(events.DynamoDBEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	main()
	require.True(t, called)
}

func TestNewStreamRouterHandler_ErrorBranches_Round12(t *testing.T) {
	origLambda := lambdaCtx
	origNewStreamer := newStreamerClient
	t.Cleanup(func() {
		lambdaCtx = origLambda
		newStreamerClient = origNewStreamer
	})

	newStreamerClient = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
		return &fakeStreamerClient{}, nil
	}

	lambdaCtx = &common.LambdaContext{
		Logger:      zap.NewNop(),
		Config:      &appconfig.Config{DynamoTableName: "tbl"},
		AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		DynamoDB:    fakeDB{},
	}

	_, err := NewStreamRouterHandler()
	require.Error(t, err)
	appErr, ok := errors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, errors.CodeInternal, appErr.Code)
	require.Equal(t, "WEBSOCKET_ENDPOINT", appErr.Metadata["variable_name"])
}

func TestInitializeManualServices_Round12(t *testing.T) {
	origLambda := lambdaCtx
	origNewClient := newLambdaOptimizedClient
	t.Cleanup(func() {
		lambdaCtx = origLambda
		newLambdaOptimizedClient = origNewClient
	})

	newLambdaOptimizedClient = func(context.Context, string) (dynamormCore.DB, error) {
		return fakeDB{}, nil
	}

	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "x")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "y")

	t.Run("wss endpoint is normalized to https", func(t *testing.T) {
		lambdaCtx = &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &appconfig.Config{},
		}
		t.Setenv("DYNAMO_TABLE_NAME", "tbl")
		t.Setenv("STREAMING_SUBSCRIPTIONS_TABLE", "subs")
		t.Setenv("WEBSOCKET_API_URL", "wss://ws.example.com/dev")
		require.NoError(t, initializeManualServices())
		require.Equal(t, "us-east-1", lambdaCtx.Config.Region)
		require.Equal(t, "tbl", lambdaCtx.Config.DynamoTableName)
		require.Equal(t, "subs", lambdaCtx.Config.SubscriptionsTable)
		require.Equal(t, "https://ws.example.com/dev", lambdaCtx.Config.WebSocketEndpoint)
	})

	t.Run("api id fallback builds endpoint", func(t *testing.T) {
		lambdaCtx = &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &appconfig.Config{Region: "us-east-1"},
		}
		t.Setenv("WEBSOCKET_API_URL", "")
		t.Setenv("WEBSOCKET_ENDPOINT", "")
		t.Setenv("WEBSOCKET_API_ID", "abc123")
		t.Setenv("WEBSOCKET_STAGE", "")
		require.NoError(t, initializeManualServices())
		require.Equal(t, "https://abc123.execute-api.us-east-1.amazonaws.com/development", lambdaCtx.Config.WebSocketEndpoint)
	})

	t.Run("domain fallback builds ws endpoint", func(t *testing.T) {
		lambdaCtx = &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &appconfig.Config{Region: "us-east-1", Domain: "https://example.com"},
		}
		t.Setenv("WEBSOCKET_API_ID", "")
		t.Setenv("WEBSOCKET_STAGE", "")
		t.Setenv("WEBSOCKET_API_URL", "")
		t.Setenv("WEBSOCKET_ENDPOINT", "")
		require.NoError(t, initializeManualServices())
		require.Equal(t, "https://ws.example.com", lambdaCtx.Config.WebSocketEndpoint)
	})
}

func TestNewStreamRouterHandler_Round12(t *testing.T) {
	origLambda := lambdaCtx
	origNewStreamer := newStreamerClient
	t.Cleanup(func() {
		lambdaCtx = origLambda
		newStreamerClient = origNewStreamer
	})

	newStreamerClient = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
		return &fakeStreamerClient{}, nil
	}

	lambdaCtx = &common.LambdaContext{
		Logger:      zap.NewNop(),
		Config:      &appconfig.Config{DynamoTableName: "tbl", WebSocketEndpoint: "https://ws.example.com"},
		AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		DynamoDB:    fakeDB{},
	}

	h, err := NewStreamRouterHandler()
	require.NoError(t, err)
	require.NotNil(t, h)
	require.Equal(t, "tbl", h.tableName)
	require.Equal(t, "localhost", h.domain) // default when DOMAIN_NAME not set
}
