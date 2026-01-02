package federation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// RelayBudgetService manages relay budgets and cost limits
type RelayBudgetService struct {
	costRepo relayBudgetCostRepository
	logger   *zap.Logger
}

type relayBudgetCostRepository interface {
	CreateRelayBudget(ctx context.Context, budget *models.RelayBudget) error
	GetRelayBudget(ctx context.Context, relayURL, period string) (*models.RelayBudget, error)
	UpdateRelayBudget(ctx context.Context, budget *models.RelayBudget) error
	AggregateRelayCosts(ctx context.Context, relayURL, period string, windowStart, windowEnd time.Time) error
	GetRelayCostSummary(ctx context.Context, relayURL string, startTime, endTime time.Time) (*repositories.RelayCostSummary, error)
}

// NewRelayBudgetService creates a new relay budget service
func NewRelayBudgetService(costRepo relayBudgetCostRepository, logger *zap.Logger) *RelayBudgetService {
	return &RelayBudgetService{
		costRepo: costRepo,
		logger:   logger,
	}
}

// CreateRelayBudget creates a new budget configuration for a relay
func (rbs *RelayBudgetService) CreateRelayBudget(ctx context.Context, relayURL, period string, limitMicroCents int64, warningThreshold, criticalThreshold float64) error {
	budget := &models.RelayBudget{
		RelayURL:                 relayURL,
		Domain:                   extractDomainFromRelayURL(relayURL),
		Period:                   period,
		LimitMicroCents:          limitMicroCents,
		WarningThresholdPercent:  warningThreshold,
		CriticalThresholdPercent: criticalThreshold,
		CurrentUsageMicroCents:   0,
		LastResetAt:              time.Now(),
		PauseRelay:               false,
		NotifyAdmin:              true,
		ReduceFrequency:          false,
	}

	err := rbs.costRepo.CreateRelayBudget(ctx, budget)
	if err != nil {
		rbs.logger.Error("failed to create relay budget",
			zap.String("relay_url", relayURL),
			zap.String("period", period),
			zap.Int64("limit_micro_cents", limitMicroCents),
			zap.Error(err))
		return errors.Join(ErrRelayBudgetCreationFailed, err)
	}

	rbs.logger.Info("created relay budget",
		zap.String("relay_url", relayURL),
		zap.String("period", period),
		zap.Int64("limit_micro_cents", limitMicroCents),
		zap.Float64(storage.FieldWarningThreshold, warningThreshold),
		zap.Float64("critical_threshold", criticalThreshold))

	return nil
}

// UpdateRelayBudgetUsage updates the current usage for a relay budget
func (rbs *RelayBudgetService) UpdateRelayBudgetUsage(ctx context.Context, relayURL, period string, additionalCostMicroCents int64) error {
	// Get existing budget
	budget, err := rbs.costRepo.GetRelayBudget(ctx, relayURL, period)
	if err != nil {
		// No budget configured - nothing to update
		return nil
	}

	// Check if budget period needs reset
	if rbs.shouldResetBudget(budget) {
		budget.CurrentUsageMicroCents = 0
		budget.LastResetAt = time.Now()
		budget.WarningAlertSent = false
		budget.CriticalAlertSent = false
		budget.BudgetExceeded = false
		budget.CurrentUsagePercent = 0

		rbs.logger.Info("reset relay budget for new period",
			zap.String("relay_url", relayURL),
			zap.String("period", period))
	}

	// Update usage
	budget.CurrentUsageMicroCents += additionalCostMicroCents
	if budget.LimitMicroCents > 0 {
		budget.CurrentUsagePercent = float64(budget.CurrentUsageMicroCents) / float64(budget.LimitMicroCents) * 100.0
		budget.BudgetExceeded = budget.CurrentUsageMicroCents > budget.LimitMicroCents
	}

	// Check thresholds and send alerts
	if err := rbs.checkBudgetThresholds(ctx, budget); err != nil {
		rbs.logger.Error("failed to check budget thresholds", zap.Error(err))
	}

	// Update budget in storage
	return rbs.costRepo.UpdateRelayBudget(ctx, budget)
}

