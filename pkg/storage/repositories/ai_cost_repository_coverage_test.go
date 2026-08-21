package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestAICostRepository_CreateAndGet_Coverage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewAICostRepository(mockDB, "test-table", logger, nil)

	costRecord := &models.AICost{
		OperationID:     "op-1",
		OperationType:   "sentiment_analysis",
		ModelName:       "claude-3-haiku",
		InputTokens:     10,
		OutputTokens:    5,
		ComplexityScore: 0.2,
		Timestamp:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	require.NoError(t, repo.CreateAICost(ctx, costRecord))
	require.NotZero(t, costRecord.InputTokenCost)
	require.NotZero(t, costRecord.OutputTokenCost)

	// GetAICost (BaseRepository.Get -> First)
	mockGetQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockGetQuery).Once()
	mockGetQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockGetQuery).Maybe()
	mockGetQuery.On("First", mock.AnythingOfType("*models.AICost")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.AICost)
		dest.OperationID = "op-1"
		dest.OperationType = "sentiment_analysis"
		dest.ModelName = "claude-3-haiku"
		dest.TotalCostMicroCents = 12345
		dest.InputTokens = 10
		dest.OutputTokens = 5
		dest.Timestamp = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}).Return(nil).Once()

	got, err := repo.GetAICost(ctx, "op-1")
	require.NoError(t, err)
	require.Equal(t, int64(12345), got.TotalCostMicroCents)
}

func TestAICostRepository_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockCreateQuery := new(mocks.MockQuery)
	repo := NewAICostRepository(mockDB, "test-table", logger, nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(errors.New("create failed")).Once()

	err := repo.CreateAICost(ctx, &models.AICost{OperationID: "op-err", ModelName: "claude-3-haiku"})
	require.Error(t, err)

	mockGetQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockGetQuery).Once()
	mockGetQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockGetQuery).Maybe()
	mockGetQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	_, err = repo.GetAICost(ctx, "missing")
	require.Error(t, err)
}

