package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestSearchCostRepository_ResetBudgets(t *testing.T) {
	t.Run("success resets and updates budgets", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

		ctx := context.Background()

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.SearchBudget")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.SearchBudget)
			*dest = []models.SearchBudget{
				{UserID: "user-1", UsedBudgetMicros: 100, CurrentRequests: 3},
				{UserID: "user-2", UsedBudgetMicros: 200, CurrentRequests: 7},
			}
		}).Return(nil).Once()

		mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

		require.NoError(t, repo.ResetBudgets(ctx, "daily"))
	})

	t.Run("scan error returns query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

		ctx := context.Background()

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.SearchBudget")).Return(errors.New("scan failed")).Once()

		require.Error(t, repo.ResetBudgets(ctx, "daily"))
	})
}

func TestSearchCostRepository_CalculateTotalCostMicros_CoversBedrockAndLambda(t *testing.T) {
	repo := &SearchCostRepository{}
	costData := &models.SearchCostTracking{
		DynamoReads:      1_000_000, // 25 micros
		DynamoWrites:     2_000_000, // 250 micros
		BedrockRequests:  1,
		EmbeddingTokens:  1000, // 100 micros
		LambdaDurationMs: 1000,
		LambdaMemoryMB:   1024, // 1 GB-second => 1667 micros
	}

	require.Equal(t, int64(2042), repo.calculateTotalCostMicros(costData))
}

func TestSearchCostRepository_GetPeriodDate(t *testing.T) {
	repo := &SearchCostRepository{}

	require.Regexp(t, `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`, repo.getPeriodDate("daily"))
	require.Regexp(t, `^[0-9]{4}-W[0-9]{1,2}$`, repo.getPeriodDate("weekly"))
	require.Regexp(t, `^[0-9]{4}-[0-9]{2}$`, repo.getPeriodDate("monthly"))
	require.Regexp(t, `^[0-9]{4}$`, repo.getPeriodDate("yearly"))
	require.Regexp(t, `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`, repo.getPeriodDate("nope"))
}

func TestSearchCostRepository_UpdateQueryStats_UsesUpdateAndCoversCacheHit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchQueryStats")).Run(func(args mock.Arguments) {
		stats := args.Get(0).(*models.SearchQueryStats)
		stats.QueryCount = 3
		stats.TotalResultCount = 30
		stats.MinResponseTimeMs = 1000
		stats.MaxResponseTimeMs = 1000
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	costData := &models.SearchCostTracking{
		UserID:          "user-1",
		OperationType:   "search",
		SearchType:      "full_text",
		Query:           "hello world",
		QueryLength:     11,
		ResultCount:     10,
		ResponseTimeMs:  500,
		TotalCostMicros: 123,
		Timestamp:       time.Now(),
		CacheHit:        true,
	}

	require.NoError(t, repo.updateQueryStats(ctx, costData))
	mockQuery.AssertNotCalled(t, "Create")
}

func TestSearchCostRepository_UpdateQueryStats_EmptyQueryIsNoop(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	require.NoError(t, repo.updateQueryStats(context.Background(), &models.SearchCostTracking{}))
}

func TestSearchCostRepository_CheckBudget_GetUserBudgetError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Return(errors.New("db error")).Once()

	require.Error(t, repo.CheckBudget(ctx, "user-1", "text_search", 100))
}
