package repositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// NotificationCostRepository handles notification cost tracking operations
type NotificationCostRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewNotificationCostRepository creates a new notification cost repository
func NewNotificationCostRepository(db core.DB, tableName string, logger *zap.Logger) *NotificationCostRepository {
	return &NotificationCostRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateCostTracking creates a new notification cost tracking record
func (r *NotificationCostRepository) CreateCostTracking(ctx context.Context, tracking *models.NotificationCostTracking) error {
	if err := tracking.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare cost tracking for creation: %w", err)
	}

	err := r.db.WithContext(ctx).Model(tracking).Create()
	if err != nil {
		return fmt.Errorf("failed to create notification cost tracking: %w", err)
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
	
	query := r.db.WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&trackingRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to get cost tracking by notification: %w", err)
	}

	return trackingRecords, nil
}

// GetCostTrackingByUser retrieves cost tracking records for a user within a time range
func (r *NotificationCostRepository) GetCostTrackingByUser(ctx context.Context, username string, startTime, endTime time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	var trackingRecords []*models.NotificationCostTracking
	
	startSK := fmt.Sprintf("COST#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("COST#%s", endTime.Format("20060102150405"))
	
	query := r.db.WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Index("gsi1").
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&trackingRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to get cost tracking by user: %w", err)
	}

	return trackingRecords, nil
}

// GetCostTrackingByMethod retrieves cost tracking records by delivery method within a time range
func (r *NotificationCostRepository) GetCostTrackingByMethod(ctx context.Context, deliveryMethod string, startTime, endTime time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	var trackingRecords []*models.NotificationCostTracking
	
	startSK := fmt.Sprintf("TIMESTAMP#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("TIMESTAMP#%s", endTime.Format("20060102150405"))
	
	query := r.db.WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Index("gsi2").
		Where("GSI2PK", "=", fmt.Sprintf("METHOD#%s", deliveryMethod)).
		Where("GSI2SK", ">=", startSK).
		Where("GSI2SK", "<=", endSK).
		OrderBy("GSI2SK", "DESC").
		Limit(limit)

	err := query.All(&trackingRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to get cost tracking by method: %w", err)
	}

	return trackingRecords, nil
}

// GetDailyCostTracking retrieves cost tracking records for a specific date
func (r *NotificationCostRepository) GetDailyCostTracking(ctx context.Context, date time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	var trackingRecords []*models.NotificationCostTracking
	
	dateStr := date.Format("20060102")
	
	query := r.db.WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Index("gsi3").
		Where("GSI3PK", "=", fmt.Sprintf("DAILY#%s", dateStr)).
		OrderBy("GSI3SK", "DESC").
		Limit(limit)

	err := query.All(&trackingRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily cost tracking: %w", err)
	}

	return trackingRecords, nil
}

// CreateAggregation creates a notification cost aggregation record
func (r *NotificationCostRepository) CreateAggregation(ctx context.Context, aggregation *models.NotificationCostAggregation) error {
	if err := aggregation.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare aggregation for creation: %w", err)
	}

	err := r.db.WithContext(ctx).Model(aggregation).Create()
	if err != nil {
		return fmt.Errorf("failed to create notification cost aggregation: %w", err)
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
		return fmt.Errorf("failed to prepare aggregation for update: %w", err)
	}

	err := r.db.WithContext(ctx).Model(aggregation).Update()
	if err != nil {
		return fmt.Errorf("failed to update notification cost aggregation: %w", err)
	}

	return nil
}

// GetAggregation retrieves a notification cost aggregation record
func (r *NotificationCostRepository) GetAggregation(ctx context.Context, period, deliveryMethod string, windowStart time.Time) (*models.NotificationCostAggregation, error) {
	var aggregation models.NotificationCostAggregation

	pk := fmt.Sprintf("NOTIF_AGG#%s#%s", period, deliveryMethod)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	err := r.db.WithContext(ctx).Model(&models.NotificationCostAggregation{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&aggregation)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification cost aggregation: %w", err)
	}

	return &aggregation, nil
}

