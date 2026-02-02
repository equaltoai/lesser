package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchCostTracking_UpdateKeys_AndBudgets(t *testing.T) {
	t.Run("SearchCostTracking UpdateKeys sets keys, defaults, and TTL", func(t *testing.T) {
		ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
		sct := &SearchCostTracking{
			UserID:        "user-1",
			OperationType: "text_search",
			Timestamp:     ts,
		}

		before := time.Now()
		require.NoError(t, sct.UpdateKeys())

		assert.Equal(t, "2024-01-02", sct.Date)
		assert.Equal(t, "SEARCH_COST#2024-01-02#user-1", sct.PK)
		assert.Contains(t, sct.SK, "OPERATION#03:04:05")
		assert.Contains(t, sct.SK, "text_search")
		assert.Equal(t, "SearchCostTracking", sct.Type)
		assert.True(t, sct.TTL > 0)
		assert.True(t, time.Unix(sct.TTL, 0).After(before.Add(89*24*time.Hour)))
		assert.Equal(t, MainTableName, sct.TableName())
		assert.Equal(t, sct.PK, sct.GetPK())
		assert.Equal(t, sct.SK, sct.GetSK())
	})

	t.Run("SearchBudget UpdateKeys sets TTL by period", func(t *testing.T) {
		now := time.Now()
		cases := []struct {
			period      string
			expectedDur time.Duration
		}{
			{PeriodDaily, 7 * 24 * time.Hour},
			{PeriodMonthly, 365 * 24 * time.Hour},
			{"yearly", 5 * 365 * 24 * time.Hour},
			{"other", 30 * 24 * time.Hour},
		}

		for _, tc := range cases {
			sb := &SearchBudget{
				UserID:     "user-1",
				Period:     tc.period,
				PeriodDate: "2024-01",
			}
			require.NoError(t, sb.UpdateKeys())
			assert.Equal(t, "SEARCH_BUDGET#user-1", sb.PK)
			assert.Equal(t, "PERIOD#2024-01", sb.SK)
			assert.Equal(t, "SearchBudget", sb.Type)
			assert.WithinDuration(t, now.Add(tc.expectedDur), time.Unix(sb.TTL, 0), 2*time.Second)
			assert.Equal(t, MainTableName, sb.TableName())
			assert.Equal(t, sb.PK, sb.GetPK())
			assert.Equal(t, sb.SK, sb.GetSK())
		}
	})

	t.Run("SearchBudget CanMakeRequest enforces limits", func(t *testing.T) {
		sb := &SearchBudget{
			BudgetLimitMicros:    100,
			SearchBudgetMicros:   60,
			SemanticBudgetMicros: 30,
			IndexingBudgetMicros: 20,
			MaxRequestsPerHour:   2,
			MaxSemanticPerHour:   1,
		}

		// Overall budget cap.
		sb.UsedBudgetMicros = 90
		assert.False(t, sb.CanMakeRequest("text_search", 20))

		// Semantic cap.
		sb.UsedBudgetMicros = 0
		sb.SemanticUsedMicros = 25
		assert.False(t, sb.CanMakeRequest("semantic_search", 10))
		sb.SemanticUsedMicros = 0
		sb.CurrentSemanticRequests = 1
		assert.False(t, sb.CanMakeRequest("semantic_search", 1))

		// Regular search cap + rate limit.
		sb.CurrentSemanticRequests = 0
		sb.SearchUsedMicros = 55
		assert.False(t, sb.CanMakeRequest("text_search", 10))
		sb.SearchUsedMicros = 0
		sb.CurrentRequests = 2
		assert.False(t, sb.CanMakeRequest("hashtag_search", 1))

		// Indexing cap.
		sb.CurrentRequests = 0
		sb.IndexingUsedMicros = 15
		assert.False(t, sb.CanMakeRequest("search_indexing", 10))

		// Unknown operation relies on overall budget only.
		sb.UsedBudgetMicros = 0
		assert.True(t, sb.CanMakeRequest("other", 1))
	})

	t.Run("SearchBudget RecordUsage tracks counters and budget exceeded", func(t *testing.T) {
		sb := &SearchBudget{
			BudgetLimitMicros: 10,
		}

		sb.RecordUsage("semantic_search", 4)
		assert.Equal(t, int64(4), sb.UsedBudgetMicros)
		assert.Equal(t, int64(4), sb.SemanticUsedMicros)
		assert.Equal(t, 1, sb.CurrentSemanticRequests)
		assert.False(t, sb.BudgetExceeded)

		sb.RecordUsage("text_search", 4)
		assert.Equal(t, int64(8), sb.UsedBudgetMicros)
		assert.Equal(t, int64(4), sb.SearchUsedMicros)
		assert.Equal(t, 1, sb.CurrentRequests)
		assert.False(t, sb.BudgetExceeded)

		sb.RecordUsage("search_indexing", 2)
		assert.Equal(t, int64(10), sb.UsedBudgetMicros)
		assert.Equal(t, int64(2), sb.IndexingUsedMicros)
		assert.True(t, sb.BudgetExceeded)
		assert.False(t, sb.LastUsageTime.IsZero())
		assert.False(t, sb.UpdatedAt.IsZero())
	})

	t.Run("SearchCostAggregation UpdateKeys sets keys and TTL", func(t *testing.T) {
		sca := &SearchCostAggregation{
			Date:            "2024-01-02",
			AggregationType: "daily",
			MetricName:      "total_cost",
		}
		before := time.Now()
		require.NoError(t, sca.UpdateKeys())
		assert.Equal(t, "SEARCH_AGG#2024-01-02#daily", sca.PK)
		assert.Equal(t, "METRIC#total_cost", sca.SK)
		assert.Equal(t, "SearchCostAggregation", sca.Type)
		assert.True(t, time.Unix(sca.TTL, 0).After(before.Add(364*24*time.Hour)))
		assert.Equal(t, MainTableName, sca.TableName())
		assert.Equal(t, sca.PK, sca.GetPK())
		assert.Equal(t, sca.SK, sca.GetSK())
	})

	t.Run("SearchQueryStats UpdateKeys sets keys and TTL by period", func(t *testing.T) {
		now := time.Now()
		cases := []struct {
			period      string
			expectedDur time.Duration
		}{
			{PeriodDaily, 30 * 24 * time.Hour},
			{PeriodWeekly, 90 * 24 * time.Hour},
			{PeriodMonthly, 365 * 24 * time.Hour},
			{"other", 30 * 24 * time.Hour},
		}

		for _, tc := range cases {
			sqs := &SearchQueryStats{
				QueryHash: "h1",
				Period:    tc.period,
			}
			require.NoError(t, sqs.UpdateKeys())
			assert.Equal(t, "SEARCH_STATS#h1", sqs.PK)
			assert.Equal(t, "STATS#"+tc.period, sqs.SK)
			assert.Equal(t, "SearchQueryStats", sqs.Type)
			assert.WithinDuration(t, now.Add(tc.expectedDur), time.Unix(sqs.TTL, 0), 2*time.Second)
			assert.Equal(t, MainTableName, sqs.TableName())
			assert.Equal(t, sqs.PK, sqs.GetPK())
			assert.Equal(t, sqs.SK, sqs.GetSK())
		}
	})

	t.Run("SearchCostTracking UpdateKeys uses provided Date", func(t *testing.T) {
		sct := &SearchCostTracking{
			UserID:        "user-1",
			OperationType: "text_search",
			Date:          "2024-02-03",
			Timestamp:     time.Unix(1700000000, 0).UTC(),
		}
		require.NoError(t, sct.UpdateKeys())
		assert.Equal(t, "SEARCH_COST#2024-02-03#user-1", sct.PK)
		assert.Equal(t, "2024-02-03", sct.Date)
	})
}
