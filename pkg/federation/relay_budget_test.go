package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// === shouldResetBudget Tests ===

func TestShouldResetBudget(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &RelayBudgetService{
		logger: logger,
	}

	// Use a fixed "now" time for deterministic tests
	// We'll manipulate LastResetAt relative to this
	fixedNow := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC) // Saturday, June 15, 2024

	t.Run("daily_reset_same_day", func(t *testing.T) {
		now := time.Now()
		budget := &models.RelayBudget{
			Period: "daily",
			// Always same calendar day regardless of when the test runs.
			LastResetAt: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		}

		// Should NOT reset - same day
		result := service.shouldResetBudget(budget)
		assert.False(t, result)
	})

	t.Run("daily_reset_different_day", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "daily",
			LastResetAt: fixedNow.Add(-25 * time.Hour), // Yesterday
		}

		result := service.shouldResetBudget(budget)
		assert.True(t, result)
	})

	t.Run("daily_reset_different_month", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "daily",
			LastResetAt: time.Date(2024, 5, 31, 14, 30, 0, 0, time.UTC), // Last month
		}

		result := service.shouldResetBudget(budget)
		assert.True(t, result)
	})

	t.Run("daily_reset_different_year", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "daily",
			LastResetAt: time.Date(2023, 6, 15, 14, 30, 0, 0, time.UTC), // Last year, same day
		}

		result := service.shouldResetBudget(budget)
		assert.True(t, result)
	})

	t.Run("weekly_reset_within_7_days", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "weekly",
			LastResetAt: time.Now().Add(-6 * 24 * time.Hour), // 6 days ago
		}

		result := service.shouldResetBudget(budget)
		assert.False(t, result)
	})

	t.Run("weekly_reset_exactly_7_days", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "weekly",
			LastResetAt: time.Now().Add(-7 * 24 * time.Hour), // Exactly 7 days ago
		}

		result := service.shouldResetBudget(budget)
		assert.True(t, result)
	})

	t.Run("weekly_reset_more_than_7_days", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "weekly",
			LastResetAt: time.Now().Add(-10 * 24 * time.Hour), // 10 days ago
		}

		result := service.shouldResetBudget(budget)
		assert.True(t, result)
	})

	t.Run("monthly_reset_same_month", func(t *testing.T) {
		now := time.Now()
		budget := &models.RelayBudget{
			Period:      "monthly",
			LastResetAt: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), // First of current month
		}

		result := service.shouldResetBudget(budget)
		assert.False(t, result)
	})

	t.Run("monthly_reset_different_month", func(t *testing.T) {
		now := time.Now()
		lastMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
		budget := &models.RelayBudget{
			Period:      "monthly",
			LastResetAt: lastMonth,
		}

		result := service.shouldResetBudget(budget)
		assert.True(t, result)
	})

	t.Run("monthly_reset_different_year_same_month", func(t *testing.T) {
		now := time.Now()
		budget := &models.RelayBudget{
			Period:      "monthly",
			LastResetAt: time.Date(now.Year()-1, now.Month(), 15, 12, 0, 0, 0, now.Location()), // Same month, last year
		}

		result := service.shouldResetBudget(budget)
		assert.True(t, result)
	})

	t.Run("unknown_period_no_reset", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "quarterly",
			LastResetAt: time.Now().Add(-365 * 24 * time.Hour), // A year ago
		}

		result := service.shouldResetBudget(budget)
		assert.False(t, result)
	})

	t.Run("empty_period_no_reset", func(t *testing.T) {
		budget := &models.RelayBudget{
			Period:      "",
			LastResetAt: time.Now().Add(-365 * 24 * time.Hour),
		}

		result := service.shouldResetBudget(budget)
		assert.False(t, result)
	})
}

// === checkBudgetThresholds Tests ===

