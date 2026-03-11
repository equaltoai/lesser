package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActorRepository_SearchAccounts_and_numericID_branches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("username search error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

		repo := NewActorRepository(mockDB, "test-table", logger)
		_, err := repo.SearchAccounts(ctx, "al", 10, false, 0)
		assert.Error(t, err)
	})

	t.Run("name fallback success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		// Username search returns no results
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Actor)
			*out = nil
		}).Once()
		// Name search returns a result
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Actor)
			*out = []models.Actor{
				{Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice-id"}, PreferredUsername: "alice"}},
			}
		}).Once()
		// SearchAccounts loads full actor records via GetActor.
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.Username = "alice"
			m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice-id"}, PreferredUsername: "alice"}
		}).Once()

		repo := NewActorRepository(mockDB, "test-table", logger)
		actors, err := repo.SearchAccounts(ctx, "al", 10, false, 0)
		assert.NoError(t, err)
		assert.Len(t, actors, 1)
		assert.Equal(t, "alice", actors[0].PreferredUsername)
	})

	t.Run("name search error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		// Username search returns no results
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Actor)
			*out = nil
		}).Once()
		// Name search errors
		mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

		repo := NewActorRepository(mockDB, "test-table", logger)
		_, err := repo.SearchAccounts(ctx, "al", 10, false, 0)
		assert.Error(t, err)
	})

	t.Run("numeric ID mapping not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Actor)
			*out = nil
		}).Once()

		repo := NewActorRepository(mockDB, "test-table", logger)
		_, err := repo.GetActorByNumericID(ctx, "123")
		assert.Error(t, err)
	})
}

func TestActorRepository_UpdateActor_seeds_version_without_failing(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	// Load existing actor with Version == 0 triggers seedBuilder path.
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Username = "alice"
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}
		m.FollowerCount = 1
		m.Version = 0
	}).Once()

	// seed builder (conditional error should be ignored)
	mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: errors.New("ConditionalCheckFailed")}).Once()
	// main update builder
	mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: nil}).Once()

	repo := NewActorRepository(mockDB, "test-table", logger)

	err := repo.UpdateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"})
	assert.NoError(t, err)
}

func TestActorRepository_CreateActor_encryptor_missing_and_domain_resolution(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	repo := NewActorRepository(nil, "test-table", logger)

	cfg := lesserconfig.Get()
	prevKey := cfg.KMSKeyID
	defer func() { cfg.KMSKeyID = prevKey }()

	cfg.KMSKeyID = ""
	err := repo.CreateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}, "k")
	assert.Error(t, err)

	// resolveActorDomain invalid URLs return empty
	assert.Equal(t, "example.com", repo.resolveActorDomain(&activitypub.Actor{URL: "http://[::1", BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}))
	assert.Equal(t, "example.com", repo.resolveActorDomain(&activitypub.Actor{URL: "https://example.com/users/alice"}))
	assert.Equal(t, "example.com", repo.resolveActorDomain(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}))

	// Validate entity errors surface from CreateActor
	assert.Error(t, repo.CreateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: ""}, PreferredUsername: ""}, "k"))
	assert.Error(t, common.ValidateActorEntity("", ""))
}

func TestActorRepository_getRecentActiveActors_dedup_and_discoverable_filter(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "u1", Actor: &activitypub.Actor{PreferredUsername: "u1", Discoverable: true}},
			{Username: "u1", Actor: &activitypub.Actor{PreferredUsername: "u1", Discoverable: true}},
			{Username: "u2", Actor: &activitypub.Actor{PreferredUsername: "u2", Discoverable: false}},
			{Username: "u3", Actor: nil},
		}
	}).Once()

	repo := NewActorRepository(mockDB, "test-table", logger)
	actors, err := repo.getRecentActiveActors(ctx, 2)
	assert.NoError(t, err)
	assert.Len(t, actors, 1)
	assert.Equal(t, "u1", actors[0].PreferredUsername)
}

func TestActorRepository_UpdateActor_seeding_warn_and_version_condition_paths(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("seed warn path continues", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Username = "alice"
			m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}
			m.FollowerCount = 1
			m.Version = 0
		}).Once()

		mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: errors.New("boom")}).Once()
		mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: nil}).Once()

		repo := NewActorRepository(mockDB, "test-table", logger)
		assert.NoError(t, repo.UpdateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}))
	})

	t.Run("version condition path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Username = "alice"
			m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}
			m.FollowerCount = 1
			m.Version = 2
		}).Once()

		mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: nil}).Once()

		repo := NewActorRepository(mockDB, "test-table", logger)
		assert.NoError(t, repo.UpdateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}))
	})
}
