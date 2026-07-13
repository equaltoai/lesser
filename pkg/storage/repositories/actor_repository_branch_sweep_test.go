package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestActorRepository_branch_sweep_more_lines(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("DeleteActor success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Delete").Return(nil).Twice()

		repo := NewActorRepository(mockDB, "test-table", logger)
		assert.NoError(t, repo.DeleteActor(ctx, "alice"))
	})

	t.Run("getUserFollowing error path", func(t *testing.T) {
		deps := &stubActorRepoDeps{getFollowingErr: errors.New("following failed")}
		repo := NewActorRepository(nil, "test-table", logger)
		repo.SetDependencies(deps)

		_, err := repo.getUserFollowing(ctx, "alice", logger)
		assert.Error(t, err)
	})

	t.Run("UpdateMovedTo notfound and actor nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		assert.Error(t, repo.UpdateMovedTo(ctx, "missing", "x"))

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Actor = nil
		}).Once()
		assert.Error(t, repo.UpdateMovedTo(ctx, "alice", "x"))

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Actor = &activitypub.Actor{}
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		assert.NoError(t, repo.UpdateMovedTo(ctx, "alice", "https://remote/users/alice"))
	})

	t.Run("UpdateAlsoKnownAs success and update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		// Update error
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Actor = &activitypub.Actor{}
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
		assert.Error(t, repo.UpdateAlsoKnownAs(ctx, "alice", []string{"a"}))

		// Success
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Actor = &activitypub.Actor{}
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		assert.NoError(t, repo.UpdateAlsoKnownAs(ctx, "alice", []string{"a"}))
	})

	t.Run("CheckAlsoKnownAs query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Select", mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		mockQuery.On("First", mock.Anything).Return(errors.New("query failed")).Once()
		_, err := repo.CheckAlsoKnownAs(ctx, "alice", "x")
		assert.Error(t, err)
	})

	t.Run("GetActorMigrationInfo notfound and query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Select", mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err := repo.GetActorMigrationInfo(ctx, "missing")
		assert.Error(t, err)

		mockQuery.On("First", mock.Anything).Return(errors.New("query failed")).Once()
		_, err = repo.GetActorMigrationInfo(ctx, "alice")
		assert.Error(t, err)
	})

	t.Run("GetActorPrivateKey notfound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Select", mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err := repo.GetActorPrivateKey(ctx, "missing")
		assert.Error(t, err)

		mockQuery.On("First", mock.Anything).Return(errors.New("db failed")).Once()
		_, err = repo.GetActorPrivateKey(ctx, "alice")
		assert.Error(t, err)
	})

	t.Run("SetActorFields notfound and update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		assert.Error(t, repo.SetActorFields(ctx, "missing", []storage.ActorField{}))

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Actor = &activitypub.Actor{}
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
		assert.Error(t, repo.SetActorFields(ctx, "alice", []storage.ActorField{{Name: "n", Value: "v"}}))

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Actor = &activitypub.Actor{}
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		assert.NoError(t, repo.SetActorFields(ctx, "alice", []storage.ActorField{{Name: "n", Value: "v"}}))
	})

	t.Run("UpdateActorLastStatusTime notfound and success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		assert.Error(t, repo.UpdateActorLastStatusTime(ctx, "missing"))

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.PK = "ACTOR#alice"
			m.SK = "PROFILE"
			m.Actor = &activitypub.Actor{}
		}).Once()
		expectActorLastStatusUpdateBuilder(mockQuery, nil)
		assert.NoError(t, repo.UpdateActorLastStatusTime(ctx, "alice"))
	})

	t.Run("UpdateActor notfound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		repo := NewActorRepository(mockDB, "test-table", logger)

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		err := repo.UpdateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/missing"}, PreferredUsername: "missing"})
		assert.Error(t, err)
	})

	t.Run("GetActorByNumericID mapping and actor success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.NumericIDMapping")).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.NumericIDMapping)
			m.Username = "alice"
		}).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Actor)
			m.Actor = &activitypub.Actor{PreferredUsername: "alice"}
		}).Once()

		repo := NewActorRepository(mockDB, "test-table", logger)
		actor, err := repo.GetActorByNumericID(ctx, "123")
		assert.NoError(t, err)
		assert.Equal(t, "alice", actor.PreferredUsername)
	})
}
