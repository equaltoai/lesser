package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// FederationCostRepository handles federation cost tracking operations using DynamORM with BaseRepository
type FederationCostRepository struct {
	*BaseRepository[*models.FederationCostTracking]
	budgetRepo *BaseRepository[*models.FederationBudget]
}

// NewFederationCostRepository creates a new federation cost repository
func NewFederationCostRepository(baseRepo *BaseRepository[*models.FederationCostTracking], budgetRepo *BaseRepository[*models.FederationBudget]) *FederationCostRepository {
	return &FederationCostRepository{
		BaseRepository: baseRepo,
		budgetRepo:     budgetRepo,
	}
}

// NewFederationCostRepositoryWithCostTracking creates a new federation cost repository with integrated cost tracking
func NewFederationCostRepositoryWithCostTracking(baseRepo *BaseRepository[*models.FederationCostTracking], budgetRepo *BaseRepository[*models.FederationBudget], costService *cost.TrackingService) *FederationCostRepository {
	// Set cost service on base repositories
	baseRepo.SetCostService(costService)
	baseRepo.SetRepoName("federation_cost")
	budgetRepo.SetCostService(costService)
	budgetRepo.SetRepoName("federation_budget")

	return &FederationCostRepository{
		BaseRepository: baseRepo,
		budgetRepo:     budgetRepo,
	}
}

