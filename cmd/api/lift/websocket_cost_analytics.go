// +build ignore

package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
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
	Summary              *WebSocketOverallSummary                             `json:"summary"`
	TopUsers             []*repositories.WebSocketUserCostRanking             `json:"top_users"`
	HighCostOperations   []*WebSocketCostOperationSummary                     `json:"high_cost_operations"`
	CostTrends           *WebSocketCostTrends                                 `json:"cost_trends"`
	UserDetails          *repositories.WebSocketUserCostSummary               `json:"user_details,omitempty"`
	BudgetStatus         *repositories.BudgetStatus                           `json:"budget_status,omitempty"`
}

// WebSocketOverallSummary represents overall WebSocket cost summary
type WebSocketOverallSummary struct {
	DateRange            string  `json:"date_range"`
	TotalCostDollars     float64 `json:"total_cost_dollars"`
	TotalConnections     int64   `json:"total_connections"`
	TotalMessages        int64   `json:"total_messages"`
	TotalConnectionHours float64 `json:"total_connection_hours"`
	UniqueUsers          int64   `json:"unique_users"`
	AverageCostPerUser   float64 `json:"average_cost_per_user"`
	AverageCostPerConnection float64 `json:"average_cost_per_connection"`
	AverageCostPerMessage float64    `json:"average_cost_per_message"`
	
	// Cost breakdown
	ConnectionCostPercent float64 `json:"connection_cost_percent"`
	MessageCostPercent    float64 `json:"message_cost_percent"`
	LambdaCostPercent     float64 `json:"lambda_cost_percent"`
	DynamoDBCostPercent   float64 `json:"dynamodb_cost_percent"`
	
	// Efficiency metrics
	MessagesPerConnection float64 `json:"messages_per_connection"`
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
	Period     string                    `json:"period"`
	DataPoints []WebSocketCostDataPoint  `json:"data_points"`
	TrendAnalysis *WebSocketTrendAnalysis `json:"trend_analysis"`
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
	TrendDirection   string  `json:"trend_direction"` // increasing, decreasing, stable
	TrendPercentage  float64 `json:"trend_percentage"`
	PeakHour         string  `json:"peak_hour,omitempty"`
	LowHour          string  `json:"low_hour,omitempty"`
	WeeklyPattern    string  `json:"weekly_pattern,omitempty"`
	GrowthRate       float64 `json:"growth_rate"`
	SeasonalFactors  []string `json:"seasonal_factors,omitempty"`
}

