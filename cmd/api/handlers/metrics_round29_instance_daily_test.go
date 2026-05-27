package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestInstanceMetricsDaily_Round29_AuthAndDateValidation(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	cfg := round11TestConfig()
	cfg.InstanceAPIKey = "instance-metrics-key-42"

	state := &round10QueryState{
		costAggregations: []*storagemodels.DynamoDBCostAggregation{
			{
				Period:                  "day",
				OperationType:           "all",
				WindowStart:             today.AddDate(0, 0, -2),
				TotalOperations:         100,
				TotalReadCapacityUnits:  50,
				TotalWriteCapacityUnits: 25,
				AverageDuration:         10.0,
				TotalCostDollars:        0.05,
			},
			{
				Period:                  "day",
				OperationType:           "all",
				WindowStart:             today.AddDate(0, 0, -1),
				TotalOperations:         200,
				TotalReadCapacityUnits:  80,
				TotalWriteCapacityUnits: 40,
				AverageDuration:         12.0,
				TotalCostDollars:        0.08,
			},
			{
				Period:                  "day",
				OperationType:           "all",
				WindowStart:             today,
				TotalOperations:         150,
				TotalReadCapacityUnits:  60,
				TotalWriteCapacityUnits: 30,
				AverageDuration:         11.0,
				TotalCostDollars:        0.06,
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	t.Run("missing auth is rejected", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", nil, map[string]string{"days": "3"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleGetInstanceMetricsDailyLift(ctx))
	})

	t.Run("invalid key is rejected", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer wrong-key"}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{"days": "3"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleGetInstanceMetricsDailyLift(ctx))
	})

	t.Run("valid key returns daily aggregates", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{"days": "3"}, nil)
		require.NoError(t, err)

		resp, err := h.HandleGetInstanceMetricsDailyLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))

		// Verify period metadata.
		period, ok := body["period"].(map[string]any)
		require.True(t, ok, "period missing or wrong type")
		require.NotEmpty(t, period["start"])
		require.NotEmpty(t, period["end"])
		require.Equal(t, "UTC", period["timezone"])
		daysVal, ok := period["days"].(float64)
		require.True(t, ok)
		require.GreaterOrEqual(t, int(daysVal), 1)
		require.LessOrEqual(t, int(daysVal), 3)

		// Verify daily rows.
		daily, ok := body["daily"].([]any)
		require.True(t, ok, "daily missing or wrong type")
		require.NotEmpty(t, daily, "should have aggregate rows from seeded data")

		// Check row shape.
		firstRow, ok := daily[0].(map[string]any)
		require.True(t, ok)

		require.NotEmpty(t, firstRow["date"])
		_, hasRequests := firstRow["total_requests"]
		require.True(t, hasRequests, "total_requests missing from row")
		_, hasUsers := firstRow["unique_users"]
		require.True(t, hasUsers, "unique_users missing from row")
		_, hasReads := firstRow["dynamodb_reads"]
		require.True(t, hasReads, "dynamodb_reads missing from row")
		_, hasWrites := firstRow["dynamodb_writes"]
		require.True(t, hasWrites, "dynamodb_writes missing from row")
		_, hasDuration := firstRow["lambda_duration_ms"]
		require.True(t, hasDuration, "lambda_duration_ms missing from row")
		_, hasCents := firstRow["cost_cents"]
		require.True(t, hasCents, "cost_cents missing from row")
		_, hasDollars := firstRow["cost_dollars"]
		require.True(t, hasDollars, "cost_dollars missing from row")
		require.Equal(t, "USD", firstRow["currency"])
	})

	t.Run("empty data returns empty daily array", func(t *testing.T) {
		// Handler with empty cost aggregations (explicit empty slice avoids harness defaults).
		emptyState := &round10QueryState{
			costAggregations: []*storagemodels.DynamoDBCostAggregation{},
		}
		hEmpty, _, _ := round11NewHandler(t, cfg, emptyState)

		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{"days": "3"}, nil)
		require.NoError(t, err)

		resp, err := hEmpty.HandleGetInstanceMetricsDailyLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		daily, ok := body["daily"].([]any)
		require.True(t, ok)
		require.Empty(t, daily, "empty data should produce empty daily array")
	})

	t.Run("from/to query params take precedence", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		from := today.AddDate(0, 0, -2).Format("2006-01-02")
		to := today.Format("2006-01-02")
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{
			"from": from,
			"to":   to,
		}, nil)
		require.NoError(t, err)

		resp, err := h.HandleGetInstanceMetricsDailyLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))

		period, ok := body["period"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, from, period["start"])
		require.Equal(t, to, period["end"])
	})

	t.Run("invalid from date format is rejected", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{
			"from": "not-a-date",
			"to":   today.Format("2006-01-02"),
		}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleGetInstanceMetricsDailyLift(ctx))
	})

	t.Run("invalid to date format is rejected", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{
			"from": today.Format("2006-01-02"),
			"to":   "bad-date",
		}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleGetInstanceMetricsDailyLift(ctx))
	})

	t.Run("from after to is rejected", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{
			"from": today.Format("2006-01-02"),
			"to":   today.AddDate(0, 0, -2).Format("2006-01-02"),
		}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleGetInstanceMetricsDailyLift(ctx))
	})

	t.Run("days bounded to max 30", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/metrics/daily", headers, map[string]string{"days": "100"}, nil)
		require.NoError(t, err)

		resp, err := h.HandleGetInstanceMetricsDailyLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		period, ok := body["period"].(map[string]any)
		require.True(t, ok)
		daysVal, ok := period["days"].(float64)
		require.True(t, ok)
		require.LessOrEqual(t, int(daysVal), 30)
	})
}
