package handlers

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const bytesPerGB = 1024 * 1024 * 1024

// HandleGetInstanceMetricsLift returns current instance metrics
func (h *Handler) HandleGetInstanceMetricsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("HandleGetInstanceMetricsLift called")

	// Get current metrics from DynamoDB
	activeUsers, err := h.repos.Analytics().GetActiveUserCount(ctx.Context(), 30) // Active in last 30 days
	if err != nil {
		h.logger.Warn("failed to get active user count", zap.Error(err))
		activeUsers = 0
	}

	// Get request metrics from cost data if available
	requestsPerMinute := 0.0
	avgLatencyMs := 0.0

	// Calculate actual request rate from time-series data
	requestsPerMinute = h.calculateRequestRateLift(ctx.Context())

	// Get today's metrics from cost tracking repository
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	dailyCosts, err := h.repos.Cost().GetCostsByDateRange(ctx.Context(), startOfDay, endOfDay)
	if err == nil && len(dailyCosts) > 0 {
		// Count operations as proxy for requests
		todayRequests := int64(len(dailyCosts))

		if todayRequests > 0 {
			// Calculate requests per minute
			hoursSinceStart := time.Since(startOfDay).Hours()
			if hoursSinceStart > 0 {
				requestsPerMinute = float64(todayRequests) / (hoursSinceStart * 60.0)
			}

			// Get real latency data from MetricRecord repository
			avgLatencyMs = h.calculateRealLatencyMetricsLift(ctx.Context(), startOfDay, endOfDay)
		}
	}

	metrics := map[string]any{
		"current": map[string]any{
			"active_users":        activeUsers,
			"requests_per_minute": requestsPerMinute,
			"avg_latency_ms":      avgLatencyMs,
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
		},
		"system": map[string]any{
			"version":     "2.0.0",
			"uptime_days": 30, // Placeholder - serverless doesn't have traditional uptime
			"region":      h.cfg.Region,
		},
	}

	return okJSON(metrics)
}

// HandleGetDailyAggregatesLift returns daily aggregated metrics
func (h *Handler) HandleGetDailyAggregatesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("HandleGetDailyAggregatesLift called")

	// Parse query parameters
	daysStr := queryValue(ctx, "days")
	days, err := common.ParseAndValidateIntWithBounds("days", daysStr, 0, 30, 7)
	if err != nil {
		days = 7 // Default to last 7 days
	}

	// Get daily aggregates
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days+1)

	var dailyMetrics []map[string]any

	// Get aggregated daily costs from repository
	dailyAggregates, err := h.repos.Cost().GetDailyAggregates(ctx.Context(), startDate, endDate)
	if err != nil {
		h.logger.Error("failed to get daily aggregates", zap.Error(err))
	} else {
		for _, daily := range dailyAggregates {
			metrics := map[string]any{
				"date": daily.Date.Format(common.DateFormat),
				"metrics": map[string]any{
					"total_requests":     daily.TotalRequests,
					"unique_users":       daily.UniqueUsers,
					"dynamodb_reads":     daily.TotalReads,
					"dynamodb_writes":    daily.TotalWrites,
					"lambda_duration_ms": daily.TotalDurationMs,
					"cost_cents":         daily.TotalCostDollars * 100, // Convert dollars to cents
				},
			}
			dailyMetrics = append(dailyMetrics, metrics)
		}
	}

	// Return empty data if no real metrics available
	// Never generate fabricated data that could mislead users about actual system performance

	response := map[string]any{
		"period": map[string]any{
			"start": startDate.Format(common.DateFormat),
			"end":   endDate.Format(common.DateFormat),
			"days":  days,
		},
		"daily_aggregates": dailyMetrics,
	}

	return okJSON(response)
}

