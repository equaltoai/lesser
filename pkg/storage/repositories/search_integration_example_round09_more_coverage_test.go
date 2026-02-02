package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestSearchServiceIntegration_Round09_MoreCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("GetCostTracker and GetCostSummary", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		s := NewSearchServiceIntegration(mockDB, nil, "req-1", zap.NewNop())
		require.NotNil(t, s.GetCostTracker())
		require.NotNil(t, s.GetCostSummary())
	})

	t.Run("BudgetManagementExample status branches", func(t *testing.T) {
		cases := []struct {
			name string
			used int64
			want string
		}{
			{name: "healthy", used: 10, want: "healthy"},
			{name: "warning", used: 80, want: "warning"},
			{name: "critical", used: 95, want: "critical"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				mockDB := new(mocks.MockDB)
				mockQuery := new(mocks.MockQuery)

				mockQuery.
					On("First", mockMatchedByType[*models.SearchBudget]()).
					Run(func(args mock.Arguments) {
						b := args.Get(0).(*models.SearchBudget)
						b.UserID = "user-1"
						b.BudgetLimitMicros = 100
						b.UsedBudgetMicros = tc.used
						b.SearchBudgetMicros = 100
						b.SemanticBudgetMicros = 100
						b.IndexingBudgetMicros = 100
						b.MaxRequestsPerHour = 1000
						b.MaxSemanticPerHour = 1000
					}).
					Return(nil).
					Once()

				// Search costs: allow empty list.
				mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Maybe()

				setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

				s := NewSearchServiceIntegration(mockDB, nil, "req-1", zap.NewNop())
				summary, err := s.BudgetManagementExample(ctx, "user-1")
				require.NoError(t, err)
				require.Equal(t, tc.want, summary.Status)
			})
		}
	})

	t.Run("ComprehensiveSearchExample budget exceeded returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once() // create default budget path
		mockQuery.On("Create").Return(nil).Once()                                          // createDefaultBudget
		mockQuery.
			On("First", mockMatchedByType[*models.SearchBudget]()).
			Run(func(args mock.Arguments) {
				b := args.Get(0).(*models.SearchBudget)
				b.UserID = "user-1"
				b.BudgetLimitMicros = 0
				b.UsedBudgetMicros = 0
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		s := NewSearchServiceIntegration(mockDB, nil, "req-1", zap.NewNop())
		_, err := s.ComprehensiveSearchExample(ctx, "user-1", "hello", false)
		require.Error(t, err)
	})

	t.Run("ComprehensiveSearchExample tolerates RecordBudgetUsage failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("First", mockMatchedByType[*models.SearchBudget]()).
			Run(func(args mock.Arguments) {
				b := args.Get(0).(*models.SearchBudget)
				b.UserID = "user-1"
				b.BudgetLimitMicros = 1_000_000_000
				b.UsedBudgetMicros = 0
				b.SearchBudgetMicros = 1_000_000_000
				b.MaxRequestsPerHour = 1000
				b.MaxSemanticPerHour = 1000
			}).
			Return(nil).
			Maybe()

		// RecordBudgetUsage ends with Model(budget).Update(); DynamORM mock passes fields slice.
		mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		s := NewSearchServiceIntegration(mockDB, nil, "req-1", zap.NewNop())
		results, err := s.ComprehensiveSearchExample(ctx, "user-1", "hello", false)
		require.NoError(t, err)
		require.NotNil(t, results)
	})
}
