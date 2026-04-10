package relationships

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_ManualFollowDecisions_QueueTruthfulResponseActivities(t *testing.T) {
	ctx := context.Background()

	buildService := func(t *testing.T) (*Service, *inmemory.RelationshipRepository, *activitypub.Actor, *activitypub.Actor, *MockFederationService) {
		t.Helper()

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

		return service, relationshipRepo, localActor, remoteActor, fed
	}

	t.Run("manual accept queues Accept to the resolved remote follower", func(t *testing.T) {
		service, relationshipRepo, localActor, remoteActor, fed := buildService(t)
		require.NoError(t, relationshipRepo.CreateRelationship(ctx, "zoe@remote.social", "alice", "follow-remote-accept"))

		_, err := service.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{
			RequesterID: "alice",
			FollowerID:  remoteActor.ID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, fed.Calls)

		activity, ok := fed.Calls[len(fed.Calls)-1].Arguments.Get(1).(*activitypub.Activity)
		require.True(t, ok)
		require.Equal(t, activitypub.AcceptType, activity.Type)
		require.Equal(t, localActor.ID, activity.Actor)
		require.Equal(t, []string{remoteActor.ID}, activity.To)
		require.Equal(t, "follow-remote-accept", activity.Object)
	})

	t.Run("manual reject queues Reject to the resolved remote follower", func(t *testing.T) {
		service, relationshipRepo, localActor, remoteActor, fed := buildService(t)
		require.NoError(t, relationshipRepo.CreateRelationship(ctx, "zoe@remote.social", "alice", "follow-remote-reject"))

		_, err := service.RejectFollowRequest(ctx, &RejectFollowRequestCommand{
			RequesterID: "alice",
			FollowerID:  "zoe@remote.social",
		})
		require.NoError(t, err)
		require.NotEmpty(t, fed.Calls)

		activity, ok := fed.Calls[len(fed.Calls)-1].Arguments.Get(1).(*activitypub.Activity)
		require.True(t, ok)
		require.Equal(t, activitypub.RejectType, activity.Type)
		require.Equal(t, localActor.ID, activity.Actor)
		require.Equal(t, []string{remoteActor.ID}, activity.To)
		require.Equal(t, "follow-remote-reject", activity.Object)
	})

	t.Run("local manual accept does not queue outbound federation", func(t *testing.T) {
		service, relationshipRepo, _, _, fed := buildService(t)
		require.NoError(t, relationshipRepo.CreateRelationship(ctx, "bob", "alice", "follow-local-accept"))

		_, err := service.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{
			RequesterID: "alice",
			FollowerID:  "bob",
		})
		require.NoError(t, err)
		require.Empty(t, fed.Calls)
	})
}
