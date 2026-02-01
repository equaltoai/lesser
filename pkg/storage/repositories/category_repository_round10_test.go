package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_CategoryRepository_CRUDAndList(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewCategoryRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	category := &models.Category{ID: "cat-1", Name: "Cat", Slug: "cat"}
	require.NoError(t, repo.CreateCategory(ctx, category))

	got, err := repo.GetCategory(ctx, "cat-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	all, err := repo.ListCategories(ctx, nil, 2)
	require.NoError(t, err)
	require.NotEmpty(t, all)

	parentID := "parent-1"
	children, err := repo.ListCategories(ctx, &parentID, 2)
	require.NoError(t, err)
	require.NotEmpty(t, children)

	root := ""
	_, err = repo.ListCategories(ctx, &root, 2)
	require.NoError(t, err)
}

func TestRound10_CategoryRepository_UpdateArticleCount(t *testing.T) {
	ctx := context.Background()

	t.Run("no-op inputs", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewCategoryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.UpdateArticleCount(ctx, "  ", 1))
		require.NoError(t, repo.UpdateArticleCount(ctx, "cat-1", 0))
	})

	t.Run("not found is ignored", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Execute").Return(dynamormErrors.ErrItemNotFound)

		repo := NewCategoryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.UpdateArticleCount(ctx, "cat-1", 1))
	})

	t.Run("other errors are returned", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Execute").Return(errors.New("boom"))

		repo := NewCategoryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.UpdateArticleCount(ctx, "cat-1", 1))
	})
}
