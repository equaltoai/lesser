package routing

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler_ProcessAcceptActivity_ReturnsErrorForMalformedStoredActivity(t *testing.T) {
	env := newFollowResponseReconciliationEnv(t)
	followID := env.cfg.BaseURL() + "/activities/follow-broken-accept"
	seedPendingLocalOutboundFollow(t, env, followID, "")

	mockActivityRepo := testmocks.NewMockActivityRepository()
	mockActivityRepo.
		On("GetActivity", mock.Anything, followID).
		Return(&activitypub.Activity{
			Actor:  env.local.ID,
			Object: env.remoteActorID,
		}, nil).
		Once()
	env.handler.activityRepository = mockActivityRepo

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-broken",
		},
		Actor:  env.remoteActorID,
		Object: followID,
	}

	err := env.handler.processAcceptActivity(context.Background(), accept, env.local)
	require.ErrorIs(t, err, storage.ErrInvalidInput)
	require.ErrorContains(t, err, "missing required routing fields")
	require.Equal(t, models.RelationshipPending, relationshipState(t, env))
	mockActivityRepo.AssertExpectations(t)
}

func TestInboxHandler_ProcessRejectActivity_ReturnsErrorForMalformedStoredActivity(t *testing.T) {
	env := newFollowResponseReconciliationEnv(t)
	followID := env.cfg.BaseURL() + "/activities/follow-broken-reject"
	seedPendingLocalOutboundFollow(t, env, followID, "")

	mockActivityRepo := testmocks.NewMockActivityRepository()
	mockActivityRepo.
		On("GetActivity", mock.Anything, followID).
		Return(&activitypub.Activity{
			Actor:  env.local.ID,
			Object: env.remoteActorID,
		}, nil).
		Once()
	env.handler.activityRepository = mockActivityRepo

	reject := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.RejectType,
			ID:      env.cfg.BaseURL() + "/activities/reject-broken",
		},
		Actor:  env.remoteActorID,
		Object: followID,
	}

	err := env.handler.processRejectActivity(context.Background(), reject, env.local)
	require.ErrorIs(t, err, storage.ErrInvalidInput)
	require.ErrorContains(t, err, "missing required routing fields")
	require.Equal(t, models.RelationshipPending, relationshipState(t, env))
	mockActivityRepo.AssertExpectations(t)
}

func TestInboxHandler_ProcessUndoActivity_ReturnsErrorForMalformedStoredActivity(t *testing.T) {
	env := newInboxTestEnv(t)
	activityID := env.cfg.BaseURL() + "/activities/undo-broken"

	mockActivityRepo := testmocks.NewMockActivityRepository()
	mockActivityRepo.
		On("GetActivity", mock.Anything, activityID).
		Return(&activitypub.Activity{
			Actor:  env.local.ID,
			Object: env.remoteActorID,
		}, nil).
		Once()
	env.handler.activityRepository = mockActivityRepo

	undo := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.UndoType,
			ID:      env.cfg.BaseURL() + "/activities/undo-request",
		},
		Actor:  env.remoteActorID,
		Object: activityID,
	}

	err := env.handler.processUndoActivity(context.Background(), undo, env.local)
	require.ErrorIs(t, err, storage.ErrInvalidInput)
	require.ErrorContains(t, err, "missing required routing fields")
	mockActivityRepo.AssertExpectations(t)
}
