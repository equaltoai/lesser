package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	dynamock "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityProcessor_RemoteAnnounceAndRemoteObjectReference(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	objectRepo := testmocks.NewMockObjectRepository()
	actorRepo := testmocks.NewMockActorRepository()

	ap := &ActivityProcessor{
		db:            mockDB,
		logger:        zap.NewNop(),
		objectRepo:    objectRepo,
		actorRepo:     actorRepo,
		baseURL:       "https://example.com",
		retryAttempts: 1,
		retryDelay:    4 * time.Nanosecond,
	}

	origFetch := fetchAuthorizedObjectFn
	t.Cleanup(func() { fetchAuthorizedObjectFn = origFetch })

	fetchAuthorizedObjectFn = func(_ context.Context, _ *federation.AuthorizedFetchService, objectURL string, _ *activitypub.Actor) (any, error) {
		return map[string]any{
			"id":           objectURL,
			"type":         "Note",
			"attributedTo": "https://remote.example/users/bob",
			"content":      "remote content",
		}, nil
	}

	// Remote announce content path.
	actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil)
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil)

	content, author := ap.getRemoteAnnouncedContent(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, "https://remote.example/objects/1")
	require.Equal(t, "remote content", content)
	require.Equal(t, "https://remote.example/users/bob", author)

	// Remote string object reference path.
	objectRepo.On("GetObject", mock.Anything, "https://remote.example/objects/2").Return(nil, errors.New("not found"))
	actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil)

	obj, err := ap.processStringObject(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, "https://remote.example/objects/2")
	require.NoError(t, err)
	require.True(t, obj.IsRemote)
	require.Contains(t, obj.Content, "remote")

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	objectRepo.AssertExpectations(t)
	actorRepo.AssertExpectations(t)
}

func TestActivityProcessor_DeletionHandlersAndErrorBranches(t *testing.T) {
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

	create := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object:     "https://example.com/objects/1",
	}
	require.NoError(t, ap.handleCreateActivityDeletion(ctx, create, "alice"))

	announce := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-2", Type: activitypub.AnnounceType},
		Actor:      "https://example.com/users/alice",
		Object:     "https://example.com/objects/2",
	}
	require.NoError(t, ap.handleAnnounceActivityDeletion(ctx, announce, "alice"))

	follow := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-3", Type: activitypub.FollowType},
		Actor:      "https://example.com/users/alice",
		Object:     "https://remote.example/users/bob",
	}
	require.NoError(t, ap.handleFollowActivityDeletion(ctx, follow))

	del := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-4", Type: activitypub.DeleteType},
		Actor:      "https://example.com/users/alice",
		Object:     "https://example.com/objects/3",
	}
	require.NoError(t, ap.handleDeleteActivityDeletion(ctx, del))

	// processRecord REMOVE event missing OldImage should error.
	err := ap.processRecord(ctx, events.DynamoDBEventRecord{EventName: activityRemove, EventID: "evt-old-missing"})
	require.Error(t, err)

	// Announce fanout missing object ID should error.
	err = ap.fanOutAnnounceToTimelines(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "announce-err", Type: activitypub.AnnounceType}}, "alice")
	require.Error(t, err)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

