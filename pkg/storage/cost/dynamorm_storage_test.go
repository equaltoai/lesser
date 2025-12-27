package storagecost

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestDailyAggregateFromModel(t *testing.T) {
	agg := &models.DynamoDBCostAggregation{
		WindowStart:             time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		TotalCostMicroCents:     123,
		TotalOperations:         10,
		TotalReadCapacityUnits:  2,
		TotalWriteCapacityUnits: 3,
		AverageDuration:         5, // ms
		TableBreakdown: map[string]*models.DynamoDBTableCostStats{
			"users":    {UniqueUsers: 2},
			"statuses": {UniqueUsers: 3},
		},
		ServiceBreakdown: map[string]*models.DynamoDBServiceCostStats{
			"s3":         {DataTransferBytes: 100},
			"cloudfront": {DataTransferBytes: 50},
			"other":      {DataTransferBytes: 999},
		},
	}

	daily := dailyAggregateFromModel(agg)
	require.Equal(t, "2025-01-02", daily.Date)
	require.Equal(t, int64(123), daily.TotalCostMicrocents)
	require.Equal(t, int64(10), daily.RequestCount)
	require.Equal(t, int64(5), daily.UniqueUsers)
	require.Equal(t, int64(2), daily.DynamoDBReads)
	require.Equal(t, int64(3), daily.DynamoDBWrites)
	require.Equal(t, int64(10), daily.LambdaInvocations)
	require.Equal(t, int64(50), daily.LambdaDurationMs)
	require.Equal(t, int64(150), daily.DataTransferBytes)
}

func TestMonthlyAggregateFromModel(t *testing.T) {
	agg := &models.DynamoDBCostAggregation{
		WindowStart:             time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		TotalCostMicroCents:     9_876_543,
		TotalCostDollars:        1.5,
		TotalOperations:         20,
		TotalReadCapacityUnits:  10,
		TotalWriteCapacityUnits: 7,
		AverageDuration:         2.5,
		TableBreakdown: map[string]*models.DynamoDBTableCostStats{
			"users": {UniqueUsers: 4},
		},
		ServiceBreakdown: map[string]*models.DynamoDBServiceCostStats{
			"s3":         {DataTransferBytes: 1024},
			"cloudfront": {DataTransferBytes: 1024},
			"other":      {DataTransferBytes: 1024},
		},
	}

	monthly := monthlyAggregateFromModel(agg)
	require.Equal(t, 2025, monthly.Year)
	require.Equal(t, 2, monthly.Month)
	require.Equal(t, int64(9_876_543), monthly.TotalCostMicrocents)
	require.Equal(t, int64(1_500_000), monthly.ProjectedCostMicrocents)
	require.Equal(t, int64(20), monthly.RequestCount)
	require.Equal(t, int64(4), monthly.UniqueUsers)
	require.Equal(t, int64(10), monthly.DynamoDBReads)
	require.Equal(t, int64(7), monthly.DynamoDBWrites)
	require.Equal(t, int64(20), monthly.LambdaInvocations)
	require.Equal(t, int64(50), monthly.LambdaDurationMs)
	require.InDelta(t, float64(3072)/(1024*1024*1024), monthly.DataTransferGB, 1e-12)
}

func TestUniqueUsersFromTableBreakdown_nil(t *testing.T) {
	require.Equal(t, int64(0), uniqueUsersFromTableBreakdown(nil))
}
