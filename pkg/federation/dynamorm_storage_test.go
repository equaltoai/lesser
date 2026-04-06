package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamormtesting "github.com/theory-cloud/tabletheory/pkg/testing"
	"go.uber.org/zap/zaptest"
)

func TestNormalizeRemoteActorHandle(t *testing.T) {
	assert.Equal(t, "alice@example.com", models.NormalizeRemoteActorHandle("https://example.com/users/alice"))
	assert.Equal(t, "alice@example.com", models.NormalizeRemoteActorHandle("http://example.com/users/alice"))
	assert.Equal(t, "alice@example.com", models.NormalizeRemoteActorHandle("https://example.com/@alice"))
	assert.Equal(t, "", models.NormalizeRemoteActorHandle("https://example.com/users/"))
	assert.Equal(t, "", models.NormalizeRemoteActorHandle("example.com/users/alice"))
	assert.Equal(t, "", models.NormalizeRemoteActorHandle(""))
}

func TestNewDynamORMFederationStorage_Smoke(t *testing.T) {
	testDB := dynamormtesting.NewTestDB()
	logger := zaptest.NewLogger(t)

	svc := NewDynamORMFederationStorage(testDB.MockDB, "table", logger)
	require.NotNil(t, svc)
	require.NotNil(t, svc.db)
	require.NotNil(t, svc.actorRepository)
	require.NotNil(t, svc.federationActivityRepository)
	require.NotNil(t, svc.relationshipRepository)
}

type dynamormFederationStorageActorRepoStub struct {
	privateKey string
	privateErr error

	actor    *activitypub.Actor
	actorErr error

	getPrivateKeyUsernames []string
	getActorUsernames      []string
}

func (s *dynamormFederationStorageActorRepoStub) GetActorPrivateKey(_ context.Context, username string) (string, error) {
	s.getPrivateKeyUsernames = append(s.getPrivateKeyUsernames, username)
	return s.privateKey, s.privateErr
}

func (s *dynamormFederationStorageActorRepoStub) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	s.getActorUsernames = append(s.getActorUsernames, username)
	return s.actor, s.actorErr
}

type dynamormFederationStorageRelationshipRepoStub struct {
	followers []string
	cursor    string
	err       error

	getFollowersCalls []struct {
		username string
		limit    int
		cursor   string
	}
}

func (s *dynamormFederationStorageRelationshipRepoStub) GetFollowers(_ context.Context, username string, limit int, cursor string) ([]string, string, error) {
	s.getFollowersCalls = append(s.getFollowersCalls, struct {
		username string
		limit    int
		cursor   string
	}{username: username, limit: limit, cursor: cursor})
	return s.followers, s.cursor, s.err
}

type dynamormFederationStorageActivityRepoStub struct {
	created []*models.FederationActivity
	err     error
}

func (s *dynamormFederationStorageActivityRepoStub) Create(_ context.Context, activity *models.FederationActivity) error {
	s.created = append(s.created, activity)
	return s.err
}

func TestDynamORMFederationStorage_DelegatesToRepositories(t *testing.T) {
	ctx := context.Background()

	actorRepo := &dynamormFederationStorageActorRepoStub{
		privateKey: "pem",
		actor:      &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
	}
	relationshipRepo := &dynamormFederationStorageRelationshipRepoStub{
		followers: []string{"bob", "carol"},
		cursor:    "next",
	}

	svc := &DynamORMFederationStorage{
		actorRepository:        actorRepo,
		relationshipRepository: relationshipRepo,
	}

	key, err := svc.GetActorPrivateKey(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, "pem", key)
	require.Equal(t, []string{"alice"}, actorRepo.getPrivateKeyUsernames)

	actor, err := svc.GetActor(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, actor)
	assert.Equal(t, "https://example.com/users/alice", actor.ID)
	require.Equal(t, []string{"alice"}, actorRepo.getActorUsernames)

	followers, cursor, err := svc.GetFollowers(ctx, "alice", 10, "cur")
	require.NoError(t, err)
	assert.Equal(t, []string{"bob", "carol"}, followers)
	assert.Equal(t, "next", cursor)
	require.Len(t, relationshipRepo.getFollowersCalls, 1)
	assert.Equal(t, "alice", relationshipRepo.getFollowersCalls[0].username)
	assert.Equal(t, 10, relationshipRepo.getFollowersCalls[0].limit)
	assert.Equal(t, "cur", relationshipRepo.getFollowersCalls[0].cursor)
}

