package repositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// NotificationCostRepository handles notification cost tracking operations using enhanced patterns
type NotificationCostRepository struct {
	*EnhancedBaseRepository[*models.NotificationCostTracking]
	logger     *zap.Logger
	costHelper *CostTrackingQueryHelper
}

// NewNotificationCostRepository creates a new notification cost repository with enhanced functionality
func NewNotificationCostRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *NotificationCostRepository {
	// Create enhanced repository optimized for notification cost operations
	enhancedRepo := NewEnhancedBaseRepository[*models.NotificationCostTracking](db, tableName, logger, costService, "NotificationCostRepository", "notificationcost")

	// Set up enhanced services for notification cost operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cost data cached for analytics
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for cost monitoring

	return &NotificationCostRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
		costHelper:             NewCostTrackingQueryHelper(db, tableName, logger),
	}
}

// CreateCostTracking creates a new notification cost tracking record
func (r *NotificationCostRepository) CreateCostTracking(ctx context.Context, tracking *models.NotificationCostTracking) error {
	if err := tracking.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "notification cost", "preparation")
	}

	err := r.ValidateAndCreate(ctx, tracking)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "notification cost", tracking.ID)
	}

	r.logger.Debug("created notification cost tracking",
		zap.String("id", tracking.ID),
		zap.String("notification_id", tracking.NotificationID),
		zap.String("delivery_method", tracking.DeliveryMethod),
		zap.Int64("total_cost_micro_cents", tracking.TotalCostMicroCents),
		zap.Bool("success", tracking.Success))

	return nil
}

// GetCostTrackingByNotification retrieves cost tracking records for a notification
func (r *NotificationCostRepository) GetCostTrackingByNotification(ctx context.Context, notificationID string, limit int) ([]*models.NotificationCostTracking, error) {
	var trackingRecords []*models.NotificationCostTracking

	pk := fmt.Sprintf("NOTIF_COST#%s", notificationID)

	query := r.GetDB().WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&trackingRecords)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "notification cost", "by notification")
	}

	return trackingRecords, nil
}

// GetCostTrackingByUser retrieves cost tracking records for a user within a time range
func (r *NotificationCostRepository) GetCostTrackingByUser(ctx context.Context, username string, startTime, endTime time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	gsiPKValue := fmt.Sprintf("USER#%s", username)
	return r.costHelper.GetCostTrackingByTimeRange(ctx, "gsi1", gsiPKValue, "COST", startTime, endTime, limit)
}

// GetCostTrackingByMethod retrieves cost tracking records by delivery method within a time range
func (r *NotificationCostRepository) GetCostTrackingByMethod(ctx context.Context, deliveryMethod string, startTime, endTime time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	gsiPKValue := fmt.Sprintf("METHOD#%s", deliveryMethod)
	return r.costHelper.GetCostTrackingByTimeRange(ctx, "gsi2", gsiPKValue, "TIMESTAMP", startTime, endTime, limit)
}

// GetDailyCostTracking retrieves cost tracking records for a specific date
func (r *NotificationCostRepository) GetDailyCostTracking(ctx context.Context, date time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	var trackingRecords []*models.NotificationCostTracking

	dateStr := date.Format(common.CompactDateFormat)

	query := r.GetDB().WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Index("gsi3").
		Where("GSI3PK", "=", fmt.Sprintf("DAILY#%s", dateStr)).
		OrderBy("GSI3SK", "DESC").
		Limit(limit)

	err := query.All(&trackingRecords)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "notification cost", "daily tracking")
	}

	return trackingRecords, nil
}

// CreateAggregation creates a notification cost aggregation record
func (r *NotificationCostRepository) CreateAggregation(ctx context.Context, aggregation *models.NotificationCostAggregation) error {
	if err := aggregation.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "cost aggregation", "preparation")
	}

	err := r.GetDB().WithContext(ctx).Model(aggregation).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "cost aggregation", aggregation.Period)
	}

	r.logger.Debug("created notification cost aggregation",
		zap.String("period", aggregation.Period),
		zap.String("delivery_method", aggregation.DeliveryMethod),
		zap.Time("window_start", aggregation.WindowStart),
		zap.Int64("total_notifications", aggregation.TotalNotifications),
		zap.Int64("total_cost_micro_cents", aggregation.TotalCostMicroCents))

	return nil
}

