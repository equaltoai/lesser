package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGetAggregatedCostsByPeriod(t *testing.T) {
	ctx := context.Background()
	startDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

	// Sample data
	agg1 := &models.DynamoDBCostAggregation{OperationType: "GetItem", WindowStart: startDate, TotalOperations: 10, TotalCostMicroCents: 100}
	agg2 := &models.DynamoDBCostAggregation{OperationType: "PutItem", WindowStart: startDate, TotalOperations: 5, TotalCostMicroCents: 200}
	agg3 := &models.DynamoDBCostAggregation{OperationType: "GetItem", WindowStart: startDate.Add(1 * time.Hour), TotalOperations: 20, TotalCostMicroCents: 150}

	t.Run("successful aggregation", func(t *testing.T) {
		repo := &TrackingRepository{
			EnhancedBaseRepository: &EnhancedBaseRepository[*models.DynamoDBCostRecord]{
				BaseRepository: &BaseRepository[*models.DynamoDBCostRecord]{
					logger: zap.NewNop(),
				},
			},
		}
		repo.listAggregatedByPeriodFn = func(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error) {
			if operationType == "GetItem" {
				return []*models.DynamoDBCostAggregation{agg1, agg3}, "", nil
			}
			if operationType == "PutItem" {
				return []*models.DynamoDBCostAggregation{agg2}, "", nil
			}
			return nil, "", nil
		}

		result, err := repo.GetAggregatedCostsByPeriod(ctx, "day", startDate, endDate)
		require.NoError(t, err)
		require.Len(t, result, 2)

		// First window
		require.Equal(t, int64(15), result[0].TotalOperations)
		require.Equal(t, int64(300), result[0].TotalCostMicroCents)
		require.Equal(t, float64(300)/1_000_000.0, result[0].TotalCostDollars)

		// Second window
		require.Equal(t, int64(20), result[1].TotalOperations)
		require.Equal(t, int64(150), result[1].TotalCostMicroCents)
	})

	t.Run("empty dataset", func(t *testing.T) {
		repo := &TrackingRepository{
			EnhancedBaseRepository: &EnhancedBaseRepository[*models.DynamoDBCostRecord]{
				BaseRepository: &BaseRepository[*models.DynamoDBCostRecord]{
					logger: zap.NewNop(),
				},
			},
		}
		repo.listAggregatedByPeriodFn = func(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error) {
			return nil, "", nil
		}

		result, err := repo.GetAggregatedCostsByPeriod(ctx, "day", startDate, endDate)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("pagination", func(t *testing.T) {
		callCount := 0
		repo := &TrackingRepository{
			EnhancedBaseRepository: &EnhancedBaseRepository[*models.DynamoDBCostRecord]{
				BaseRepository: &BaseRepository[*models.DynamoDBCostRecord]{
					logger: zap.NewNop(),
				},
			},
		}
		repo.listAggregatedByPeriodFn = func(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error) {
			callCount++
			if cursor == "" {
				return []*models.DynamoDBCostAggregation{agg1}, "next", nil
			}
			return []*models.DynamoDBCostAggregation{agg3}, "", nil
		}

		_, err := repo.GetAggregatedCostsByPeriod(ctx, "day", startDate, endDate)
		require.NoError(t, err)
		require.True(t, callCount > 1)
	})
}

func TestMergeAggregatesByWindow(t *testing.T) {
	startDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	agg1 := &models.DynamoDBCostAggregation{WindowStart: startDate, TotalOperations: 10, TotalCostMicroCents: 100, TableBreakdown: map[string]*models.DynamoDBTableCostStats{"table1": {OperationCount: 10}}}
	agg2 := &models.DynamoDBCostAggregation{WindowStart: startDate, TotalOperations: 5, TotalCostMicroCents: 200, TableBreakdown: map[string]*models.DynamoDBTableCostStats{"table1": {OperationCount: 5}}}
	agg3 := &models.DynamoDBCostAggregation{WindowStart: startDate.Add(1 * time.Hour), TotalOperations: 20, TotalCostMicroCents: 150}

	aggregates := []*models.DynamoDBCostAggregation{agg1, agg2, agg3}
	merged := mergeAggregatesByWindow(aggregates)

	require.Len(t, merged, 2)

	key1 := startDate.Format(time.RFC3339)
	require.Equal(t, int64(15), merged[key1].TotalOperations)
	require.Equal(t, int64(300), merged[key1].TotalCostMicroCents)
	require.Equal(t, int64(15), merged[key1].TableBreakdown["table1"].OperationCount)

	key2 := startDate.Add(1 * time.Hour).Format(time.RFC3339)
	require.Equal(t, int64(20), merged[key2].TotalOperations)
}

func TestFinalizeCostMetrics(t *testing.T) {
	startDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	merged := map[string]*models.DynamoDBCostAggregation{
		startDate.Add(1 * time.Hour).Format(time.RFC3339): {WindowStart: startDate.Add(1 * time.Hour), TotalOperations: 20, TotalCostMicroCents: 150000},
		startDate.Format(time.RFC3339):                    {WindowStart: startDate, TotalOperations: 15, TotalCostMicroCents: 300000},
	}

	result := finalizeCostMetrics(merged)
	require.Len(t, result, 2)

	// Check sorting
	require.True(t, result[0].WindowStart.Before(result[1].WindowStart))

	// Check calculations
	require.Equal(t, float64(0.3), result[0].TotalCostDollars)
	require.Equal(t, float64(0.02), result[0].AverageCostPerOperation)
	require.Equal(t, float64(0.15), result[1].TotalCostDollars)
}