// GetRelayBudgetStatus returns the current budget status for a relay
func (rbs *RelayBudgetService) GetRelayBudgetStatus(ctx context.Context, relayURL, period string) (*RelayBudgetStatus, error) {
	budget, err := rbs.costRepo.GetRelayBudget(ctx, relayURL, period)
	if err != nil {
		return &RelayBudgetStatus{
			RelayURL:       relayURL,
			Period:         period,
			HasBudget:      false,
			BudgetExceeded: false,
		}, nil
	}

	// Check if budget needs reset
	if rbs.shouldResetBudget(budget) {
		budget.CurrentUsageMicroCents = 0
		budget.LastResetAt = time.Now()
		budget.WarningAlertSent = false
		budget.CriticalAlertSent = false
		budget.BudgetExceeded = false
		budget.CurrentUsagePercent = 0

		// Update in storage
		if updateErr := rbs.costRepo.UpdateRelayBudget(ctx, budget); updateErr != nil {
			rbs.logger.Error("failed to reset budget", zap.Error(updateErr))
		}
	}

	return &RelayBudgetStatus{
		RelayURL:                 relayURL,
		Period:                   period,
		HasBudget:                true,
		LimitMicroCents:          budget.LimitMicroCents,
		CurrentUsageMicroCents:   budget.CurrentUsageMicroCents,
		UsagePercent:             budget.CurrentUsagePercent,
		BudgetExceeded:           budget.BudgetExceeded,
		WarningThresholdReached:  budget.CurrentUsagePercent >= budget.WarningThresholdPercent,
		CriticalThresholdReached: budget.CurrentUsagePercent >= budget.CriticalThresholdPercent,
		PauseRelay:               budget.PauseRelay,
		ReduceFrequency:          budget.ReduceFrequency,
		LastResetAt:              budget.LastResetAt,
	}, nil
}

// CheckRelayBudget checks if a relay operation would exceed budget
func (rbs *RelayBudgetService) CheckRelayBudget(ctx context.Context, relayURL string, estimatedCostMicroCents int64) error {
	// Check daily budget first (most common)
	dailyStatus, err := rbs.GetRelayBudgetStatus(ctx, relayURL, "daily")
	if err != nil {
		rbs.logger.Debug("failed to get daily budget status", zap.Error(err))
	} else if dailyStatus.HasBudget {
		if dailyStatus.BudgetExceeded {
			rbs.logger.Error("daily budget already exceeded for relay",
				zap.String("relay_url", relayURL),
				zap.String("period", "daily"),
				zap.Int64("current_usage_micro_cents", dailyStatus.CurrentUsageMicroCents),
				zap.Int64("limit_micro_cents", dailyStatus.LimitMicroCents))
			return ErrRelayBudgetAlreadyExceeded
		}

		if dailyStatus.CurrentUsageMicroCents+estimatedCostMicroCents > dailyStatus.LimitMicroCents {
			rbs.logger.Error("operation would exceed daily budget",
				zap.String("relay_url", relayURL),
				zap.String("period", "daily"),
				zap.Int64("current_usage_micro_cents", dailyStatus.CurrentUsageMicroCents),
				zap.Int64("estimated_cost_micro_cents", estimatedCostMicroCents),
				zap.Int64("limit_micro_cents", dailyStatus.LimitMicroCents),
				zap.String("operation", "budget_check"))
			return ErrRelayBudgetExceeded
		}

		if dailyStatus.PauseRelay {
			rbs.logger.Warn("relay operations paused due to budget limit",
				zap.String("relay_url", relayURL),
				zap.String("period", "daily"),
				zap.Float64("usage_percent", dailyStatus.UsagePercent))
			return ErrRelayOperationsPaused
		}
	}

	// Check monthly budget
	monthlyStatus, err := rbs.GetRelayBudgetStatus(ctx, relayURL, "monthly")
	if err != nil {
		rbs.logger.Debug("failed to get monthly budget status", zap.Error(err))
	} else if monthlyStatus.HasBudget {
		if monthlyStatus.BudgetExceeded {
			rbs.logger.Error("monthly budget already exceeded for relay",
				zap.String("relay_url", relayURL),
				zap.String("period", "monthly"),
				zap.Int64("current_usage_micro_cents", monthlyStatus.CurrentUsageMicroCents),
				zap.Int64("limit_micro_cents", monthlyStatus.LimitMicroCents))
			return ErrRelayBudgetAlreadyExceeded
		}

		if monthlyStatus.CurrentUsageMicroCents+estimatedCostMicroCents > monthlyStatus.LimitMicroCents {
			rbs.logger.Error("operation would exceed monthly budget",
				zap.String("relay_url", relayURL),
				zap.String("period", "monthly"),
				zap.Int64("current_usage_micro_cents", monthlyStatus.CurrentUsageMicroCents),
				zap.Int64("estimated_cost_micro_cents", estimatedCostMicroCents),
				zap.Int64("limit_micro_cents", monthlyStatus.LimitMicroCents),
				zap.String("operation", "budget_check"))
			return ErrRelayBudgetExceeded
		}
	}

	return nil
}