// UpdateAggregation updates an existing notification cost aggregation record
func (r *NotificationCostRepository) UpdateAggregation(ctx context.Context, aggregation *models.NotificationCostAggregation) error {
	if err := aggregation.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "cost aggregation", "preparation")
	}

	err := r.GetDB().WithContext(ctx).Model(aggregation).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "cost aggregation", aggregation.Period)
	}

	return nil
}

// GetAggregation retrieves a notification cost aggregation record
func (r *NotificationCostRepository) GetAggregation(ctx context.Context, period, deliveryMethod string, windowStart time.Time) (*models.NotificationCostAggregation, error) {
	var aggregation models.NotificationCostAggregation

	pk := fmt.Sprintf("NOTIF_AGG#%s#%s", period, deliveryMethod)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	err := r.GetDB().WithContext(ctx).Model(&models.NotificationCostAggregation{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&aggregation)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, ErrorHandler.HandleGetError(err, "cost aggregation", period)
	}

	return &aggregation, nil
}

// ListAggregationsByPeriod lists notification cost aggregations for a period and method
func (r *NotificationCostRepository) ListAggregationsByPeriod(ctx context.Context, period, deliveryMethod string, startTime, endTime time.Time, limit int) ([]*models.NotificationCostAggregation, error) {
	var aggregations []*models.NotificationCostAggregation

	pk := fmt.Sprintf("NOTIF_AGG#%s#%s", period, deliveryMethod)
	startSK := fmt.Sprintf("WINDOW#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("WINDOW#%s", endTime.Format(time.RFC3339))

	query := r.GetDB().WithContext(ctx).Model(&models.NotificationCostAggregation{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&aggregations)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "cost aggregation", "by period")
	}

	return aggregations, nil
}

// CreateBudget creates a notification budget record
func (r *NotificationCostRepository) CreateBudget(ctx context.Context, budget *models.NotificationBudget) error {
	if err := budget.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "notification budget", "preparation")
	}

	err := r.GetDB().WithContext(ctx).Model(budget).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "notification budget", budget.Username)
	}

	r.logger.Info("created notification budget",
		zap.String("username", budget.Username),
		zap.String("period", budget.Period),
		zap.Int64("limit_micro_cents", budget.LimitMicroCents),
		zap.Bool("enabled", budget.Enabled))

	return nil
}

// UpdateBudget updates an existing notification budget record
func (r *NotificationCostRepository) UpdateBudget(ctx context.Context, budget *models.NotificationBudget) error {
	if err := budget.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "notification budget", "preparation")
	}

	err := r.GetDB().WithContext(ctx).Model(budget).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "notification budget", budget.Username)
	}

	return nil
}

// GetBudget retrieves a notification budget record
func (r *NotificationCostRepository) GetBudget(ctx context.Context, username, period string) (*models.NotificationBudget, error) {
	var budget models.NotificationBudget

	pk := fmt.Sprintf("NOTIF_BUDGET#%s", username)
	sk := fmt.Sprintf("PERIOD#%s", period)

	err := r.GetDB().WithContext(ctx).Model(&models.NotificationBudget{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&budget)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, ErrorHandler.HandleGetError(err, "notification budget", username)
	}

	return &budget, nil
}

// GetUserBudgets retrieves all budget records for a user
func (r *NotificationCostRepository) GetUserBudgets(ctx context.Context, username string) ([]*models.NotificationBudget, error) {
	var budgets []*models.NotificationBudget

	pk := fmt.Sprintf("NOTIF_BUDGET#%s", username)

	query := r.GetDB().WithContext(ctx).Model(&models.NotificationBudget{}).
		Where("PK", "=", pk).
		OrderBy("SK", "ASC")

	err := query.All(&budgets)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "notification budget", "user budgets")
	}

	return budgets, nil
}

// AggregateNotificationCosts aggregates raw cost data into aggregation records
func (r *NotificationCostRepository) AggregateNotificationCosts(ctx context.Context, period, deliveryMethod string, windowStart, windowEnd time.Time) error {
	// Collect all cost records in the window
	allCosts := r.collectCostRecords(ctx, deliveryMethod, windowStart, windowEnd)
	if err := common.ValidateSliceNotEmpty("allCosts", allCosts); err != nil {
		return nil // Nothing to aggregate
	}

	// Create and populate aggregation
	aggregation := r.createAggregation(period, deliveryMethod, windowStart, windowEnd)
	timings := r.aggregateCostData(aggregation, allCosts)

	// Calculate statistics
	r.calculateAverages(aggregation, timings)
	r.calculateBreakdownStatistics(aggregation)

	// Save aggregation
	return r.saveAggregation(ctx, aggregation)
}

