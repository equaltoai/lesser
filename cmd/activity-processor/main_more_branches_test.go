package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityProcessor_ExtractContentFromMap_Branches(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	content, author := ap.extractContentFromMap("nope")
	require.Empty(t, content)
	require.Empty(t, author)

	content, author = ap.extractContentFromMap(map[string]any{
		"content":      "hello",
		"attributedTo": "https://remote.example/users/bob",
	})
	require.Equal(t, "hello", content)
	require.Equal(t, "https://remote.example/users/bob", author)

	content, author = ap.extractContentFromMap(map[string]any{"content": "only"})
	require.Equal(t, "only", content)
	require.Empty(t, author)
}

func TestActivityProcessor_ProcessRemoteObjectForAnnounce_MapBranches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	objectRepo := testmocks.NewMockObjectRepository()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil)

	ap := &ActivityProcessor{
		logger:     zap.NewNop(),
		objectRepo: objectRepo,
	}

	content, author := ap.processRemoteObjectForAnnounce(ctx, map[string]any{
		"id":           "https://remote.example/objects/1",
		"type":         "Article",
		"attributedTo": "https://remote.example/users/bob",
		"content":      "remote content",
		"to":           []any{"https://www.w3.org/ns/activitystreams#Public", 12},
		"published":    "2024-01-01T00:00:00Z",
	}, "https://remote.example/objects/1")
	require.Equal(t, "remote content", content)
	require.Equal(t, "https://remote.example/users/bob", author)

	content, author = ap.processRemoteObjectForAnnounce(ctx, map[string]any{
		"id":           "https://remote.example/objects/2",
		"type":         "Article",
		"attributedTo": "https://remote.example/users/bob",
	}, "https://remote.example/objects/2")
	require.Contains(t, content, "Boosted:")
	require.Empty(t, author)

	content, author = ap.processRemoteObjectForAnnounce(ctx, "nope", "https://remote.example/objects/3")
	require.Contains(t, content, "Boosted:")
	require.Empty(t, author)

	objectRepo.AssertExpectations(t)
}

func TestActivityProcessor_IsRetryableError_Branches(t *testing.T) {
	ap := &ActivityProcessor{}

	require.False(t, ap.isRetryableError(nil))
	require.True(t, ap.isRetryableError(errors.New("connection reset by peer")))
	require.True(t, ap.isRetryableError(errors.New("status 503")))
	require.True(t, ap.isRetryableError(errors.New("status 429")))
	require.True(t, ap.isRetryableError(errors.New("no such host")))
	require.False(t, ap.isRetryableError(errors.New("status 400")))
	require.False(t, ap.isRetryableError(errors.New("some other error")))
}

func TestActivityProcessor_ObjectExtractionHelpers_Branches(t *testing.T) {
	ap := &ActivityProcessor{}
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	require.Equal(t, "content", ap.extractObjectContent(map[string]any{"content": "content"}))
	require.Equal(t, "name", ap.extractObjectContent(map[string]any{"name": "name"}))
	require.Equal(t, "summary", ap.extractObjectContent(map[string]any{"summary": "summary"}))
	require.Equal(t, "", ap.extractObjectContent(map[string]any{}))

	require.Equal(t, now, ap.extractObjectPublishedTime(map[string]any{}, now))
	require.Equal(t, now, ap.extractObjectPublishedTime(map[string]any{"published": "not-a-time"}, now))
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), ap.extractObjectPublishedTime(map[string]any{"published": "2024-01-01T00:00:00Z"}, now))

	require.Nil(t, ap.extractAddressingField(map[string]any{}, "to"))
	require.Equal(t, []string{"a", "b"}, ap.extractAddressingField(map[string]any{"to": []any{"a", "b", 1}}, "to"))
	require.Nil(t, ap.extractAddressingField(map[string]any{"to": "not-a-slice"}, "to"))
}

