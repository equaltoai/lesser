package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestSearchServiceIntegration_Round09_Coverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)

	t.Run("estimateSearchCost includes semantic surcharge", func(t *testing.T) {
		s := &SearchServiceIntegration{}
		require.Greater(t, s.estimateSearchCost("abc", true), s.estimateSearchCost("abc", false))
	})

	t.Run("ComprehensiveSearchExample executes happy path without semantic search", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// Ensure budget fetch succeeds with a generous limit.
		mockQuery.
			On("First", mockMatchedByType[*models.SearchBudget]()).
			Run(func(args mock.Arguments) {
				b := args.Get(0).(*models.SearchBudget)
				b.UserID = "user-1"
				b.BudgetLimitMicros = 1_000_000_000
				b.SearchBudgetMicros = 1_000_000_000
				b.SemanticBudgetMicros = 1_000_000_000
				b.IndexingBudgetMicros = 1_000_000_000
				b.MaxRequestsPerHour = 10_000
				b.MaxSemanticPerHour = 10_000
			}).
			Return(nil).
			Maybe()

		// Search cost lookups in summaries.
		mockQuery.
			On("All", mockMatchedByType[*[]models.SearchCostTracking]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.SearchCostTracking)
				*out = append(*out, models.SearchCostTracking{
					UserID:          "user-1",
					OperationType:   "text_search",
					TotalCostMicros: 2000,
					ResultCount:     2,
					Timestamp:       baseTime,
				})
			}).
			Return(nil).
			Maybe()

		mockQuery.
			On("All", mockMatchedByType[*[]models.SearchQueryStats]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.SearchQueryStats)
				*out = append(*out,
					models.SearchQueryStats{
						QueryCount:            10,
						TotalCostMicros:       100000,
						TotalResultCount:      1,
						AverageResponseTimeMs: 10,
						CacheHitCount:         1,
					},
				)
			}).
			Return(nil).
			Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		s := NewSearchServiceIntegration(mockDB, nil, "req-1", zap.NewNop())

		results, err := s.ComprehensiveSearchExample(ctx, "user-1", "hello", false)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.NotNil(t, results.CostBreakdown)

		budget, err := s.BudgetManagementExample(ctx, "user-1")
		require.NoError(t, err)
		require.NotNil(t, budget)

		analytics, err := s.PerformanceAnalyticsExample(ctx, "user-1", 10)
		require.NoError(t, err)
		require.NotNil(t, analytics)
	})
}
