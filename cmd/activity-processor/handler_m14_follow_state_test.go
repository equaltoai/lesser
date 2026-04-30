package main

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityHandler_M14_InlineFollowResponsesRequirePersistedPendingState(t *testing.T) {
	ctx := context.Background()

	t.Run("accept rejects forged inline follow id", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
			State:      models.RelationshipPending,
			ActivityID: "stored-follow",
		}, nil).Once()

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}
		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "accept-forged-inline", Type: ActivityTypeAccept},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"id":     "forged-follow",
				"type":   ActivityTypeFollow,
				"actor":  "https://remote.example/users/bob",
				"object": "https://example.com/users/alice",
			},
		}

		require.ErrorIs(t, handler.processAcceptActivity(ctx, accept, "ignored"), services.ErrActorNotAuthorizedUndo)
		relationshipRepo.AssertNotCalled(t, "AcceptFollowRequest", mock.Anything, "bob", "alice")
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("reject rejects forged inline follow id", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
			State:      models.RelationshipPending,
			ActivityID: "stored-follow",
		}, nil).Once()

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}
		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "reject-forged-inline", Type: ActivityTypeReject},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"id":     "forged-follow",
				"type":   ActivityTypeFollow,
				"actor":  "https://remote.example/users/bob",
				"object": "https://example.com/users/alice",
			},
		}

		require.ErrorIs(t, handler.processRejectActivity(ctx, reject, "ignored"), services.ErrActorNotAuthorizedUndo)
		relationshipRepo.AssertNotCalled(t, "RejectFollowRequest", mock.Anything, "bob", "alice")
		relationshipRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_M14_UndoRejectRequiresPersistedRejectedState(t *testing.T) {
	ctx := context.Background()
	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
		State:      models.RelationshipPending,
		ActivityID: "follow-1",
	}, nil).Once()

	handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}
	reject := map[string]any{
		"type":  ActivityTypeReject,
		"actor": "https://example.com/users/alice",
		"object": map[string]any{
			"id":     "follow-1",
			"type":   ActivityTypeFollow,
			"actor":  "https://remote.example/users/bob",
			"object": "https://example.com/users/alice",
		},
	}
	undo := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "undo-reject-pending", Type: ActivityTypeUndo},
		Actor:      "https://example.com/users/alice",
	}

	require.ErrorIs(t, handler.processUndoReject(ctx, undo, reject, "alice"), services.ErrActorNotAuthorizedUndo)
	relationshipRepo.AssertNotCalled(t, "CreateRelationship", mock.Anything, "bob", "alice", "follow-1")
	relationshipRepo.AssertExpectations(t)
}