func TestCheckBudgetThresholds(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &RelayBudgetService{
		logger: logger,
	}
	ctx := context.Background()

	t.Run("below_warning_no_flags_set", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   500000, // 50%
			CurrentUsagePercent:      50.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         false,
			CriticalAlertSent:        false,
		}

		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.False(t, budget.WarningAlertSent)
		assert.False(t, budget.CriticalAlertSent)
	})

	t.Run("at_warning_threshold_sets_warning_flag", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   750000, // 75%
			CurrentUsagePercent:      75.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         false,
			CriticalAlertSent:        false,
			NotifyAdmin:              true,
		}

		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)
		assert.False(t, budget.CriticalAlertSent)
	})

	t.Run("above_warning_below_critical_only_warning_flag", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   850000, // 85%
			CurrentUsagePercent:      85.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         false,
			CriticalAlertSent:        false,
		}

		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)
		assert.False(t, budget.CriticalAlertSent)
	})

	t.Run("at_critical_threshold_sets_both_flags", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   900000, // 90%
			CurrentUsagePercent:      90.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         false,
			CriticalAlertSent:        false,
			NotifyAdmin:              true,
		}

		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)
		assert.True(t, budget.CriticalAlertSent)
	})

	t.Run("warning_already_sent_not_re_triggered", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   800000, // 80%
			CurrentUsagePercent:      80.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         true, // Already sent
			CriticalAlertSent:        false,
		}

		// Call twice - should not change state
		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)

		err = service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)
	})

	t.Run("critical_already_sent_not_re_triggered", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   950000, // 95%
			CurrentUsagePercent:      95.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         true, // Already sent
			CriticalAlertSent:        true, // Already sent
		}

		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		// State should remain unchanged
		assert.True(t, budget.WarningAlertSent)
		assert.True(t, budget.CriticalAlertSent)
	})

	t.Run("flips_warning_exactly_once", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   800000,
			CurrentUsagePercent:      80.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         false,
			CriticalAlertSent:        false,
		}

		// First call - should flip
		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)
		assert.False(t, budget.CriticalAlertSent)

		// Second call - should NOT flip again (already true)
		err = service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)
	})

	t.Run("flips_critical_exactly_once", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   950000,
			CurrentUsagePercent:      95.0,
			WarningThresholdPercent:  75.0,
			CriticalThresholdPercent: 90.0,
			WarningAlertSent:         true, // Already sent from before
			CriticalAlertSent:        false,
			NotifyAdmin:              true,
			PauseRelay:               true,
			ReduceFrequency:          true,
		}

		// First call - should flip critical
		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.CriticalAlertSent)

		// Second call - should NOT flip again
		err = service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.CriticalAlertSent)
	})

	t.Run("zero_thresholds_work_correctly", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   100,
			CurrentUsagePercent:      0.01,
			WarningThresholdPercent:  0.0, // Always trigger
			CriticalThresholdPercent: 0.0, // Always trigger
			WarningAlertSent:         false,
			CriticalAlertSent:        false,
		}

		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.True(t, budget.WarningAlertSent)
		assert.True(t, budget.CriticalAlertSent)
	})

	t.Run("very_high_threshold_never_triggers", func(t *testing.T) {
		budget := &models.RelayBudget{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   999999,
			CurrentUsagePercent:      99.9999,
			WarningThresholdPercent:  100.0, // Never trigger
			CriticalThresholdPercent: 100.0, // Never trigger
			WarningAlertSent:         false,
			CriticalAlertSent:        false,
		}

		err := service.checkBudgetThresholds(ctx, budget)
		assert.NoError(t, err)
		assert.False(t, budget.WarningAlertSent)
		assert.False(t, budget.CriticalAlertSent)
	})
}

// === RelayBudgetStatus Struct Tests ===

func TestRelayBudgetStatus(t *testing.T) {
	t.Run("struct_fields_accessible", func(t *testing.T) {
		status := RelayBudgetStatus{
			RelayURL:                 "https://relay.example.com/inbox",
			Period:                   "daily",
			HasBudget:                true,
			LimitMicroCents:          1000000,
			CurrentUsageMicroCents:   500000,
			UsagePercent:             50.0,
			BudgetExceeded:           false,
			WarningThresholdReached:  false,
			CriticalThresholdReached: false,
			PauseRelay:               false,
			ReduceFrequency:          false,
			LastResetAt:              time.Now(),
		}

		assert.Equal(t, "https://relay.example.com/inbox", status.RelayURL)
		assert.Equal(t, "daily", status.Period)
		assert.True(t, status.HasBudget)
		assert.Equal(t, 50.0, status.UsagePercent)
	})
}

// === RelayBudgetRecommendations Struct Tests ===

