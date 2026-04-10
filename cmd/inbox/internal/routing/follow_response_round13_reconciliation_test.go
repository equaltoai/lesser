package routing

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func newFollowResponseReconciliationEnv(t *testing.T) *inboxTestEnv {
	t.Helper()

	env := newInboxTestEnv(t)
	env.handler.activityRepository = inmemory.NewActivityRepository()
	env.handler.relationshipRepository = inmemory.NewRelationshipRepository()

	return env
}

func seedPendingLocalOutboundFollow(t *testing.T, env *inboxTestEnv, relationshipActivityID, storedActivityID string) {
	t.Helper()

	ctx := context.Background()
	followerHandle := env.local.PreferredUsername
	followeeHandle := env.handler.extractHandleFromActorID(env.remoteActorID)
	require.NoError(t, env.handler.relationshipRepository.CreateRelationship(ctx, followerHandle, followeeHandle, relationshipActivityID))

	if storedActivityID == "" {
		return
	}

	require.NoError(t, env.handler.activityRepository.CreateActivity(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      storedActivityID,
		},
		Actor:  env.local.ID,
		Object: env.remoteActorID,
	}))
}

func relationshipState(t *testing.T, env *inboxTestEnv) string {
	t.Helper()

	record, err := env.handler.relationshipRepository.GetRelationship(
		context.Background(),
		env.local.PreferredUsername,
		env.handler.extractHandleFromActorID(env.remoteActorID),
	)
	require.NoError(t, err)
	return record.State
}

func TestInboxHandler_ProcessAcceptActivity_ReconcilesStoredOriginalFollow(t *testing.T) {
	env := newFollowResponseReconciliationEnv(t)
	followID := env.cfg.BaseURL() + "/activities/follow-stored-accept"
	seedPendingLocalOutboundFollow(t, env, followID, followID)

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-stored",
		},
		Actor:  env.remoteActorID,
		Object: followID,
	}

	require.NoError(t, env.handler.processAcceptActivity(context.Background(), accept, env.local))
	require.Equal(t, models.RelationshipAccepted, relationshipState(t, env))
}

func TestInboxHandler_ProcessAcceptActivity_FallbackReconcilesLegacyBareActivityID(t *testing.T) {
	env := newFollowResponseReconciliationEnv(t)
	followSuffix := "follow-legacy-accept"
	seedPendingLocalOutboundFollow(t, env, followSuffix, "")

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-fallback",
		},
		Actor:  env.remoteActorID,
		Object: env.cfg.BaseURL() + "/activities/" + followSuffix,
	}

	require.NoError(t, env.handler.processAcceptActivity(context.Background(), accept, env.local))
	require.Equal(t, models.RelationshipAccepted, relationshipState(t, env))
}

func TestInboxHandler_ProcessRejectActivity_ReconcilesStoredOriginalFollow(t *testing.T) {
	env := newFollowResponseReconciliationEnv(t)
	followID := env.cfg.BaseURL() + "/activities/follow-stored-reject"
	seedPendingLocalOutboundFollow(t, env, followID, followID)

	reject := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.RejectType,
			ID:      env.cfg.BaseURL() + "/activities/reject-stored",
		},
		Actor:  env.remoteActorID,
		Object: followID,
	}

	require.NoError(t, env.handler.processRejectActivity(context.Background(), reject, env.local))
	require.Equal(t, models.RelationshipRejected, relationshipState(t, env))
}

func TestInboxHandler_ProcessRejectActivity_FallbackReconcilesLegacyBareActivityID(t *testing.T) {
	env := newFollowResponseReconciliationEnv(t)
	followSuffix := "follow-legacy-reject"
	seedPendingLocalOutboundFollow(t, env, followSuffix, "")

	reject := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.RejectType,
			ID:      env.cfg.BaseURL() + "/activities/reject-fallback",
		},
		Actor:  env.remoteActorID,
		Object: env.cfg.BaseURL() + "/activities/" + followSuffix,
	}

	require.NoError(t, env.handler.processRejectActivity(context.Background(), reject, env.local))
	require.Equal(t, models.RelationshipRejected, relationshipState(t, env))
}

func TestInboxHandler_ProcessFollowResponses_DoNotMutateOnMismatchedObjectID(t *testing.T) {
	for _, responseType := range []string{activitypub.AcceptType, activitypub.RejectType} {
		t.Run(responseType, func(t *testing.T) {
			env := newFollowResponseReconciliationEnv(t)
			expectedID := env.cfg.BaseURL() + "/activities/follow-match"
			seedPendingLocalOutboundFollow(t, env, expectedID, "")

			response := &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					Type:    responseType,
					ID:      fmt.Sprintf("%s/activities/%s-response", env.cfg.BaseURL(), responseType),
				},
				Actor:  env.remoteActorID,
				Object: env.cfg.BaseURL() + "/activities/follow-mismatch",
			}

			if responseType == activitypub.AcceptType {
				require.NoError(t, env.handler.processAcceptActivity(context.Background(), response, env.local))
			} else {
				require.NoError(t, env.handler.processRejectActivity(context.Background(), response, env.local))
			}

			require.Equal(t, models.RelationshipPending, relationshipState(t, env))
		})
	}
}