// RecordFederationCost records a federation cost tracking entry using BaseRepository
func (r *FederationCostRepository) RecordFederationCost(ctx context.Context, cost *models.FederationCostTracking) error {
	err := r.Create(ctx, cost)
	if err != nil {
		r.logger.Error("Failed to record federation cost",
			zap.String("domain", cost.Domain),
			zap.String("activity_type", cost.ActivityType),
			zap.String("activity_id", cost.ActivityID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFederationCostRecordFailed, err)
	}

	r.logger.Debug("Recorded federation cost",
		zap.String("domain", cost.Domain),
		zap.String("activity_type", cost.ActivityType),
		zap.Int64("total_cost_micro_cents", cost.TotalCostMicroCents),
		zap.Bool("success", cost.Success))

	return nil
}

// GetFederationCosts retrieves federation costs for a domain within a time range
// PRESERVED: Critical cost tracking business logic - time-based GSI queries with domain filtering
func (r *FederationCostRepository) GetFederationCosts(ctx context.Context, domain string, startTime, endTime time.Time, limit int) ([]*models.FederationCostTracking, error) {
	var costs []*models.FederationCostTracking

	// Use GSI1 for time-based queries
	startDate := startTime.Format(common.CompactDateFormat)
	endDate := endTime.Format(common.CompactDateFormat)

	query := r.GetDB().WithContext(ctx).Model(&models.FederationCostTracking{}).
		Index("GSI1").
		Where("GSI1PK", ">=", fmt.Sprintf("FED_COSTS#%s", startDate)).
		Where("GSI1PK", "<=", fmt.Sprintf("FED_COSTS#%s", endDate)).
		Filter("GSI1SK", "CONTAINS", domain). // Filter for specific domain
		Limit(limit)

	err := query.All(&costs)
	if err != nil {
		r.logger.Error("Failed to get federation costs",
			zap.String("domain", domain),
			zap.Time("start_time", startTime),
			zap.Time("end_time", endTime),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrFederationCostQueryFailed, err)
	}

	return costs, nil
}

// GetFederationCostsByActivityType retrieves federation costs by activity type within a time range
// PRESERVED: Critical cost tracking business logic - activity type GSI queries for cost analysis
func (r *FederationCostRepository) GetFederationCostsByActivityType(ctx context.Context, activityType string, startTime, endTime time.Time, limit int) ([]*models.FederationCostTracking, error) {
	var costs []*models.FederationCostTracking

	// Use GSI2 for activity type queries
	timestampStart := startTime.Format(common.CompactTimeFormat)
	timestampEnd := endTime.Format(common.CompactTimeFormat)

	query := r.GetDB().WithContext(ctx).Model(&models.FederationCostTracking{}).
		Index("GSI2").
		Where("GSI2PK", "=", fmt.Sprintf("FED_TYPE#%s", activityType)).
		Where("GSI2SK", ">=", fmt.Sprintf("DOMAIN#%s", timestampStart)).
		Where("GSI2SK", "<=", fmt.Sprintf("DOMAIN#%s", timestampEnd)).
		Limit(limit)

	err := query.All(&costs)
	if err != nil {
		r.logger.Error("Failed to get federation costs by activity type",
			zap.String("activity_type", activityType),
			zap.Time("start_time", startTime),
			zap.Time("end_time", endTime),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrFederationCostActivityQueryFailed, err)
	}

	return costs, nil
}

// GetDailyCostSummary retrieves daily cost summary for a domain
// PRESERVED: Critical cost tracking business logic - complex cost aggregation and analytics
func (r *FederationCostRepository) GetDailyCostSummary(ctx context.Context, domain string, date time.Time) (*DailyCostSummary, error) {
	// Get all costs for the domain on the specific date
	startTime := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endTime := startTime.Add(24 * time.Hour)

	costs, err := r.GetFederationCosts(ctx, domain, startTime, endTime, 10000) // Get all costs for the day
	if err != nil {
		return nil, err
	}

	summary := &DailyCostSummary{
		Domain:                domain,
		Date:                  date,
		TotalActivities:       int64(len(costs)),
		ActivityTypeBreakdown: make(map[string]*ActivityTypeCostStats),
	}

	var totalCost int64
	var totalDataTransfer int64
	var totalResponseTime int64
	var successCount int64

	for _, cost := range costs {
		totalCost += cost.TotalCostMicroCents
		totalDataTransfer += cost.DataTransferBytes
		totalResponseTime += cost.ResponseTimeMs

		if cost.Success {
			successCount++
		}

		// Update activity type breakdown
		if summary.ActivityTypeBreakdown[cost.ActivityType] == nil {
			summary.ActivityTypeBreakdown[cost.ActivityType] = &ActivityTypeCostStats{
				ActivityType: cost.ActivityType,
			}
		}

		stats := summary.ActivityTypeBreakdown[cost.ActivityType]
		stats.Count++
		stats.TotalCostMicroCents += cost.TotalCostMicroCents
		stats.TotalDataTransferred += cost.DataTransferBytes
		if cost.Success {
			stats.SuccessCount++
		} else {
			stats.FailureCount++
		}
	}

	summary.TotalCostMicroCents = totalCost
	summary.TotalDataTransferBytes = totalDataTransfer
	if summary.TotalActivities > 0 {
		summary.AverageResponseTime = float64(totalResponseTime) / float64(summary.TotalActivities)
		summary.SuccessRate = float64(successCount) / float64(summary.TotalActivities)
		summary.AverageCostPerActivity = float64(totalCost) / float64(summary.TotalActivities)
	}

	// Calculate activity type success rates and averages
	for _, stats := range summary.ActivityTypeBreakdown {
		if stats.Count > 0 {
			stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.Count)
			stats.AverageCostMicroCents = stats.TotalCostMicroCents / stats.Count
			stats.AverageResponseTime = float64(totalResponseTime) / float64(stats.Count)
		}
	}

	return summary, nil
}

// CreateOrUpdateBudget creates or updates a federation budget for a domain using BaseRepository
func (r *FederationCostRepository) CreateOrUpdateBudget(ctx context.Context, budget *models.FederationBudget) error {
	err := r.budgetRepo.Create(ctx, budget)
	if err != nil {
		r.logger.Error("Failed to create/update federation budget",
			zap.String("domain", budget.Domain),
			zap.String("period", budget.Period),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFederationBudgetCreateFailed, err)
	}

	r.logger.Info("Created/updated federation budget",
		zap.String("domain", budget.Domain),
		zap.String("period", budget.Period),
		zap.Int64("combined_limit", budget.CombinedLimitMicroCents))

	return nil
}

// GetBudget retrieves a federation budget for a domain and period using BaseRepository
func (r *FederationCostRepository) GetBudget(ctx context.Context, domain, period string) (*models.FederationBudget, error) {
	budget := &models.FederationBudget{}
	pk := fmt.Sprintf("FED_BUDGET#%s#%s", domain, period)
	sk := models.SKConfig

	err := r.budgetRepo.Get(ctx, pk, sk, budget)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: domain %s period %s", ErrFederationBudgetNotFound, domain, period)
		}
		r.logger.Error("Failed to get federation budget",
			zap.String("domain", domain),
			zap.String("period", period),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrFederationBudgetQueryFailed, err)
	}

	return budget, nil
}