func TestRelayBudgetRecommendations(t *testing.T) {
	t.Run("struct_fields_accessible", func(t *testing.T) {
		recommendations := RelayBudgetRecommendations{
			RelayURL:         "https://relay.example.com/inbox",
			AnalysisPeriod:   "30 days",
			Recommendations:  []string{"Reduce forwarding frequency", "Consider batching"},
			EstimatedSavings: 50000,
		}

		assert.Equal(t, "https://relay.example.com/inbox", recommendations.RelayURL)
		assert.Equal(t, "30 days", recommendations.AnalysisPeriod)
		assert.Len(t, recommendations.Recommendations, 2)
		assert.Equal(t, int64(50000), recommendations.EstimatedSavings)
	})
}

type relayBudgetCostRepoStub struct {
	budgets map[string]*models.RelayBudget

	createCalls []*models.RelayBudget
	createErr   error

	updateCalls []*models.RelayBudget
	updateErr   error

	aggregateCalls []struct {
		relayURL string
		period   string
		start    time.Time
		end      time.Time
	}
	aggregateErr error

	summary    *repositories.RelayCostSummary
	summaryErr error
}

func (r *relayBudgetCostRepoStub) CreateRelayBudget(_ context.Context, budget *models.RelayBudget) error {
	r.createCalls = append(r.createCalls, budget)
	if r.budgets != nil {
		r.budgets[budget.RelayURL+"#"+budget.Period] = budget
	}
	return r.createErr
}

func (r *relayBudgetCostRepoStub) GetRelayBudget(_ context.Context, relayURL, period string) (*models.RelayBudget, error) {
	if r.budgets == nil {
		return nil, errors.New("not found")
	}
	budget, ok := r.budgets[relayURL+"#"+period]
	if !ok {
		return nil, errors.New("not found")
	}
	return budget, nil
}

func (r *relayBudgetCostRepoStub) UpdateRelayBudget(_ context.Context, budget *models.RelayBudget) error {
	r.updateCalls = append(r.updateCalls, budget)
	if r.budgets != nil {
		r.budgets[budget.RelayURL+"#"+budget.Period] = budget
	}
	return r.updateErr
}

func (r *relayBudgetCostRepoStub) AggregateRelayCosts(_ context.Context, relayURL, period string, windowStart, windowEnd time.Time) error {
	r.aggregateCalls = append(r.aggregateCalls, struct {
		relayURL string
		period   string
		start    time.Time
		end      time.Time
	}{relayURL: relayURL, period: period, start: windowStart, end: windowEnd})
	return r.aggregateErr
}

func (r *relayBudgetCostRepoStub) GetRelayCostSummary(_ context.Context, _ string, _, _ time.Time) (*repositories.RelayCostSummary, error) {
	if r.summaryErr != nil {
		return nil, r.summaryErr
	}
	if r.summary == nil {
		return &repositories.RelayCostSummary{}, nil
	}
	return r.summary, nil
}

func TestRelayBudgetService_CreateUpdateStatusAndChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	costRepo := &relayBudgetCostRepoStub{budgets: map[string]*models.RelayBudget{}}
	service := &RelayBudgetService{
		costRepo: costRepo,
		logger:   logger,
	}

	relayURL := "https://relay.example/actor"
	require.NoError(t, service.CreateRelayBudget(ctx, relayURL, "daily", 100, 75, 90))
	require.Len(t, costRepo.createCalls, 1)
	assert.Equal(t, relayURL, costRepo.createCalls[0].RelayURL)
	assert.Equal(t, "daily", costRepo.createCalls[0].Period)
	assert.Equal(t, int64(100), costRepo.createCalls[0].LimitMicroCents)
	assert.Equal(t, 75.0, costRepo.createCalls[0].WarningThresholdPercent)
	assert.Equal(t, 90.0, costRepo.createCalls[0].CriticalThresholdPercent)
	assert.Equal(t, "relay.example", costRepo.createCalls[0].Domain)

	// Update usage and ensure percent is recalculated before threshold checks.
	require.NoError(t, service.UpdateRelayBudgetUsage(ctx, relayURL, "daily", 80))
	require.Len(t, costRepo.updateCalls, 1)
	assert.InDelta(t, 80.0, costRepo.updateCalls[0].CurrentUsagePercent, 0.01)
	assert.True(t, costRepo.updateCalls[0].WarningAlertSent)

	// Check status when a budget exists.
	status, err := service.GetRelayBudgetStatus(ctx, relayURL, "daily")
	require.NoError(t, err)
	assert.True(t, status.HasBudget)
	assert.Equal(t, int64(80), status.CurrentUsageMicroCents)

	// CheckRelayBudget returns budget exceeded when an operation would exceed the limit.
	require.ErrorIs(t, service.CheckRelayBudget(ctx, relayURL, 50), ErrRelayBudgetExceeded)
}

