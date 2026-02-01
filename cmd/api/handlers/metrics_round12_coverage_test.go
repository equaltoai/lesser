package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAnyToFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected float64
		ok       bool
	}{
		{name: "float64", input: float64(1.25), expected: 1.25, ok: true},
		{name: "float32", input: float32(2.5), expected: 2.5, ok: true},
		{name: "int", input: int(-3), expected: -3, ok: true},
		{name: "int64", input: int64(4), expected: 4, ok: true},
		{name: "int32", input: int32(-5), expected: -5, ok: true},
		{name: "uint", input: uint(6), expected: 6, ok: true},
		{name: "uint64", input: uint64(7), expected: 7, ok: true},
		{name: "uint32", input: uint32(8), expected: 8, ok: true},
		{name: "unsupported", input: "nope", expected: 0, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := anyToFloat64(tt.input)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestMetricsHelpers_Round12Coverage(t *testing.T) {
	now := time.Now().UTC()

	state := &round10QueryState{
		// Allow explicit empty slice for request-rate fallback.
		costRecords: []*storagemodels.DynamoDBCostRecord{},
		instanceMetrics: map[string]storagemodels.InstanceMetrics{
			// InstanceRepository.GetStorageUsage uses PK=INSTANCE#METRICS, SK=STORAGE_USAGE.
			"INSTANCE#METRICS#STORAGE_USAGE": {Value: int64(10 * bytesPerGB)},
		},
		// Instance history is used by both GetStorageHistory and GetUserGrowthHistory formatters.
		instanceHistories: []storagemodels.InstanceHistory{
			{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), StorageBytes: int64(10 * bytesPerGB), NewUsers: 1000, RecordedAt: now.AddDate(0, 0, -2)},
			{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), StorageBytes: int64(20 * bytesPerGB), NewUsers: 1000, RecordedAt: now.AddDate(0, 0, -1)},
		},
		metricRecords: []storagemodels.MetricRecord{
			{MetricType: "api_endpoint", Count: 10, Sum: 0, P50: 30},
			{MetricType: "api_endpoint", Count: 5, Sum: 250, P50: 0},
			{MetricType: "database_operation", Count: 4, Sum: 200, P50: 0},
			{MetricType: "database_operation", Count: 0, Sum: 0, P50: 0},
		},
	}

	h, _, _ := round11NewHandlerSliceC(t, state)

	t.Run("calculateRequestRateLift fallback", func(t *testing.T) {
		// With no cost records, the code falls back to active-user estimation.
		require.Equal(t, 0.0, h.calculateRequestRateLift(context.Background()))
	})

	t.Run("calculateRequestRateLift fallback analytics error", func(t *testing.T) {
		state := &round10QueryState{
			costRecords: []*storagemodels.DynamoDBCostRecord{},
			allErrorByType: map[string]error{
				"*[]models.Activity": errors.New("boom"),
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateRequestRateLift(context.Background()))
	})

	t.Run("calculateStorageGrowthRateLift caps high", func(t *testing.T) {
		// 10GB -> 20GB over 2 points yields a very high monthly growth rate; cap at 200%.
		require.Equal(t, 200.0, h.calculateStorageGrowthRateLift(context.Background()))
	})

	t.Run("calculateStorageGrowthRateLift returns uncapped value", func(t *testing.T) {
		state := &round10QueryState{
			instanceHistories: []storagemodels.InstanceHistory{
				{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), StorageBytes: int64(10 * bytesPerGB), RecordedAt: now.AddDate(0, 0, -2)},
				{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), StorageBytes: int64(11 * bytesPerGB), RecordedAt: now.AddDate(0, 0, -1)},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.InDelta(t, 150.0, h.calculateStorageGrowthRateLift(context.Background()), 0.001)
	})

	t.Run("calculateStorageGrowthRateLift caps low", func(t *testing.T) {
		state := &round10QueryState{
			instanceHistories: []storagemodels.InstanceHistory{
				{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), StorageBytes: int64(100 * bytesPerGB), RecordedAt: now.AddDate(0, 0, -2)},
				{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), StorageBytes: int64(1 * bytesPerGB), RecordedAt: now.AddDate(0, 0, -1)},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, -50.0, h.calculateStorageGrowthRateLift(context.Background()))
	})

	t.Run("calculateStorageGrowthRateLift insufficient history", func(t *testing.T) {
		state := &round10QueryState{
			instanceHistories: []storagemodels.InstanceHistory{
				{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), StorageBytes: int64(1 * bytesPerGB), RecordedAt: now.AddDate(0, 0, -1)},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateStorageGrowthRateLift(context.Background()))
	})

	t.Run("calculateStorageProjectionLift reads bytes map", func(t *testing.T) {
		// With growth capped at 200%, projected storage for 30 days is current * (1 + 0.066..*30) = 3x.
		require.InDelta(t, 30.0, h.calculateStorageProjectionLift(context.Background(), 30), 0.001)
	})

	t.Run("calculateStorageProjectionLift falls back on repository error", func(t *testing.T) {
		state := &round10QueryState{
			firstErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateStorageProjectionLift(context.Background(), 30))
	})

	t.Run("calculateStorageProjectionLift falls back when storage is unknown", func(t *testing.T) {
		state := &round10QueryState{
			instanceMetrics: map[string]storagemodels.InstanceMetrics{
				"INSTANCE#METRICS#STORAGE_USAGE": {Value: 0},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateStorageProjectionLift(context.Background(), 30))
	})

	t.Run("calculateStorageGrowthRateLift returns 0 when no base usage", func(t *testing.T) {
		state := &round10QueryState{
			instanceHistories: []storagemodels.InstanceHistory{
				{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), StorageBytes: 0, RecordedAt: now.AddDate(0, 0, -2)},
				{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), StorageBytes: int64(1 * bytesPerGB), RecordedAt: now.AddDate(0, 0, -1)},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateStorageGrowthRateLift(context.Background()))
	})

	t.Run("calculateUserGrowthRateLift caps high", func(t *testing.T) {
		require.Equal(t, 100.0, h.calculateUserGrowthRateLift(context.Background()))
	})

	t.Run("calculateUserGrowthRateLift returns 0 when no new users", func(t *testing.T) {
		state := &round10QueryState{
			instanceHistories: []storagemodels.InstanceHistory{
				{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), NewUsers: 0, RecordedAt: now.AddDate(0, 0, -2)},
				{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), NewUsers: 0, RecordedAt: now.AddDate(0, 0, -1)},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateUserGrowthRateLift(context.Background()))
	})

	t.Run("calculateUserGrowthRateLift returns 0 with insufficient history", func(t *testing.T) {
		state := &round10QueryState{
			instanceHistories: []storagemodels.InstanceHistory{
				{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), NewUsers: 10, RecordedAt: now.AddDate(0, 0, -1)},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateUserGrowthRateLift(context.Background()))
	})

	t.Run("calculateUserGrowthRateLift returns 0 on total users error", func(t *testing.T) {
		state := &round10QueryState{
			instanceHistories: []storagemodels.InstanceHistory{
				{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), NewUsers: 10, RecordedAt: now.AddDate(0, 0, -2)},
				{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), NewUsers: 10, RecordedAt: now.AddDate(0, 0, -1)},
			},
			allErrorByType: map[string]error{
				"*[]models.User": errors.New("boom"),
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateUserGrowthRateLift(context.Background()))
	})

	t.Run("calculateUserProjectionLift returns fallback on MAU error", func(t *testing.T) {
		state := &round10QueryState{
			allErrorByType: map[string]error{
				"*[]models.Activity": errors.New("boom"),
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 100, h.calculateUserProjectionLift(context.Background(), 30))
	})

	t.Run("calculateRealLatencyMetricsLift uses P50 and sum fallback", func(t *testing.T) {
		avg := h.calculateRealLatencyMetricsLift(context.Background(), now.Add(-1*time.Hour), now)
		// Weighted average: (30*10 + (250/5)*5) / (10+5) = 550/15.
		require.InDelta(t, 36.6666667, avg, 0.001)
	})

	t.Run("calculateRealLatencyMetricsLift returns 0 when counts are zero", func(t *testing.T) {
		state := &round10QueryState{
			metricRecords: []storagemodels.MetricRecord{
				{MetricType: "api_endpoint", Count: 0, Sum: 0, P50: 0},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateRealLatencyMetricsLift(context.Background(), now.Add(-1*time.Hour), now))
	})

	t.Run("calculateRealLatencyMetricsLift falls back to db metrics", func(t *testing.T) {
		state := &round10QueryState{
			metricRecords: []storagemodels.MetricRecord{
				{MetricType: "database_operation", Count: 4, Sum: 200, P50: 0},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.InDelta(t, 50.0, h.calculateRealLatencyMetricsLift(context.Background(), now.Add(-1*time.Hour), now), 0.001)
	})

	t.Run("calculateRealLatencyMetricsLift returns 0 on repo error", func(t *testing.T) {
		state := &round10QueryState{
			allErrorByType: map[string]error{
				"*[]*models.MetricRecord": errors.New("boom"),
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		require.Equal(t, 0.0, h.calculateRealLatencyMetricsLift(context.Background(), now.Add(-1*time.Hour), now))
	})

	t.Run("HandleGetDailyAggregatesLift defaults days on parse error", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		ctx, err := round10NewLiftContext("GET", "/api/v1/metrics/daily", nil, map[string]string{"days": "nope"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleGetDailyAggregatesLift(ctx))
	})

	t.Run("HandleGetInstanceMetricsLift continues on active-user error", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, &round10QueryState{
			allErrorByType: map[string]error{
				"*[]models.Activity": errors.New("boom"),
			},
		})
		ctx, err := round10NewLiftContext("GET", "/api/v1/metrics", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleGetInstanceMetricsLift(ctx))
	})
}
