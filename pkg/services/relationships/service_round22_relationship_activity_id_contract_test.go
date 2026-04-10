package relationships

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
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

type relationshipLookupErrorRepo struct {
	*inmemory.RelationshipRepository
	err error
}

func (r *relationshipLookupErrorRepo) GetRelationship(context.Context, string, string) (*models.RelationshipRecord, error) {
	return nil, r.err
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
	_, err = activityRepo.GetActivity(ctx, winnerRepo.observedActivityID)
	require.ErrorIs(t, err, storage.ErrNotFound)

	outboxActivities, nextCursor, err := activityRepo.GetOutboxActivities(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Empty(t, nextCursor)
	require.Len(t, outboxActivities, 1)
	require.Equal(t, winnerRepo.winnerActivityID, outboxActivities[0].ID)

	record, err := winnerRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, winnerRepo.winnerActivityID, record.ActivityID)
}

func TestService_ReconcileStoredFollowActivity_RebuildsWinnerWhenActivityMissing(t *testing.T) {
	ctx := context.Background()
	activityRepo := inmemory.NewActivityRepository()
	service, relationshipRepo, localActor, remoteActor, _ := buildRemoteFollowPersistenceService(t, activityRepo)

	storedWinnerID := "https://example.com/activities/winner-missing"
	require.NoError(t, relationshipRepo.CreateRelationship(ctx, "alice", "bob@remote.social", storedWinnerID))

	loserActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      "https://example.com/activities/loser-missing",
			Type:    activitypub.FollowType,
			To:      []string{remoteActor.ID},
		},
		Actor:  localActor.ID,
		Object: remoteActor.ID,
	}
	require.NoError(t, activityRepo.CreateActivity(ctx, loserActivity))

	follower := &storage.Account{User: &storage.User{Username: "alice"}, Actor: localActor}
	following := &storage.Account{User: &storage.User{Username: "bob@remote.social"}, Actor: remoteActor}

	reconciledActivity, result, err := service.reconcileStoredFollowActivity(ctx, follower, following, "alice", "bob@remote.social", loserActivity)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, reconciledActivity)
	require.Equal(t, storedWinnerID, reconciledActivity.ID)
	require.Equal(t, storedWinnerID, result.RequestID)
	require.True(t, result.Relationship.Requested)
	_, err = activityRepo.GetActivity(ctx, loserActivity.ID)
	require.ErrorIs(t, err, storage.ErrNotFound)
}

func TestService_ReconcileStoredFollowActivity_AdoptsAcceptedWinnerState(t *testing.T) {
	ctx := context.Background()
	activityRepo := inmemory.NewActivityRepository()
	service, relationshipRepo, localActor, remoteActor, _ := buildRemoteFollowPersistenceService(t, activityRepo)

	storedWinnerID := "https://example.com/activities/winner-accepted"
	require.NoError(t, relationshipRepo.CreateRelationship(ctx, "alice", "bob@remote.social", storedWinnerID))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(ctx, "alice", "bob@remote.social"))

	loserActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      "https://example.com/activities/loser-accepted",
			Type:    activitypub.FollowType,
			To:      []string{remoteActor.ID},
		},
		Actor:  localActor.ID,
		Object: remoteActor.ID,
	}
	require.NoError(t, activityRepo.CreateActivity(ctx, loserActivity))

	follower := &storage.Account{User: &storage.User{Username: "alice"}, Actor: localActor}
	following := &storage.Account{User: &storage.User{Username: "bob@remote.social"}, Actor: remoteActor}

	reconciledActivity, result, err := service.reconcileStoredFollowActivity(ctx, follower, following, "alice", "bob@remote.social", loserActivity)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, reconciledActivity)
	require.True(t, result.IsFollowing)
	require.Empty(t, result.RequestID)
	require.Equal(t, storedWinnerID, reconciledActivity.ID)
}