// AggregateRelayCosts aggregates relay costs into metrics for budget tracking
func (rbs *RelayBudgetService) AggregateRelayCosts(ctx context.Context, relayURL string) error {
	return rbs.aggregateRelayCostsAt(ctx, relayURL, time.Now())
}

func (rbs *RelayBudgetService) aggregateRelayCostsAt(ctx context.Context, relayURL string, now time.Time) error {

	// Aggregate daily costs
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	if err := rbs.costRepo.AggregateRelayCosts(ctx, relayURL, "daily", dayStart, dayEnd); err != nil {
		rbs.logger.Error("failed to aggregate daily relay costs",
			zap.String("relay_url", relayURL),
			zap.Error(err))
	}

	// Aggregate weekly costs (if it's Sunday)
	if now.Weekday() == time.Sunday {
		weekStart := dayStart.AddDate(0, 0, -6) // 7 days ago
		if err := rbs.costRepo.AggregateRelayCosts(ctx, relayURL, "weekly", weekStart, dayEnd); err != nil {
			rbs.logger.Error("failed to aggregate weekly relay costs",
				zap.String("relay_url", relayURL),
				zap.Error(err))
		}
	}

	// Aggregate monthly costs (if it's first day of month)
	if now.Day() == 1 {
		monthStart := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		monthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		if err := rbs.costRepo.AggregateRelayCosts(ctx, relayURL, "monthly", monthStart, monthEnd); err != nil {
			rbs.logger.Error("failed to aggregate monthly relay costs",
				zap.String("relay_url", relayURL),
				zap.Error(err))
		}
	}

	return nil
}

// GetRelayBudgetRecommendations provides budget optimization recommendations
func (rbs *RelayBudgetService) GetRelayBudgetRecommendations(ctx context.Context, relayURL string) (*RelayBudgetRecommendations, error) {
	// Get recent cost summary
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -30) // Last 30 days

	summary, err := rbs.costRepo.GetRelayCostSummary(ctx, relayURL, startTime, endTime)
	if err != nil {
		rbs.logger.Error("failed to get relay cost summary",
			zap.String("relay_url", relayURL),
			zap.Time("start_time", startTime),
			zap.Time("end_time", endTime),
			zap.String("operation", "cost_analysis"),
			zap.Error(err))
		return nil, errors.Join(ErrRelayCostSummaryFailed, err)
	}

	recommendations := &RelayBudgetRecommendations{
		RelayURL:         relayURL,
		AnalysisPeriod:   "30 days",
		Recommendations:  []string{},
		EstimatedSavings: 0,
	}

	// Analyze costs and provide recommendations
	if summary.Count == 0 {
		recommendations.Recommendations = append(recommendations.Recommendations,
			"No relay activity found in the last 30 days. Consider removing unused relay.")
		return recommendations, nil
	}

	// Success rate analysis
	if summary.SuccessRate < 0.8 {
		recommendations.Recommendations = append(recommendations.Recommendations,
			fmt.Sprintf("Low success rate (%.1f%%). Consider reviewing relay configuration or removing unreliable relay.",
				summary.SuccessRate*100))
	}

	// Cost efficiency analysis
	avgCostPerOp := summary.AverageCostPerOperation
	if avgCostPerOp > 0.001 { // More than $0.001 per operation
		recommendations.Recommendations = append(recommendations.Recommendations,
			fmt.Sprintf("High cost per operation ($%.6f). Consider optimizing payload sizes or reducing forwarding frequency.",
				avgCostPerOp))
		recommendations.EstimatedSavings = int64(float64(summary.TotalCostMicroCents) * 0.2) // 20% potential savings
	}

	// Volume analysis
	dailyOps := float64(summary.TotalOperations) / 30.0
	if dailyOps > 1000 {
		recommendations.Recommendations = append(recommendations.Recommendations,
			fmt.Sprintf("High daily operation volume (%.0f ops/day). Consider implementing batching or reducing forwarding scope.",
				dailyOps))
	}

	// Budget recommendations based on current usage
	dailyCost := summary.TotalCostMicroCents / 30
	monthlyCost := dailyCost * 30

	recommendations.Recommendations = append(recommendations.Recommendations,
		fmt.Sprintf("Suggested daily budget: %d microcents ($%.6f)",
			dailyCost*12/10, float64(dailyCost*12/10)/1_000_000)) // 20% buffer

	recommendations.Recommendations = append(recommendations.Recommendations,
		fmt.Sprintf("Suggested monthly budget: %d microcents ($%.6f)",
			monthlyCost*12/10, float64(monthlyCost*12/10)/1_000_000)) // 20% buffer

	if len(recommendations.Recommendations) == 2 {
		recommendations.Recommendations = append([]string{
			"Relay performance looks good! Current costs are within reasonable limits.",
		}, recommendations.Recommendations...)
	}

	return recommendations, nil
}