func TestDynamORMFederationStorage_GetCachedRemoteActor(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid_handle_returns_not_found", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		svc := &DynamORMFederationStorage{db: testDB.MockDB}

		actor, err := svc.GetCachedRemoteActor(ctx, "https://remote.example/users/")
		require.ErrorIs(t, err, storage.ErrNotFound)
		assert.Nil(t, actor)
		testDB.MockDB.AssertNotCalled(t, "Model", mock.Anything)
	})

	t.Run("not_found_returns_storage_not_found", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE").
			ExpectNotFound()
		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#@bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE").
			ExpectNotFound()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		actor, err := svc.GetCachedRemoteActor(ctx, "https://remote.example/users/bob")
		require.ErrorIs(t, err, storage.ErrNotFound)
		assert.Nil(t, actor)
		testDB.AssertExpectations(t)
	})

	t.Run("query_error_returns_joined_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE")

		boom := errors.New("boom")
		testDB.MockQuery.On("First", mock.Anything).Return(boom).Once()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		actor, err := svc.GetCachedRemoteActor(ctx, "https://remote.example/users/bob")
		require.Error(t, err)
		assert.Nil(t, actor)
		assert.ErrorIs(t, err, ErrRemoteActorCacheRetrieveFailed)
		assert.ErrorIs(t, err, boom)
		testDB.AssertExpectations(t)
	})

	t.Run("expired_cache_returns_storage_not_found", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		stored := &models.RemoteActor{
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/users/bob",
					Type: activitypub.PersonType,
				},
				PreferredUsername: "bob",
				Inbox:             "https://remote.example/users/bob/inbox",
			},
			Handle:    "bob@remote.example",
			ExpiresAt: time.Now().Add(-time.Second),
		}

		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE")
		testDB.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.RemoteActor)
			*dest = *stored
		}).Return(nil).Once()
		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#@bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE").
			ExpectNotFound()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		actor, err := svc.GetCachedRemoteActor(ctx, "https://remote.example/users/bob")
		require.ErrorIs(t, err, storage.ErrNotFound)
		assert.Nil(t, actor)
		testDB.AssertExpectations(t)
	})

	t.Run("invalid_cached_actor_returns_storage_not_found", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		stored := &models.RemoteActor{
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/users/bob",
					Type: activitypub.PersonType,
				},
			},
			Handle:    "bob@remote.example",
			ExpiresAt: time.Now().Add(5 * time.Minute),
		}

		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE")
		testDB.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.RemoteActor)
			*dest = *stored
		}).Return(nil).Once()
		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#@bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE").
			ExpectNotFound()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		actor, err := svc.GetCachedRemoteActor(ctx, "https://remote.example/users/bob")
		require.ErrorIs(t, err, storage.ErrNotFound)
		assert.Nil(t, actor)
		testDB.AssertExpectations(t)
	})

	t.Run("success_returns_cached_actor", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		expected := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://remote.example/users/bob",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
		}
		stored := &models.RemoteActor{
			Actor:     expected,
			Handle:    "bob@remote.example",
			ExpiresAt: time.Now().Add(5 * time.Minute),
		}

		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE")
		testDB.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.RemoteActor)
			*dest = *stored
		}).Return(nil).Once()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		actor, err := svc.GetCachedRemoteActor(ctx, "https://remote.example/users/bob")
		require.NoError(t, err)
		assert.Equal(t, expected, actor)
		testDB.AssertExpectations(t)
	})
}

func TestDynamORMFederationStorage_CacheRemoteActor(t *testing.T) {
	ctx := context.Background()
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.example/users/bob/inbox",
	}

	t.Run("create_success", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectCreate()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		require.NoError(t, svc.CacheRemoteActor(ctx, "bob@remote.example", actor, time.Minute))
		testDB.AssertExpectations(t)
	})

	t.Run("create_condition_failed_then_update_success", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectCreateError(dynamormerrors.ErrConditionFailed)
		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE").
			ExpectUpdate()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		require.NoError(t, svc.CacheRemoteActor(ctx, "bob@remote.example", actor, time.Minute))
		testDB.AssertExpectations(t)
	})

	t.Run("create_condition_failed_then_update_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectCreateError(dynamormerrors.ErrConditionFailed)
		testDB.ExpectWhere("PK", "=", "REMOTE_ACTOR#bob@remote.example").
			ExpectWhere("SK", "=", "PROFILE")

		boom := errors.New("boom")
		testDB.MockQuery.On("Update", mock.Anything).Return(boom).Once()

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		err := svc.CacheRemoteActor(ctx, "bob@remote.example", actor, time.Minute)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRemoteActorCacheUpdateFailed)
		assert.ErrorIs(t, err, boom)
		testDB.AssertExpectations(t)
	})

	t.Run("create_error_returns_joined_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		boom := errors.New("boom")
		testDB.ExpectCreateError(boom)

		svc := &DynamORMFederationStorage{db: testDB.MockDB}
		err := svc.CacheRemoteActor(ctx, "bob@remote.example", actor, time.Minute)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRemoteActorCacheStoreFailed)
		assert.ErrorIs(t, err, boom)
		testDB.AssertExpectations(t)
	})
}

func TestDynamORMFederationStorage_RecordFederationActivity(t *testing.T) {
	ctx := context.Background()

	repo := &dynamormFederationStorageActivityRepoStub{}
	svc := &DynamORMFederationStorage{
		federationActivityRepository: repo,
	}

	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	require.NoError(t, svc.RecordFederationActivity(ctx, &storage.FederationActivity{
		Type:         "egress",
		Domain:       "remote.example",
		ActivityType: "Create",
		Success:      true,
		ResponseTime: 123,
		ByteSize:     42,
		ErrorMessage: "nope",
		Timestamp:    ts,
	}))
	require.Len(t, repo.created, 1)
	assert.Equal(t, int64(42), repo.created[0].OutboundSize)
	assert.Equal(t, int64(0), repo.created[0].InboundSize)

	require.NoError(t, svc.RecordFederationActivity(ctx, &storage.FederationActivity{
		Type:         "ingress",
		Domain:       "remote.example",
		ActivityType: "Create",
		Success:      false,
		ResponseTime: 7,
		ByteSize:     99,
		Timestamp:    ts,
	}))
	require.Len(t, repo.created, 2)
	assert.Equal(t, int64(0), repo.created[1].OutboundSize)
	assert.Equal(t, int64(99), repo.created[1].InboundSize)
}
