package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityHandler_HelperParsingAndFiltering(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	h := &ActivityHandler{Logger: zap.NewNop()}

	require.Equal(t, "alice", h.extractUsernameFromActorURI("https://example.com/users/alice"))
	require.Equal(t, "bob", h.extractUsernameFromActorURI("https://example.com/@bob"))
	require.Equal(t, "carol", h.extractUsernameFromActorURI("https://remote.example/users/carol/"))

	recipients := []string{
		"https://www.w3.org/ns/activitystreams#Public",
		"as:Public",
		"Public",
		"https://example.com/users/alice",
		"",
		"https://remote.example/users/bob",
	}
	require.Equal(t, []string{"https://remote.example/users/bob"}, h.filterRemoteRecipients(recipients))

	require.Equal(t, "list-123", h.extractListIDFromCollection("https://example.com/lists/list-123/accounts"))
	require.Equal(t, "collection-9", h.extractListIDFromCollection("https://example.com/collections/collection-9"))
}

func TestActivityHandler_ProcessFollowAcceptCreateLikeAnnounceDeleteBlockFlagMoveListAndUndo(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
	objectRepo := testmocks.NewMockObjectRepository()
	likeRepo := testmocks.NewMockLikeRepository()
	socialRepo := testmocks.NewMockSocialRepository()
	moderationRepo := testmocks.NewMockModerationRepository()
	actorRepo := testmocks.NewMockActorRepository()
	listRepo := testmocks.NewMockListRepository()
	activityRepo := testmocks.NewMockActivityRepository()
	notificationRepo := testmocks.NewMockNotificationRepository()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	h := &ActivityHandler{
		DB:               mockDB,
		TableName:        "test-table",
		Logger:           zap.NewNop(),
		ActivityRepo:     activityRepo,
		ObjectRepo:       objectRepo,
		ActorRepo:        actorRepo,
		TimelineRepo:     timelineRepo,
		RelationshipRepo: relationshipRepo,
		LikeRepo:         likeRepo,
		SocialRepo:       socialRepo,
		ModerationRepo:   moderationRepo,
		ListRepo:         listRepo,
		NotificationRepo: notificationRepo,
	}

	// Follow
	relationshipRepo.On("CreateRelationship", mock.Anything, "bob", "alice", "follow-1").Return(nil)
	notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()

	follow := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "follow-1", Type: ActivityTypeFollow},
		Actor:      "https://remote.example/users/bob",
		Object: map[string]any{
			"id": "https://example.com/users/alice",
		},
	}
	require.NoError(t, h.processFollowActivity(ctx, follow, "alice"))

	// Accept resolves the referenced persisted Follow before mutating relationship state.
	activityRepo.On("GetActivity", mock.Anything, "follow-1").Return(follow, nil).Once()
	relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
		State:      models.RelationshipPending,
		ActivityID: "follow-1",
	}, nil).Once()
	relationshipRepo.On("AcceptFollowRequest", mock.Anything, "bob", "alice").Return(nil).Once()
	notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "accept-1", Type: ActivityTypeAccept},
		Actor:      "https://example.com/users/alice",
		Object:     "follow-1",
	}
	require.NoError(t, h.processAcceptActivity(ctx, accept, "bob"))

	// Create (public + local timeline entries)
	var publicEntryCount int
	timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		entries := args.Get(1).([]*models.Timeline)
		publicEntryCount = len(entries)
	}).Return(nil).Once()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Once()

	create := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "create-1",
			Type: ActivityTypeCreate,
			To:   []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
		Actor: "https://example.com/users/alice",
		Object: map[string]any{
			"id":           "https://example.com/objects/1",
			"type":         ObjectTypeNote,
			"content":      "<p>hello</p>",
			"attributedTo": "https://example.com/users/alice",
		},
	}
	require.NoError(t, h.processCreateActivity(ctx, create, "alice"))
	require.Equal(t, 2, publicEntryCount)

	// Create (private path distributes to followers)
	relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob", "carol"}, "", nil)
	var privateFollowerEntries int
	timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		entries := args.Get(1).([]*models.Timeline)
		privateFollowerEntries = len(entries)
	}).Return(nil).Once()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Once()

	createPrivate := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "create-2",
			Type: ActivityTypeCreate,
			To:   []string{"https://example.com/users/alice/followers"},
		},
		Actor: "https://example.com/users/alice",
		Object: map[string]any{
			"id":           "https://example.com/objects/2",
			"type":         ObjectTypeNote,
			"content":      "<p>private</p>",
			"attributedTo": "https://example.com/users/alice",
		},
	}
	require.NoError(t, h.processCreateActivity(ctx, createPrivate, "alice"))
	require.Equal(t, 2, privateFollowerEntries)

	// Like (creates record + notification)
	likeRepo.On("CreateLike", mock.Anything, "https://remote.example/users/bob", "https://example.com/objects/1", "alice").Return(&models.Like{}, nil)
	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{AttributedTo: "https://example.com/users/alice"}, nil).Once()
	notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

	like := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "like-1", Type: ActivityTypeLike},
		Actor:      "https://remote.example/users/bob",
		Object:     "https://example.com/objects/1",
	}
	require.NoError(t, h.processLikeActivity(ctx, like, "alice"))

	// Announce (creates record + notification)
	socialRepo.On("CreateAnnounce", mock.Anything, mock.Anything).Return(nil)
	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{AttributedTo: "https://example.com/users/alice"}, nil).Once()
	notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

	announce := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "announce-1", Type: ActivityTypeAnnounce},
		Actor:      "https://remote.example/users/bob",
		Object:     map[string]any{"id": "https://example.com/objects/1"},
	}
	require.NoError(t, h.processAnnounceActivity(ctx, announce, "alice"))

	// Delete (creates tombstone + cascade)
	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{
		ID:           "https://example.com/objects/1",
		Type:         ObjectTypeNote,
		AttributedTo: "https://remote.example/users/bob",
	}, nil)
	timelineRepo.On("RemoveFromTimelines", mock.Anything, "https://example.com/objects/1").Return(nil)

	deleteActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "delete-1", Type: ActivityTypeDelete},
		Actor:      "https://remote.example/users/bob",
		Object:     "https://example.com/objects/1",
	}
	require.NoError(t, h.processDeleteActivity(ctx, deleteActivity, "bob"))

	// Block (creates block + attempts to remove follow relationships)
	relationshipRepo.On("CreateBlock", mock.Anything, "https://example.com/users/alice", "https://remote.example/users/bob", "block-1").Return(nil)
	relationshipRepo.On("DeleteRelationship", mock.Anything, "alice", "bob").Return(nil)
	relationshipRepo.On("DeleteRelationship", mock.Anything, "bob", "alice").Return(nil)

	block := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "block-1", Type: ActivityTypeBlock},
		Actor:      "https://example.com/users/alice",
		Object:     "https://remote.example/users/bob",
	}
	require.NoError(t, h.processBlockActivity(ctx, block, "alice"))

	// Flag (creates flags + moderation event)
	moderationRepo.On("CreateFlag", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		flag := args.Get(1).(*storage.Flag)
		if len(flag.Object) > 0 {
			flag.ID = "flag-" + flag.Object[0]
		}
	}).Return(nil)
	moderationRepo.On("CreateModerationEvent", mock.Anything, mock.Anything).Return(nil)

	flag := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "flag-activity-1", Type: ActivityTypeFlag, Summary: "spam"},
		Actor:      "https://remote.example/users/bob",
		Object:     []any{"https://example.com/objects/1", "https://example.com/objects/2"},
	}
	require.NoError(t, h.processFlagActivity(ctx, flag, "alice"))

	// Move (updates actor migration fields)
	actorRepo.On("UpdateMovedTo", mock.Anything, "old", "https://example.com/users/new").Return(nil)
	actorRepo.On("GetActorMigrationInfo", mock.Anything, "new").Return(&interfaces.MigrationInfo{AlsoKnownAs: []string{}}, nil)
	actorRepo.On("UpdateAlsoKnownAs", mock.Anything, "new", mock.MatchedBy(func(aka []string) bool {
		for _, v := range aka {
			if v == "https://example.com/users/old" {
				return true
			}
		}
		return false
	})).Return(nil)

	move := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "move-1", Type: ActivityTypeMove},
		Actor:      "https://example.com/users/old",
		Target:     "https://example.com/users/new",
	}
	require.NoError(t, h.processMoveActivity(ctx, move, "old"))

	// List Add/Remove (verifies ownership and mutates membership)
	listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "alice"}, nil)
	listRepo.On("AddListMember", mock.Anything, "list-123", "bob").Return(nil)
	listRepo.On("AddListMember", mock.Anything, "list-123", "carol").Return(nil)
	listRepo.On("RemoveListMember", mock.Anything, "list-123", "bob").Return(nil)
	listRepo.On("RemoveListMember", mock.Anything, "list-123", "carol").Return(nil)

	add := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "add-1", Type: ActivityTypeAdd},
		Actor:      "https://example.com/users/alice",
		Target:     "https://example.com/lists/list-123/accounts",
		Object: []any{
			"https://example.com/users/bob",
			map[string]any{"id": "https://example.com/users/carol"},
		},
	}
	require.NoError(t, h.processAddActivity(ctx, add, "alice"))
	require.NoError(t, h.processRemoveActivity(ctx, add, "alice"))

	// Undo Like (map path)
	likeRepo.On("DeleteLike", mock.Anything, "https://remote.example/users/bob", "https://example.com/objects/1").Return(nil)
	undoLike := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "undo-1", Type: ActivityTypeUndo},
		Actor:      "https://remote.example/users/bob",
		Object: map[string]any{
			"type":   ActivityTypeLike,
			"actor":  "https://remote.example/users/bob",
			"object": "https://example.com/objects/1",
		},
	}
	require.NoError(t, h.processUndoActivity(ctx, undoLike, "bob"))

	// Undo Like (string path -> fetches activity)
	activityRepo.On("GetActivity", mock.Anything, "like-activity-id").Return(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "like-activity-id", Type: ActivityTypeLike},
		Actor:      "https://remote.example/users/bob",
		Object:     "https://example.com/objects/1",
	}, nil)
	likeRepo.On("DeleteLike", mock.Anything, "https://remote.example/users/bob", "https://example.com/objects/1").Return(nil)

	undoLikeFetch := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "undo-2", Type: ActivityTypeUndo},
		Actor:      "https://remote.example/users/bob",
		Object:     "like-activity-id",
	}
	require.NoError(t, h.processUndoActivity(ctx, undoLikeFetch, "bob"))

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	relationshipRepo.AssertExpectations(t)
	timelineRepo.AssertExpectations(t)
	objectRepo.AssertExpectations(t)
	likeRepo.AssertExpectations(t)
	socialRepo.AssertExpectations(t)
	moderationRepo.AssertExpectations(t)
	actorRepo.AssertExpectations(t)
	listRepo.AssertExpectations(t)
	activityRepo.AssertExpectations(t)
	notificationRepo.AssertExpectations(t)
}
