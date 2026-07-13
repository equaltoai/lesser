package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityProcessor_AnnounceFanoutAndObjectReferencePaths(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
	objectRepo := testmocks.NewMockObjectRepository()
	actorRepo := testmocks.NewMockActorRepository()
	relationshipRepo := testmocks.NewMockRelationshipRepository()

	ap := &ActivityProcessor{
		db:               mockDB,
		logger:           zap.NewNop(),
		timelineRepo:     timelineRepo,
		objectRepo:       objectRepo,
		actorRepo:        actorRepo,
		relationshipRepo: relationshipRepo,
		baseURL:          "https://example.com",
		retryAttempts:    1,
		retryDelay:       4 * time.Nanosecond,
	}

	// Announce fanout uses local content when present.
	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{Content: "boosted"}, nil)
	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil)
	relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob"}, "", nil)

	var gotEntries int
	timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		entries := args.Get(1).([]*models.Timeline)
		gotEntries = len(entries)
	}).Return(nil)

	announce := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "announce-1", Type: activitypub.AnnounceType, To: []string{activitypub.PublicAddress}},
		Actor:      "https://example.com/users/alice",
		Object:     "https://example.com/objects/1",
	}
	require.NoError(t, ap.fanOutAnnounceToTimelines(ctx, announce, "alice"))
	require.Equal(t, 4, gotEntries)

	// String object references: local object path + missing local object fallback.
	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/local").Return(&models.Object{Content: "local"}, nil)
	obj, err := ap.processStringObject(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{To: []string{"https://www.w3.org/ns/activitystreams#Public"}}}, "https://example.com/objects/local")
	require.NoError(t, err)
	require.Equal(t, "local", obj.Content)

	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/missing").Return(nil, errors.New("not found"))
	obj, err = ap.processStringObject(ctx, &activitypub.Activity{}, "https://example.com/objects/missing")
	require.NoError(t, err)
	require.Contains(t, obj.Content, "Missing local object")

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	timelineRepo.AssertExpectations(t)
	objectRepo.AssertExpectations(t)
	actorRepo.AssertExpectations(t)
	relationshipRepo.AssertExpectations(t)
}

func TestActivityProcessor_StoreRemoteObject(t *testing.T) {
	ctx := context.Background()

	objectRepo := testmocks.NewMockObjectRepository()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil)

	ap := &ActivityProcessor{
		logger:     zap.NewNop(),
		objectRepo: objectRepo,
	}

	now := time.Now()
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/objects/1",
			Type:      "Note",
			Published: &now,
			To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
		Content:      "remote",
		AttributedTo: "https://remote.example/users/bob",
	}

	ap.storeRemoteObject(ctx, note)

	objectRepo.AssertExpectations(t)
}

func TestActivityProcessor_AnnounceFanoutPreservesPrivateVisibility(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
	objectRepo := testmocks.NewMockObjectRepository()
	actorRepo := testmocks.NewMockActorRepository()
	relationshipRepo := testmocks.NewMockRelationshipRepository()

	ap := &ActivityProcessor{
		db:               mockDB,
		logger:           zap.NewNop(),
		timelineRepo:     timelineRepo,
		objectRepo:       objectRepo,
		actorRepo:        actorRepo,
		relationshipRepo: relationshipRepo,
		baseURL:          "https://example.com",
	}

	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/private").Return(&models.Object{Content: "private boost"}, nil).Once()
	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil).Once()
	relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob"}, "", nil).Once()

	var entries []*models.Timeline
	timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		entries = args.Get(1).([]*models.Timeline)
	}).Return(nil).Once()

	announce := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/private-announce",
			Type: activitypub.AnnounceType,
			To:   []string{"https://example.com/users/alice/followers"},
		},
		Actor:  "https://example.com/users/alice",
		Object: "https://example.com/objects/private",
	}

	require.NoError(t, ap.fanOutAnnounceToTimelines(ctx, announce, "alice"))
	require.Len(t, entries, 2)
	require.ElementsMatch(t, []string{"alice", "bob"}, []string{entries[0].TimelineID, entries[1].TimelineID})
	for _, entry := range entries {
		require.Equal(t, VisibilityPrivate, entry.Visibility)
		require.Equal(t, timelineHome, entry.TimelineType)
	}

	timelineRepo.AssertExpectations(t)
	objectRepo.AssertExpectations(t)
	actorRepo.AssertExpectations(t)
	relationshipRepo.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}
