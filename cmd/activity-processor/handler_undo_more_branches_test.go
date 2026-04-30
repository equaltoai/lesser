package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityHandler_ProcessUndoUpdate_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("missing object id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.processUndoUpdate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: ""}, "ignored")
		require.ErrorIs(t, err, services.ErrExtractObjectIDFromUpdate)
	})

	t.Run("missing actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.processUndoUpdate(ctx, &activitypub.Activity{Actor: ""}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.ErrorIs(t, err, services.ErrUndoUpdateMissingActor)
	})

	t.Run("history retrieval fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory(nil), errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoUpdate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("history empty", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoUpdate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("previous state missing", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{Version: 2},
		}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoUpdate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("update fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{Version: 2, PreviousState: map[string]any{"id": "https://example.com/objects/1"}},
		}, nil).Once()
		objectRepo.On("UpdateObject", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoUpdate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("success", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{Version: 2, PreviousState: map[string]any{"id": "https://example.com/objects/1"}},
		}, nil).Once()
		objectRepo.On("UpdateObject", mock.Anything, mock.Anything).Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoUpdate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.NoError(t, err)
		objectRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_ProcessUndoDelete_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("missing object id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: ""}, "ignored")
		require.ErrorIs(t, err, services.ErrExtractObjectIDFromDelete)
	})

	t.Run("missing actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: ""}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.ErrorIs(t, err, services.ErrUndoDeleteMissingActor)
	})

	t.Run("tombstone status check fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(false, errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("object not deleted", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(false, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("tombstone retrieval fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(true, nil).Once()
		objectRepo.On("GetTombstone", mock.Anything, "https://example.com/objects/1").Return((*models.Tombstone)(nil), errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("history retrieval fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(true, nil).Once()
		objectRepo.On("GetTombstone", mock.Anything, "https://example.com/objects/1").Return(&models.Tombstone{ID: "https://example.com/objects/1"}, nil).Once()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory(nil), errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("previous state missing", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(true, nil).Once()
		objectRepo.On("GetTombstone", mock.Anything, "https://example.com/objects/1").Return(&models.Tombstone{ID: "https://example.com/objects/1"}, nil).Once()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{Version: 3},
		}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("restoration fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(true, nil).Once()
		objectRepo.On("GetTombstone", mock.Anything, "https://example.com/objects/1").Return(&models.Tombstone{ID: "https://example.com/objects/1"}, nil).Once()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{Version: 3, PreviousState: map[string]any{"id": "https://example.com/objects/1"}},
		}, nil).Once()
		objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		objectRepo.AssertExpectations(t)
	})

	t.Run("success", func(t *testing.T) {
		now := time.Now()

		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("IsTombstoned", mock.Anything, "https://example.com/objects/1").Return(true, nil).Once()
		objectRepo.On("GetTombstone", mock.Anything, "https://example.com/objects/1").Return(&models.Tombstone{
			ID:         "https://example.com/objects/1",
			FormerType: "Note",
			Deleted:    now,
		}, nil).Once()
		objectRepo.On("GetObjectHistory", mock.Anything, "https://example.com/objects/1").Return([]*storage.UpdateHistory{
			{Version: 3, PreviousState: map[string]any{"id": "https://example.com/objects/1"}},
		}, nil).Once()
		objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		err := h.processUndoDelete(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.NoError(t, err)
		objectRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_ProcessUndoReject_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("invalid reject activity type", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.processUndoReject(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, 123, "alice")
		require.ErrorIs(t, err, services.ErrUndoInvalidObjectType)
	})

	t.Run("follow lookup fails without mutating relationship", func(t *testing.T) {
		activityRepo := testmocks.NewMockActivityRepository()
		activityRepo.On("GetActivity", mock.Anything, "follow-1").Return((*activitypub.Activity)(nil), errors.New("not found")).Once()

		h := &ActivityHandler{
			Logger:       zap.NewNop(),
			ActivityRepo: activityRepo,
		}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-1"}, Actor: "https://remote.example/users/bob"}
		reject := map[string]any{
			"type":   ActivityTypeReject,
			"actor":  "https://remote.example/users/bob",
			"object": "follow-1",
		}
		require.Error(t, h.processUndoReject(ctx, undo, reject, "alice"))

		activityRepo.AssertExpectations(t)
	})

	t.Run("undo actor does not match reject actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}

		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-actor-mismatch", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
		}
		reject := map[string]any{
			"type":  ActivityTypeReject,
			"actor": "https://example.com/users/bob",
			"object": map[string]any{
				"type":   ActivityTypeFollow,
				"actor":  "https://remote.example/users/carol",
				"id":     "follow-1",
				"object": "https://example.com/users/bob",
			},
		}

		err := h.processUndoReject(ctx, undo, reject, "alice")
		require.ErrorIs(t, err, services.ErrActorNotAuthorizedUndo)
	})

	t.Run("missing follower after fallbacks errors", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		h := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-2"}, Actor: "https://example.com/users/alice"}
		reject := map[string]any{
			"actor":  "https://remote.example/users/bob",
			"object": 123,
		}
		require.Error(t, h.processUndoReject(ctx, undo, reject, ""))
	})
}

