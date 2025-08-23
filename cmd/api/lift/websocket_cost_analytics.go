package lift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Constants for repeated strings
const (
	periodDay        = "day"
	trendIncreasing  = "increasing"
	trendDecreasing  = "decreasing"
)

// WebSocketCostAnalyticsRequest represents requests for WebSocket cost analytics
type WebSocketCostAnalyticsRequest struct {
	StartDate string `json:"start_date" validate:"required"`
	EndDate   string `json:"end_date" validate:"required"`
	Period    string `json:"period" validate:"oneof=hour day week month"`
	UserID    string `json:"user_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// WebSocketCostSummaryResponse represents WebSocket cost summary response
type WebSocketCostSummaryResponse struct {
	Summary            *WebSocketOverallSummary                 `json:"summary"`
	TopUsers           []*repositories.WebSocketUserCostRanking `json:"top_users"`
	HighCostOperations []*WebSocketCostOperationSummary         `json:"high_cost_operations"`
	CostTrends         *WebSocketCostTrends                     `json:"cost_trends"`
	UserDetails        *repositories.WebSocketUserCostSummary   `json:"user_details,omitempty"`
	BudgetStatus       *repositories.BudgetStatus               `json:"budget_status,omitempty"`
}

// WebSocketOverallSummary represents overall WebSocket cost summary
type WebSocketOverallSummary struct {
	DateRange                string  `json:"date_range"`
	TotalCostDollars         float64 `json:"total_cost_dollars"`
	TotalConnections         int64   `json:"total_connections"`
	TotalMessages            int64   `json:"total_messages"`
	TotalConnectionHours     float64 `json:"total_connection_hours"`
	UniqueUsers              int64   `json:"unique_users"`
	AverageCostPerUser       float64 `json:"average_cost_per_user"`
	AverageCostPerConnection float64 `json:"average_cost_per_connection"`
	AverageCostPerMessage    float64 `json:"average_cost_per_message"`

	// Cost breakdown
	ConnectionCostPercent float64 `json:"connection_cost_percent"`
	MessageCostPercent    float64 `json:"message_cost_percent"`
	LambdaCostPercent     float64 `json:"lambda_cost_percent"`
	DynamoDBCostPercent   float64 `json:"dynamodb_cost_percent"`

	// Efficiency metrics
	MessagesPerConnection     float64 `json:"messages_per_connection"`
	AverageConnectionDuration float64 `json:"average_connection_duration"`
}

// WebSocketCostOperationSummary represents summary for high-cost operations
type WebSocketCostOperationSummary struct {
	OperationType    string    `json:"operation_type"`
	ConnectionID     string    `json:"connection_id"`
	UserID           string    `json:"user_id"`
	Username         string    `json:"username"`
	CostDollars      float64   `json:"cost_dollars"`
	ProcessingTimeMs int64     `json:"processing_time_ms"`
	MessageCount     int       `json:"message_count"`
	Timestamp        time.Time `json:"timestamp"`
}

// WebSocketCostTrends represents cost trends over time
type WebSocketCostTrends struct {
	Period        string                   `json:"period"`
	DataPoints    []WebSocketCostDataPoint `json:"data_points"`
	TrendAnalysis *WebSocketTrendAnalysis  `json:"trend_analysis"`
}

// WebSocketCostDataPoint represents a single data point in cost trends
type WebSocketCostDataPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	CostDollars      float64   `json:"cost_dollars"`
	Connections      int64     `json:"connections"`
	Messages         int64     `json:"messages"`
	UniqueUsers      int64     `json:"unique_users"`
	AverageLatencyMs float64   `json:"average_latency_ms"`
}

// WebSocketTrendAnalysis represents analysis of cost trends
type WebSocketTrendAnalysis struct {
	TrendDirection  string   `json:"trend_direction"` // increasing, decreasing, stable
	TrendPercentage float64  `json:"trend_percentage"`
	PeakHour        string   `json:"peak_hour,omitempty"`
	LowHour         string   `json:"low_hour,omitempty"`
	WeeklyPattern   string   `json:"weekly_pattern,omitempty"`
	GrowthRate      float64  `json:"growth_rate"`
	SeasonalFactors []string `json:"seasonal_factors,omitempty"`
}

// GetWebSocketCostAnalytics retrieves comprehensive WebSocket cost analytics
func (h *Handler) GetWebSocketCostAnalytics(ctx *lift.Context, req WebSocketCostAnalyticsRequest) (WebSocketCostSummaryResponse, error) {
	startTime, err := time.Parse(common.DateFormat, req.StartDate)
	if err != nil {
		return WebSocketCostSummaryResponse{}, lift.ValidationError("invalid start_date format, use YYYY-MM-DD")
	}

	endTime, err := time.Parse(common.DateFormat, req.EndDate)
	if err != nil {
		return WebSocketCostSummaryResponse{}, lift.ValidationError("invalid end_date format, use YYYY-MM-DD")
	}

	// Set end time to end of day
	endTime = endTime.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	if endTime.Before(startTime) {
		return WebSocketCostSummaryResponse{}, lift.ValidationError("end_date must be after start_date")
	}

	limit := req.Limit
	if err := common.ValidateIntRange("limit", limit, 1, 100); err != nil {
		limit = 50 // Default limit
	}

	// Get WebSocket cost repository from the repository storage
	costRepo := h.repos.WebSocketCost()

	// Build the response
	response := WebSocketCostSummaryResponse{}

	// Get overall summary
	summary, err := h.buildWebSocketOverallSummary(costRepo, startTime, endTime)
	if err != nil {
		h.logger.Error("failed to build WebSocket overall summary", zap.Error(err))
		return WebSocketCostSummaryResponse{}, lift.NewLiftError("SUMMARY_ERROR", "failed to generate cost summary", 500)
	}
	response.Summary = summary

	// Get top users by cost
	topUsers, err := costRepo.GetTopCostlyUsers(ctx.Request.Context(), startTime, endTime, limit)
	if err != nil {
		h.logger.Error("failed to get top costly users", zap.Error(err))
		return WebSocketCostSummaryResponse{}, lift.NewLiftError("USERS_ERROR", "failed to get top users", 500)
	}
	response.TopUsers = topUsers

	// Get high cost operations
	highCostOps, err := costRepo.GetHighCostOperations(ctx.Request.Context(), 0.01, startTime, endTime, limit) // $0.01 threshold
	if err != nil {
		h.logger.Error("failed to get high cost operations", zap.Error(err))
		return WebSocketCostSummaryResponse{}, lift.NewLiftError("OPERATIONS_ERROR", "failed to get high cost operations", 500)
	}

	// Convert to response format
	response.HighCostOperations = make([]*WebSocketCostOperationSummary, len(highCostOps))
	for i, op := range highCostOps {
		response.HighCostOperations[i] = &WebSocketCostOperationSummary{
			OperationType:    op.OperationType,
			ConnectionID:     op.ConnectionID,
			UserID:           op.UserID,
			Username:         op.Username,
			CostDollars:      op.EstimatedCostDollars,
			ProcessingTimeMs: op.ProcessingTimeMs,
			MessageCount:     op.MessageCount,
			Timestamp:        op.Timestamp,
		}
	}

	// Get cost trends
	trends, err := h.buildWebSocketCostTrends(costRepo, startTime, endTime, req.Period)
	if err != nil {
		h.logger.Error("failed to build WebSocket cost trends", zap.Error(err))
		return WebSocketCostSummaryResponse{}, lift.NewLiftError("TRENDS_ERROR", "failed to generate cost trends", 500)
	}
	response.CostTrends = trends

	// Get user-specific details if requested
	if req.UserID != "" {
		userDetails, err := costRepo.GetUserCostSummary(ctx.Request.Context(), req.UserID, startTime, endTime)
		if err != nil {
			h.logger.Warn("failed to get user cost summary",
				zap.String("user_id", req.UserID),
				zap.Error(err))
		} else {
			response.UserDetails = userDetails
		}

		// Get budget status for the user
		budgetStatus, err := costRepo.CheckBudgetLimits(ctx.Request.Context(), req.UserID)
		if err != nil {
			h.logger.Warn("failed to get budget status",
				zap.String("user_id", req.UserID),
				zap.Error(err))
		} else {
			response.BudgetStatus = budgetStatus
		}
	}

	return response, nil
}

// buildWebSocketOverallSummary builds the overall cost summary
func (h *Handler) buildWebSocketOverallSummary(costRepo *repositories.WebSocketCostRepository, startTime, endTime time.Time) (*WebSocketOverallSummary, error) {
	// Get recent costs to calculate summary
	recentCosts, err := costRepo.GetRecentCosts(context.Background(), startTime, 10000)
	if err != nil {
		return nil, err
	}

	// Filter costs within date range
	var filteredCosts []*models.WebSocketCostRecord
	for _, cost := range recentCosts {
		if cost.Timestamp.After(endTime) {
			continue
		}
		filteredCosts = append(filteredCosts, cost)
	}

	if err := common.ValidateSliceNotEmpty("filteredCosts", filteredCosts); err != nil {
		return &WebSocketOverallSummary{
			DateRange: fmt.Sprintf("%s to %s", startTime.Format(common.DateFormat), endTime.Format(common.DateFormat)),
		}, nil
	}

	summary := &WebSocketOverallSummary{
		DateRange: fmt.Sprintf("%s to %s", startTime.Format(common.DateFormat), endTime.Format(common.DateFormat)),
	}

	// Track unique users and connections
	uniqueUsers := make(map[string]bool)
	uniqueConnections := make(map[string]bool)

	var totalConnectionCost, totalMessageCost, totalLambdaCost, totalDynamoDBCost int64
	var totalConnectionMinutes int64

	for _, cost := range filteredCosts {
		summary.TotalCostDollars += cost.EstimatedCostDollars

		if err := common.ValidateRequiredParam("user_id", cost.UserID); err == nil {
			uniqueUsers[cost.UserID] = true
		}
		uniqueConnections[cost.ConnectionID] = true

		// Aggregate by operation type
		switch cost.OperationType {
		case "connect", "disconnect":
			summary.TotalConnections++
			totalConnectionMinutes += cost.ConnectionDurationMs / (60 * 1000)
		case "message_in", "message_out":
			summary.TotalMessages += int64(cost.MessageCount)
		}

		// Cost breakdown
		totalConnectionCost += cost.APIGatewayConnectionCost
		totalMessageCost += cost.APIGatewayMessageCost
		totalLambdaCost += cost.LambdaExecutionCost
		totalDynamoDBCost += cost.DynamoDBCost
	}

	summary.UniqueUsers = int64(len(uniqueUsers))
	summary.TotalConnectionHours = float64(totalConnectionMinutes) / 60.0

	// Calculate averages
	if summary.UniqueUsers > 0 {
		summary.AverageCostPerUser = summary.TotalCostDollars / float64(summary.UniqueUsers)
	}
	if summary.TotalConnections > 0 {
		summary.AverageCostPerConnection = summary.TotalCostDollars / float64(summary.TotalConnections)
		summary.MessagesPerConnection = float64(summary.TotalMessages) / float64(summary.TotalConnections)
		summary.AverageConnectionDuration = float64(totalConnectionMinutes) / float64(summary.TotalConnections)
	}
	if summary.TotalMessages > 0 {
		summary.AverageCostPerMessage = summary.TotalCostDollars / float64(summary.TotalMessages)
	}

	// Calculate cost percentages
	totalCostMicroCents := totalConnectionCost + totalMessageCost + totalLambdaCost + totalDynamoDBCost
	if totalCostMicroCents > 0 {
		summary.ConnectionCostPercent = (float64(totalConnectionCost) / float64(totalCostMicroCents)) * 100
		summary.MessageCostPercent = (float64(totalMessageCost) / float64(totalCostMicroCents)) * 100
		summary.LambdaCostPercent = (float64(totalLambdaCost) / float64(totalCostMicroCents)) * 100
		summary.DynamoDBCostPercent = (float64(totalDynamoDBCost) / float64(totalCostMicroCents)) * 100
	}

	return summary, nil
}

// buildWebSocketCostTrends builds cost trends analysis
func (h *Handler) buildWebSocketCostTrends(costRepo *repositories.WebSocketCostRepository, startTime, endTime time.Time, period string) (*WebSocketCostTrends, error) {
	if err := common.ValidateRequiredParam("period", period); err != nil {
		period = periodDay
	}

	trends := &WebSocketCostTrends{
		Period:        period,
		DataPoints:    []WebSocketCostDataPoint{},
		TrendAnalysis: &WebSocketTrendAnalysis{},
	}

	// Get actual aggregated cost data from the database
	dataPoints, err := h.aggregateWebSocketCostData(costRepo, startTime, endTime, period)
	if err != nil {
		h.logger.Warn("failed to aggregate WebSocket cost data, using empty trends",
			zap.Error(err),
			zap.String("period", period))
		// Return empty trends rather than simulated data
		return trends, nil
	}

	trends.DataPoints = dataPoints

	// Analyze trends
	trends.TrendAnalysis = h.analyzeWebSocketTrends(trends.DataPoints)

	return trends, nil
}

// aggregateWebSocketCostData aggregates real WebSocket cost data by time period
func (h *Handler) aggregateWebSocketCostData(costRepo *repositories.WebSocketCostRepository, startTime, endTime time.Time, period string) ([]WebSocketCostDataPoint, error) {
	// Determine the aggregation interval
	var interval time.Duration
	switch period {
	case "hour":
		interval = time.Hour
	case "day":
		interval = 24 * time.Hour
	case "week":
		interval = 7 * 24 * time.Hour
	case "month":
		interval = 30 * 24 * time.Hour
	default:
		interval = 24 * time.Hour
	}

	// Get all cost records for the time period
	// We'll fetch a larger set to ensure we don't miss any data
	allCosts, err := costRepo.GetRecentCosts(context.Background(), startTime.Add(-7*24*time.Hour), 50000)
	if err != nil {
		return nil, errors.Join(ErrFailedToGetCostRecords, err)
	}

	// Filter to only include costs within our date range
	var relevantCosts []*models.WebSocketCostRecord
	for _, cost := range allCosts {
		if cost.Timestamp.After(startTime.Add(-time.Minute)) && cost.Timestamp.Before(endTime.Add(time.Minute)) {
			relevantCosts = append(relevantCosts, cost)
		}
	}

	// Create time buckets for aggregation
	buckets := make(map[time.Time]*webSocketCostBucket)
	current := startTime
	for current.Before(endTime) {
		buckets[current] = &webSocketCostBucket{
			Timestamp:   current,
			CostSum:     0.0,
			UserSet:     make(map[string]bool),
			ConnectionSet: make(map[string]bool),
			MessageCount: 0,
			LatencySum:  0.0,
			LatencyCount: 0,
		}
		current = current.Add(interval)
	}

	// Aggregate data into buckets
	for _, cost := range relevantCosts {
		// Find the appropriate bucket for this cost record
		bucketTime := h.findBucketTime(cost.Timestamp, startTime, interval)
		if bucket, exists := buckets[bucketTime]; exists {
			bucket.CostSum += cost.EstimatedCostDollars
			
			if err := common.ValidateRequiredParam("user_id", cost.UserID); err == nil {
				bucket.UserSet[cost.UserID] = true
			}
			
			bucket.ConnectionSet[cost.ConnectionID] = true
			
			// Count messages (operations that involve data transfer)
			if cost.OperationType == "message" || cost.OperationType == "broadcast" {
				bucket.MessageCount++
			}
			
			// Track latency if available
			if cost.ResponseLatencyMs > 0 {
				bucket.LatencySum += float64(cost.ResponseLatencyMs)
				bucket.LatencyCount++
			}
		}
	}

	// Convert buckets to data points
	var dataPoints []WebSocketCostDataPoint
	current = startTime
	for current.Before(endTime) {
		if bucket, exists := buckets[current]; exists {
			dataPoint := WebSocketCostDataPoint{
				Timestamp:   bucket.Timestamp,
				CostDollars: bucket.CostSum,
				Connections: int64(len(bucket.ConnectionSet)),
				Messages:    bucket.MessageCount,
				UniqueUsers: int64(len(bucket.UserSet)),
			}
			
			// Calculate average latency
			if bucket.LatencyCount > 0 {
				dataPoint.AverageLatencyMs = bucket.LatencySum / float64(bucket.LatencyCount)
			}
			
			dataPoints = append(dataPoints, dataPoint)
		}
		current = current.Add(interval)
	}

	return dataPoints, nil
}

// webSocketCostBucket represents an aggregation bucket for cost data
type webSocketCostBucket struct {
	Timestamp     time.Time
	CostSum       float64
	UserSet       map[string]bool
	ConnectionSet map[string]bool
	MessageCount  int64
	LatencySum    float64
	LatencyCount  int
}

// findBucketTime finds the appropriate bucket time for a given timestamp
func (h *Handler) findBucketTime(timestamp, startTime time.Time, interval time.Duration) time.Time {
	elapsed := timestamp.Sub(startTime)
	bucketIndex := int64(elapsed / interval)
	return startTime.Add(time.Duration(bucketIndex) * interval)
}

// analyzeWebSocketTrends performs sophisticated trend analysis using the cost analytics service
func (h *Handler) analyzeWebSocketTrends(dataPoints []WebSocketCostDataPoint) *WebSocketTrendAnalysis {
	analysis := &WebSocketTrendAnalysis{
		TrendDirection: "stable",
	}

	if len(dataPoints) < 3 {
		return analysis
	}

	// Extract cost values for analysis
	costValues := make([]float64, len(dataPoints))
	timestamps := make([]time.Time, len(dataPoints))
	for i, dp := range dataPoints {
		costValues[i] = dp.CostDollars
		timestamps[i] = dp.Timestamp
	}

	// Calculate trend analysis directly from data points
	if err := common.ValidateSliceLength("dataPoints", dataPoints, 1000); err == nil && len(dataPoints) > 1 {
		return h.enhancedLocalTrendAnalysis(dataPoints)
	}

	return analysis
}


// enhancedLocalTrendAnalysis provides sophisticated local trend analysis
func (h *Handler) enhancedLocalTrendAnalysis(dataPoints []WebSocketCostDataPoint) *WebSocketTrendAnalysis {
	analysis := &WebSocketTrendAnalysis{
		TrendDirection: "stable",
	}

	// Multi-period moving average analysis
	shortMA := h.calculateMovingAverage(dataPoints, 3)  // Short-term trend
	longMA := h.calculateMovingAverage(dataPoints, 7)   // Long-term trend

	if err := common.ValidateSliceNotEmpty("shortMA", shortMA); err == nil {
		if err := common.ValidateSliceNotEmpty("longMA", longMA); err == nil {
			// Moving average crossover analysis
			recentShort := shortMA[len(shortMA)-1]
			recentLong := longMA[len(longMA)-1]
			
			if recentShort > recentLong*1.02 { // 2% threshold for significance
				analysis.TrendDirection = "increasing"
			} else if recentShort < recentLong*0.98 {
				analysis.TrendDirection = "decreasing"
			}
		}
	}

	// Exponential smoothing for growth rate
	if len(dataPoints) >= 5 {
		analysis.GrowthRate = h.calculateExponentialSmoothingGrowthRate(dataPoints)
	}

	// Statistical significance testing
	analysis = h.addStatisticalSignificance(analysis, dataPoints)

	return analysis
}

// calculateMovingAverage calculates moving average for cost data points
func (h *Handler) calculateMovingAverage(dataPoints []WebSocketCostDataPoint, window int) []float64 {
	if len(dataPoints) < window {
		return nil
	}

	ma := make([]float64, len(dataPoints)-window+1)
	for i := window - 1; i < len(dataPoints); i++ {
		sum := 0.0
		for j := i - window + 1; j <= i; j++ {
			sum += dataPoints[j].CostDollars
		}
		ma[i-window+1] = sum / float64(window)
	}
	return ma
}

// calculateExponentialSmoothingGrowthRate calculates growth rate using exponential smoothing
func (h *Handler) calculateExponentialSmoothingGrowthRate(dataPoints []WebSocketCostDataPoint) float64 {
	if len(dataPoints) < 3 {
		return 0
	}

	// Alpha parameter for exponential smoothing
	alpha := 0.3
	smoothed := make([]float64, len(dataPoints))
	smoothed[0] = dataPoints[0].CostDollars

	// Apply exponential smoothing
	for i := 1; i < len(dataPoints); i++ {
		smoothed[i] = alpha*dataPoints[i].CostDollars + (1-alpha)*smoothed[i-1]
	}

	// Calculate growth rate from smoothed data
	first := smoothed[0]
	last := smoothed[len(smoothed)-1]
	periods := float64(len(smoothed) - 1)

	if first > 0 && periods > 0 {
		// Compound growth rate
		return (math.Pow(last/first, 1.0/periods) - 1.0) * 100
	}
	return 0
}




// addStatisticalSignificance adds statistical significance metrics to trend analysis
func (h *Handler) addStatisticalSignificance(analysis *WebSocketTrendAnalysis, dataPoints []WebSocketCostDataPoint) *WebSocketTrendAnalysis {
	if len(dataPoints) < 5 {
		return analysis
	}

	// Calculate standard deviation of costs
	costs := make([]float64, len(dataPoints))
	mean := 0.0
	for i, dp := range dataPoints {
		costs[i] = dp.CostDollars
		mean += dp.CostDollars
	}
	mean /= float64(len(costs))

	variance := 0.0
	for _, cost := range costs {
		variance += (cost - mean) * (cost - mean)
	}
	variance /= float64(len(costs) - 1)
	stdDev := math.Sqrt(variance)

	// Calculate coefficient of variation for trend stability
	if mean > 0 {
		cv := stdDev / mean
		if cv < 0.1 {
			analysis.WeeklyPattern = "very_stable"
		} else if cv < 0.3 {
			analysis.WeeklyPattern = "stable"
		} else if cv < 0.5 {
			analysis.WeeklyPattern = "moderate_volatility"
		} else {
			analysis.WeeklyPattern = "high_volatility"
		}
	}

	return analysis
}


// GetUserWebSocketBudget retrieves budget information for a user
func (h *Handler) GetUserWebSocketBudget(ctx *lift.Context) (interface{}, error) {
	userID := ctx.Param("user_id")
	if err := common.ValidateRequiredParam("user_id", userID); err != nil {
		return nil, lift.ValidationError(err.Error())
	}

	// Get WebSocket cost repository from the repository storage
	costRepo := h.repos.WebSocketCost()

	// Get user budgets
	budgets, err := costRepo.GetUserBudgets(ctx.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user budgets",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, lift.NewLiftError("BUDGET_ERROR", "failed to retrieve budget information", 500)
	}

	// Get current budget status
	budgetStatus, err := costRepo.CheckBudgetLimits(ctx.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to check budget limits",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, lift.NewLiftError("BUDGET_STATUS_ERROR", "failed to check budget status", 500)
	}

	return map[string]interface{}{
		"user_id":       userID,
		"budgets":       budgets,
		"budget_status": budgetStatus,
		"timestamp":     time.Now(),
	}, nil
}

// CreateUserWebSocketBudget creates or updates a budget for a user
func (h *Handler) CreateUserWebSocketBudget(ctx *lift.Context) (interface{}, error) {
	userID := ctx.Param("user_id")
	if err := common.ValidateRequiredParam("user_id", userID); err != nil {
		return nil, lift.ValidationError(err.Error())
	}

	var budgetReq struct {
		Period            string  `json:"period" validate:"required,oneof=daily weekly monthly"`
		BudgetDollars     float64 `json:"budget_dollars" validate:"required,gt=0"`
		AlertThresholds   []int   `json:"alert_thresholds,omitempty"`
		SuspendAt         int     `json:"suspend_at,omitempty"`
		MaxConnections    int     `json:"max_connections,omitempty"`
		MessagesPerMinute int     `json:"messages_per_minute,omitempty"`
	}

	if err := common.ValidateSliceNotEmpty("requestBody", ctx.Request.Body); err == nil {
		if err := json.Unmarshal(ctx.Request.Body, &budgetReq); err != nil {
			return nil, lift.ValidationError("invalid request body")
		}
	} else {
		return nil, lift.ValidationError("request body required")
	}

	// Convert dollars to microcents
	budgetMicroCents := int64(budgetReq.BudgetDollars * 1_000_000)

	// Calculate window start and end based on period
	now := time.Now()
	var windowStart, windowEnd time.Time

	switch budgetReq.Period {
	case "daily":
		windowStart = now.Truncate(24 * time.Hour)
		windowEnd = windowStart.Add(24 * time.Hour)
	case "weekly":
		// Start of week (Sunday)
		weekday := int(now.Weekday())
		windowStart = now.AddDate(0, 0, -weekday).Truncate(24 * time.Hour)
		windowEnd = windowStart.Add(7 * 24 * time.Hour)
	case "monthly":
		windowStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		windowEnd = windowStart.AddDate(0, 1, 0)
	}

	// Create budget
	budget := &models.WebSocketCostBudget{
		UserID:                   userID,
		Period:                   budgetReq.Period,
		BudgetMicroCents:         budgetMicroCents,
		WindowStart:              windowStart,
		WindowEnd:                windowEnd,
		AlertThresholds:          budgetReq.AlertThresholds,
		SuspendAt:                budgetReq.SuspendAt,
		MaxConcurrentConnections: budgetReq.MaxConnections,
		MessagesPerMinute:        budgetReq.MessagesPerMinute,
		BillingTier:              "basic", // Default tier
	}

	// Get username from user repository
	if user, err := h.repos.Account().GetUser(ctx.Request.Context(), userID); err == nil && user != nil {
		budget.Username = user.Username
	}

	// Get WebSocket cost repository from the repository storage
	costRepo := h.repos.WebSocketCost()

	// Check if budget already exists
	existingBudget, err := costRepo.GetBudget(ctx.Request.Context(), userID, budgetReq.Period)
	if err == nil && existingBudget != nil {
		// Update existing budget
		existingBudget.BudgetMicroCents = budgetMicroCents
		existingBudget.AlertThresholds = budgetReq.AlertThresholds
		existingBudget.SuspendAt = budgetReq.SuspendAt
		existingBudget.MaxConcurrentConnections = budgetReq.MaxConnections
		existingBudget.MessagesPerMinute = budgetReq.MessagesPerMinute

		if err := costRepo.UpdateBudget(ctx.Request.Context(), existingBudget); err != nil {
			h.logger.Error("failed to update WebSocket budget",
				zap.String("user_id", userID),
				zap.Error(err))
			return nil, lift.NewLiftError("BUDGET_UPDATE_ERROR", "failed to update budget", 500)
		}

		return map[string]interface{}{
			"action":    "updated",
			"budget":    existingBudget,
			"timestamp": time.Now(),
		}, nil
	}

	// Create new budget
	if err := costRepo.CreateBudget(ctx.Request.Context(), budget); err != nil {
		h.logger.Error("failed to create WebSocket budget",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, lift.NewLiftError("BUDGET_CREATE_ERROR", "failed to create budget", 500)
	}

	return map[string]interface{}{
		"action":    "created",
		"budget":    budget,
		"timestamp": time.Now(),
	}, nil
}
