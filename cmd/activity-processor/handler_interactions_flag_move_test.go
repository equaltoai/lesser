package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityHandler_UndoHelpers_ProcessUndoWithObjectExtraction_ProcessUndoCreate_ProcessUndoFollow(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("processUndoWithObjectExtraction missing target id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		undo := &activitypub.Activity{Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": map[string]any{}}
		err := h.processUndoWithObjectExtraction(ctx, undo, orig, "like", func(context.Context, string, string) error { return nil })
		require.Error(t, err)
	})

	t.Run("processUndoWithObjectExtraction missing actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		undo := &activitypub.Activity{Actor: ""}
		orig := &activitypub.Activity{Object: "https://example.com/objects/1"}
		err := h.processUndoWithObjectExtraction(ctx, undo, orig, "like", func(context.Context, string, string) error { return nil })
		require.Error(t, err)
	})

	t.Run("processUndoWithObjectExtraction delete fails", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		undo := &activitypub.Activity{Actor: "https://example.com/users/alice"}
		orig := &activitypub.Activity{Object: "https://example.com/objects/1"}
		err := h.processUndoWithObjectExtraction(ctx, undo, orig, "like", func(context.Context, string, string) error { return errors.New("boom") })
		require.Error(t, err)
	})

	t.Run("processUndoWithObjectExtraction success", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		undo := &activitypub.Activity{Actor: "https://example.com/users/alice"}
		orig := &activitypub.Activity{Object: map[string]any{"id": "https://example.com/objects/1"}}
		err := h.processUndoWithObjectExtraction(ctx, undo, orig, "like", func(context.Context, string, string) error { return nil })
		require.NoError(t, err)
	})

	t.Run("processUndoCreate branches", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("DeleteObject", mock.Anything, "https://example.com/objects/1").Return(errors.New("boom")).Once()
		objectRepo.On("DeleteObject", mock.Anything, "https://example.com/objects/2").Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}

		err := h.processUndoCreate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: ""}, "ignored")
		require.ErrorIs(t, err, services.ErrExtractObjectIDFromCreate)

		err = h.processUndoCreate(ctx, &activitypub.Activity{Actor: ""}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.ErrorIs(t, err, services.ErrUndoCreateMissingActor)

		err = h.processUndoCreate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/objects/1"}, "ignored")
		require.Error(t, err)

		err = h.processUndoCreate(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: map[string]any{"id": "https://example.com/objects/2"}}, "ignored")
		require.NoError(t, err)

		objectRepo.AssertExpectations(t)
	})

	t.Run("processUndoFollow branches", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("DeleteRelationship", mock.Anything, "https://example.com/users/alice", "https://example.com/users/bob").Return(errors.New("boom")).Once()
		relationshipRepo.On("DeleteRelationship", mock.Anything, "https://example.com/users/alice", "https://example.com/users/carol").Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		err := h.processUndoFollow(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: ""}, "ignored")
		require.ErrorIs(t, err, services.ErrExtractTargetActorFromFollow)

		err = h.processUndoFollow(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: "https://example.com/users/bob"}, "ignored")
		require.Error(t, err)

		err = h.processUndoFollow(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, &activitypub.Activity{Object: map[string]any{"id": "https://example.com/users/carol"}}, "ignored")
		require.NoError(t, err)

		relationshipRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_ProcessLikeAnnounceFlagMove_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("like input validation branches", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop(), LikeRepo: testmocks.NewMockLikeRepository()}

		err := h.processLikeActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "like-1", Type: ActivityTypeLike}, Actor: "a", Object: map[string]any{}}, "alice")
		require.ErrorIs(t, err, services.ErrLikeMissingObjectID)

		err = h.processLikeActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "like-2", Type: ActivityTypeLike}, Actor: "a", Object: 123}, "alice")
		require.ErrorIs(t, err, services.ErrLikeInvalidObjectType)

		err = h.processLikeActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "like-3", Type: ActivityTypeLike}, Actor: "a", Object: ""}, "alice")
		require.ErrorIs(t, err, services.ErrLikeMissingObjectID2)

		err = h.processLikeActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "like-4", Type: ActivityTypeLike}, Actor: "", Object: "https://example.com/objects/1"}, "alice")
		require.ErrorIs(t, err, services.ErrLikeMissingActor)
	})

	t.Run("announce input validation branches", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop(), SocialRepo: testmocks.NewMockSocialRepository()}

		err := h.processAnnounceActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "ann-1", Type: ActivityTypeAnnounce}, Actor: "a", Object: map[string]any{}}, "alice")
		require.ErrorIs(t, err, services.ErrAnnounceMissingObjectID)

		err = h.processAnnounceActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "ann-2", Type: ActivityTypeAnnounce}, Actor: "a", Object: 123}, "alice")
		require.ErrorIs(t, err, services.ErrAnnounceInvalidObjectType)

		err = h.processAnnounceActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "ann-3", Type: ActivityTypeAnnounce}, Actor: "a", Object: ""}, "alice")
		require.ErrorIs(t, err, services.ErrAnnounceMissingObjectID2)

		err = h.processAnnounceActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "ann-4", Type: ActivityTypeAnnounce}, Actor: "", Object: "https://example.com/objects/1"}, "alice")
		require.ErrorIs(t, err, services.ErrAnnounceMissingActor)
	})

	t.Run("flag branches", func(t *testing.T) {
		moderationRepo := testmocks.NewMockModerationRepository()
		moderationRepo.On("CreateFlag", mock.Anything, mock.Anything).Return(nil).Once()
		moderationRepo.On("CreateModerationEvent", mock.Anything, mock.Anything).Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ModerationRepo: moderationRepo}

		err := h.processFlagActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "flag-1", Type: ActivityTypeFlag}, Actor: "https://remote.example/users/bob", Object: 123}, "alice")
		require.ErrorIs(t, err, services.ErrExtractFlaggedObjectFromFlag)

		err = h.processFlagActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "flag-2", Type: ActivityTypeFlag}, Actor: "https://remote.example/users/bob", Object: []any{123}}, "alice")
		require.ErrorIs(t, err, services.ErrNoFlaggedObjectsFound)

		err = h.processFlagActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "flag-3", Type: ActivityTypeFlag, Summary: "spam"}, Actor: "https://remote.example/users/bob", Object: "https://example.com/objects/1"}, "alice")
		require.NoError(t, err)

		moderationRepo.AssertExpectations(t)
	})

	t.Run("move branches", func(t *testing.T) {
		actorRepo := testmocks.NewMockActorRepository()
		actorRepo.On("UpdateMovedTo", mock.Anything, "old", "https://example.com/users/new").Return(errors.New("boom")).Once()
		actorRepo.On("UpdateMovedTo", mock.Anything, "old", "https://example.com/users/new2").Return(nil).Once()
		actorRepo.On("GetActorMigrationInfo", mock.Anything, "new2").Return((*interfaces.MigrationInfo)(nil), errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ActorRepo: actorRepo}

		err := h.processMoveActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "move-1", Type: ActivityTypeMove}, Actor: "https://example.com/users/old", Target: ""}, "alice")
		require.ErrorIs(t, err, services.ErrMoveMustSpecifyTarget)

		err = h.processMoveActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "move-2", Type: ActivityTypeMove}, Actor: "https://example.com/users/", Target: "https://example.com/users/new"}, "alice")
		require.Error(t, err)

		err = h.processMoveActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "move-3", Type: ActivityTypeMove}, Actor: "https://example.com/users/old", Target: "https://example.com/users/new"}, "alice")
		require.Error(t, err)

		err = h.processMoveActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "move-4", Type: ActivityTypeMove}, Actor: "https://example.com/users/old", Target: "https://example.com/users/new2"}, "alice")
		require.NoError(t, err)

		actorRepo.AssertExpectations(t)
	})
}