func TestActivityProcessor_VisibilityContentAndHostBranches(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	require.Equal(t, VisibilityDirect, ap.determineVisibility(nil, nil))
	require.Equal(t, VisibilityPublic, ap.determineVisibility([]string{activitypub.PublicAddress}, nil))
	require.Equal(t, "unlisted", ap.determineVisibility(nil, []string{activitypub.PublicAddress}))
	require.Equal(t, VisibilityDirect, ap.determineVisibility([]string{"https://example.com/users/alice/followers"}, nil))

	fromDefaultAddressing := ap.createObjectFromContent("object-1", "<p> el mundo de la prueba </p>", &activitypub.Activity{})
	require.Equal(t, VisibilityPublic, fromDefaultAddressing.Visibility)
	require.Equal(t, "es", fromDefaultAddressing.Language)

	fromActivityAddressing := ap.createObjectFromContent("object-2", " le monde de une chose ", &activitypub.Activity{
		BaseObject: activitypub.BaseObject{CC: []string{activitypub.PublicAddress}},
	})
	require.Equal(t, "unlisted", fromActivityAddressing.Visibility)
	require.Equal(t, "fr", fromActivityAddressing.Language)

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/note-1",
			InReplyTo: "https://example.com/objects/root",
			Sensitive: true,
			Summary:   "cw",
		},
		Content:    "ciao mondo",
		Attachment: []activitypub.Attachment{{Type: "Document"}},
	}
	processed := ap.processNoteObject(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{To: []string{activitypub.PublicAddress}},
	}, note)
	require.True(t, processed.HasMedia)
	require.True(t, processed.IsReply)
	require.True(t, processed.Sensitive)
	require.Equal(t, VisibilityPublic, processed.Visibility)
	require.Equal(t, "cw", processed.SpoilerText)

	converted, err := ap.convertMapToNote(map[string]any{
		"id":           "https://remote.example/objects/converted",
		"type":         "Note",
		"content":      "converted content",
		"attributedTo": "https://remote.example/users/bob",
	})
	require.NoError(t, err)
	require.Equal(t, "converted content", converted.Content)
	_, err = ap.convertMapToNote(map[string]any{"bad": make(chan int)})
	require.Error(t, err)
	_, err = ap.convertMapToNote(map[string]any{"to": "not-an-array"})
	require.Error(t, err)

	require.Equal(t, UnknownValue, ap.extractRemoteHost(""))
	require.Equal(t, "remote.example", ap.extractRemoteHost("https://remote.example:8443/objects/1"))
	require.Equal(t, "remote.example", ap.extractRemoteHost("http://remote.example/users/alice"))
	require.Equal(t, "remote.example", ap.extractRemoteHost("remote.example/path"))
}

func TestActivityProcessor_BackoffAndHTMLBranches(t *testing.T) {
	ap := &ActivityProcessor{retryDelay: time.Second}

	oddDelay := ap.calculateBackoffDelay(1)
	require.Positive(t, oddDelay)
	require.Less(t, oddDelay, time.Second)

	evenDelay := ap.calculateBackoffDelay(2)
	require.Greater(t, evenDelay, 2*time.Second)

	cappedDelay := ap.calculateBackoffDelay(10)
	require.Greater(t, cappedDelay, 30*time.Second)
	require.Less(t, cappedDelay, 31*time.Second)

	require.Equal(t, " hello  world ", stripHTMLTags("<p>hello</p><b>world</b>"))
	require.Equal(t, "unterminated <tag", stripHTMLTags("unterminated <tag"))
}