// ListAggregationsByPeriod lists notification cost aggregations for a period and method
func (r *NotificationCostRepository) ListAggregationsByPeriod(ctx context.Context, period, deliveryMethod string, startTime, endTime time.Time, limit int) ([]*models.NotificationCostAggregation, error) {
	var aggregations []*models.NotificationCostAggregation

	pk := fmt.Sprintf("NOTIF_AGG#%s#%s", period, deliveryMethod)
	startSK := fmt.Sprintf("WINDOW#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("WINDOW#%s", endTime.Format(time.RFC3339))

	query := r.db.WithContext(ctx).Model(&models.NotificationCostAggregation{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&aggregations)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification cost aggregations: %w", err)
	}

	return aggregations, nil
}

// CreateBudget creates a notification budget record
func (r *NotificationCostRepository) CreateBudget(ctx context.Context, budget *models.NotificationBudget) error {
	if err := budget.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare budget for creation: %w", err)
	}

	err := r.db.WithContext(ctx).Model(budget).Create()
	if err != nil {
		return fmt.Errorf("failed to create notification budget: %w", err)
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
		return fmt.Errorf("failed to prepare budget for update: %w", err)
	}

	err := r.db.WithContext(ctx).Model(budget).Update()
	if err != nil {
		return fmt.Errorf("failed to update notification budget: %w", err)
	}

	return nil
}

// GetBudget retrieves a notification budget record
func (r *NotificationCostRepository) GetBudget(ctx context.Context, username, period string) (*models.NotificationBudget, error) {
	var budget models.NotificationBudget

	pk := fmt.Sprintf("NOTIF_BUDGET#%s", username)
	sk := fmt.Sprintf("PERIOD#%s", period)

	err := r.db.WithContext(ctx).Model(&models.NotificationBudget{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&budget)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification budget: %w", err)
	}

	return &budget, nil
}

// GetUserBudgets retrieves all budget records for a user
func (r *NotificationCostRepository) GetUserBudgets(ctx context.Context, username string) ([]*models.NotificationBudget, error) {
	var budgets []*models.NotificationBudget

	pk := fmt.Sprintf("NOTIF_BUDGET#%s", username)

	query := r.db.WithContext(ctx).Model(&models.NotificationBudget{}).
		Where("PK", "=", pk).
		OrderBy("SK", "ASC")

	err := query.All(&budgets)
	if err != nil {
		return nil, fmt.Errorf("failed to get user budgets: %w", err)
	}

	return budgets, nil
}