// HandleGetPredictiveAnalyticsLift returns predictive analytics
func (h *Handler) HandleGetPredictiveAnalyticsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("HandleGetPredictiveAnalyticsLift called")

	// Get current month data for projections
	now := time.Now()
	monthlyProjection := 0.0
	currentMonthCost := 0.0
	// Growth rates will be calculated from actual historical data
	// No default estimates - use 0.0 if insufficient data
	storageGrowthRate := 0.0
	userGrowthRate := 0.0

	// Get current month aggregates
	monthlyAggregate, err := h.repos.Cost().GetMonthlyAggregate(ctx.Context(), now.Year(), int(now.Month()))
	if err != nil {
		h.logger.Error("failed to get monthly aggregate", zap.Error(err))
	} else if monthlyAggregate != nil {
		// Current month cost in dollars
		currentMonthCost = monthlyAggregate.TotalCostDollars

		// Calculate projection based on days elapsed
		dayOfMonth := now.Day()
		daysInMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if dayOfMonth > 0 {
			monthlyProjection = currentMonthCost * float64(daysInMonth) / float64(dayOfMonth)
		}
	}

	// Calculate growth rates from historical data
	// Get last 30 days of data to calculate trends
	endDate := now
	startDate := now.AddDate(0, 0, -30)
	dailyAggregates, err := h.repos.Cost().GetDailyAggregates(ctx.Context(), startDate, endDate)
	if err == nil && len(dailyAggregates) > 7 {
		// Simple growth rate calculation: compare last week to first week
		firstWeekRequests := int64(0)
		lastWeekRequests := int64(0)

		for i, daily := range dailyAggregates {
			if i < 7 {
				firstWeekRequests += daily.TotalRequests
			} else if i >= len(dailyAggregates)-7 {
				lastWeekRequests += daily.TotalRequests
			}
		}

		if firstWeekRequests > 0 {
			// Calculate weekly growth rate and extrapolate to monthly
			weeklyGrowth := float64(lastWeekRequests-firstWeekRequests) / float64(firstWeekRequests)
			userGrowthRate = weeklyGrowth * 4.0 * 100.0 // Convert to monthly percentage

			// Reasonable bounds
			if userGrowthRate < -50 {
				userGrowthRate = -50
			} else if userGrowthRate > 100 {
				userGrowthRate = 100
			}
		}
	}

	analytics := map[string]any{
		"projections": map[string]any{
			"monthly_cost": map[string]any{
				"current_month":    currentMonthCost,
				"projected_month":  monthlyProjection,
				"next_month":       monthlyProjection * (1.0 + userGrowthRate/100.0),
				"three_months":     monthlyProjection * (1.0 + userGrowthRate/100.0*3.0),
				"confidence_level": calculateConfidenceLevel(len(dailyAggregates)),
			},
			"storage_growth": map[string]any{
				"monthly_rate_percent": storageGrowthRate,
				"projected_gb_30_days": h.calculateStorageProjectionLift(ctx.Context(), 30),
				"projected_gb_90_days": h.calculateStorageProjectionLift(ctx.Context(), 90),
			},
			"user_growth": map[string]any{
				"monthly_rate_percent":  userGrowthRate,
				"projected_mau_30_days": h.calculateUserProjectionLift(ctx.Context(), 30),
				"projected_mau_90_days": h.calculateUserProjectionLift(ctx.Context(), 90),
			},
		},
		"recommendations": []map[string]any{
			{
				"type":                      "cost_optimization",
				"priority":                  "medium",
				"description":               "Enable S3 lifecycle policies to archive old media",
				"potential_savings_percent": 15,
			},
			{
				"type":                "performance",
				"priority":            "high",
				"description":         "Increase Lambda memory to 1GB for 20% faster response times",
				"cost_impact_percent": 5,
			},
		},
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}

	return okJSON(analytics)
}

// Removed getCostStorageLift - now using TrackingRepository

// calculateRequestRateLift calculates current requests per minute from recent data
func (h *Handler) calculateRequestRateLift(ctx context.Context) float64 {
	// Get last hour of request data for more accurate rate calculation
	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour)

	// Get request counts from cost tracking repository
	costs, err := h.repos.Cost().GetCostsByDateRange(ctx, startTime, endTime)
	if err != nil || len(costs) == 0 {
		// Fallback: estimate from active users
		activeUsers, err := h.repos.Analytics().GetActiveUserCount(ctx, 1) // Active in last day
		if err != nil {
			return 0.0
		}

		// Estimate: each active user makes ~10 requests per hour on average
		estimatedHourlyRequests := float64(activeUsers) * 10.0
		return estimatedHourlyRequests / 60.0
	}

	// Count operations as proxy for requests
	totalRequests := int64(len(costs))

	// Calculate requests per minute from hourly data
	if totalRequests > 0 {
		return float64(totalRequests) / 60.0 // Requests per minute
	}

	return 0.0
}

func anyToFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	default:
		return 0, false
	}
}