func TestActivityHandler_UndoRejectHelperBranches(t *testing.T) {
	ctx := context.Background()
	h := &ActivityHandler{Logger: zap.NewNop()}

	t.Run("extract rejects non reject activity", func(t *testing.T) {
		_, _, err := h.extractRejectStateForUndoReject(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: ActivityTypeFollow},
		})
		require.ErrorIs(t, err, services.ErrExtractActivityTypeFromUndo)
	})

	t.Run("extracts valid activity reject state", func(t *testing.T) {
		actor, follow, err := h.extractRejectStateForUndoReject(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: ActivityTypeReject},
			Actor:      "https://example.com/users/alice",
			Object:     "follow-1",
		})
		require.NoError(t, err)
		require.Equal(t, "https://example.com/users/alice", actor)
		require.Equal(t, "follow-1", follow)
	})

	t.Run("extract rejects non reject map", func(t *testing.T) {
		_, _, err := h.extractRejectStateForUndoReject(map[string]any{
			"type": ActivityTypeFollow,
		})
		require.ErrorIs(t, err, services.ErrExtractActivityTypeFromUndo)
	})

	t.Run("resolves stored follow state", func(t *testing.T) {
		activityRepo := testmocks.NewMockActivityRepository()
		activityRepo.On("GetActivity", mock.Anything, "follow-success").Return(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "follow-success",
				Type: ActivityTypeFollow,
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/users/alice",
		}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo}
		state, err := h.resolveUndoRejectFollowState(ctx, "follow-success")
		require.NoError(t, err)
		require.Equal(t, undoRejectFollowState{
			ID:     "follow-success",
			Actor:  "https://remote.example/users/bob",
			Target: "https://example.com/users/alice",
		}, state)
		activityRepo.AssertExpectations(t)
	})

	t.Run("resolves embedded follow map with object id", func(t *testing.T) {
		activityRepo := testmocks.NewMockActivityRepository()
		activityRepo.On("GetActivity", mock.Anything, "follow-embedded").Return(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "follow-embedded", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/users/alice",
		}, nil).Once()

		withActivityRepo := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo}
		state, err := withActivityRepo.resolveUndoRejectFollowState(ctx, map[string]any{
			"id":    "follow-embedded",
			"type":  ActivityTypeFollow,
			"actor": "https://remote.example/users/bob",
			"object": map[string]any{
				"id": "https://example.com/users/alice",
			},
		})
		require.NoError(t, err)
		require.Equal(t, "https://example.com/users/alice", state.Target)
		activityRepo.AssertExpectations(t)
	})

	t.Run("empty follow id string is rejected", func(t *testing.T) {
		_, err := h.resolveUndoRejectFollowState(ctx, "")
		require.ErrorIs(t, err, services.ErrExtractUsernamesFromReject)
	})

	t.Run("nil or wrong activity type is rejected", func(t *testing.T) {
		_, err := h.followStateFromActivity(nil)
		require.ErrorIs(t, err, services.ErrExtractActivityTypeFromUndo)

		_, err = h.followStateFromActivity(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: ActivityTypeAccept},
		})
		require.ErrorIs(t, err, services.ErrExtractActivityTypeFromUndo)
	})

	t.Run("missing required follow fields are rejected", func(t *testing.T) {
		_, err := h.followStateFromMap(map[string]any{
			"id":     "reject-wrong-type",
			"type":   ActivityTypeReject,
			"actor":  "https://remote.example/users/bob",
			"object": "https://example.com/users/alice",
		})
		require.ErrorIs(t, err, services.ErrExtractActivityTypeFromUndo)

		_, err = h.followStateFromMap(map[string]any{
			"id":     "follow-missing-target",
			"type":   ActivityTypeFollow,
			"actor":  "https://remote.example/users/bob",
			"object": map[string]any{},
		})
		require.ErrorIs(t, err, services.ErrExtractUsernamesFromReject)

		_, err = validateUndoRejectFollowState(undoRejectFollowState{
			ID:     "follow-missing-actor",
			Target: "https://example.com/users/alice",
		})
		require.ErrorIs(t, err, services.ErrExtractUsernamesFromReject)
	})
}