// UpdateBudgetUsage updates the usage for a federation budget
// PRESERVED: Critical budget enforcement logic - cost attribution, limit checking, status updates
func (r *FederationCostRepository) UpdateBudgetUsage(ctx context.Context, domain, period, activityType, direction string, cost int64) error {
	// Get existing budget
	budget, err := r.GetBudget(ctx, domain, period)
	if err != nil {
		// If budget doesn't exist, create a default one
		// Check if it's a "not found" error by looking for our ErrFederationBudgetNotFound
		if fmt.Sprintf("%v", err) == fmt.Sprintf("%s: domain %s period %s", ErrFederationBudgetNotFound.Error(), domain, period) {
			budget = r.createDefaultBudget(domain, period)
		} else {
			return err
		}
	}

	// Add usage
	budget.AddUsage(activityType, direction, cost)

	// Update status based on usage
	if budget.IsOverCombinedLimit() {
		budget.Status = "over_limit"
	} else if budget.GetCombinedUsagePercent() >= budget.AlertThresholdPercent {
		budget.Status = StatusWarning
	} else {
		budget.Status = "active"
	}

	// Save updated budget
	return r.CreateOrUpdateBudget(ctx, budget)
}

// GetActiveBudgets retrieves all active budgets
// PRESERVED: Critical budget monitoring - GSI queries for active budget tracking
func (r *FederationCostRepository) GetActiveBudgets(ctx context.Context, limit int) ([]*models.FederationBudget, error) {
	var budgets []*models.FederationBudget

	query := r.GetDB().WithContext(ctx).Model(&models.FederationBudget{}).
		Index("GSI1").
		Where("GSI1PK", "=", "ACTIVE_BUDGETS").
		Filter("IsActive", "=", true).
		Limit(limit)

	err := query.All(&budgets)
	if err != nil {
		r.logger.Error("Failed to get active budgets", zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrActiveBudgetsQueryFailed, err)
	}

	return budgets, nil
}

// GetBudgetsOverLimit retrieves budgets that are over their limits
// PRESERVED: Critical budget monitoring - budget limit violation detection for cost control
func (r *FederationCostRepository) GetBudgetsOverLimit(ctx context.Context, limit int) ([]*models.FederationBudget, error) {
	budgets, err := r.GetActiveBudgets(ctx, 1000) // Get all active budgets
	if err != nil {
		return nil, err
	}

	var overLimitBudgets []*models.FederationBudget
	for _, budget := range budgets {
		if budget.IsOverCombinedLimit() && len(overLimitBudgets) < limit {
			overLimitBudgets = append(overLimitBudgets, budget)
		}
	}

	return overLimitBudgets, nil
}

// GetBudgetsNeedingAlerts retrieves budgets that need alerts sent
// PRESERVED: Critical budget monitoring - alert threshold detection for cost management
func (r *FederationCostRepository) GetBudgetsNeedingAlerts(ctx context.Context, limit int) ([]*models.FederationBudget, error) {
	budgets, err := r.GetActiveBudgets(ctx, 1000) // Get all active budgets
	if err != nil {
		return nil, err
	}

	var alertBudgets []*models.FederationBudget
	for _, budget := range budgets {
		if budget.ShouldSendAlert() && len(alertBudgets) < limit {
			alertBudgets = append(alertBudgets, budget)
		}
	}

	return alertBudgets, nil
}

