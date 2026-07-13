package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_SearchRepositoryCoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	_, _ = repo.SearchActors(ctx, "alice", 2, 0, false, "")
	_, _ = repo.SearchActors(ctx, "alice", 2, 0, true, "user-1")

	_, _ = repo.GetAccountSuggestions(ctx, "user-1", 5)
	_, _ = repo.GetTrendingActors(ctx, 3)

	_, _ = repo.SearchByWebfinger(ctx, "alice@example.com")

	require.NoError(t, repo.CacheRemoteActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://remote.example/users/bob",
		},
		PreferredUsername: "bob",
		Name:              "Bob",
	}))

	_ = repo.UpdateLastSeen(ctx, "user-1")

	_, _ = repo.GetActiveUsers(ctx, baseTime.Add(-1*time.Hour), 2)
	_, _ = repo.GetInactiveUsers(ctx, baseTime.Add(-24*time.Hour), 2)

	_ = repo.getFollowingUsernames(ctx, "user-1")
	_ = extractDomainFromActorID("https://mastodon.social/users/alice")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_SearchActors_InvalidOffset(t *testing.T) {
	ctx := context.Background()
	repo := NewAccountRepository(nil, "test-table", "example.com", zaptest.NewLogger(t))

	actors, err := repo.SearchActors(ctx, "alice", 10, 10001, false, "")
	require.Error(t, err)
	require.Nil(t, actors)
}

func TestAccountRepository_SearchByWebfinger_InvalidFormat(t *testing.T) {
	ctx := context.Background()
	repo := NewAccountRepository(nil, "test-table", "example.com", zaptest.NewLogger(t))

	actor, err := repo.SearchByWebfinger(ctx, "not-a-webfinger")
	require.Error(t, err)
	require.Nil(t, actor)
}

func TestAccountRepository_CacheRemoteActor_ConditionFailedUpdatesExisting(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Override Create/Update to hit the conditional update path deterministically.
	mockQuery.On("Create").Return(errors.ErrConditionFailed).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	err := repo.CacheRemoteActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://remote.example/users/alice",
		},
		PreferredUsername: "alice",
		Name:              "Alice",
	})
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_SearchByWebfinger_RemoteNotFound(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	actor, err := repo.SearchByWebfinger(ctx, "alice@remote.example")
	require.Error(t, err)
	require.Nil(t, actor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_SearchByWebfinger_LocalDomainCanonicalizesMixedCaseUsername(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userCoreProjection)
		return ok
	})).Return(errors.ErrItemNotFound).Once()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userCoreProjection)
		return ok
	})).Run(func(args mock.Arguments) {
		user := args.Get(0).(*userCoreProjection)
		user.Table = "test-table"
		user.PK = "USER#Medic"
		user.SK = models.SKMetadata
		user.Username = "Medic"
		user.Role = "user"
		user.Approved = true
		user.Version = 1
		user.CreatedAt = baseTime
		user.UpdatedAt = baseTime
	}).Return(nil).Once()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.Actor)
		return ok
	})).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Username = "Medic"
		actor.CreatedAt = baseTime
		actor.UpdatedAt = baseTime
		actor.Actor = &activitypub.Actor{
			PreferredUsername: "Medic",
			Name:              "Medic",
			BaseObject: activitypub.BaseObject{
				ID: "https://example.com/users/Medic",
			},
		}
		_ = actor.UpdateKeys()
	}).Return(nil).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	actor, err := repo.SearchByWebfinger(ctx, "medic@example.com")
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, "medic", actor.PreferredUsername)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_SearchByWebfinger_ManagedLesserSoulAliasIsLocal(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userCoreProjection)
		return ok
	})).Return(errors.ErrItemNotFound).Once()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userCoreProjection)
		return ok
	})).Run(func(args mock.Arguments) {
		user := args.Get(0).(*userCoreProjection)
		user.Table = "test-table"
		user.PK = "USER#Agent-0"
		user.SK = models.SKMetadata
		user.Username = "Agent-0"
		user.Role = "user"
		user.Approved = true
		user.Version = 1
		user.CreatedAt = baseTime
		user.UpdatedAt = baseTime
	}).Return(nil).Once()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.Actor)
		return ok
	})).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Username = "Agent-0"
		actor.CreatedAt = baseTime
		actor.UpdatedAt = baseTime
		actor.Actor = &activitypub.Actor{
			PreferredUsername: "Agent-0",
			Name:              "Agent-0",
			BaseObject: activitypub.BaseObject{
				ID: "https://example.com/users/Agent-0",
			},
		}
		_ = actor.UpdateKeys()
	}).Return(nil).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	actor, err := repo.SearchByWebfinger(ctx, "agent-0@lessersoul.ai")
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, "agent-0", actor.PreferredUsername)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_SearchAllActors_FilteringAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	t.Run("filters_offset_and_limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*models.Actor)
			return ok
		})).Return(errors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*userCoreProjection)
			return ok
		})).Return(errors.ErrItemNotFound).Once()

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Actor)
			*dest = []models.Actor{
				{Username: "alice", Actor: &activitypub.Actor{PreferredUsername: "alice", Name: "Alice"}},
				{Username: "bob", Actor: &activitypub.Actor{PreferredUsername: "bob", Name: "Bobby"}},
				{Username: "carol", Actor: nil},
			}
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		actors := repo.searchAllActors(ctx, "bo", 1, 0)
		require.Len(t, actors, 1)
		require.Equal(t, "bob", actors[0].PreferredUsername)
	})

	t.Run("db_error_returns_partial", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Actor)
			dest.Username = "alice"
			dest.Actor = &activitypub.Actor{PreferredUsername: "alice", Name: "Alice"}
		}).Return(nil).Once()

		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		actors := repo.searchAllActors(ctx, "alice", 10, 0)
		require.NotEmpty(t, actors)
	})
}