func TestActivityProcessor_DeletionHandlerAdditionalBranches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	ap := &ActivityProcessor{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	require.NoError(t, ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-map", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object: map[string]any{
			"id": "https://example.com/objects/map",
		},
	}, "alice"))

	require.NoError(t, ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-note", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object: &activitypub.Note{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/objects/note"},
		},
	}, "alice"))

	require.NoError(t, ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-missing-map", Type: activitypub.CreateType},
		Object:     map[string]any{"name": "missing id"},
	}, "alice"))

	require.NoError(t, ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-invalid-object", Type: activitypub.CreateType},
		Object:     123,
	}, "alice"))

	require.NoError(t, ap.handleFollowActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-follow-invalid", Type: activitypub.FollowType},
		Object:     map[string]any{"id": "https://remote.example/users/bob"},
	}))

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestActivityProcessor_ConvertToNote_MarshalError(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	_, err := ap.convertToNote(map[string]any{
		"id":           "https://remote.example/objects/1",
		"type":         "Note",
		"attributedTo": "https://remote.example/users/alice",
		"content":      make(chan int),
	})
	require.Error(t, err)
}

func TestActivityProcessor_ValidateAndProcessRemoteObject_AdditionalBranches(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	// Note without content: warns but does not fail.
	obj, err := ap.validateAndProcessRemoteObject(map[string]any{
		"id":           "https://remote.example/objects/1",
		"type":         "Note",
		"attributedTo": "https://remote.example/users/bob",
	}, "https://remote.example/objects/1")
	require.NoError(t, err)
	require.IsType(t, &activitypub.Note{}, obj)

	// Note missing attributedTo should fail.
	_, err = ap.validateAndProcessRemoteObject(map[string]any{
		"id":   "https://remote.example/objects/2",
		"type": "Note",
	}, "https://remote.example/objects/2")
	require.Error(t, err)

	// Event missing startTime should fail.
	_, err = ap.validateAndProcessRemoteObject(map[string]any{
		"id":   "https://remote.example/objects/3",
		"type": "Event",
	}, "https://remote.example/objects/3")
	require.Error(t, err)

	// Event with startTime should succeed.
	obj, err = ap.validateAndProcessRemoteObject(map[string]any{
		"id":        "https://remote.example/objects/4",
		"type":      "Event",
		"startTime": "2024-01-01T00:00:00Z",
	}, "https://remote.example/objects/4")
	require.NoError(t, err)
	_, ok := obj.(map[string]any)
	require.True(t, ok)

	// Media with url should succeed.
	obj, err = ap.validateAndProcessRemoteObject(map[string]any{
		"id":   "https://remote.example/objects/5",
		"type": "Video",
		"url":  "https://remote.example/media/1.mp4",
	}, "https://remote.example/objects/5")
	require.NoError(t, err)
	_, ok = obj.(map[string]any)
	require.True(t, ok)

	// Article returns validated map.
	obj, err = ap.validateAndProcessRemoteObject(map[string]any{
		"id":           "https://remote.example/objects/6",
		"type":         "Article",
		"attributedTo": "https://remote.example/users/alice",
		"content":      "hello",
	}, "https://remote.example/objects/6")
	require.NoError(t, err)
	_, ok = obj.(map[string]any)
	require.True(t, ok)
}