// Private helper methods

// shouldResetBudget checks if the budget period has passed and needs reset
func (rbs *RelayBudgetService) shouldResetBudget(budget *models.RelayBudget) bool {
	now := time.Now()

	switch budget.Period {
	case "daily":
		// Reset if it's a new day
		return budget.LastResetAt.Day() != now.Day() ||
			budget.LastResetAt.Month() != now.Month() ||
			budget.LastResetAt.Year() != now.Year()

	case "weekly":
		// Reset if it's been more than 7 days
		return now.Sub(budget.LastResetAt) >= 7*24*time.Hour

	case "monthly":
		// Reset if it's a new month
		return budget.LastResetAt.Month() != now.Month() ||
			budget.LastResetAt.Year() != now.Year()

	default:
		return false
	}
}

// checkBudgetThresholds checks and handles budget threshold alerts
func (rbs *RelayBudgetService) checkBudgetThresholds(_ context.Context, budget *models.RelayBudget) error {
	usagePercent := budget.CurrentUsagePercent

	// Check warning threshold
	if usagePercent >= budget.WarningThresholdPercent && !budget.WarningAlertSent {
		rbs.logger.Warn("relay budget warning threshold reached",
			zap.String("relay_url", budget.RelayURL),
			zap.String("period", budget.Period),
			zap.Float64("usage_percent", usagePercent),
			zap.Float64(storage.FieldWarningThreshold, budget.WarningThresholdPercent))

		budget.WarningAlertSent = true

		// Send notification if enabled
		if budget.NotifyAdmin {
			// In a full implementation, you'd send an actual notification here
			rbs.logger.Info("would send warning notification to admin",
				zap.String("relay_url", budget.RelayURL),
				zap.Float64("usage_percent", usagePercent))
		}
	}

	// Check critical threshold
	if usagePercent >= budget.CriticalThresholdPercent && !budget.CriticalAlertSent {
		rbs.logger.Error("relay budget critical threshold reached",
			zap.String("relay_url", budget.RelayURL),
			zap.String("period", budget.Period),
			zap.Float64("usage_percent", usagePercent),
			zap.Float64("critical_threshold", budget.CriticalThresholdPercent))

		budget.CriticalAlertSent = true

		// Take configured actions
		if budget.PauseRelay {
			rbs.logger.Warn("pausing relay due to budget threshold",
				zap.String("relay_url", budget.RelayURL))
		}

		if budget.ReduceFrequency {
			rbs.logger.Info("reducing relay forwarding frequency",
				zap.String("relay_url", budget.RelayURL))
		}

		// Send critical notification
		if budget.NotifyAdmin {
			rbs.logger.Error("would send critical alert to admin",
				zap.String("relay_url", budget.RelayURL),
				zap.Float64("usage_percent", usagePercent))
		}
	}

	return nil
}

// Supporting types

// RelayBudgetStatus represents the current budget status for a relay
type RelayBudgetStatus struct {
	RelayURL                 string    `json:"relay_url"`
	Period                   string    `json:"period"`
	HasBudget                bool      `json:"has_budget"`
	LimitMicroCents          int64     `json:"limit_micro_cents"`
	CurrentUsageMicroCents   int64     `json:"current_usage_micro_cents"`
	UsagePercent             float64   `json:"usage_percent"`
	BudgetExceeded           bool      `json:"budget_exceeded"`
	WarningThresholdReached  bool      `json:"warning_threshold_reached"`
	CriticalThresholdReached bool      `json:"critical_threshold_reached"`
	PauseRelay               bool      `json:"pause_relay"`
	ReduceFrequency          bool      `json:"reduce_frequency"`
	LastResetAt              time.Time `json:"last_reset_at"`
}

// RelayBudgetRecommendations provides budget optimization recommendations
type RelayBudgetRecommendations struct {
	RelayURL         string   `json:"relay_url"`
	AnalysisPeriod   string   `json:"analysis_period"`
	Recommendations  []string `json:"recommendations"`
	EstimatedSavings int64    `json:"estimated_savings_micro_cents"`
}
