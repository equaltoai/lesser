package main

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityHandler_UndoCoverageForMoreTypes(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("undo follow", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("DeleteRelationship", mock.Anything, "https://example.com/users/alice", "https://remote.example/users/bob").Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-follow", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeFollow,
				"actor":  "https://example.com/users/alice",
				"object": "https://remote.example/users/bob",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "alice"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("undo announce", func(t *testing.T) {
		socialRepo := testmocks.NewMockSocialRepository()
		socialRepo.On("DeleteAnnounce", mock.Anything, "https://remote.example/users/bob", "https://example.com/objects/1").Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), SocialRepo: socialRepo}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-announce", Type: ActivityTypeUndo},
			Actor:      "https://remote.example/users/bob",
			Object: map[string]any{
				"type":   ActivityTypeAnnounce,
				"actor":  "https://remote.example/users/bob",
				"object": "https://example.com/objects/1",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "bob"))
		socialRepo.AssertExpectations(t)
	})

	t.Run("undo block", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("DeleteBlock", mock.Anything, "https://example.com/users/alice", "https://remote.example/users/bob").Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-block", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeBlock,
				"actor":  "https://example.com/users/alice",
				"object": "https://remote.example/users/bob",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "alice"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("undo create", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("DeleteObject", mock.Anything, "https://example.com/objects/1").Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-create", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeCreate,
				"actor":  "https://example.com/users/alice",
				"object": "https://example.com/objects/1",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "alice"))
		objectRepo.AssertExpectations(t)
	})

	t.Run("undo update", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1", 0).Return(nil, nil).Maybe()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{ObjectID: "https://example.com/objects/1", Version: 2, PreviousState: map[string]any{"id": "https://example.com/objects/1"}},
		}, nil)
		objectRepo.On("UpdateObject", mock.Anything, mock.Anything).Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-update", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeUpdate,
				"actor":  "https://example.com/users/alice",
				"object": "https://example.com/objects/1",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "alice"))
		objectRepo.AssertExpectations(t)
	})

	t.Run("undo delete", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		now := time.Now()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(true, nil)
		objectRepo.On("GetTombstone", mock.Anything, "https://example.com/objects/1").Return(&models.Tombstone{
			ID:         "https://example.com/objects/1",
			FormerType: ObjectTypeNote,
			Deleted:    now,
		}, nil)
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{ObjectID: "https://example.com/objects/1", Version: 2, PreviousState: map[string]any{"id": "https://example.com/objects/1"}},
		}, nil)
		objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-delete", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeDelete,
				"actor":  "https://example.com/users/alice",
				"object": "https://example.com/objects/1",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "alice"))
		objectRepo.AssertExpectations(t)
	})

	t.Run("undo accept", func(t *testing.T) {
		handler := &ActivityHandler{Logger: zap.NewNop()}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-accept", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeAccept,
				"actor":  "https://example.com/users/alice",
				"object": "accept-1",
			},
		}
		require.NoError(t, handler.processUndoActivity(ctx, undo, "alice"))
	})

	t.Run("undo flag", func(t *testing.T) {
		moderationRepo := testmocks.NewMockModerationRepository()
		moderationRepo.On("GetFlagsByObject", mock.Anything, "https://example.com/objects/1", 50, "").Return([]*storage.Flag{
			{ID: "flag-1", Actor: "https://remote.example/users/bob", Status: FlagStatusPending},
		}, "", nil)
		moderationRepo.On("DeleteFlag", mock.Anything, "flag-1").Return(nil)
		moderationRepo.On("CreateModerationEvent", mock.Anything, mock.Anything).Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), ModerationRepo: moderationRepo}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-flag", Type: ActivityTypeUndo},
			Actor:      "https://remote.example/users/bob",
			Object: map[string]any{
				"type":   ActivityTypeFlag,
				"actor":  "https://remote.example/users/bob",
				"object": "https://example.com/objects/1",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "bob"))
		moderationRepo.AssertExpectations(t)
	})

	t.Run("undo move", func(t *testing.T) {
		actorRepo := testmocks.NewMockActorRepository()
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		notificationRepo := testmocks.NewMockNotificationRepository()

		actorURI := "https://example.com/users/old"
		targetURI := "https://example.com/users/new"

		actorRepo.On("UpdateMovedTo", mock.Anything, "old", "").Return(nil)
		actorRepo.On("GetActorMigrationInfo", mock.Anything, "new").Return(&interfaces.MigrationInfo{AlsoKnownAs: []string{actorURI, "other"}}, nil)
		actorRepo.On("UpdateAlsoKnownAs", mock.Anything, "new", mock.MatchedBy(func(aka []string) bool {
			return len(aka) == 1 && aka[0] == "other"
		})).Return(nil)
		relationshipRepo.On("GetFollowers", mock.Anything, "old", 1000, "").Return([]string{"bob", "carol"}, "", nil)
		notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

		handler := &ActivityHandler{
			Logger:           zap.NewNop(),
			ActorRepo:        actorRepo,
			RelationshipRepo: relationshipRepo,
			NotificationRepo: notificationRepo,
		}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-move", Type: ActivityTypeUndo},
			Actor:      actorURI,
			Object: map[string]any{
				"type":   ActivityTypeMove,
				"actor":  actorURI,
				"object": targetURI,
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "old"))
		actorRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
		notificationRepo.AssertExpectations(t)
	})

	t.Run("undo add/remove list activity", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "alice"}, nil)
		listRepo.On("RemoveListMember", mock.Anything, "list-123", "bob").Return(nil)
		listRepo.On("AddListMember", mock.Anything, "list-123", "bob").Return(nil)

		handler := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}

		undoAdd := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-add", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeAdd,
				"actor":  "https://example.com/users/alice",
				"object": "https://example.com/users/bob",
				"target": "https://example.com/lists/list-123/accounts",
			},
		}
		require.NoError(t, handler.processUndoActivity(ctx, undoAdd, "alice"))

		undoRemove := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-remove", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"type":   ActivityTypeRemove,
				"actor":  "https://example.com/users/alice",
				"object": "https://example.com/users/bob",
				"target": "https://example.com/lists/list-123/accounts",
			},
		}
		require.NoError(t, handler.processUndoActivity(ctx, undoRemove, "alice"))

		listRepo.AssertExpectations(t)
	})
}

