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
		activityRepo := testmocks.NewMockActivityRepository()
		activityRepo.On("GetActivity", mock.Anything, "forged-follow").Return(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "forged-follow", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/users/alice",
		}, nil).Once()
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
			State:      models.RelationshipPending,
			ActivityID: "stored-follow",
		}, nil).Once()

		handler := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo, RelationshipRepo: relationshipRepo}
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
		activityRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("reject rejects forged inline follow id", func(t *testing.T) {
		activityRepo := testmocks.NewMockActivityRepository()
		activityRepo.On("GetActivity", mock.Anything, "forged-follow").Return(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "forged-follow", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/users/alice",
		}, nil).Once()
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
			State:      models.RelationshipPending,
			ActivityID: "stored-follow",
		}, nil).Once()

		handler := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo, RelationshipRepo: relationshipRepo}
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
		activityRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("accept rejects inline follow actor and target forged around exact persisted id", func(t *testing.T) {
		followID := "https://remote.example/activities/follow-1"
		activityRepo := testmocks.NewMockActivityRepository()
		activityRepo.On("GetActivity", mock.Anything, followID).Return(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: followID, Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/users/alice",
		}, nil).Once()
		relationshipRepo := testmocks.NewMockRelationshipRepository()

		handler := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo, RelationshipRepo: relationshipRepo}
		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "accept-forged-target", Type: ActivityTypeAccept},
			Actor:      "https://evil.example/users/alice",
			Object: map[string]any{
				"id":     followID,
				"type":   ActivityTypeFollow,
				"actor":  "https://remote.example/users/bob",
				"object": "https://evil.example/users/alice",
			},
		}

		require.ErrorIs(t, handler.processAcceptActivity(ctx, accept, "ignored"), services.ErrActorNotAuthorizedUndo)
		relationshipRepo.AssertNotCalled(t, "GetRelationship", mock.Anything, "bob", "alice")
		relationshipRepo.AssertNotCalled(t, "AcceptFollowRequest", mock.Anything, "bob", "alice")
		activityRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("accept rejects inline follow id matching only the last path segment", func(t *testing.T) {
		activityRepo := testmocks.NewMockActivityRepository()
		activityRepo.On("GetActivity", mock.Anything, "https://evil.example/activities/follow-1").Return(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "https://evil.example/activities/follow-1", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/users/alice",
		}, nil).Once()
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
			State:      models.RelationshipPending,
			ActivityID: "https://remote.example/activities/follow-1",
		}, nil).Once()

		handler := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo, RelationshipRepo: relationshipRepo}
		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "accept-forged-id-segment", Type: ActivityTypeAccept},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"id":     "https://evil.example/activities/follow-1",
				"type":   ActivityTypeFollow,
				"actor":  "https://remote.example/users/bob",
				"object": "https://example.com/users/alice",
			},
		}

		require.ErrorIs(t, handler.processAcceptActivity(ctx, accept, "ignored"), services.ErrActorNotAuthorizedUndo)
		relationshipRepo.AssertNotCalled(t, "AcceptFollowRequest", mock.Anything, "bob", "alice")
		activityRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_M14_FollowResponseTrustMatchingIsStrict(t *testing.T) {
	handler := &ActivityHandler{Logger: zap.NewNop()}

	require.True(t, handler.followTargetMatchesActor("https://example.com/users/alice/", "https://example.com/users/alice"))
	require.False(t, handler.followTargetMatchesActor("https://evil.example/users/alice", "https://example.com/users/alice"))

	require.True(t, followActivityIDMatches("https://remote.example/activities/follow-1/", "https://remote.example/activities/follow-1"))
	require.False(t, followActivityIDMatches("https://remote.example/activities/follow-1", "https://evil.example/activities/follow-1"))
}

func TestActivityHandler_M14_UndoRejectRequiresPersistedRejectedState(t *testing.T) {
	ctx := context.Background()
	activityRepo := testmocks.NewMockActivityRepository()
	activityRepo.On("GetActivity", mock.Anything, "follow-1").Return(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "follow-1", Type: ActivityTypeFollow},
		Actor:      "https://remote.example/users/bob",
		Object:     "https://example.com/users/alice",
	}, nil).Once()
	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
		State:      models.RelationshipPending,
		ActivityID: "follow-1",
	}, nil).Once()

	handler := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo, RelationshipRepo: relationshipRepo}
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
	activityRepo.AssertExpectations(t)
	relationshipRepo.AssertExpectations(t)
}

func TestActivityHandler_M14_UndoRejectRequiresExactFollowTarget(t *testing.T) {
	ctx := context.Background()
	followID := "https://remote.example/activities/follow-1"
	activityRepo := testmocks.NewMockActivityRepository()
	activityRepo.On("GetActivity", mock.Anything, followID).Return(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: followID, Type: ActivityTypeFollow},
		Actor:      "https://remote.example/users/bob",
		Object:     "https://example.com/users/alice",
	}, nil).Once()
	relationshipRepo := testmocks.NewMockRelationshipRepository()

	handler := &ActivityHandler{Logger: zap.NewNop(), ActivityRepo: activityRepo, RelationshipRepo: relationshipRepo}
	reject := map[string]any{
		"type":  ActivityTypeReject,
		"actor": "https://evil.example/users/alice",
		"object": map[string]any{
			"id":     followID,
			"type":   ActivityTypeFollow,
			"actor":  "https://remote.example/users/bob",
			"object": "https://evil.example/users/alice",
		},
	}
	undo := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "undo-reject-forged-target", Type: ActivityTypeUndo},
		Actor:      "https://evil.example/users/alice",
	}

	require.ErrorIs(t, handler.processUndoReject(ctx, undo, reject, "alice"), services.ErrActorNotAuthorizedUndo)
	relationshipRepo.AssertNotCalled(t, "GetRelationship", mock.Anything, "bob", "alice")
	relationshipRepo.AssertNotCalled(t, "CreateRelationship", mock.Anything, "bob", "alice", followID)
	activityRepo.AssertExpectations(t)
	relationshipRepo.AssertExpectations(t)
}