// CheckBudgetLimits checks if an activity would exceed budget limits
// PRESERVED: CRITICAL budget enforcement logic - this is the core cost control mechanism for federation
func (r *FederationCostRepository) CheckBudgetLimits(ctx context.Context, domain, period, activityType, _ string, estimatedCost int64) (*BudgetCheckResult, error) {
	budget, err := r.GetBudget(ctx, domain, period)
	if err != nil {
		// If no budget exists, allow the activity but warn
		// Check if it's a "not found" error by looking for our ErrFederationBudgetNotFound
		if fmt.Sprintf("%v", err) == fmt.Sprintf("%s: domain %s period %s", ErrFederationBudgetNotFound.Error(), domain, period) {
			return &BudgetCheckResult{
				Allowed:      true,
				WarningLevel: "info",
				Message:      "No budget configured for domain",
				CurrentUsage: 0,
				LimitPercent: 0,
			}, nil
		}
		return nil, err
	}

	// Check if adding this cost would exceed limits
	projectedCombinedCost := budget.CurrentCombinedCost + estimatedCost
	projectedUsagePercent := float64(projectedCombinedCost) / float64(budget.CombinedLimitMicroCents) * 100.0

	result := &BudgetCheckResult{
		CurrentUsage:          budget.CurrentCombinedCost,
		LimitAmount:           budget.CombinedLimitMicroCents,
		LimitPercent:          budget.GetCombinedUsagePercent(),
		ProjectedUsagePercent: projectedUsagePercent,
	}

	// Check activity type specific limits
	if limit, exists := budget.ActivityTypeLimits[activityType]; exists {
		activityUsage := budget.ActivityTypeUsage[activityType]
		projectedActivityUsage := activityUsage + estimatedCost
		activityUsagePercent := float64(projectedActivityUsage) / float64(limit) * 100.0

		if projectedActivityUsage >= limit {
			result.Allowed = false
			result.WarningLevel = StatusError
			result.Message = fmt.Sprintf("Activity type %s would exceed limit (%d%%)", activityType, int(activityUsagePercent))
			return result, nil
		}
	}

	// Check combined limits
	if projectedCombinedCost >= budget.CombinedLimitMicroCents {
		result.Allowed = false
		result.WarningLevel = StatusError
		result.Message = fmt.Sprintf("Combined budget would be exceeded (%d%%)", int(projectedUsagePercent))
		return result, nil
	}

	// Check if we should rate limit
	if budget.RateLimitOnThreshold && projectedUsagePercent >= budget.AlertThresholdPercent {
		result.Allowed = true
		result.ShouldRateLimit = true
		result.WarningLevel = StatusWarning
		result.Message = fmt.Sprintf("Budget usage high (%d%%), rate limiting recommended", int(projectedUsagePercent))
		return result, nil
	}

	// Check if we should send alerts
	if projectedUsagePercent >= budget.AlertThresholdPercent {
		result.Allowed = true
		result.WarningLevel = StatusWarning
		result.Message = fmt.Sprintf("Budget usage approaching limit (%d%%)", int(projectedUsagePercent))
		return result, nil
	}

	// All good
	result.Allowed = true
	result.WarningLevel = "info"
	result.Message = fmt.Sprintf("Budget usage normal (%d%%)", int(projectedUsagePercent))

	return result, nil
}

// ResetPeriodBudgets resets budget usage for a new period
// PRESERVED: Critical budget lifecycle management - period reset functionality for cost control cycles
func (r *FederationCostRepository) ResetPeriodBudgets(ctx context.Context, period string, newPeriodStart, newPeriodEnd time.Time) error {
	budgets, err := r.GetActiveBudgets(ctx, 10000) // Get all active budgets
	if err != nil {
		return err
	}

	var resetCount int
	for _, budget := range budgets {
		if budget.Period == period {
			budget.ResetPeriod(newPeriodStart, newPeriodEnd)
			if err := r.CreateOrUpdateBudget(ctx, budget); err != nil {
				r.logger.Error("Failed to reset budget period",
					zap.String("domain", budget.Domain),
					zap.String("period", period),
					zap.Error(err))
				continue
			}
			resetCount++
		}
	}

	r.logger.Info("Reset period budgets",
		zap.String("period", period),
		zap.Int("reset_count", resetCount),
		zap.Time("new_period_start", newPeriodStart),
		zap.Time("new_period_end", newPeriodEnd))

	return nil
}

