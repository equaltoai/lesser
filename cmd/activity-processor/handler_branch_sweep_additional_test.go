package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityHandler_ProcessFollowActivity_AdditionalErrorBranches(t *testing.T) {
	ctx := context.Background()

	handler := &ActivityHandler{Logger: zap.NewNop()}

	t.Run("object map missing id", func(t *testing.T) {
		follow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "follow-missing-id", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     map[string]any{},
		}

		err := handler.processFollowActivity(ctx, follow, "ignored")
		require.ErrorIs(t, err, services.ErrFollowMissingObjectID)
	})

	t.Run("object invalid type", func(t *testing.T) {
		follow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "follow-invalid-type", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     123,
		}

		err := handler.processFollowActivity(ctx, follow, "ignored")
		require.ErrorIs(t, err, services.ErrFollowInvalidObjectType)
	})

	t.Run("target user missing", func(t *testing.T) {
		follow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "follow-missing-target", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     "",
		}

		err := handler.processFollowActivity(ctx, follow, "ignored")
		require.ErrorIs(t, err, services.ErrFollowMissingTargetUser)
	})

	t.Run("follower username extraction fails", func(t *testing.T) {
		follow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "follow-missing-follower-username", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/",
			Object:     "https://example.com/users/alice",
		}

		err := handler.processFollowActivity(ctx, follow, "ignored")
		require.ErrorIs(t, err, services.ErrExtractUsernamesFromFollow)
	})

	t.Run("target username extraction fails", func(t *testing.T) {
		follow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "follow-missing-target-username", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/users/",
		}

		err := handler.processFollowActivity(ctx, follow, "ignored")
		require.ErrorIs(t, err, services.ErrExtractUsernamesFromFollow)
	})
}

func TestActivityHandler_ProcessAcceptActivity_AdditionalErrorBranches(t *testing.T) {
	ctx := context.Background()

	handler := &ActivityHandler{Logger: zap.NewNop()}

	t.Run("invalid object type", func(t *testing.T) {
		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "accept-invalid-object", Type: ActivityTypeAccept},
			Actor:      "https://example.com/users/alice",
			Object:     123,
		}

		err := handler.processAcceptActivity(ctx, accept, "ignored")
		require.ErrorIs(t, err, services.ErrAcceptInvalidObjectType)
	})

	t.Run("missing follower from context username", func(t *testing.T) {
		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "accept-missing-follower", Type: ActivityTypeAccept},
			Actor:      "https://example.com/users/alice",
			Object:     "follow-1",
		}

		err := handler.processAcceptActivity(ctx, accept, "")
		require.Error(t, err)
	})

	t.Run("accepter username extraction fails", func(t *testing.T) {
		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "accept-missing-accepter", Type: ActivityTypeAccept},
			Actor:      "https://example.com/users/",
			Object:     "follow-1",
		}

		err := handler.processAcceptActivity(ctx, accept, "bob")
		require.ErrorIs(t, err, services.ErrExtractUsernamesFromAccept)
	})
}

func TestActivityHandler_ProcessCreateActivity_NoteExtractionFailed(t *testing.T) {
	ctx := context.Background()

	handler := &ActivityHandler{Logger: zap.NewNop()}

	create := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "create-note-extraction", Type: ActivityTypeCreate},
		Actor:      "https://example.com/users/alice",
		Object:     123,
	}

	require.Error(t, handler.processCreateActivity(ctx, create, "alice"))
}

func TestActivityHandler_NoteMappingBranches(t *testing.T) {
	handler := &ActivityHandler{Logger: zap.NewNop()}

	note, err := handler.mapToNote(map[string]any{
		"id":           "https://example.com/objects/1",
		"type":         "Note",
		"content":      "hello",
		"attributedTo": "https://example.com/users/alice",
		"sensitive":    true,
		"to":           []any{activitypub.PublicAddress, 123},
		"cc":           []any{"https://example.com/users/alice/followers"},
		"bto":          []any{"https://example.com/users/bob"},
		"bcc":          []any{"https://example.com/users/carol"},
		"inReplyTo":    "https://example.com/objects/root",
		"tag": []any{
			map[string]any{"type": "Hashtag", "href": "https://example.com/tags/test", "name": "#test"},
			map[string]any{},
			"not-a-tag",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/objects/1", note.ID)
	require.Equal(t, "Note", note.Type)
	require.Equal(t, "hello", note.Content)
	require.Equal(t, "https://example.com/users/alice", note.AttributedTo)
	require.True(t, note.Sensitive)
	require.Equal(t, []string{activitypub.PublicAddress, ""}, note.To)
	require.Equal(t, []string{"https://example.com/users/alice/followers"}, note.CC)
	require.Equal(t, []string{"https://example.com/users/bob"}, note.BTo)
	require.Equal(t, []string{"https://example.com/users/carol"}, note.BCC)
	require.Equal(t, "https://example.com/objects/root", note.InReplyTo)
	require.Len(t, note.Tag, 1)
	require.Equal(t, "#test", note.Tag[0].Name)

	extracted, err := handler.extractNoteFromActivity(&activitypub.Activity{Object: map[string]any{
		"id":      "https://example.com/objects/2",
		"type":    "Note",
		"content": "from map",
	}})
	require.NoError(t, err)
	require.Equal(t, "from map", extracted.Content)
}

func TestActivityHandler_ProcessUndoActivity_AdditionalErrorBranches(t *testing.T) {
	ctx := context.Background()

	handler := &ActivityHandler{Logger: zap.NewNop()}

	t.Run("invalid undo object type", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-invalid-object", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object:     123,
		}

		err := handler.processUndoActivity(ctx, undo, "alice")
		require.ErrorIs(t, err, services.ErrUndoInvalidObjectType)
	})

	t.Run("cannot extract activity type", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-missing-type", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]any{"actor": "https://example.com/users/alice"},
		}

		err := handler.processUndoActivity(ctx, undo, "alice")
		require.ErrorIs(t, err, services.ErrExtractActivityTypeFromUndo)
	})

	t.Run("actor not authorized to undo", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-unauthorized", Type: ActivityTypeUndo},
			Actor:      "https://remote.example/users/bob",
			Object: map[string]any{
				"type":  ActivityTypeLike,
				"actor": "https://remote.example/users/alice",
			},
		}

		err := handler.processUndoActivity(ctx, undo, "ignored")
		require.ErrorIs(t, err, services.ErrActorNotAuthorizedUndo)
	})

	t.Run("unsupported undo activity type", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-unsupported-type", Type: ActivityTypeUndo},
			Actor:      "https://remote.example/users/bob",
			Object: map[string]any{
				"type":  "CustomType",
				"actor": "https://remote.example/users/bob",
			},
		}

		require.NoError(t, handler.processUndoActivity(ctx, undo, "ignored"))
	})
}