func TestActivityProcessor_ProcessRecord_Remove_DeletionSwitchCases(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	ap := &ActivityProcessor{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	cases := []struct {
		name         string
		activityJSON string
	}{
		{name: "create", activityJSON: `{"id":"https://example.com/activities/act-1","type":"Create","actor":"https://example.com/users/alice","object":"https://example.com/objects/1"}`},
		{name: "announce", activityJSON: `{"id":"https://example.com/activities/act-2","type":"Announce","actor":"https://example.com/users/alice","object":"https://example.com/objects/2"}`},
		{name: "follow", activityJSON: `{"id":"https://example.com/activities/act-3","type":"Follow","actor":"https://example.com/users/alice","object":"https://remote.example/users/bob"}`},
		{name: "delete", activityJSON: `{"id":"https://example.com/activities/act-4","type":"Delete","actor":"https://example.com/users/alice","object":"https://example.com/objects/3"}`},
		{name: "default", activityJSON: `{"id":"https://example.com/activities/act-5","type":"Like","actor":"https://example.com/users/alice","object":"https://example.com/objects/4"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ap.processRecord(ctx, events.DynamoDBEventRecord{
				EventID:   "evt-" + tc.name,
				EventName: activityRemove,
				Change: events.DynamoDBStreamRecord{
					OldImage: map[string]events.DynamoDBAttributeValue{
						"PK":        events.NewStringAttribute("ACTIVITY#" + tc.name),
						"SK":        events.NewStringAttribute("SK#" + tc.name),
						"direction": events.NewStringAttribute("outbox"),
						"username":  events.NewStringAttribute("alice"),
						"type":      events.NewStringAttribute("Delete"),
						"activity":  events.NewStringAttribute(tc.activityJSON),
					},
				},
			})
			require.NoError(t, err)
		})
	}

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestActivityProcessor_ProcessRecord_Insert_Outbox_CreateAndAnnounce(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
	var createEntryCount int
	timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		createEntryCount = len(args.Get(1).([]*models.Timeline))
	}).Return(nil).Once()

	ap := &ActivityProcessor{
		db:           mockDB,
		logger:       zap.NewNop(),
		timelineRepo: timelineRepo,
		baseURL:      "https://example.com",
	}

	err := ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-create",
		EventName: activityInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#create"),
				"SK":        events.NewStringAttribute("SK#create"),
				"direction": events.NewStringAttribute("outbox"),
				"username":  events.NewStringAttribute("alice"),
				"actor_id":  events.NewStringAttribute("https://example.com/users/alice"),
				"type":      events.NewStringAttribute("Create"),
				"activity": events.NewStringAttribute(`{
					"id":"https://example.com/activities/act-create",
					"type":"Create",
					"actor":"https://example.com/users/alice",
					"object":{
						"id":"https://example.com/objects/1",
						"type":"Note",
						"content":"hello",
						"attributedTo":"https://example.com/users/alice"
					}
				}`),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, createEntryCount)

	announceTimelineRepo := testmocks.NewMockTimelineRepositoryInterface()
	var announceEntryCount int
	announceTimelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		announceEntryCount = len(args.Get(1).([]*models.Timeline))
	}).Return(nil).Once()

	objectRepo := testmocks.NewMockObjectRepository()
	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{ID: "https://example.com/objects/1", Content: "hi"}, nil).Once()

	actorRepo := testmocks.NewMockActorRepository()
	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil).Once()

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{}, "", nil).Once()

	ap.timelineRepo = announceTimelineRepo
	ap.objectRepo = objectRepo
	ap.actorRepo = actorRepo
	ap.relationshipRepo = relationshipRepo

	err = ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-announce",
		EventName: activityInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#announce"),
				"SK":        events.NewStringAttribute("SK#announce"),
				"direction": events.NewStringAttribute("outbox"),
				"username":  events.NewStringAttribute("alice"),
				"actor_id":  events.NewStringAttribute("https://example.com/users/alice"),
				"type":      events.NewStringAttribute("Announce"),
				"activity":  events.NewStringAttribute(`{"id":"https://example.com/activities/act-announce","type":"Announce","actor":"https://example.com/users/alice","to":["https://www.w3.org/ns/activitystreams#Public"],"object":"https://example.com/objects/1"}`),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 3, announceEntryCount)

	// Invalid activity JSON should fail parsing.
	ap.timelineRepo = testmocks.NewMockTimelineRepositoryInterface()
	err = ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-outbox-invalid-json",
		EventName: activityInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#bad"),
				"SK":        events.NewStringAttribute("SK#bad"),
				"direction": events.NewStringAttribute("outbox"),
				"username":  events.NewStringAttribute("alice"),
				"actor_id":  events.NewStringAttribute("https://example.com/users/alice"),
				"type":      events.NewStringAttribute("Create"),
				"activity":  events.NewStringAttribute("{"),
			},
		},
	})
	require.Error(t, err)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	timelineRepo.AssertExpectations(t)
	announceTimelineRepo.AssertExpectations(t)
	objectRepo.AssertExpectations(t)
	actorRepo.AssertExpectations(t)
	relationshipRepo.AssertExpectations(t)
}
