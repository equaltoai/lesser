package main

import (
	"context"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	lesstesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorymocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type stubRepositoryStorage struct {
	*lesstesting.MockRepositoryStorage
	listRepo *repositories.ListRepository
}

func (s *stubRepositoryStorage) List() *repositories.ListRepository {
	return s.listRepo
}

func TestAuthorizeListStream_Round34(t *testing.T) {
	ctx := context.Background()
	origRepos := repos
	t.Cleanup(func() { repos = origRepos })

	t.Run("repos nil", func(t *testing.T) {
		repos = nil
		err := authorizeListStream(ctx, "list-1", "alice")
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeInternal))
	})

	t.Run("list repo nil", func(t *testing.T) {
		repos = &stubRepositoryStorage{MockRepositoryStorage: lesstesting.NewMockRepositoryStorage()}
		err := authorizeListStream(ctx, "list-1", "alice")
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeInternal))
	})

	t.Run("not found when list belongs to another user", func(t *testing.T) {
		mockDB := new(theorymocks.MockDB)
		mockQuery := new(theorymocks.MockQuery)

		listRepo := repositories.NewListRepository(mockDB, "test-table", zap.NewNop(), nil)
		repos = &stubRepositoryStorage{
			MockRepositoryStorage: lesstesting.NewMockRepositoryStorage(),
			listRepo:              listRepo,
		}

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "LIST#list-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.List)
			dest.ID = "list-1"
			dest.Username = "bob"
		}).Return(nil).Once()

		err := authorizeListStream(ctx, "list-1", "alice")
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))
	})

	t.Run("allows owner", func(t *testing.T) {
		mockDB := new(theorymocks.MockDB)
		mockQuery := new(theorymocks.MockQuery)

		listRepo := repositories.NewListRepository(mockDB, "test-table", zap.NewNop(), nil)
		repos = &stubRepositoryStorage{
			MockRepositoryStorage: lesstesting.NewMockRepositoryStorage(),
			listRepo:              listRepo,
		}

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "LIST#list-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.List)
			dest.ID = "list-1"
			dest.Username = "alice"
		}).Return(nil).Once()

		require.NoError(t, authorizeListStream(ctx, "list-1", "alice"))
	})
}