// createDefaultBudget creates a default budget for a domain and period
// PRESERVED: Critical budget initialization logic - default cost limits aligned with <$0.01 per user per month target
func (r *FederationCostRepository) createDefaultBudget(domain, period string) *models.FederationBudget {
	now := time.Now()

	// Default limits (in microcents) - these are conservative defaults aligned with cost target
	var inboundLimit, outboundLimit, combinedLimit int64
	var periodStart, periodEnd time.Time

	switch period {
	case PeriodDaily:
		inboundLimit = 10000   // $0.01 per day inbound
		outboundLimit = 50000  // $0.05 per day outbound
		combinedLimit = 100000 // $0.10 per day combined
		periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.Add(24 * time.Hour)
	case PeriodWeekly:
		inboundLimit = 70000   // $0.07 per week inbound
		outboundLimit = 350000 // $0.35 per week outbound
		combinedLimit = 700000 // $0.70 per week combined
		// Start of week (Sunday)
		weekday := int(now.Weekday())
		periodStart = now.AddDate(0, 0, -weekday)
		periodStart = time.Date(periodStart.Year(), periodStart.Month(), periodStart.Day(), 0, 0, 0, 0, periodStart.Location())
		periodEnd = periodStart.Add(7 * 24 * time.Hour)
	case PeriodMonthly:
		inboundLimit = 300000   // $0.30 per month inbound
		outboundLimit = 1500000 // $1.50 per month outbound
		combinedLimit = 3000000 // $3.00 per month combined
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 1, 0)
	default:
		// Default to daily
		inboundLimit = 10000
		outboundLimit = 50000
		combinedLimit = 100000
		periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.Add(24 * time.Hour)
	}

	return &models.FederationBudget{
		Domain:                  domain,
		Period:                  period,
		InboundLimitMicroCents:  inboundLimit,
		OutboundLimitMicroCents: outboundLimit,
		CombinedLimitMicroCents: combinedLimit,
		PeriodStart:             periodStart,
		PeriodEnd:               periodEnd,
		AlertThresholdPercent:   75.0, // Alert at 75% usage
		AlertSendingEnabled:     true,
		BlockOnLimitExceeded:    false, // Don't block by default
		RateLimitOnThreshold:    true,  // Rate limit at threshold
		IsActive:                true,
		Status:                  models.StatusActive,
		ActivityTypeLimits:      make(map[string]int64),
		ActivityTypeUsage:       make(map[string]int64),
	}
}

// Helper types for cost summaries and budget checks

// DailyCostSummary represents a daily cost summary for a domain
type DailyCostSummary struct {
	Domain                 string                            `json:"domain"`
	Date                   time.Time                         `json:"date"`
	TotalActivities        int64                             `json:"total_activities"`
	TotalCostMicroCents    int64                             `json:"total_cost_micro_cents"`
	TotalDataTransferBytes int64                             `json:"total_data_transfer_bytes"`
	AverageResponseTime    float64                           `json:"average_response_time"`
	SuccessRate            float64                           `json:"success_rate"`
	AverageCostPerActivity float64                           `json:"average_cost_per_activity"`
	ActivityTypeBreakdown  map[string]*ActivityTypeCostStats `json:"activity_type_breakdown"`
}

// ActivityTypeCostStats represents cost statistics for a specific activity type
type ActivityTypeCostStats struct {
	ActivityType          string  `json:"activity_type"`
	Count                 int64   `json:"count"`
	SuccessCount          int64   `json:"success_count"`
	FailureCount          int64   `json:"failure_count"`
	SuccessRate           float64 `json:"success_rate"`
	TotalCostMicroCents   int64   `json:"total_cost_micro_cents"`
	AverageCostMicroCents int64   `json:"average_cost_micro_cents"`
	TotalDataTransferred  int64   `json:"total_data_transferred"`
	AverageResponseTime   float64 `json:"average_response_time"`
}

// BudgetCheckResult represents the result of a budget check
type BudgetCheckResult struct {
	Allowed               bool    `json:"allowed"`
	ShouldRateLimit       bool    `json:"should_rate_limit"`
	WarningLevel          string  `json:"warning_level"` // info, warning, error
	Message               string  `json:"message"`
	CurrentUsage          int64   `json:"current_usage"`
	LimitAmount           int64   `json:"limit_amount"`
	LimitPercent          float64 `json:"limit_percent"`
	ProjectedUsagePercent float64 `json:"projected_usage_percent"`
}