// GetWebSocketCostAnalytics retrieves comprehensive WebSocket cost analytics
func (h *Handler) GetWebSocketCostAnalytics(ctx *lift.Context, req WebSocketCostAnalyticsRequest) (WebSocketCostSummaryResponse, error) {
	startTime, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return WebSocketCostSummaryResponse{}, lift.ValidationError("invalid start_date format, use YYYY-MM-DD")
	}

	endTime, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return WebSocketCostSummaryResponse{}, lift.ValidationError("invalid end_date format, use YYYY-MM-DD")
	}

	// Set end time to end of day
	endTime = endTime.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	if endTime.Before(startTime) {
		return WebSocketCostSummaryResponse{}, lift.ValidationError("end_date must be after start_date")
	}

	limit := req.Limit
	if limit == 0 || limit > 100 {
		limit = 50 // Default limit
	}

	// Initialize WebSocket cost repository (this would be injected in a real implementation)
	costRepo := repositories.NewWebSocketCostRepository(h.storageAdapter.DB(), "lesser-main", h.logger)

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

	if len(filteredCosts) == 0 {
		return &WebSocketOverallSummary{
			DateRange: fmt.Sprintf("%s to %s", startTime.Format("2006-01-02"), endTime.Format("2006-01-02")),
		}, nil
	}

	summary := &WebSocketOverallSummary{
		DateRange: fmt.Sprintf("%s to %s", startTime.Format("2006-01-02"), endTime.Format("2006-01-02")),
	}

	// Track unique users and connections
	uniqueUsers := make(map[string]bool)
	uniqueConnections := make(map[string]bool)
	
	var totalConnectionCost, totalMessageCost, totalLambdaCost, totalDynamoDBCost int64
	var totalConnectionMinutes int64

	for _, cost := range filteredCosts {
		summary.TotalCostDollars += cost.EstimatedCostDollars
		
		if cost.UserID != "" {
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
	if period == "" {
		period = "day"
	}

	trends := &WebSocketCostTrends{
		Period:     period,
		DataPoints: []WebSocketCostDataPoint{},
		TrendAnalysis: &WebSocketTrendAnalysis{},
	}

	// Get aggregated data for the period
	// For now, simulate trend data since we'd need actual aggregation records
	// In a real implementation, this would query WebSocketCostAggregation records

	// Generate sample data points based on period
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

	// Create data points (this would be replaced with actual database queries)
	current := startTime
	for current.Before(endTime) {
		// In a real implementation, query aggregated data for this time window
		dataPoint := WebSocketCostDataPoint{
			Timestamp:        current,
			CostDollars:      0.0, // Would be calculated from actual data
			Connections:      0,   // Would be calculated from actual data
			Messages:         0,   // Would be calculated from actual data
			UniqueUsers:      0,   // Would be calculated from actual data
			AverageLatencyMs: 0.0, // Would be calculated from actual data
		}
		
		trends.DataPoints = append(trends.DataPoints, dataPoint)
		current = current.Add(interval)
	}

	// Analyze trends
	trends.TrendAnalysis = h.analyzeWebSocketTrends(trends.DataPoints)

	return trends, nil
}

// analyzeWebSocketTrends analyzes trend patterns in the data
func (h *Handler) analyzeWebSocketTrends(dataPoints []WebSocketCostDataPoint) *WebSocketTrendAnalysis {
	analysis := &WebSocketTrendAnalysis{
		TrendDirection: "stable",
	}

	if len(dataPoints) < 2 {
		return analysis
	}

	// Calculate trend direction and percentage
	firstCost := dataPoints[0].CostDollars
	lastCost := dataPoints[len(dataPoints)-1].CostDollars

	if lastCost > firstCost {
		analysis.TrendDirection = "increasing"
		if firstCost > 0 {
			analysis.TrendPercentage = ((lastCost - firstCost) / firstCost) * 100
		}
	} else if lastCost < firstCost {
		analysis.TrendDirection = "decreasing"
		if firstCost > 0 {
			analysis.TrendPercentage = ((firstCost - lastCost) / firstCost) * 100
		}
	}

	// Find peak and low points
	var maxCost, minCost float64
	var maxTime, minTime time.Time
	
	for i, dp := range dataPoints {
		if i == 0 || dp.CostDollars > maxCost {
			maxCost = dp.CostDollars
			maxTime = dp.Timestamp
		}
		if i == 0 || dp.CostDollars < minCost {
			minCost = dp.CostDollars
			minTime = dp.Timestamp
		}
	}

	analysis.PeakHour = maxTime.Format("15:04 MST")
	analysis.LowHour = minTime.Format("15:04 MST")

	// Simple growth rate calculation (would be more sophisticated in real implementation)
	if len(dataPoints) > 1 {
		totalGrowth := lastCost - firstCost
		periods := float64(len(dataPoints) - 1)
		if periods > 0 && firstCost > 0 {
			analysis.GrowthRate = (totalGrowth / firstCost / periods) * 100
		}
	}

	return analysis
}

// GetUserWebSocketBudget retrieves budget information for a user
func (h *Handler) GetUserWebSocketBudget(ctx *lift.Context) (interface{}, error) {
	userID := ctx.Param("user_id")
	if userID == "" {
		return nil, lift.ValidationError("user_id is required")
	}

	// Initialize cost repository
	costRepo := repositories.NewWebSocketCostRepository(h.storageAdapter.DB(), "lesser-main", h.logger)

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
	if userID == "" {
		return nil, lift.ValidationError("user_id is required")
	}

	var budgetReq struct {
		Period           string  `json:"period" validate:"required,oneof=daily weekly monthly"`
		BudgetDollars    float64 `json:"budget_dollars" validate:"required,gt=0"`
		AlertThresholds  []int   `json:"alert_thresholds,omitempty"`
		SuspendAt        int     `json:"suspend_at,omitempty"`
		MaxConnections   int     `json:"max_connections,omitempty"`
		MessagesPerMinute int    `json:"messages_per_minute,omitempty"`
	}

	if ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
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
		UserID:               userID,
		Period:               budgetReq.Period,
		BudgetMicroCents:     budgetMicroCents,
		WindowStart:          windowStart,
		WindowEnd:            windowEnd,
		AlertThresholds:      budgetReq.AlertThresholds,
		SuspendAt:            budgetReq.SuspendAt,
		MaxConcurrentConnections: budgetReq.MaxConnections,
		MessagesPerMinute:    budgetReq.MessagesPerMinute,
		BillingTier:          "basic", // Default tier
	}

	// Get username from user repository
	if user, err := h.storageAdapter.GetUser(ctx.Request.Context(), userID); err == nil && user != nil {
		budget.Username = user.Username
	}

	// Initialize cost repository
	costRepo := repositories.NewWebSocketCostRepository(h.storageAdapter.DB(), "lesser-main", h.logger)

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