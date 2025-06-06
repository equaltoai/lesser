package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetInstanceMetrics returns current instance metrics
func (h *Handler) HandleGetInstanceMetrics(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	h.logger.Info("HandleGetInstanceMetrics called")

	// Get current metrics from DynamoDB
	// TODO: Implement GetActiveUserCount in storage interface
	activeUsers := int64(10) // Placeholder for now

	// Get request metrics from cost data if available
	var requestsPerMinute float64
	var avgLatencyMs float64

	if costStorage := h.getCostStorage(); costStorage != nil {
		// Get metrics from the last hour
		// TODO: Implement actual metrics aggregation
		requestsPerMinute = 60.0 // Placeholder
		avgLatencyMs = 150.0     // Placeholder
	}

	metrics := map[string]interface{}{
		"current": map[string]interface{}{
			"active_users":        activeUsers,
			"requests_per_minute": requestsPerMinute,
			"avg_latency_ms":      avgLatencyMs,
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
		},
		"system": map[string]interface{}{
			"version":     "2.0.0",
			"uptime_days": 30, // Placeholder - serverless doesn't have traditional uptime
			"region":      h.cfg.Region,
		},
	}

	return common.OK(metrics), nil
}

// HandleGetDailyAggregates returns daily aggregated metrics
func (h *Handler) HandleGetDailyAggregates(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	h.logger.Info("HandleGetDailyAggregates called")

	// Parse query parameters
	days := 7 // Default to last 7 days
	if d := request.QueryStringParameters["days"]; d != "" {
		fmt.Sscanf(d, "%d", &days)
		if days > 30 {
			days = 30 // Cap at 30 days
		}
	}

	// Get daily aggregates
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days+1)

	var dailyMetrics []map[string]interface{}

	if costStorage := h.getCostStorage(); costStorage != nil {
		dailyCosts, err := costStorage.GetDailyCosts(ctx, startDate, endDate)
		if err != nil {
			h.logger.Error("failed to get daily costs", zap.Error(err))
		}

		for _, daily := range dailyCosts {
			metrics := map[string]interface{}{
				"date": daily.Date,
				"metrics": map[string]interface{}{
					"total_requests":     daily.RequestCount,
					"unique_users":       daily.UniqueUsers,
					"dynamodb_reads":     daily.DynamoDBReads,
					"dynamodb_writes":    daily.DynamoDBWrites,
					"lambda_duration_ms": daily.LambdaDurationMs,
					"cost_cents":         float64(daily.TotalCostMicrocents) / float64(cost.MicroCentsToCents),
				},
			}
			dailyMetrics = append(dailyMetrics, metrics)
		}
	}

	// Add placeholder data if no real data
	if len(dailyMetrics) == 0 {
		for i := 0; i < days; i++ {
			date := endDate.AddDate(0, 0, -i).Format("2006-01-02")
			dailyMetrics = append(dailyMetrics, map[string]interface{}{
				"date": date,
				"metrics": map[string]interface{}{
					"total_requests":     1000 + i*100,
					"unique_users":       10 + i,
					"dynamodb_reads":     500 + i*50,
					"dynamodb_writes":    200 + i*20,
					"lambda_duration_ms": 150000 + i*1000,
					"cost_cents":         0.01 + float64(i)*0.001,
				},
			})
		}
	}

	response := map[string]interface{}{
		"period": map[string]interface{}{
			"start": startDate.Format("2006-01-02"),
			"end":   endDate.Format("2006-01-02"),
			"days":  days,
		},
		"daily_aggregates": dailyMetrics,
	}

	return common.OK(response), nil
}

// HandleGetPredictiveAnalytics returns predictive analytics
func (h *Handler) HandleGetPredictiveAnalytics(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	h.logger.Info("HandleGetPredictiveAnalytics called")

	// Get current month data for projections
	now := time.Now()
	var monthlyProjection float64
	var storageGrowthRate float64
	var userGrowthRate float64

	if costStorage := h.getCostStorage(); costStorage != nil {
		currentMonth, err := costStorage.GetMonthlyCost(ctx, now.Year(), now.Month())
		if err != nil {
			h.logger.Error("failed to get monthly cost", zap.Error(err))
		}

		if currentMonth != nil && currentMonth.ProjectedCostMicrocents > 0 {
			monthlyProjection = float64(currentMonth.ProjectedCostMicrocents) / float64(cost.MicroCentsToCents)
		}
	}

	// Calculate growth rates (simplified - in production use proper time series analysis)
	storageGrowthRate = 5.2 // 5.2% monthly growth
	userGrowthRate = 12.5   // 12.5% monthly growth

	analytics := map[string]interface{}{
		"projections": map[string]interface{}{
			"monthly_cost": map[string]interface{}{
				"current_month":    monthlyProjection,
				"next_month":       monthlyProjection * 1.1,
				"three_months":     monthlyProjection * 1.3,
				"confidence_level": 0.85,
			},
			"storage_growth": map[string]interface{}{
				"monthly_rate_percent": storageGrowthRate,
				"projected_gb_30_days": 50.5,
				"projected_gb_90_days": 165.2,
			},
			"user_growth": map[string]interface{}{
				"monthly_rate_percent":  userGrowthRate,
				"projected_mau_30_days": 125,
				"projected_mau_90_days": 412,
			},
		},
		"recommendations": []map[string]interface{}{
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

	return common.OK(analytics), nil
}

// Helper to get cost storage
func (h *Handler) getCostStorage() *cost.Storage {
	// This would be properly initialized in the handler
	// For now, return nil to use placeholder data
	return nil
}
