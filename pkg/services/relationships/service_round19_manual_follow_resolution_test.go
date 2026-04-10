package relationships

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_ManualFollowDecisions_RemoteAwareResolution(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)

	actorRepo := inmemory.NewActorRepository()
	relationshipRepo := inmemory.NewRelationshipRepository()
	storageHarness.actorRepo = actorRepo
	storageHarness.relationshipRepo = relationshipRepo

	localActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}
	require.NoError(t, actorRepo.CreateActor(ctx, localActor, ""))

	localFollower := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://example.com/users/bob/inbox",
		Outbox:            "https://example.com/users/bob/outbox",
	}
	require.NoError(t, actorRepo.CreateActor(ctx, localFollower, ""))

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/zoe",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "zoe",
		URL:               "https://remote.social/@zoe",
		Inbox:             "https://remote.social/users/zoe/inbox",
		Outbox:            "https://remote.social/users/zoe/outbox",
	}
	actorRepo.SetCachedRemoteActor("zoe@remote.social", remoteActor, time.Hour)

	fed := &MockFederationService{}
	fed.On("QueueActivity", mock.Anything, mock.Anything).Return(nil).Maybe()
	service.federation = fed

	t.Run("remote follower actor URL accepts without local hydration", func(t *testing.T) {
		require.NoError(t, relationshipRepo.CreateRelationship(ctx, "zoe@remote.social", "alice", "follow-remote-accept"))

		result, err := service.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{
			RequesterID: "alice",
			FollowerID:  remoteActor.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		record, err := relationshipRepo.GetRelationship(ctx, "zoe@remote.social", "alice")
		require.NoError(t, err)
		require.Equal(t, models.RelationshipAccepted, record.State)
	})

	t.Run("remote follower reject preserves remote actor identity", func(t *testing.T) {
		require.NoError(t, relationshipRepo.CreateRelationship(ctx, "zoe@remote.social", "alice", "follow-remote-reject"))

		beforeCalls := len(fed.Calls)
		result, err := service.RejectFollowRequest(ctx, &RejectFollowRequestCommand{
			RequesterID: "alice",
			FollowerID:  "zoe@remote.social",
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		record, err := relationshipRepo.GetRelationship(ctx, "zoe@remote.social", "alice")
		require.NoError(t, err)
		require.Equal(t, models.RelationshipRejected, record.State)

		require.Greater(t, len(fed.Calls), beforeCalls)
		activity, ok := fed.Calls[len(fed.Calls)-1].Arguments.Get(1).(*activitypub.Activity)
		require.True(t, ok)
		require.Equal(t, activitypub.RejectType, activity.Type)

		followObject, ok := activity.Object.(*activitypub.Activity)
		require.True(t, ok)
		require.Equal(t, remoteActor.ID, followObject.Actor)
		require.Equal(t, localActor.ID, followObject.Object)
	})

	t.Run("local follower behavior remains stable", func(t *testing.T) {
		require.NoError(t, relationshipRepo.CreateRelationship(ctx, "bob", "alice", "follow-local-accept"))

		result, err := service.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{
			RequesterID: "alice",
			FollowerID:  "bob",
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		record, err := relationshipRepo.GetRelationship(ctx, "bob", "alice")
		require.NoError(t, err)
		require.Equal(t, models.RelationshipAccepted, record.State)
	})
}
