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

func TestTrackingRepository_ClampHelpers(t *testing.T) {
	require.Equal(t, costTableDefaultLimit, clampCostTableLimit(0))
	require.Equal(t, costTableMaxLimit, clampCostTableLimit(costTableMaxLimit+1))

	require.Equal(t, relayCostDefaultLimit, clampRelayCostLimit(0))
	require.Equal(t, relayCostMaxLimit, clampRelayCostLimit(relayCostMaxLimit+1))

	require.Equal(t, relayMetricsDefaultLimit, clampRelayMetricsLimit(0))
	require.Equal(t, relayMetricsMaxLimit, clampRelayMetricsLimit(relayMetricsMaxLimit+1))
}

func TestTrackingRepository_GetActivityCost_NotFoundAndQueryError(t *testing.T) {
	ctx := context.Background()

	t.Run("not found maps to storage not found error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

		_, err := repo.GetActivityCost(ctx, "activity-1")
		require.Error(t, err)
	})

	t.Run("query error returns query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("db error")).Once()

		_, err := repo.GetActivityCost(ctx, "activity-1")
		require.Error(t, err)
	})
}

func TestTrackingRepository_GetCostProjections_NoProjectionsReturnsDefault(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.CostProjection")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.CostProjection)
		*dest = []*models.CostProjection{}
	}).Return(nil).Once()

	projection, err := repo.GetCostProjections(ctx, "daily")
	require.NoError(t, err)
	require.Equal(t, "daily", projection.Period)
	require.Equal(t, float64(0), projection.CurrentCost)
	require.Equal(t, float64(0), projection.ProjectedCost)
	require.Empty(t, projection.TopDrivers)
	require.Empty(t, projection.Recommendations)

	// Keep baseTime referenced for determinism in future changes.
	_ = baseTime
}

func TestTrackingRepository_GetMonthlyAggregate_NotFound(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	repo := &TrackingRepository{
		EnhancedBaseRepository: &EnhancedBaseRepository[*models.DynamoDBCostRecord]{
			BaseRepository: &BaseRepository[*models.DynamoDBCostRecord]{logger: zap.NewNop()},
		},
		listAggregatedByPeriodFn: func(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error) {
			return nil, "", nil
		},
	}

	_, err := repo.GetMonthlyAggregate(ctx, baseTime.Year(), int(baseTime.Month()))
	require.Error(t, err)
}

func TestTrackingRepository_PercentileHelpers_EdgeCases(t *testing.T) {
	require.Equal(t, map[string]float64{
		"p50": 0,
		"p90": 0,
		"p95": 0,
		"p99": 0,
	}, calculatePercentiles(nil))

	require.Equal(t, float64(0), getPercentileValue(nil, 50))
	require.Equal(t, float64(42), getPercentileValue([]float64{42}, 99))
	require.Equal(t, float64(2), getPercentileValue([]float64{1, 2, 3}, 50))
}

func TestTrackingRepository_StatisticalHelpers_SmallInputs(t *testing.T) {
	repo := &TrackingRepository{}

	tau, pValue := repo.mannKendallTest([]float64{1, 2})
	require.Equal(t, float64(0), tau)
	require.Equal(t, float64(1), pValue)

	require.Equal(t, float64(0), repo.theilSenSlope([]float64{1}))

	// Exercise additional branches.
	require.Equal(t, float64(1), repo.theilSenSlope([]float64{0, 1, 2, 3}))

	tau, pValue = repo.mannKendallTest([]float64{3, 2, 1})
	require.Equal(t, float64(-1), tau)
	require.Equal(t, float64(0.20), pValue)
}

func TestTrackingRepository_CreateUpdateAggregated_ValidationErrors(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	require.Error(t, repo.CreateAggregated(ctx, &models.DynamoDBCostAggregation{}))
	require.Error(t, repo.UpdateAggregated(ctx, &models.DynamoDBCostAggregation{}))
}