func TestService_DeleteSupersededFollowActivity_NoopBranches(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, "example.com")
	require.NoError(t, service.deleteSupersededFollowActivity(ctx, "loser", "winner"))

	serviceWithStorage, storageHarness := newServiceWithStorageHarness(t)
	require.NoError(t, serviceWithStorage.deleteSupersededFollowActivity(ctx, "", "winner"))
	require.NoError(t, serviceWithStorage.deleteSupersededFollowActivity(ctx, "same", "same"))
	storageHarness.activityRepo = nil
	require.NoError(t, serviceWithStorage.deleteSupersededFollowActivity(ctx, "loser", "winner"))

	unsupportedRepo := testmocks.NewMockActivityRepository()
	storageHarness.activityRepo = unsupportedRepo
	require.NoError(t, serviceWithStorage.deleteSupersededFollowActivity(ctx, "loser", "winner"))

	notFoundRepo := &deleteNotFoundActivityRepo{ActivityRepository: inmemory.NewActivityRepository()}
	storageHarness.activityRepo = notFoundRepo
	require.NoError(t, serviceWithStorage.deleteSupersededFollowActivity(ctx, "loser", "winner"))
}

type deleteNotFoundActivityRepo struct {
	interfaces.ActivityRepository
}

func (r *deleteNotFoundActivityRepo) DeleteActivity(context.Context, string) error {
	return storage.ErrNotFound
}

type deleteErrorActivityRepo struct {
	interfaces.ActivityRepository
}

func (r *deleteErrorActivityRepo) DeleteActivity(context.Context, string) error {
	return errors.New("delete boom")
}

func TestService_DeleteSupersededFollowActivity_DeleteErrorReturnsError(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)
	storageHarness.activityRepo = &deleteErrorActivityRepo{ActivityRepository: inmemory.NewActivityRepository()}

	err := service.deleteSupersededFollowActivity(ctx, "loser", "winner")
	require.ErrorContains(t, err, "delete boom")
}

func TestService_LoadStoredFollowActivity_FallbackBranches(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, "example.com")
	require.Nil(t, service.loadStoredFollowActivity(ctx, "winner"))

	serviceWithStorage, storageHarness := newServiceWithStorageHarness(t)
	storageHarness.activityRepo = nil
	require.Nil(t, serviceWithStorage.loadStoredFollowActivity(ctx, "winner"))

	lookupFailingRepo := testmocks.NewMockActivityRepository()
	lookupFailingRepo.On("GetActivity", ctx, "winner").Return(nil, errors.New("lookup boom")).Once()
	storageHarness.activityRepo = lookupFailingRepo
	require.Nil(t, serviceWithStorage.loadStoredFollowActivity(ctx, "winner"))
}

func TestService_ReconcileStoredFollowActivity_RelationshipRepoFallbackBranches(t *testing.T) {
	ctx := context.Background()
	followActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/loser",
			Type: activitypub.FollowType,
		},
	}

	serviceWithMissingRepo, missingRepoHarness := newServiceWithStorageHarness(t)
	missingRepoHarness.relationshipRepo = nil
	reconciledActivity, result, err := serviceWithMissingRepo.reconcileStoredFollowActivity(ctx, nil, nil, "alice", "bob@remote.social", followActivity)
	require.Error(t, err)
	require.Same(t, followActivity, reconciledActivity)
	require.Nil(t, result)

	lookupErr := errors.New("relationship lookup boom")
	serviceWithLookupErr, lookupErrHarness := newServiceWithStorageHarness(t)
	lookupErrHarness.relationshipRepo = &relationshipLookupErrorRepo{
		RelationshipRepository: inmemory.NewRelationshipRepository(),
		err:                    lookupErr,
	}
	reconciledActivity, result, err = serviceWithLookupErr.reconcileStoredFollowActivity(ctx, nil, nil, "alice", "bob@remote.social", followActivity)
	require.ErrorIs(t, err, lookupErr)
	require.Same(t, followActivity, reconciledActivity)
	require.Nil(t, result)
}
