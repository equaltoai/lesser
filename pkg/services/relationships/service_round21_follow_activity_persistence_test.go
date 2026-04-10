package relationships

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func buildRemoteFollowPersistenceService(
	t *testing.T,
	activityRepo interfaces.ActivityRepository,
) (*Service, *inmemory.RelationshipRepository, *activitypub.Actor, *activitypub.Actor, *MockFederationService) {
	t.Helper()

	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)
	actorRepo := inmemory.NewActorRepository()
	relationshipRepo := inmemory.NewRelationshipRepository()
	storageHarness.actorRepo = actorRepo
	storageHarness.activityRepo = activityRepo
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

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.social/users/bob/inbox",
		Outbox:            "https://remote.social/users/bob/outbox",
	}
	actorRepo.SetCachedRemoteActor("bob@remote.social", remoteActor, time.Hour)

	fed := &MockFederationService{}
	fed.On("QueueActivity", mock.Anything, mock.Anything).Return(nil).Maybe()
	service.federation = fed

	return service, relationshipRepo, localActor, remoteActor, fed
}

func TestService_Follow_PersistsOutboundLocalActivityBeforeQueueing(t *testing.T) {
	ctx := context.Background()
	activityRepo := inmemory.NewActivityRepository()
	service, relationshipRepo, localActor, remoteActor, fed := buildRemoteFollowPersistenceService(t, activityRepo)

	result, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Activity)

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, result.Activity.ID, record.ActivityID)

	persisted, err := activityRepo.GetActivity(ctx, result.Activity.ID)
	require.NoError(t, err)
	require.Equal(t, activitypub.FollowType, persisted.Type)
	require.Equal(t, result.Activity.ID, persisted.ID)
	require.Equal(t, localActor.ID, persisted.Actor)
	require.Equal(t, remoteActor.ID, persisted.Object)

	require.Len(t, fed.Calls, 1)
	queued, ok := fed.Calls[0].Arguments.Get(1).(*activitypub.Activity)
	require.True(t, ok)
	require.Equal(t, persisted.ID, queued.ID)
}

func TestService_Follow_PersistenceErrorStopsBeforeFederationQueueing(t *testing.T) {
	ctx := context.Background()
	activityRepo := testmocks.NewMockActivityRepository()
	service, _, _, _, fed := buildRemoteFollowPersistenceService(t, activityRepo)

	activityRepo.
		On("CreateActivity", ctx, mock.MatchedBy(func(activity *activitypub.Activity) bool {
			return activity != nil &&
				activity.Type == activitypub.FollowType &&
				strings.HasPrefix(activity.ID, "https://example.com/activities/")
		})).
		Return(errors.New("persist boom")).
		Once()

	result, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "persist boom")
	require.Empty(t, fed.Calls)
}