func TestRelayBudgetService_GetRelayBudgetStatus_NoBudget(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	service := &RelayBudgetService{
		costRepo: &relayBudgetCostRepoStub{},
		logger:   logger,
	}

	status, err := service.GetRelayBudgetStatus(ctx, "https://relay.example/actor", "daily")
	require.NoError(t, err)
	assert.False(t, status.HasBudget)
}

func TestRelayBudgetService_GetRelayBudgetStatus_ResetsExpiredBudget(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	costRepo := &relayBudgetCostRepoStub{
		budgets: map[string]*models.RelayBudget{
			relayURL + "#daily": {
				RelayURL:               relayURL,
				Period:                 "daily",
				LimitMicroCents:        100,
				CurrentUsageMicroCents: 99,
				CurrentUsagePercent:    99,
				LastResetAt:            time.Now().Add(-25 * time.Hour),
				BudgetExceeded:         true,
			},
		},
	}

	service := &RelayBudgetService{
		costRepo: costRepo,
		logger:   logger,
	}

	status, err := service.GetRelayBudgetStatus(ctx, relayURL, "daily")
	require.NoError(t, err)
	assert.True(t, status.HasBudget)
	assert.Equal(t, int64(0), status.CurrentUsageMicroCents)
	assert.Equal(t, 0.0, status.UsagePercent)
	assert.False(t, status.BudgetExceeded)
	require.Len(t, costRepo.updateCalls, 1)
}

func TestRelayBudgetService_CheckRelayBudget_PauseAndMonthly(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	now := time.Now()
	costRepo := &relayBudgetCostRepoStub{
		budgets: map[string]*models.RelayBudget{
			relayURL + "#daily": {
				RelayURL:               relayURL,
				Period:                 "daily",
				LimitMicroCents:        100,
				CurrentUsageMicroCents: 10,
				CurrentUsagePercent:    10,
				LastResetAt:            now,
				PauseRelay:             true,
			},
			relayURL + "#monthly": {
				RelayURL:               relayURL,
				Period:                 "monthly",
				LimitMicroCents:        100,
				CurrentUsageMicroCents: 90,
				CurrentUsagePercent:    90,
				LastResetAt:            now,
			},
		},
	}

	service := &RelayBudgetService{
		costRepo: costRepo,
		logger:   logger,
	}

	require.ErrorIs(t, service.CheckRelayBudget(ctx, relayURL, 1), ErrRelayOperationsPaused)

	// Remove daily budget so monthly is considered.
	delete(costRepo.budgets, relayURL+"#daily")
	require.ErrorIs(t, service.CheckRelayBudget(ctx, relayURL, 20), ErrRelayBudgetExceeded)
}

func TestRelayBudgetService_GetRelayBudgetRecommendations(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"

	t.Run("no_activity", func(t *testing.T) {
		service := &RelayBudgetService{
			costRepo: &relayBudgetCostRepoStub{summary: &repositories.RelayCostSummary{Count: 0}},
			logger:   logger,
		}

		recs, err := service.GetRelayBudgetRecommendations(ctx, relayURL)
		require.NoError(t, err)
		require.NotNil(t, recs)
		assert.NotEmpty(t, recs.Recommendations)
	})

	t.Run("multi_signal_recommendations", func(t *testing.T) {
		service := &RelayBudgetService{
			costRepo: &relayBudgetCostRepoStub{summary: &repositories.RelayCostSummary{
				Count:                   1,
				SuccessRate:             0.5,
				AverageCostPerOperation: 0.01,
				TotalCostMicroCents:     300_000,
				TotalOperations:         60_000, // 2000/day
			}},
			logger: logger,
		}

		recs, err := service.GetRelayBudgetRecommendations(ctx, relayURL)
		require.NoError(t, err)
		assert.Greater(t, len(recs.Recommendations), 2)
		assert.Greater(t, recs.EstimatedSavings, int64(0))
	})

	t.Run("summary_error", func(t *testing.T) {
		service := &RelayBudgetService{
			costRepo: &relayBudgetCostRepoStub{summaryErr: errors.New("boom")},
			logger:   logger,
		}

		_, err := service.GetRelayBudgetRecommendations(ctx, relayURL)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRelayCostSummaryFailed)
	})
}