func TestActivityHandler_ProcessUndoFlag_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("missing object id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.processUndoFlag(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: ""}, "ignored")
		require.ErrorIs(t, err, services.ErrExtractFlaggedObjectIDFromFlag)
	})

	t.Run("missing actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.processUndoFlag(ctx, &activitypub.Activity{Actor: ""}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.ErrorIs(t, err, services.ErrUndoFlagMissingActor)
	})

	t.Run("flags retrieval fails", func(t *testing.T) {
		moderationRepo := testmocks.NewMockModerationRepository()
		moderationRepo.On("GetFlagsByObject", mock.Anything, "https://example.com/objects/1", 50, "").Return([]*storage.Flag(nil), "", errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ModerationRepo: moderationRepo}
		err := h.processUndoFlag(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		moderationRepo.AssertExpectations(t)
	})

	t.Run("no pending flag found is not error", func(t *testing.T) {
		moderationRepo := testmocks.NewMockModerationRepository()
		moderationRepo.On("GetFlagsByObject", mock.Anything, "https://example.com/objects/1", 50, "").Return([]*storage.Flag{
			{ID: "flag-1", Actor: "https://example.com/users/alice", Status: "resolved"},
		}, "", nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ModerationRepo: moderationRepo}
		err := h.processUndoFlag(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.NoError(t, err)
		moderationRepo.AssertExpectations(t)
	})

	t.Run("delete flag fails", func(t *testing.T) {
		moderationRepo := testmocks.NewMockModerationRepository()
		moderationRepo.On("GetFlagsByObject", mock.Anything, "https://example.com/objects/1", 50, "").Return([]*storage.Flag{
			{ID: "flag-1", Actor: "https://example.com/users/alice", Status: FlagStatusPending},
		}, "", nil).Once()
		moderationRepo.On("DeleteFlag", mock.Anything, "flag-1").Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ModerationRepo: moderationRepo}
		err := h.processUndoFlag(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)
		moderationRepo.AssertExpectations(t)
	})

	t.Run("moderation event error is swallowed", func(t *testing.T) {
		moderationRepo := testmocks.NewMockModerationRepository()
		moderationRepo.On("GetFlagsByObject", mock.Anything, "https://example.com/objects/1", 50, "").Return([]*storage.Flag{
			{ID: "flag-1", Actor: "https://example.com/users/alice", Status: FlagStatusPending},
		}, "", nil).Once()
		moderationRepo.On("DeleteFlag", mock.Anything, "flag-1").Return(nil).Once()
		moderationRepo.On("CreateModerationEvent", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ModerationRepo: moderationRepo}
		err := h.processUndoFlag(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-1"}, Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.NoError(t, err)
		moderationRepo.AssertExpectations(t)
	})
}