// collectCostRecords collects all cost tracking records in the time window
func (r *NotificationCostRepository) collectCostRecords(ctx context.Context, deliveryMethod string, windowStart, windowEnd time.Time) []*models.NotificationCostTracking {
	var allCosts []*models.NotificationCostTracking

	currentDate := windowStart
	for currentDate.Before(windowEnd) || currentDate.Equal(windowEnd) {
		dailyCosts := r.fetchDailyCosts(ctx, currentDate)
		filtered := r.filterCosts(dailyCosts, deliveryMethod, windowStart, windowEnd)
		allCosts = append(allCosts, filtered...)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return allCosts
}

// fetchDailyCostsWithContext fetches costs for a specific date with context-specific logging
func (r *NotificationCostRepository) fetchDailyCostsWithContext(ctx context.Context, date time.Time, contextMsg string) []*models.NotificationCostTracking {
	dailyCosts, err := r.GetDailyCostTracking(ctx, date, 10000)
	if err != nil {
		r.logger.Warn(contextMsg,
			zap.Time("date", date),
			zap.Error(err))
		return nil
	}
	return dailyCosts
}

// fetchDailyCosts fetches costs for a specific date for aggregation
func (r *NotificationCostRepository) fetchDailyCosts(ctx context.Context, date time.Time) []*models.NotificationCostTracking {
	return r.fetchDailyCostsWithContext(ctx, date, "failed to get daily costs for aggregation")
}

// filterCosts filters costs by delivery method and time range
func (r *NotificationCostRepository) filterCosts(costs []*models.NotificationCostTracking, deliveryMethod string, windowStart, windowEnd time.Time) []*models.NotificationCostTracking {
	var filtered []*models.NotificationCostTracking

	for _, cost := range costs {
		if r.shouldIncludeCost(cost, deliveryMethod, windowStart, windowEnd) {
			filtered = append(filtered, cost)
		}
	}

	return filtered
}

// shouldIncludeCost checks if a cost record should be included
func (r *NotificationCostRepository) shouldIncludeCost(cost *models.NotificationCostTracking, deliveryMethod string, windowStart, windowEnd time.Time) bool {
	return (deliveryMethod == "all" || cost.DeliveryMethod == deliveryMethod) &&
		!cost.Timestamp.Before(windowStart) &&
		!cost.Timestamp.After(windowEnd)
}

// createAggregation creates a new aggregation object
func (r *NotificationCostRepository) createAggregation(period, deliveryMethod string, windowStart, windowEnd time.Time) *models.NotificationCostAggregation {
	return &models.NotificationCostAggregation{
		Period:           period,
		DeliveryMethod:   deliveryMethod,
		WindowStart:      windowStart,
		WindowEnd:        windowEnd,
		TypeBreakdown:    make(map[string]*models.NotificationTypeCostStats),
		ChannelBreakdown: make(map[string]*models.NotificationChannelCostStats),
		UserBreakdown:    make(map[string]*models.NotificationUserCostStats),
	}
}

// aggregateTimings holds timing data for averaging
type aggregateTimings struct {
	totalProcessingTime float64
	totalDeliveryTime   float64
	totalTime           float64
}

// aggregateCostData aggregates all cost data into the aggregation
func (r *NotificationCostRepository) aggregateCostData(aggregation *models.NotificationCostAggregation, costs []*models.NotificationCostTracking) aggregateTimings {
	timings := aggregateTimings{}

	for _, cost := range costs {
		r.updateTotalCounts(aggregation, cost)
		r.updateCostTotals(aggregation, cost)
		r.updateTimings(&timings, cost)
		r.updateTypeBreakdown(aggregation, cost)
		r.updateChannelBreakdown(aggregation, cost)
		r.updateUserBreakdown(aggregation, cost)
	}

	return timings
}

// updateTotalCounts updates total notification counts
func (r *NotificationCostRepository) updateTotalCounts(aggregation *models.NotificationCostAggregation, cost *models.NotificationCostTracking) {
	aggregation.TotalNotifications++

	if cost.Success {
		aggregation.SuccessfulDeliveries++
	} else {
		aggregation.FailedDeliveries++
	}

	aggregation.TotalRetries += int64(cost.RetryCount)
}

// updateCostTotals updates total cost amounts
func (r *NotificationCostRepository) updateCostTotals(aggregation *models.NotificationCostAggregation, cost *models.NotificationCostTracking) {
	aggregation.TotalPushCostMicroCents += cost.PushCostMicroCents
	aggregation.TotalWebSocketCostMicroCents += cost.WebSocketCostMicroCents
	aggregation.TotalLambdaCostMicroCents += cost.LambdaCostMicroCents
	aggregation.TotalDynamoDBCostMicroCents += cost.DynamoDBCostMicroCents
	aggregation.TotalCostMicroCents += cost.TotalCostMicroCents
}

// updateTimings updates timing totals
func (r *NotificationCostRepository) updateTimings(timings *aggregateTimings, cost *models.NotificationCostTracking) {
	timings.totalProcessingTime += float64(cost.ProcessingTimeMs)
	timings.totalDeliveryTime += float64(cost.DeliveryTimeMs)
	timings.totalTime += float64(cost.TotalTimeMs)
}

// updateTypeBreakdown updates notification type breakdown
func (r *NotificationCostRepository) updateTypeBreakdown(aggregation *models.NotificationCostAggregation, cost *models.NotificationCostTracking) {
	stats := r.getOrCreateTypeStats(aggregation, cost.NotificationType)
	r.updateBreakdownStats(stats, cost)
}

// getOrCreateTypeStats gets or creates type stats
func (r *NotificationCostRepository) getOrCreateTypeStats(aggregation *models.NotificationCostAggregation, notifType string) *models.NotificationTypeCostStats {
	stats, exists := aggregation.TypeBreakdown[notifType]
	if !exists {
		stats = &models.NotificationTypeCostStats{Type: notifType}
		aggregation.TypeBreakdown[notifType] = stats
	}
	return stats
}

// updateChannelBreakdown updates channel breakdown
func (r *NotificationCostRepository) updateChannelBreakdown(aggregation *models.NotificationCostAggregation, cost *models.NotificationCostTracking) {
	stats := r.getOrCreateChannelStats(aggregation, cost.Channel)
	r.updateBreakdownStats(stats, cost)
}

// getOrCreateChannelStats gets or creates channel stats
func (r *NotificationCostRepository) getOrCreateChannelStats(aggregation *models.NotificationCostAggregation, channel string) *models.NotificationChannelCostStats {
	stats, exists := aggregation.ChannelBreakdown[channel]
	if !exists {
		stats = &models.NotificationChannelCostStats{Channel: channel}
		aggregation.ChannelBreakdown[channel] = stats
	}
	return stats
}

// updateUserBreakdown updates user breakdown
func (r *NotificationCostRepository) updateUserBreakdown(aggregation *models.NotificationCostAggregation, cost *models.NotificationCostTracking) {
	stats := r.getOrCreateUserStats(aggregation, cost.Username)
	r.updateBreakdownStats(stats, cost)
}

// getOrCreateUserStats gets or creates user stats
func (r *NotificationCostRepository) getOrCreateUserStats(aggregation *models.NotificationCostAggregation, username string) *models.NotificationUserCostStats {
	stats, exists := aggregation.UserBreakdown[username]
	if !exists {
		stats = &models.NotificationUserCostStats{Username: username}
		aggregation.UserBreakdown[username] = stats
	}
	return stats
}

// updateBreakdownStats updates common breakdown statistics
func (r *NotificationCostRepository) updateBreakdownStats(stats interface{}, cost *models.NotificationCostTracking) {
	switch s := stats.(type) {
	case *models.NotificationTypeCostStats:
		s.Count++
		if cost.Success {
			s.SuccessfulDeliveries++
		} else {
			s.FailedDeliveries++
		}
		s.TotalCostMicroCents += cost.TotalCostMicroCents
	case *models.NotificationChannelCostStats:
		s.Count++
		if cost.Success {
			s.SuccessfulDeliveries++
		} else {
			s.FailedDeliveries++
		}
		s.TotalCostMicroCents += cost.TotalCostMicroCents
	case *models.NotificationUserCostStats:
		s.Count++
		if cost.Success {
			s.SuccessfulDeliveries++
		} else {
			s.FailedDeliveries++
		}
		s.TotalCostMicroCents += cost.TotalCostMicroCents
	}
}

// calculateAverages calculates average times
func (r *NotificationCostRepository) calculateAverages(aggregation *models.NotificationCostAggregation, timings aggregateTimings) {
	if aggregation.TotalNotifications > 0 {
		count := float64(aggregation.TotalNotifications)
		aggregation.AverageProcessingTimeMs = timings.totalProcessingTime / count
		aggregation.AverageDeliveryTimeMs = timings.totalDeliveryTime / count
		aggregation.AverageTotalTimeMs = timings.totalTime / count
	}
}

// calculateBreakdownStatistics calculates statistics for all breakdowns
func (r *NotificationCostRepository) calculateBreakdownStatistics(aggregation *models.NotificationCostAggregation) {
	r.calculateTypeStatistics(aggregation.TypeBreakdown)
	r.calculateChannelStatistics(aggregation.ChannelBreakdown)
	r.calculateUserStatistics(aggregation.UserBreakdown)
}

// calculateTypeStatistics calculates statistics for type breakdown
func (r *NotificationCostRepository) calculateTypeStatistics(breakdown map[string]*models.NotificationTypeCostStats) {
	for _, stats := range breakdown {
		r.calculateCostStatistics(&stats.Count, &stats.TotalCostMicroCents, &stats.AverageCostMicroCents,
			&stats.TotalCostDollars, &stats.AverageCostDollars, &stats.SuccessfulDeliveries, &stats.SuccessRate)
	}
}

// calculateChannelStatistics calculates statistics for channel breakdown
func (r *NotificationCostRepository) calculateChannelStatistics(breakdown map[string]*models.NotificationChannelCostStats) {
	for _, stats := range breakdown {
		r.calculateCostStatistics(&stats.Count, &stats.TotalCostMicroCents, &stats.AverageCostMicroCents,
			&stats.TotalCostDollars, &stats.AverageCostDollars, &stats.SuccessfulDeliveries, &stats.SuccessRate)
	}
}

// calculateUserStatistics calculates statistics for user breakdown
func (r *NotificationCostRepository) calculateUserStatistics(breakdown map[string]*models.NotificationUserCostStats) {
	for _, stats := range breakdown {
		r.calculateCostStatistics(&stats.Count, &stats.TotalCostMicroCents, &stats.AverageCostMicroCents,
			&stats.TotalCostDollars, &stats.AverageCostDollars, &stats.SuccessfulDeliveries, &stats.SuccessRate)
	}
}

// calculateCostStatistics calculates common cost statistics
func (r *NotificationCostRepository) calculateCostStatistics(count, totalCostMicroCents, avgCostMicroCents *int64, totalCostDollars, avgCostDollars *float64, successfulDeliveries *int64, successRate *float64) {
	*totalCostDollars = float64(*totalCostMicroCents) / 1_000_000.0
	if *count > 0 {
		*avgCostMicroCents = *totalCostMicroCents / *count
		*avgCostDollars = float64(*avgCostMicroCents) / 1_000_000.0
		*successRate = (float64(*successfulDeliveries) / float64(*count)) * 100
	}
}

// saveAggregation saves or updates the aggregation
func (r *NotificationCostRepository) saveAggregation(ctx context.Context, aggregation *models.NotificationCostAggregation) error {
	existing, err := r.GetAggregation(ctx, aggregation.Period, aggregation.DeliveryMethod, aggregation.WindowStart)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "cost aggregation", "existing check")
	}

	if existing != nil {
		aggregation.CreatedAt = existing.CreatedAt
		return r.UpdateAggregation(ctx, aggregation)
	}

	return r.CreateAggregation(ctx, aggregation)
}