func TestRelayBudgetService_UpdateRelayBudgetUsage_NoBudgetIsNoop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	service := &RelayBudgetService{
		costRepo: &relayBudgetCostRepoStub{},
		logger:   logger,
	}

	require.NoError(t, service.UpdateRelayBudgetUsage(ctx, "https://relay.example/actor", "daily", 10))
}

func TestNewRelayBudgetService_WiresCostRepo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	costRepo := &relayBudgetCostRepoStub{}

	service := NewRelayBudgetService(costRepo, logger)
	require.NotNil(t, service)
	assert.Equal(t, costRepo, service.costRepo)
}

func TestRelayBudgetService_CreateRelayBudget_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	service := &RelayBudgetService{
		costRepo: &relayBudgetCostRepoStub{createErr: errors.New("boom")},
		logger:   logger,
	}

	err := service.CreateRelayBudget(ctx, "https://relay.example/actor", "daily", 100, 75, 90)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRelayBudgetCreationFailed)
}

func TestRelayBudgetService_UpdateRelayBudgetUsage_ResetAndZeroLimit(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("reset_expired_period", func(t *testing.T) {
		relayURL := "https://relay.example/actor"
		costRepo := &relayBudgetCostRepoStub{
			budgets: map[string]*models.RelayBudget{
				relayURL + "#daily": {
					RelayURL:                 relayURL,
					Period:                   "daily",
					LimitMicroCents:          100,
					WarningThresholdPercent:  75,
					CriticalThresholdPercent: 90,
					CurrentUsageMicroCents:   50,
					CurrentUsagePercent:      50,
					WarningAlertSent:         true,
					CriticalAlertSent:        true,
					BudgetExceeded:           true,
					LastResetAt:              time.Now().Add(-25 * time.Hour),
				},
			},
		}

		service := &RelayBudgetService{
			costRepo: costRepo,
			logger:   logger,
		}

		require.NoError(t, service.UpdateRelayBudgetUsage(ctx, relayURL, "daily", 10))
		require.Len(t, costRepo.updateCalls, 1)
		updated := costRepo.updateCalls[0]
		assert.Equal(t, int64(10), updated.CurrentUsageMicroCents)
		assert.InDelta(t, 10.0, updated.CurrentUsagePercent, 0.01)
		assert.False(t, updated.WarningAlertSent)
		assert.False(t, updated.CriticalAlertSent)
		assert.False(t, updated.BudgetExceeded)
	})

	t.Run("zero_limit_does_not_recalculate_percent", func(t *testing.T) {
		relayURL := "https://relay.example/actor"
		costRepo := &relayBudgetCostRepoStub{
			budgets: map[string]*models.RelayBudget{
				relayURL + "#daily": {
					RelayURL:                 relayURL,
					Period:                   "daily",
					LimitMicroCents:          0,
					WarningThresholdPercent:  75,
					CriticalThresholdPercent: 90,
					CurrentUsageMicroCents:   0,
					CurrentUsagePercent:      0,
					LastResetAt:              time.Now(),
				},
			},
		}

		service := &RelayBudgetService{
			costRepo: costRepo,
			logger:   logger,
		}

		require.NoError(t, service.UpdateRelayBudgetUsage(ctx, relayURL, "daily", 10))
		require.Len(t, costRepo.updateCalls, 1)
		updated := costRepo.updateCalls[0]
		assert.Equal(t, int64(10), updated.CurrentUsageMicroCents)
		assert.Equal(t, 0.0, updated.CurrentUsagePercent)
		assert.False(t, updated.BudgetExceeded)
	})
}

