package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound10_SeriesRepository_CRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewSeriesRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	series := &models.Series{AuthorID: "user-1", ID: "series-1", Title: "Series", Slug: "series"}
	require.NoError(t, repo.CreateSeries(ctx, series))

	got, err := repo.GetSeries(ctx, "user-1", "series-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	_, _, err = repo.ListSeriesByAuthorPaginated(ctx, "   ", 1, "")
	require.Error(t, err)

	items, next, err := repo.ListSeriesByAuthorPaginated(ctx, "user-1", 1, "series-1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotEmpty(t, next)
}

func TestRound10_SeriesRepository_UpdateArticleCount(t *testing.T) {
	ctx := context.Background()

	t.Run("no-op inputs", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewSeriesRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.UpdateArticleCount(ctx, "  ", "series-1", 1))
		require.NoError(t, repo.UpdateArticleCount(ctx, "user-1", "  ", 1))
		require.NoError(t, repo.UpdateArticleCount(ctx, "user-1", "series-1", 0))
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

		repo := NewSeriesRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.UpdateArticleCount(ctx, "user-1", "series-1", 1))
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

		repo := NewSeriesRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.UpdateArticleCount(ctx, "user-1", "series-1", 1))
	})

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder)
		mockUpdateBuilder.On("Execute").Return(nil)

		repo := NewSeriesRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.UpdateArticleCount(ctx, "user-1", "series-1", 1))
	})
}

func TestRound10_SeriesRepository_ListSeriesWrapperAndCursorPrefix(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewSeriesRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	items, err := repo.ListSeriesByAuthor(ctx, "user-1", 1)
	require.NoError(t, err)
	require.Len(t, items, 1)

	items, next, err := repo.ListSeriesByAuthorPaginated(ctx, "user-1", 10, "ID#series-1")
	require.NoError(t, err)
	require.NotEmpty(t, items)
	require.Empty(t, next)
}