// calculateStorageProjectionLift projects storage usage for the given number of days
func (h *Handler) calculateStorageProjectionLift(ctx context.Context, days int) float64 {
	// Get current storage usage
	storageResult, err := h.repos.Instance().GetStorageUsage(ctx)
	var currentStorageGB float64
	if err != nil {
		h.logger.Warn("failed to get storage usage", zap.Error(err))
		// Fallback estimate based on user count
		activeUsers, _ := h.repos.Analytics().GetActiveUserCount(ctx, 30)
		currentStorageGB = float64(activeUsers) * 0.5 // 500MB per active user estimate
	} else {
		// InstanceRepository.GetStorageUsage returns bytes in a map; also accept historical numeric formats.
		switch v := storageResult.(type) {
		case map[string]any:
			switch {
			case v["UsageGB"] != nil:
				if usageGB, ok := anyToFloat64(v["UsageGB"]); ok {
					currentStorageGB = usageGB
				}
			case v["total_bytes"] != nil:
				if totalBytes, ok := anyToFloat64(v["total_bytes"]); ok {
					currentStorageGB = totalBytes / bytesPerGB
				}
			case v["total_gb"] != nil:
				if totalGB, ok := anyToFloat64(v["total_gb"]); ok {
					currentStorageGB = totalGB
				}
			}
		default:
			if usageGB, ok := anyToFloat64(storageResult); ok {
				currentStorageGB = usageGB
			}
		}

		if currentStorageGB <= 0 {
			// Fallback if type assertion fails or storage is unknown
			activeUsers, _ := h.repos.Analytics().GetActiveUserCount(ctx, 30)
			currentStorageGB = float64(activeUsers) * 0.5
		}
	}

	// Calculate historical growth rate
	growthRate := h.calculateStorageGrowthRateLift(ctx)

	// Project forward using compound growth
	dailyGrowthRate := growthRate / 30.0 / 100.0 // Convert monthly % to daily decimal
	projectedStorage := currentStorageGB * (1.0 + dailyGrowthRate*float64(days))

	return projectedStorage
}

// calculateStorageGrowthRateLift calculates monthly storage growth rate percentage
func (h *Handler) calculateStorageGrowthRateLift(ctx context.Context) float64 {
	// Get storage usage over the last 60 days
	days := 60

	// Try to get historical storage data
	storageHistory, err := h.repos.Instance().GetStorageHistory(ctx, days)
	if err != nil || len(storageHistory) < 2 {
		// Return 0.0 if insufficient historical data - no fabricated estimates
		return 0.0
	}

	// Calculate growth rate between first and last data points
	var firstUsage, lastUsage float64

	// Safe type assertion for first usage
	if firstMap, ok := storageHistory[0].(map[string]any); ok {
		switch {
		case firstMap["UsageGB"] != nil:
			if usageGB, ok := anyToFloat64(firstMap["UsageGB"]); ok {
				firstUsage = usageGB
			}
		case firstMap["total_bytes"] != nil:
			if totalBytes, ok := anyToFloat64(firstMap["total_bytes"]); ok {
				firstUsage = totalBytes / bytesPerGB
			}
		}
	}

	// Safe type assertion for last usage
	if lastMap, ok := storageHistory[len(storageHistory)-1].(map[string]any); ok {
		switch {
		case lastMap["UsageGB"] != nil:
			if usageGB, ok := anyToFloat64(lastMap["UsageGB"]); ok {
				lastUsage = usageGB
			}
		case lastMap["total_bytes"] != nil:
			if totalBytes, ok := anyToFloat64(lastMap["total_bytes"]); ok {
				lastUsage = totalBytes / bytesPerGB
			}
		}
	}

	if firstUsage <= 0 {
		return 0.0 // Return 0.0 if no base usage data available
	}

	// Calculate growth rate and annualize to monthly
	totalGrowth := (lastUsage - firstUsage) / firstUsage
	daysSpan := float64(len(storageHistory))
	monthlyGrowthRate := (totalGrowth / daysSpan) * 30.0 * 100.0

	// Cap growth rate to reasonable bounds
	if monthlyGrowthRate < -50 {
		return -50
	} else if monthlyGrowthRate > 200 {
		return 200
	}

	return monthlyGrowthRate
}

// calculateUserProjectionLift projects user count for the given number of days
func (h *Handler) calculateUserProjectionLift(ctx context.Context, days int) int {
	// Get current active user count
	currentMAU, err := h.repos.Analytics().GetActiveUserCount(ctx, 30)
	if err != nil {
		h.logger.Warn("failed to get current MAU", zap.Error(err))
		return 100 // Fallback estimate
	}

	// Calculate user growth rate from historical data
	growthRate := h.calculateUserGrowthRateLift(ctx)

	// Project forward using compound growth
	dailyGrowthRate := growthRate / 30.0 / 100.0 // Convert monthly % to daily decimal
	projectedUsers := float64(currentMAU) * (1.0 + dailyGrowthRate*float64(days))

	return int(projectedUsers)
}

