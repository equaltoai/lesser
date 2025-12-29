package lift

import (
	"net/http"
	"testing"
	"time"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestMetricsHandlers(t *testing.T) {
	now := time.Now()
	state := &round10QueryState{
		costRecords: []*storagemodels.DynamoDBCostRecord{
			{ID: "c1", OperationType: "GetItem", Timestamp: now.Add(-10 * time.Minute), TotalCostMicroCents: 1000},
		},
		costAggregations: []*storagemodels.DynamoDBCostAggregation{
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -8), TotalOperations: 5, TotalCostMicroCents: 1000},
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -7), TotalOperations: 6, TotalCostMicroCents: 1200},
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -6), TotalOperations: 7, TotalCostMicroCents: 1300},
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -5), TotalOperations: 8, TotalCostMicroCents: 1400},
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -4), TotalOperations: 9, TotalCostMicroCents: 1500},
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -3), TotalOperations: 10, TotalCostMicroCents: 1600},
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -2), TotalOperations: 11, TotalCostMicroCents: 1700},
			{Period: "day", OperationType: "GetItem", WindowStart: now.AddDate(0, 0, -1), TotalOperations: 12, TotalCostMicroCents: 1800},
		},
		metricRecords: []storagemodels.MetricRecord{
			{MetricType: "api_endpoint", Count: 5, Sum: 250, P50: 40},
		},
		instanceHistories: []storagemodels.InstanceHistory{
			{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), NewUsers: 2, RecordedAt: now.AddDate(0, 0, -2)},
			{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), NewUsers: 3, RecordedAt: now.AddDate(0, 0, -1)},
		},
	}

	h, _, _ := round11NewHandlerSliceC(t, state)

	ctxMetrics, err := round10NewLiftContext(http.MethodGet, "/api/v1/metrics", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetInstanceMetricsLift(ctxMetrics))

	ctxDaily, err := round10NewLiftContext(http.MethodGet, "/api/v1/metrics/daily", nil, map[string]string{"days": "7"}, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetDailyAggregatesLift(ctxDaily))

	ctxPredictive, err := round10NewLiftContext(http.MethodGet, "/api/v1/metrics/predictive", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetPredictiveAnalyticsLift(ctxPredictive))
}

func TestMetricsHelpers(t *testing.T) {
	require.Equal(t, 0.0, calculateConfidenceLevel(0))
	require.Equal(t, 0.3, calculateConfidenceLevel(3))
	require.Equal(t, 0.5, calculateConfidenceLevel(10))
	require.Equal(t, 0.7, calculateConfidenceLevel(20))
	require.Equal(t, 0.9, calculateConfidenceLevel(40))
}