// GetNotificationCostSummary calculates cost summary for notifications within a time range
func (r *NotificationCostRepository) GetNotificationCostSummary(ctx context.Context, startTime, endTime time.Time) (*NotificationCostSummary, error) {
	// Collect all costs in the time range
	allCosts := r.collectCostsInRange(ctx, startTime, endTime)

	// Initialize summary
	summary := r.initializeSummary(startTime, endTime, allCosts)
	if summary.Count == 0 {
		return summary, nil
	}

	// Process all costs
	r.processCosts(summary, allCosts)

	// Calculate final statistics
	r.calculateSummaryStatistics(summary)
	r.calculateMethodStatistics(summary)
	r.calculateNotificationTypeStatistics(summary)

	return summary, nil
}

// collectCostsInRange collects all notification costs within the time range
func (r *NotificationCostRepository) collectCostsInRange(ctx context.Context, startTime, endTime time.Time) []*models.NotificationCostTracking {
	var allCosts []*models.NotificationCostTracking

	currentDate := startTime
	for currentDate.Before(endTime) || currentDate.Equal(endTime) {
		dailyCosts := r.fetchDailySummaryCosts(ctx, currentDate)
		filteredCosts := r.filterCostsByTimeRange(dailyCosts, startTime, endTime)
		allCosts = append(allCosts, filteredCosts...)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return allCosts
}

// fetchDailySummaryCosts fetches costs for a specific date for summary
func (r *NotificationCostRepository) fetchDailySummaryCosts(ctx context.Context, date time.Time) []*models.NotificationCostTracking {
	return r.fetchDailyCostsWithContext(ctx, date, "failed to get daily costs for summary")
}

// filterCostsByTimeRange filters costs to only include those within the time range
func (r *NotificationCostRepository) filterCostsByTimeRange(costs []*models.NotificationCostTracking, startTime, endTime time.Time) []*models.NotificationCostTracking {
	var filtered []*models.NotificationCostTracking
	for _, cost := range costs {
		if !cost.Timestamp.Before(startTime) && !cost.Timestamp.After(endTime) {
			filtered = append(filtered, cost)
		}
	}
	return filtered
}

// initializeSummary creates and initializes a new summary
func (r *NotificationCostRepository) initializeSummary(startTime, endTime time.Time, costs []*models.NotificationCostTracking) *NotificationCostSummary {
	return &NotificationCostSummary{
		StartTime:                 startTime,
		EndTime:                   endTime,
		Count:                     len(costs),
		DeliveryMethodBreakdown:   make(map[string]*DeliveryMethodSummaryStats),
		NotificationTypeBreakdown: make(map[string]*NotificationTypeSummaryStats),
	}
}

// processCosts processes all costs and updates the summary
func (r *NotificationCostRepository) processCosts(summary *NotificationCostSummary, costs []*models.NotificationCostTracking) {
	for _, cost := range costs {
		r.updateBasicCounts(summary, cost)
		r.updateMethodBreakdown(summary, cost)
		r.updateNotificationTypeBreakdown(summary, cost)
	}
}

// updateBasicCounts updates basic count fields in the summary
func (r *NotificationCostRepository) updateBasicCounts(summary *NotificationCostSummary, cost *models.NotificationCostTracking) {
	summary.TotalNotifications++
	if cost.Success {
		summary.SuccessfulDeliveries++
	} else {
		summary.FailedDeliveries++
	}
	summary.TotalRetries += int64(cost.RetryCount)
	summary.TotalCostMicroCents += cost.TotalCostMicroCents
}

// updateMethodBreakdown updates delivery method breakdown statistics
func (r *NotificationCostRepository) updateMethodBreakdown(summary *NotificationCostSummary, cost *models.NotificationCostTracking) {
	methodStats := r.getOrCreateMethodStats(summary, cost.DeliveryMethod)
	methodStats.Count++
	if cost.Success {
		methodStats.SuccessfulDeliveries++
	} else {
		methodStats.FailedDeliveries++
	}
	methodStats.TotalCostMicroCents += cost.TotalCostMicroCents
}

// getOrCreateMethodStats gets or creates method statistics
func (r *NotificationCostRepository) getOrCreateMethodStats(summary *NotificationCostSummary, method string) *DeliveryMethodSummaryStats {
	stats, exists := summary.DeliveryMethodBreakdown[method]
	if !exists {
		stats = &DeliveryMethodSummaryStats{
			Method: method,
		}
		summary.DeliveryMethodBreakdown[method] = stats
	}
	return stats
}

// updateTypeBreakdown updates notification type breakdown statistics
func (r *NotificationCostRepository) updateNotificationTypeBreakdown(summary *NotificationCostSummary, cost *models.NotificationCostTracking) {
	typeStats := r.getOrCreateNotificationTypeStats(summary, cost.NotificationType)
	typeStats.Count++
	if cost.Success {
		typeStats.SuccessfulDeliveries++
	} else {
		typeStats.FailedDeliveries++
	}
	typeStats.TotalCostMicroCents += cost.TotalCostMicroCents
}

// getOrCreateNotificationTypeStats gets or creates type statistics
func (r *NotificationCostRepository) getOrCreateNotificationTypeStats(summary *NotificationCostSummary, notifType string) *NotificationTypeSummaryStats {
	stats, exists := summary.NotificationTypeBreakdown[notifType]
	if !exists {
		stats = &NotificationTypeSummaryStats{
			Type: notifType,
		}
		summary.NotificationTypeBreakdown[notifType] = stats
	}
	return stats
}

// calculateSummaryStatistics calculates overall summary statistics
func (r *NotificationCostRepository) calculateSummaryStatistics(summary *NotificationCostSummary) {
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	if summary.TotalNotifications == 0 {
		return
	}
	summary.AverageCostPerNotification = summary.TotalCostDollars / float64(summary.TotalNotifications)
	summary.SuccessRate = (float64(summary.SuccessfulDeliveries) / float64(summary.TotalNotifications)) * 100
	summary.RetryRate = (float64(summary.TotalRetries) / float64(summary.TotalNotifications)) * 100
}

// calculateMethodStatistics calculates statistics for each delivery method
func (r *NotificationCostRepository) calculateMethodStatistics(summary *NotificationCostSummary) {
	for _, stats := range summary.DeliveryMethodBreakdown {
		r.calculateBreakdownStats(stats.Count, stats.TotalCostMicroCents, stats.SuccessfulDeliveries,
			&stats.TotalCostDollars, &stats.AverageCostMicroCents, &stats.AverageCostDollars, &stats.SuccessRate)
	}
}

// calculateTypeStatistics calculates statistics for each notification type
func (r *NotificationCostRepository) calculateNotificationTypeStatistics(summary *NotificationCostSummary) {
	for _, stats := range summary.NotificationTypeBreakdown {
		r.calculateBreakdownStats(stats.Count, stats.TotalCostMicroCents, stats.SuccessfulDeliveries,
			&stats.TotalCostDollars, &stats.AverageCostMicroCents, &stats.AverageCostDollars, &stats.SuccessRate)
	}
}

// calculateBreakdownStats calculates common statistics for breakdowns
func (r *NotificationCostRepository) calculateBreakdownStats(count, totalCostMicroCents, successfulDeliveries int64,
	totalCostDollars *float64, avgCostMicroCents *int64, avgCostDollars, successRate *float64) {
	*totalCostDollars = float64(totalCostMicroCents) / 1_000_000.0
	if count > 0 {
		*avgCostMicroCents = totalCostMicroCents / count
		*avgCostDollars = float64(*avgCostMicroCents) / 1_000_000.0
		*successRate = (float64(successfulDeliveries) / float64(count)) * 100
	}
}

// GetHighCostNotifications returns notifications that exceed a cost threshold
func (r *NotificationCostRepository) GetHighCostNotifications(ctx context.Context, thresholdMicroCents int64, startTime, endTime time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	// Get all costs in the time range
	var allCosts []*models.NotificationCostTracking

	currentDate := startTime
	for currentDate.Before(endTime) || currentDate.Equal(endTime) {
		dailyCosts, err := r.GetDailyCostTracking(ctx, currentDate, 10000)
		if err != nil {
			r.logger.Warn("failed to get daily costs for high cost query",
				zap.Time("date", currentDate),
				zap.Error(err))
		} else {
			allCosts = append(allCosts, dailyCosts...)
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Filter by threshold and time range
	var highCostNotifications []*models.NotificationCostTracking
	for _, cost := range allCosts {
		if cost.Timestamp.After(endTime) || cost.Timestamp.Before(startTime) {
			continue
		}
		if cost.TotalCostMicroCents >= thresholdMicroCents {
			highCostNotifications = append(highCostNotifications, cost)
		}
	}

	// Sort by cost (highest first)
	sort.Slice(highCostNotifications, func(i, j int) bool {
		return highCostNotifications[i].TotalCostMicroCents > highCostNotifications[j].TotalCostMicroCents
	})

	// Limit results
	if len(highCostNotifications) > limit {
		highCostNotifications = highCostNotifications[:limit]
	}

	return highCostNotifications, nil
}

// GetUserSpending calculates total spending for a user in a time period
func (r *NotificationCostRepository) GetUserSpending(ctx context.Context, username string, startTime, endTime time.Time) (*UserSpendingSummary, error) {
	costs, err := r.GetCostTrackingByUser(ctx, username, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	summary := &UserSpendingSummary{
		Username:                username,
		StartTime:               startTime,
		EndTime:                 endTime,
		DeliveryMethodBreakdown: make(map[string]*DeliveryMethodSpending),
	}

	for _, cost := range costs {
		summary.TotalNotifications++
		if cost.Success {
			summary.SuccessfulDeliveries++
		} else {
			summary.FailedDeliveries++
		}
		summary.TotalCostMicroCents += cost.TotalCostMicroCents

		// Track by delivery method
		methodSpending, exists := summary.DeliveryMethodBreakdown[cost.DeliveryMethod]
		if !exists {
			methodSpending = &DeliveryMethodSpending{
				Method: cost.DeliveryMethod,
			}
			summary.DeliveryMethodBreakdown[cost.DeliveryMethod] = methodSpending
		}

		methodSpending.Count++
		methodSpending.TotalCostMicroCents += cost.TotalCostMicroCents
	}

	// Calculate totals and averages
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	if summary.TotalNotifications > 0 {
		summary.AverageCostPerNotification = summary.TotalCostDollars / float64(summary.TotalNotifications)
		summary.SuccessRate = (float64(summary.SuccessfulDeliveries) / float64(summary.TotalNotifications)) * 100
	}

	// Calculate method spending
	for method := range summary.DeliveryMethodBreakdown {
		spending := summary.DeliveryMethodBreakdown[method]
		spending.TotalCostDollars = float64(spending.TotalCostMicroCents) / 1_000_000.0
		if spending.Count > 0 {
			spending.AverageCostMicroCents = spending.TotalCostMicroCents / spending.Count
			spending.AverageCostDollars = float64(spending.AverageCostMicroCents) / 1_000_000.0
		}
	}

	return summary, nil
}

// NotificationCostSummary represents a summary of notification costs
type NotificationCostSummary struct {
	StartTime                  time.Time
	EndTime                    time.Time
	Count                      int
	TotalNotifications         int64
	SuccessfulDeliveries       int64
	FailedDeliveries           int64
	TotalRetries               int64
	TotalCostMicroCents        int64
	TotalCostDollars           float64
	AverageCostPerNotification float64
	SuccessRate                float64
	RetryRate                  float64
	DeliveryMethodBreakdown    map[string]*DeliveryMethodSummaryStats
	NotificationTypeBreakdown  map[string]*NotificationTypeSummaryStats
}

// DeliveryMethodSummaryStats represents cost statistics for a delivery method
type DeliveryMethodSummaryStats struct {
	Method                string
	Count                 int64
	SuccessfulDeliveries  int64
	FailedDeliveries      int64
	TotalCostMicroCents   int64
	TotalCostDollars      float64
	AverageCostMicroCents int64
	AverageCostDollars    float64
	SuccessRate           float64
}

// NotificationTypeSummaryStats represents cost statistics for a notification type
type NotificationTypeSummaryStats struct {
	Type                  string
	Count                 int64
	SuccessfulDeliveries  int64
	FailedDeliveries      int64
	TotalCostMicroCents   int64
	TotalCostDollars      float64
	AverageCostMicroCents int64
	AverageCostDollars    float64
	SuccessRate           float64
}

// UserSpendingSummary represents spending summary for a user
type UserSpendingSummary struct {
	Username                   string
	StartTime                  time.Time
	EndTime                    time.Time
	TotalNotifications         int64
	SuccessfulDeliveries       int64
	FailedDeliveries           int64
	TotalCostMicroCents        int64
	TotalCostDollars           float64
	AverageCostPerNotification float64
	SuccessRate                float64
	DeliveryMethodBreakdown    map[string]*DeliveryMethodSpending
}

// DeliveryMethodSpending represents spending for a specific delivery method
type DeliveryMethodSpending struct {
	Method                string
	Count                 int64
	TotalCostMicroCents   int64
	TotalCostDollars      float64
	AverageCostMicroCents int64
	AverageCostDollars    float64
}

// GetDailySpending retrieves the current daily spending for a user
func (r *NotificationCostRepository) GetDailySpending(ctx context.Context, username string) (int64, error) {
	// Calculate today's date range
	today := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	// Query cost records for today
	var costs []models.NotificationCostTracking

	// Query by GSI1 (user index) with timestamp range
	gsi1PK := fmt.Sprintf("USER#%s", username)
	startSK := fmt.Sprintf("COST#%s", today.Format(time.RFC3339))
	endSK := fmt.Sprintf("COST#%s", tomorrow.Format(time.RFC3339))

	err := r.GetDB().WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Where("GSI1PK", "=", gsi1PK).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<", endSK).
		All(&costs)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil // No costs found, return 0
		}
		return 0, ErrorHandler.HandleQueryError(err, "notification cost", "daily spending")
	}

	// Sum up the total costs
	var totalCost int64
	for _, cost := range costs {
		totalCost += cost.TotalCostMicroCents
	}

	r.logger.Debug("calculated daily spending",
		zap.String("username", username),
		zap.String("date", today.Format("2006-01-02")),
		zap.Int("cost_records", len(costs)),
		zap.Int64("total_cost_micro_cents", totalCost),
		zap.Float64("total_cost_dollars", float64(totalCost)/1_000_000.0))

	return totalCost, nil
}