func TestRelayBudgetService_CheckRelayBudget_AlreadyExceeded(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("daily_budget_already_exceeded", func(t *testing.T) {
		relayURL := "https://relay.example/actor"
		now := time.Now()

		service := &RelayBudgetService{
			costRepo: &relayBudgetCostRepoStub{
				budgets: map[string]*models.RelayBudget{
					relayURL + "#daily": {
						RelayURL:               relayURL,
						Period:                 "daily",
						LimitMicroCents:        100,
						CurrentUsageMicroCents: 120,
						CurrentUsagePercent:    120,
						BudgetExceeded:         true,
						LastResetAt:            now,
					},
				},
			},
			logger: logger,
		}

		require.ErrorIs(t, service.CheckRelayBudget(ctx, relayURL, 1), ErrRelayBudgetAlreadyExceeded)
	})

	t.Run("monthly_budget_already_exceeded", func(t *testing.T) {
		relayURL := "https://relay.example/actor"
		now := time.Now()

		service := &RelayBudgetService{
			costRepo: &relayBudgetCostRepoStub{
				budgets: map[string]*models.RelayBudget{
					relayURL + "#monthly": {
						RelayURL:               relayURL,
						Period:                 "monthly",
						LimitMicroCents:        100,
						CurrentUsageMicroCents: 120,
						CurrentUsagePercent:    120,
						BudgetExceeded:         true,
						LastResetAt:            now,
					},
				},
			},
			logger: logger,
		}

		require.ErrorIs(t, service.CheckRelayBudget(ctx, relayURL, 1), ErrRelayBudgetAlreadyExceeded)
	})
}

func TestRelayBudgetService_GetRelayBudgetStatus_ResetUpdateErrorIsIgnored(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	costRepo := &relayBudgetCostRepoStub{
		budgets: map[string]*models.RelayBudget{
			relayURL + "#daily": {
				RelayURL:               relayURL,
				Period:                 "daily",
				LimitMicroCents:        100,
				CurrentUsageMicroCents: 99,
				CurrentUsagePercent:    99,
				LastResetAt:            time.Now().Add(-25 * time.Hour),
				BudgetExceeded:         true,
			},
		},
		updateErr: errors.New("boom"),
	}

	service := &RelayBudgetService{
		costRepo: costRepo,
		logger:   logger,
	}

	status, err := service.GetRelayBudgetStatus(ctx, relayURL, "daily")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.HasBudget)
	assert.Equal(t, int64(0), status.CurrentUsageMicroCents)
}

func TestRelayBudgetService_AggregateRelayCostsAt(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	findFirstSundayMonth := func(year int) time.Time {
		for month := time.January; month <= time.December; month++ {
			now := time.Date(year, month, 1, 12, 0, 0, 0, time.UTC)
			if now.Weekday() == time.Sunday {
				return now
			}
		}
		return time.Date(year, time.January, 1, 12, 0, 0, 0, time.UTC)
	}

	now := findFirstSundayMonth(2024)
	relayURL := "https://relay.example/actor"
	costRepo := &relayBudgetCostRepoStub{
		aggregateErr: errors.New("boom"),
	}
	service := &RelayBudgetService{
		costRepo: costRepo,
		logger:   logger,
	}

	require.NoError(t, service.aggregateRelayCostsAt(ctx, relayURL, now))
	require.Len(t, costRepo.aggregateCalls, 3)
	assert.Equal(t, "daily", costRepo.aggregateCalls[0].period)
	assert.Equal(t, "weekly", costRepo.aggregateCalls[1].period)
	assert.Equal(t, "monthly", costRepo.aggregateCalls[2].period)

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)
	assert.Equal(t, dayStart, costRepo.aggregateCalls[0].start)
	assert.Equal(t, dayEnd, costRepo.aggregateCalls[0].end)

	assert.Equal(t, dayStart.AddDate(0, 0, -6), costRepo.aggregateCalls[1].start)
	assert.Equal(t, dayEnd, costRepo.aggregateCalls[1].end)

	monthStart := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
	monthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	assert.Equal(t, monthStart, costRepo.aggregateCalls[2].start)
	assert.Equal(t, monthEnd, costRepo.aggregateCalls[2].end)

	// Also cover the public wrapper.
	require.NoError(t, service.AggregateRelayCosts(ctx, relayURL))
}