// calculateUserGrowthRateLift calculates monthly user growth rate percentage
func (h *Handler) calculateUserGrowthRateLift(ctx context.Context) float64 {
	// Get user registration history for the last 60 days
	days := 60

	userHistory, err := h.repos.Instance().GetUserGrowthHistory(ctx, days)
	if err != nil || len(userHistory) < 2 {
		// Return 0.0 if insufficient historical data - no fabricated estimates
		return 0.0
	}

	// Calculate new user registrations trend
	totalNewUsers := 0
	for _, dailyInterface := range userHistory {
		if dailyMap, ok := dailyInterface.(map[string]any); ok {
			switch {
			case dailyMap["new_users"] != nil:
				if newUsers, ok := anyToFloat64(dailyMap["new_users"]); ok {
					totalNewUsers += int(newUsers)
				}
			case dailyMap["NewRegistrations"] != nil:
				if newRegs, ok := anyToFloat64(dailyMap["NewRegistrations"]); ok {
					totalNewUsers += int(newRegs)
				}
			}
		}
	}

	if totalNewUsers <= 0 {
		return 0.0 // Return 0.0 if no new registrations tracked
	}

	// Calculate monthly growth rate based on new registrations
	daysSpan := float64(len(userHistory))
	dailyNewUsers := float64(totalNewUsers) / daysSpan
	monthlyNewUsers := dailyNewUsers * 30.0

	// Get current total users for growth rate calculation
	currentUsers, err := h.repos.Analytics().GetTotalUserCount(ctx)
	if err != nil || currentUsers <= 0 {
		return 0.0 // Return 0.0 if current user count unavailable
	}

	// Growth rate as percentage of current user base
	monthlyGrowthRate := (monthlyNewUsers / float64(currentUsers)) * 100.0

	// Cap growth rate to reasonable bounds
	if monthlyGrowthRate < 0 {
		return 0
	} else if monthlyGrowthRate > 100 {
		return 100
	}

	return monthlyGrowthRate
}

// calculateConfidenceLevel calculates confidence based on available data points
// More data points = higher confidence in projections
func calculateConfidenceLevel(dataPoints int) float64 {
	if dataPoints == 0 {
		return 0.0 // No confidence with no data
	}
	if dataPoints < 7 {
		return 0.3 // Low confidence with less than a week of data
	}
	if dataPoints < 14 {
		return 0.5 // Medium confidence with less than two weeks
	}
	if dataPoints < 30 {
		return 0.7 // Good confidence with less than a month
	}
	return 0.9 // High confidence with a month or more of data
}

// calculateRealLatencyMetricsLift calculates real average latency from stored metrics
func (h *Handler) calculateRealLatencyMetricsLift(ctx context.Context, startTime, endTime time.Time) float64 {
	// Get latency metrics from the MetricRecord repository
	latencyRecords, err := h.repos.MetricRecord().GetMetricsByType(ctx, "api_endpoint", startTime, endTime)
	if err != nil {
		h.logger.Warn("failed to get latency metrics", zap.Error(err))
		return 0.0
	}

	if err := common.ValidateSliceNotEmpty("latencyRecords", latencyRecords); err != nil {
		// Also try database operation latencies as fallback
		dbLatencyRecords, dbErr := h.repos.MetricRecord().GetMetricsByType(ctx, "database_operation", startTime, endTime)
		if dbErr != nil || len(dbLatencyRecords) == 0 {
			return 0.0
		}
		latencyRecords = dbLatencyRecords
	}

	// Calculate weighted average latency
	var totalLatency float64
	var totalCount int64

	for _, record := range latencyRecords {
		if record.Count > 0 {
			// Use P50 percentile as representative latency, fallback to average
			latency := record.P50
			if latency == 0 {
				latency = record.Sum / float64(record.Count)
			}

			totalLatency += latency * float64(record.Count)
			totalCount += record.Count
		}
	}

	if totalCount == 0 {
		return 0.0
	}

	avgLatency := totalLatency / float64(totalCount)

	h.logger.Debug("calculated real latency metrics",
		zap.Float64("avg_latency_ms", avgLatency),
		zap.Int64("total_requests", totalCount),
		zap.Int("metric_records", len(latencyRecords)),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))

	return avgLatency
}
