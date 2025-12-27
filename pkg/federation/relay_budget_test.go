package federation

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
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
			Period:      "daily",
			LastResetAt: now.Add(-1 * time.Hour), // Same day, 1 hour ago
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
		lastMonth := now.AddDate(0, -1, 0)
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
