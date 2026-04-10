package relationships

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_Follow_StoresCanonicalActivityIDOnPendingRelationshipRow(t *testing.T) {
	ctx := context.Background()
	service, relationshipRepo, _, _, _ := buildRemoteFollowPersistenceService(t, inmemory.NewActivityRepository())

	result, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.RequestID)
	require.True(t, strings.HasPrefix(result.RequestID, "https://example.com/activities/"))

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, result.RequestID, record.ActivityID)
}

func TestService_Follow_ReusesCanonicalRelationshipActivityIDOnIdempotentRetry(t *testing.T) {
	ctx := context.Background()
	service, relationshipRepo, _, _, _ := buildRemoteFollowPersistenceService(t, inmemory.NewActivityRepository())

	first, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, second)

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, first.RequestID, record.ActivityID)
	require.Equal(t, second.RequestID, record.ActivityID)
}

func TestService_Follow_ReopensRejectedRelationshipWithFreshActivityID(t *testing.T) {
	ctx := context.Background()
	service, relationshipRepo, _, _, _ := buildRemoteFollowPersistenceService(t, inmemory.NewActivityRepository())

	first, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	require.NoError(t, relationshipRepo.RejectFollowRequest(ctx, "alice", "bob@remote.social"))

	second, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.NotEmpty(t, second.RequestID)
	require.NotEqual(t, first.RequestID, second.RequestID)
	require.NotNil(t, second.Relationship)
	require.True(t, second.Relationship.Requested)
	require.False(t, second.Relationship.Following)

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, models.RelationshipPending, record.State)
	require.Equal(t, second.RequestID, record.ActivityID)
}

type raceWinningRelationshipRepository struct {
	*inmemory.RelationshipRepository
	winnerActivityID   string
	observedActivityID string
}

func (r *raceWinningRelationshipRepository) CreateRelationship(ctx context.Context, followerUsername, followingUsername, activityID string) error {
	r.observedActivityID = activityID

	if _, err := r.RelationshipRepository.GetRelationship(ctx, followerUsername, followingUsername); err == nil {
		return nil
	}

	return r.RelationshipRepository.CreateRelationship(ctx, followerUsername, followingUsername, r.winnerActivityID)
}

func buildRemoteFollowPersistenceServiceWithRelationshipRepo(
	t *testing.T,
	activityRepo interfaces.ActivityRepository,
	relationshipRepo interfaces.ConcreteRelationshipRepository,
) (*Service, *activitypub.Actor, *activitypub.Actor, *MockFederationService) {
	t.Helper()

	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)
	actorRepo := inmemory.NewActorRepository()
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

	return service, localActor, remoteActor, fed
}

func TestService_Follow_AdoptsStoredCanonicalActivityIDWhenCreateWinsRace(t *testing.T) {
	ctx := context.Background()
	activityRepo := inmemory.NewActivityRepository()
	winnerRepo := &raceWinningRelationshipRepository{
		RelationshipRepository: inmemory.NewRelationshipRepository(),
		winnerActivityID:       "https://example.com/activities/winner",
	}
	service, localActor, remoteActor, fed := buildRemoteFollowPersistenceServiceWithRelationshipRepo(t, activityRepo, winnerRepo)

	winnerActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      winnerRepo.winnerActivityID,
			Type:    activitypub.FollowType,
			To:      []string{remoteActor.ID},
		},
		Actor:  localActor.ID,
		Object: remoteActor.ID,
	}
	require.NoError(t, activityRepo.CreateActivity(ctx, winnerActivity))

	result, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Activity)
	require.Equal(t, winnerRepo.winnerActivityID, result.Activity.ID)
	require.Equal(t, winnerRepo.winnerActivityID, result.RequestID)
	require.True(t, result.Relationship.Requested)
	require.NotEqual(t, winnerRepo.winnerActivityID, winnerRepo.observedActivityID)
	require.Empty(t, fed.Calls)

	record, err := winnerRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, winnerRepo.winnerActivityID, record.ActivityID)
}