func TestAccountRepository_SearchFollowedActors_Branches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Follow)
		*dest = []models.Follow{
			{FollowedUsername: "bob", State: models.FollowStateAccepted},
			{FollowedUsername: "carol", State: models.FollowStateAccepted},
			{FollowedUsername: "dave", State: models.FollowStatePending},
		}
	}).Return(nil).Once()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Actor)
		dest.Username = "bob"
		dest.Actor = &activitypub.Actor{PreferredUsername: "bob", Name: "Bobby"}
	}).Return(nil).Once()
	mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	actors := repo.searchFollowedActors(ctx, "alice", "bo", 10, 0)
	require.Len(t, actors, 1)
	require.Equal(t, "bob", actors[0].PreferredUsername)
}

func TestAccountRepository_FriendOfFriendSuggestions_MutualsAndEmptyFollowing(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	t.Run("empty_following_returns_empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		suggestions := repo.getFriendOfFriendSuggestions(ctx, "alice", map[string]bool{}, 10)
		require.Empty(t, suggestions)
	})

	t.Run("mutual_followers_threshold_and_actor_lookup", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Follow)
			*dest = []models.Follow{
				{FollowedUsername: "bob", State: models.FollowStateAccepted},
				{FollowedUsername: "carol", State: models.FollowStateAccepted},
			}
		}).Return(nil).Once()

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Follow)
			*dest = []models.Follow{
				{FollowedUsername: "dave", State: models.FollowStateAccepted},
				{FollowedUsername: "erin", State: models.FollowStateAccepted},
			}
		}).Return(nil).Once()

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Follow)
			*dest = []models.Follow{
				{FollowedUsername: "dave", State: models.FollowStateAccepted},
			}
		}).Return(nil).Once()

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Actor)
			dest.Username = "dave"
			dest.Actor = &activitypub.Actor{PreferredUsername: "dave", Name: "Dave"}
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		suggestions := repo.getFriendOfFriendSuggestions(ctx, "alice", map[string]bool{}, 10)
		require.Len(t, suggestions, 1)
		require.Equal(t, "dave", suggestions[0].Actor.PreferredUsername)
	})
}

func TestAccountRepository_CacheRemoteActor_ConditionFailedThenLookupFails(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(errors.ErrConditionFailed).Once()
	mockQuery.On("First", mock.Anything).Return(fmt.Errorf("lookup failed")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	err := repo.CacheRemoteActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://remote.example/users/alice",
		},
		PreferredUsername: "alice",
		Name:              "Alice",
	})
	require.Error(t, err)
}

func TestAccountRepository_SearchByWebfinger_RemoteQueryError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	actor, err := repo.SearchByWebfinger(ctx, "alice@remote.example")
	require.Error(t, err)
	require.Nil(t, actor)
}

func TestAccountRepository_SearchRepo_QueryErrors(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	_, err := repo.GetTrendingActors(ctx, 1)
	require.Error(t, err)
	_, err = repo.GetActiveUsers(ctx, baseTime, 1)
	require.Error(t, err)
	_, err = repo.GetInactiveUsers(ctx, baseTime, 1)
	require.Error(t, err)

	require.Equal(t, "", extractDomainFromActorID("not-a-url"))
}