// AggregateNotificationCosts aggregates raw cost data into aggregation records
func (r *NotificationCostRepository) AggregateNotificationCosts(ctx context.Context, period, deliveryMethod string, windowStart, windowEnd time.Time) error {
	// Get all cost tracking records in the window
	var allCosts []*models.NotificationCostTracking
	
	// Query by day to get all records in the time window
	currentDate := windowStart
	for currentDate.Before(windowEnd) || currentDate.Equal(windowEnd) {
		dailyCosts, err := r.GetDailyCostTracking(ctx, currentDate, 10000)
		if err != nil {
			r.logger.Warn("failed to get daily costs for aggregation",
				zap.Time("date", currentDate),
				zap.Error(err))
		} else {
			// Filter by delivery method and time range
			for _, cost := range dailyCosts {
				if (deliveryMethod == "all" || cost.DeliveryMethod == deliveryMethod) &&
					!cost.Timestamp.Before(windowStart) &&
					!cost.Timestamp.After(windowEnd) {
					allCosts = append(allCosts, cost)
				}
			}
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	if len(allCosts) == 0 {
		return nil // Nothing to aggregate
	}

	// Calculate aggregated values
	aggregation := &models.NotificationCostAggregation{
		Period:          period,
		DeliveryMethod:  deliveryMethod,
		WindowStart:     windowStart,
		WindowEnd:       windowEnd,
		TypeBreakdown:   make(map[string]*models.NotificationTypeCostStats),
		ChannelBreakdown: make(map[string]*models.NotificationChannelCostStats),
		UserBreakdown:   make(map[string]*models.NotificationUserCostStats),
	}

	// Track processing times for averages
	var totalProcessingTime, totalDeliveryTime, totalTime float64

	// Aggregate costs
	for _, cost := range allCosts {
		aggregation.TotalNotifications++
		
		if cost.Success {
			aggregation.SuccessfulDeliveries++
		} else {
			aggregation.FailedDeliveries++
		}
		
		aggregation.TotalRetries += int64(cost.RetryCount)
		
		// Add costs
		aggregation.TotalEmailCostMicroCents += cost.EmailCostMicroCents
		aggregation.TotalPushCostMicroCents += cost.PushCostMicroCents
		aggregation.TotalSMSCostMicroCents += cost.SMSCostMicroCents
		aggregation.TotalWebSocketCostMicroCents += cost.WebSocketCostMicroCents
		aggregation.TotalLambdaCostMicroCents += cost.LambdaCostMicroCents
		aggregation.TotalDynamoDBCostMicroCents += cost.DynamoDBCostMicroCents
		aggregation.TotalCostMicroCents += cost.TotalCostMicroCents
		
		// Add processing times
		totalProcessingTime += float64(cost.ProcessingTimeMs)
		totalDeliveryTime += float64(cost.DeliveryTimeMs)
		totalTime += float64(cost.TotalTimeMs)
		
		// Aggregate by notification type
		typeStats, exists := aggregation.TypeBreakdown[cost.NotificationType]
		if !exists {
			typeStats = &models.NotificationTypeCostStats{
				Type: cost.NotificationType,
			}
			aggregation.TypeBreakdown[cost.NotificationType] = typeStats
		}
		
		typeStats.Count++
		if cost.Success {
			typeStats.SuccessfulDeliveries++
		} else {
			typeStats.FailedDeliveries++
		}
		typeStats.TotalCostMicroCents += cost.TotalCostMicroCents
		
		// Aggregate by channel
		channelStats, exists := aggregation.ChannelBreakdown[cost.Channel]
		if !exists {
			channelStats = &models.NotificationChannelCostStats{
				Channel: cost.Channel,
			}
			aggregation.ChannelBreakdown[cost.Channel] = channelStats
		}
		
		channelStats.Count++
		if cost.Success {
			channelStats.SuccessfulDeliveries++
		} else {
			channelStats.FailedDeliveries++
		}
		channelStats.TotalCostMicroCents += cost.TotalCostMicroCents
		
		// Aggregate by user
		userStats, exists := aggregation.UserBreakdown[cost.Username]
		if !exists {
			userStats = &models.NotificationUserCostStats{
				Username: cost.Username,
			}
			aggregation.UserBreakdown[cost.Username] = userStats
		}
		
		userStats.Count++
		if cost.Success {
			userStats.SuccessfulDeliveries++
		} else {
			userStats.FailedDeliveries++
		}
		userStats.TotalCostMicroCents += cost.TotalCostMicroCents
	}

	// Calculate averages
	if aggregation.TotalNotifications > 0 {
		aggregation.AverageProcessingTimeMs = totalProcessingTime / float64(aggregation.TotalNotifications)
		aggregation.AverageDeliveryTimeMs = totalDeliveryTime / float64(aggregation.TotalNotifications)
		aggregation.AverageTotalTimeMs = totalTime / float64(aggregation.TotalNotifications)
	}

	// Calculate statistics for type breakdown
	for notifType := range aggregation.TypeBreakdown {
		stats := aggregation.TypeBreakdown[notifType]
		stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
		if stats.Count > 0 {
			stats.AverageCostMicroCents = stats.TotalCostMicroCents / stats.Count
			stats.AverageCostDollars = float64(stats.AverageCostMicroCents) / 1_000_000.0
			stats.SuccessRate = (float64(stats.SuccessfulDeliveries) / float64(stats.Count)) * 100
		}
	}

	// Calculate statistics for channel breakdown
	for channel := range aggregation.ChannelBreakdown {
		stats := aggregation.ChannelBreakdown[channel]
		stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
		if stats.Count > 0 {
			stats.AverageCostMicroCents = stats.TotalCostMicroCents / stats.Count
			stats.AverageCostDollars = float64(stats.AverageCostMicroCents) / 1_000_000.0
			stats.SuccessRate = (float64(stats.SuccessfulDeliveries) / float64(stats.Count)) * 100
		}
	}

	// Calculate statistics for user breakdown
	for username := range aggregation.UserBreakdown {
		stats := aggregation.UserBreakdown[username]
		stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
		if stats.Count > 0 {
			stats.AverageCostMicroCents = stats.TotalCostMicroCents / stats.Count
			stats.AverageCostDollars = float64(stats.AverageCostMicroCents) / 1_000_000.0
			stats.SuccessRate = (float64(stats.SuccessfulDeliveries) / float64(stats.Count)) * 100
		}
	}

	// Check if aggregation already exists
	existing, err := r.GetAggregation(ctx, period, deliveryMethod, windowStart)
	if err != nil {
		return fmt.Errorf("failed to check existing aggregation: %w", err)
	}

	if existing != nil {
		// Update existing
		aggregation.CreatedAt = existing.CreatedAt
		return r.UpdateAggregation(ctx, aggregation)
	}

	// Create new aggregation
	return r.CreateAggregation(ctx, aggregation)
}

// GetNotificationCostSummary calculates cost summary for notifications within a time range
func (r *NotificationCostRepository) GetNotificationCostSummary(ctx context.Context, startTime, endTime time.Time) (*NotificationCostSummary, error) {
	// Get costs for each day in the range
	var allCosts []*models.NotificationCostTracking
	
	currentDate := startTime
	for currentDate.Before(endTime) || currentDate.Equal(endTime) {
		dailyCosts, err := r.GetDailyCostTracking(ctx, currentDate, 10000)
		if err != nil {
			r.logger.Warn("failed to get daily costs for summary",
				zap.Time("date", currentDate),
				zap.Error(err))
		} else {
			// Filter by time range
			for _, cost := range dailyCosts {
				if !cost.Timestamp.Before(startTime) && !cost.Timestamp.After(endTime) {
					allCosts = append(allCosts, cost)
				}
			}
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	summary := &NotificationCostSummary{
		StartTime: startTime,
		EndTime:   endTime,
		Count:     len(allCosts),
		DeliveryMethodBreakdown: make(map[string]*DeliveryMethodSummaryStats),
		NotificationTypeBreakdown: make(map[string]*NotificationTypeSummaryStats),
	}

	if summary.Count == 0 {
		return summary, nil
	}

	// Calculate summary statistics
	for _, cost := range allCosts {
		summary.TotalNotifications++
		if cost.Success {
			summary.SuccessfulDeliveries++
		} else {
			summary.FailedDeliveries++
		}
		summary.TotalRetries += int64(cost.RetryCount)
		summary.TotalCostMicroCents += cost.TotalCostMicroCents

		// Aggregate by delivery method
		methodStats, exists := summary.DeliveryMethodBreakdown[cost.DeliveryMethod]
		if !exists {
			methodStats = &DeliveryMethodSummaryStats{
				Method: cost.DeliveryMethod,
			}
			summary.DeliveryMethodBreakdown[cost.DeliveryMethod] = methodStats
		}
		
		methodStats.Count++
		if cost.Success {
			methodStats.SuccessfulDeliveries++
		} else {
			methodStats.FailedDeliveries++
		}
		methodStats.TotalCostMicroCents += cost.TotalCostMicroCents

		// Aggregate by notification type
		typeStats, exists := summary.NotificationTypeBreakdown[cost.NotificationType]
		if !exists {
			typeStats = &NotificationTypeSummaryStats{
				Type: cost.NotificationType,
			}
			summary.NotificationTypeBreakdown[cost.NotificationType] = typeStats
		}
		
		typeStats.Count++
		if cost.Success {
			typeStats.SuccessfulDeliveries++
		} else {
			typeStats.FailedDeliveries++
		}
		typeStats.TotalCostMicroCents += cost.TotalCostMicroCents
	}

	// Calculate totals and averages
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	if summary.TotalNotifications > 0 {
		summary.AverageCostPerNotification = summary.TotalCostDollars / float64(summary.TotalNotifications)
		summary.SuccessRate = (float64(summary.SuccessfulDeliveries) / float64(summary.TotalNotifications)) * 100
		summary.RetryRate = (float64(summary.TotalRetries) / float64(summary.TotalNotifications)) * 100
	}

	// Calculate method averages
	for method := range summary.DeliveryMethodBreakdown {
		stats := summary.DeliveryMethodBreakdown[method]
		stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
		if stats.Count > 0 {
			stats.AverageCostMicroCents = stats.TotalCostMicroCents / stats.Count
			stats.AverageCostDollars = float64(stats.AverageCostMicroCents) / 1_000_000.0
			stats.SuccessRate = (float64(stats.SuccessfulDeliveries) / float64(stats.Count)) * 100
		}
	}

	// Calculate type averages
	for notifType := range summary.NotificationTypeBreakdown {
		stats := summary.NotificationTypeBreakdown[notifType]
		stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
		if stats.Count > 0 {
			stats.AverageCostMicroCents = stats.TotalCostMicroCents / stats.Count
			stats.AverageCostDollars = float64(stats.AverageCostMicroCents) / 1_000_000.0
			stats.SuccessRate = (float64(stats.SuccessfulDeliveries) / float64(stats.Count)) * 100
		}
	}

	return summary, nil
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
		Username:  username,
		StartTime: startTime,
		EndTime:   endTime,
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
	StartTime                   time.Time
	EndTime                     time.Time
	Count                       int
	TotalNotifications          int64
	SuccessfulDeliveries        int64
	FailedDeliveries            int64
	TotalRetries                int64
	TotalCostMicroCents         int64
	TotalCostDollars            float64
	AverageCostPerNotification  float64
	SuccessRate                 float64
	RetryRate                   float64
	DeliveryMethodBreakdown     map[string]*DeliveryMethodSummaryStats
	NotificationTypeBreakdown   map[string]*NotificationTypeSummaryStats
}

// DeliveryMethodSummaryStats represents cost statistics for a delivery method
type DeliveryMethodSummaryStats struct {
	Method                  string
	Count                   int64
	SuccessfulDeliveries    int64
	FailedDeliveries        int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostMicroCents   int64
	AverageCostDollars      float64
	SuccessRate             float64
}

// NotificationTypeSummaryStats represents cost statistics for a notification type
type NotificationTypeSummaryStats struct {
	Type                    string
	Count                   int64
	SuccessfulDeliveries    int64
	FailedDeliveries        int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostMicroCents   int64
	AverageCostDollars      float64
	SuccessRate             float64
}

// UserSpendingSummary represents spending summary for a user
type UserSpendingSummary struct {
	Username                    string
	StartTime                   time.Time
	EndTime                     time.Time
	TotalNotifications          int64
	SuccessfulDeliveries        int64
	FailedDeliveries            int64
	TotalCostMicroCents         int64
	TotalCostDollars            float64
	AverageCostPerNotification  float64
	SuccessRate                 float64
	DeliveryMethodBreakdown     map[string]*DeliveryMethodSpending
}

// DeliveryMethodSpending represents spending for a specific delivery method
type DeliveryMethodSpending struct {
	Method                  string
	Count                   int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostMicroCents   int64
	AverageCostDollars      float64
}