func TestActivityHandler_ProcessUndoReject_AdditionalBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("reject target as activity with invalid actor", func(t *testing.T) {
		handler := &ActivityHandler{Logger: zap.NewNop()}

		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "reject-1", Type: ActivityTypeReject},
			Actor:      "https://example.com/users/",
			Object:     "follow-1",
		}
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-1", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/",
		}

		err := handler.processUndoReject(ctx, undo, reject, "alice")
		require.ErrorIs(t, err, services.ErrExtractUsernamesFromReject)
	})

	t.Run("relationship creation fails", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
			State:      models.RelationshipRejected,
			ActivityID: "follow-1",
		}, nil).Once()
		relationshipRepo.On("CreateRelationship", mock.Anything, "bob", "alice", "follow-1").Return(errors.New("boom")).Once()

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		reject := map[string]any{
			"type":  ActivityTypeReject,
			"actor": "https://example.com/users/alice",
			"object": map[string]any{
				"type":   ActivityTypeFollow,
				"actor":  "https://remote.example/users/bob",
				"id":     "follow-1",
				"object": "https://example.com/users/alice",
			},
		}
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-relationship-create", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
		}

		require.Error(t, handler.processUndoReject(ctx, undo, reject, "alice"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("forged follow target is rejected", func(t *testing.T) {
		handler := &ActivityHandler{Logger: zap.NewNop()}
		reject := map[string]any{
			"type":  ActivityTypeReject,
			"actor": "https://example.com/users/alice",
			"object": map[string]any{
				"type":   ActivityTypeFollow,
				"actor":  "https://remote.example/users/bob",
				"id":     "follow-forged",
				"object": "https://example.com/users/mallory",
			},
		}
		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-forged", Type: ActivityTypeUndo}, Actor: "https://example.com/users/alice"}

		require.ErrorIs(t, handler.processUndoReject(ctx, undo, reject, "alice"), services.ErrActorNotAuthorizedUndo)
	})
}

func TestActivityHandler_ProcessUndoMove_ClearingMovedToFails(t *testing.T) {
	ctx := context.Background()

	actorRepo := testmocks.NewMockActorRepository()
	actorRepo.On("UpdateMovedTo", mock.Anything, "alice", "").Return(errors.New("boom")).Once()

	handler := &ActivityHandler{Logger: zap.NewNop(), ActorRepo: actorRepo}

	undo := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "undo-move-1", Type: ActivityTypeUndo},
		Actor:      "https://example.com/users/alice",
	}
	moveTarget := map[string]any{"object": "https://example.com/users/bob"}

	require.Error(t, handler.processUndoMove(ctx, undo, moveTarget, "ignored"))
	actorRepo.AssertExpectations(t)
}

func TestActivityHandler_ProcessMoveActivity_OldAccountAlreadyKnownAs(t *testing.T) {
	ctx := context.Background()

	actorRepo := testmocks.NewMockActorRepository()
	actorRepo.On("UpdateMovedTo", mock.Anything, "alice", "https://example.com/users/new").Return(nil).Once()
	actorRepo.On("GetActorMigrationInfo", mock.Anything, "new").Return(&interfaces.MigrationInfo{
		AlsoKnownAs: []string{"https://example.com/users/alice"},
	}, nil).Once()

	handler := &ActivityHandler{Logger: zap.NewNop(), ActorRepo: actorRepo}

	move := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "move-1", Type: ActivityTypeMove},
		Actor:      "https://example.com/users/alice",
		Target:     "https://example.com/users/new",
	}

	require.NoError(t, handler.processMoveActivity(ctx, move, "ignored"))
	actorRepo.AssertExpectations(t)
}