func TestAICostRepository_Queries_Summary_Trends_Aggregations(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	t.Run("time range filter and operation type", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAICostRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.AICost")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.AICost)
			*dest = []*models.AICost{
				{OperationID: "op-1", OperationType: "a", Timestamp: start.Add(10 * time.Minute), TotalCostMicroCents: 100, InputTokens: 1, OutputTokens: 1},
				{OperationID: "op-2", OperationType: "b", Timestamp: start.Add(30 * time.Minute), TotalCostMicroCents: 200, InputTokens: 2, OutputTokens: 2},
				{OperationID: "op-3", OperationType: "a", Timestamp: start.Add(-time.Minute), TotalCostMicroCents: 300, InputTokens: 3, OutputTokens: 3},
			}
		}).Return(nil).Once()

		costs, err := repo.GetAICostsByTimeRange(ctx, start, end, "a", 10)
		require.NoError(t, err)
		require.Len(t, costs, 1)
		require.Equal(t, "op-1", costs[0].OperationID)
	})

	t.Run("operation type query startTime and limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAICostRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.AICost)
			*dest = []models.AICost{{OperationID: "op-1"}, {OperationID: "op-2"}}
		}).Return(nil).Once()

		costs, err := repo.GetAICostsByOperationType(ctx, "a", start, 5)
		require.NoError(t, err)
		require.Len(t, costs, 2)
	})

	t.Run("top costly default tier, sorting, and limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAICostRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.AICost)
			*dest = []models.AICost{
				{OperationID: "cheap", Timestamp: start.Add(10 * time.Minute), TotalCostMicroCents: 100},
				{OperationID: "expensive", Timestamp: start.Add(20 * time.Minute), TotalCostMicroCents: 1000},
				{OperationID: "outside", Timestamp: start.Add(-time.Hour), TotalCostMicroCents: 9999},
			}
		}).Return(nil).Once()

		top, err := repo.GetTopCostlyOperations(ctx, "", start, end, 1)
		require.NoError(t, err)
		require.Len(t, top, 1)
		require.Equal(t, "expensive", top[0].OperationID)
	})

	t.Run("summary empty costs returns defaults", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAICostRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.AICost")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.AICost)
			*dest = nil
		}).Return(nil).Once()

		summary, err := repo.GetAICostSummary(ctx, start, end, "")
		require.NoError(t, err)
		require.Equal(t, int64(0), summary.TotalOperations)
	})

	t.Run("summary aggregates and efficiency", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAICostRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.AICost")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.AICost)
			*dest = []*models.AICost{
				{OperationID: "op-1", ModelName: "m1", OperationType: "a", Timestamp: start.Add(10 * time.Minute), TotalCostMicroCents: 1000, InputTokens: 10, OutputTokens: 5, RequestLatencyMs: 100, ComplexityScore: 0.5, Success: true},
				{OperationID: "op-2", ModelName: "m1", OperationType: "b", Timestamp: start.Add(20 * time.Minute), TotalCostMicroCents: 3000, InputTokens: 20, OutputTokens: 15, RequestLatencyMs: 300, ComplexityScore: 0.1, Success: false},
			}
		}).Return(nil).Once()

		summary, err := repo.GetAICostSummary(ctx, start, end, "")
		require.NoError(t, err)
		require.Equal(t, int64(2), summary.TotalOperations)
		require.Greater(t, summary.TotalCostDollars, 0.0)
		require.Greater(t, summary.SuccessRate, 0.0)
		require.Greater(t, summary.CostPerInputToken, 0.0)
		require.Greater(t, summary.CostPerOutputToken, 0.0)
		require.Contains(t, summary.ModelBreakdown, "m1")
	})

	t.Run("trends buckets and analysis", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAICostRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.AICost")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.AICost)
			*dest = []*models.AICost{
				{OperationID: "op-1", Timestamp: start.Add(10 * time.Minute), TotalCostMicroCents: 100, InputTokens: 1, OutputTokens: 1, RequestLatencyMs: 10, Success: true},
				{OperationID: "op-2", Timestamp: start.Add(70 * time.Minute), TotalCostMicroCents: 500, InputTokens: 2, OutputTokens: 3, RequestLatencyMs: 20, Success: false},
			}
		}).Return(nil).Once()

		trends, err := repo.GetAICostTrends(ctx, start, end, models.PeriodHour)
		require.NoError(t, err)
		require.NotEmpty(t, trends.DataPoints)
		require.NotNil(t, trends.Analysis)
	})

	t.Run("aggregated create and list", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockCreateQuery := new(mocks.MockQuery)
		mockQuery := new(mocks.MockQuery)
		repo := NewAICostRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)

		mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
		mockCreateQuery.On("CreateOrUpdate").Return(nil).Once()
		require.NoError(t, repo.CreateOrUpdateAggregatedCost(ctx, &models.AIAggregatedCost{Period: "day"}))

		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.AIAggregatedCost)
			*dest = []models.AIAggregatedCost{{Period: "day"}, {Period: "day"}}
		}).Return(nil).Once()

		aggs, err := repo.GetAggregatedCosts(ctx, models.PeriodTimeHour, start, end)
		require.NoError(t, err)
		require.Len(t, aggs, 2)
	})
}

func TestAICostRepository_TrendAnalysisAndPatterns_Direct(t *testing.T) {
	repo := NewAICostRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)

	// Too few points -> stable baseline.
	analysis := repo.analyzeAICostTrends([]AICostDataPoint{{TotalCost: 1}})
	require.Equal(t, "stable", analysis.TrendDirection)

	// Enough points for confidence + hourly patterns.
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	points := make([]AICostDataPoint, 0, 13)
	for i := 0; i < 13; i++ {
		points = append(points, AICostDataPoint{
			Timestamp: start.Add(time.Duration(i) * time.Hour),
			TotalCost: float64(i+1) * 10,
		})
	}

	analysis = repo.analyzeAICostTrends(points)
	require.Equal(t, "increasing", analysis.TrendDirection)
	require.NotEmpty(t, analysis.SeasonalFactors)
}
