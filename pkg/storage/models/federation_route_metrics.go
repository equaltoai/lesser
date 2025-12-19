package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

const (
	// CircuitBreakerStateOpen represents an open circuit breaker state
	CircuitBreakerStateOpen = "open"
	// CircuitBreakerStateClosed represents a closed circuit breaker state
	CircuitBreakerStateClosed = "closed"
	// CircuitBreakerStateHalfOpen represents a half-open circuit breaker state
	CircuitBreakerStateHalfOpen = "half-open"
)

// FederationRouteMetrics represents detailed per-route federation metrics with GSI optimization
type FederationRouteMetrics struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - route metrics use FED_ROUTE#{route_id}#{date} pattern
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI1 for time-based route queries - FED_ROUTES#{date}, ROUTE#{route_id}#{timestamp}
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// GSI2 for domain-route queries - FED_DOMAIN_ROUTES#{domain}, ROUTE#{route_id}#{timestamp}
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"`

	// GSI3 for performance queries - FED_ROUTE_PERF#{performance_tier}, LATENCY#{avg_latency}#{route_id}
	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsi3PK" json:"gsi3_pk"`
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsi3SK" json:"gsi3_sk"`

	// Route identification
	RouteID           string `dynamorm:"attr:routeID" json:"route_id"`                     // Unique route identifier
	DestinationDomain string `dynamorm:"attr:destinationDomain" json:"destination_domain"` // Target domain
	RouteType         string `dynamorm:"attr:routeType" json:"route_type"`                 // primary, backup, failover
	RoutePriority     int    `dynamorm:"attr:routePriority" json:"route_priority"`         // Priority order (1=highest)
	ServerEndpoint    string `dynamorm:"attr:serverEndpoint" json:"server_endpoint"`       // Full endpoint URL
	Protocol          string `dynamorm:"attr:protocol" json:"protocol"`                    // HTTP, HTTPS
	Port              int    `dynamorm:"attr:port" json:"port"`                            // Target port

	// Performance metrics (aggregated for the time period)
	TotalAttempts      int64   `dynamorm:"attr:totalAttempts" json:"total_attempts"`           // Total delivery attempts
	SuccessfulAttempts int64   `dynamorm:"attr:successfulAttempts" json:"successful_attempts"` // Successful deliveries
	FailedAttempts     int64   `dynamorm:"attr:failedAttempts" json:"failed_attempts"`         // Failed deliveries
	SuccessRate        float64 `dynamorm:"attr:successRate" json:"success_rate"`               // Success percentage
	AvgLatencyMs       int64   `dynamorm:"attr:avgLatencyMs" json:"avg_latency_ms"`            // Average response time
	MedianLatencyMs    int64   `dynamorm:"attr:medianLatencyMs" json:"median_latency_ms"`      // Median response time
	P95LatencyMs       int64   `dynamorm:"attr:p95LatencyMs" json:"p95_latency_ms"`            // 95th percentile latency
	P99LatencyMs       int64   `dynamorm:"attr:p99LatencyMs" json:"p99_latency_ms"`            // 99th percentile latency
	MinLatencyMs       int64   `dynamorm:"attr:minLatencyMs" json:"min_latency_ms"`            // Fastest response
	MaxLatencyMs       int64   `dynamorm:"attr:maxLatencyMs" json:"max_latency_ms"`            // Slowest response
	TimeoutCount       int64   `dynamorm:"attr:timeoutCount" json:"timeout_count"`             // Number of timeouts
	TimeoutRate        float64 `dynamorm:"attr:timeoutRate" json:"timeout_rate"`               // Timeout percentage

	// Error tracking
	ErrorBreakdown      map[string]int64 `dynamorm:"attr:errorBreakdown" json:"error_breakdown"`            // Error code -> count
	ConsecutiveFailures int64            `dynamorm:"attr:consecutiveFailures" json:"consecutive_failures"`  // Current failure streak
	MaxConsecutiveFails int64            `dynamorm:"attr:maxConsecutiveFails" json:"max_consecutive_fails"` // Longest failure streak
	LastErrorCode       string           `dynamorm:"attr:lastErrorCode" json:"last_error_code"`             // Most recent error
	LastErrorMessage    string           `dynamorm:"attr:lastErrorMessage" json:"last_error_message"`       // Most recent error message
	LastErrorTime       *time.Time       `dynamorm:"attr:lastErrorTime" json:"last_error_time"`             // When last error occurred
	RecoveryTime        *time.Time       `dynamorm:"attr:recoveryTime" json:"recovery_time"`                // When route recovered

	// Cost metrics (aggregated)
	TotalCostMicroCents int64   `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"` // Total cost for this route
	AvgCostPerDelivery  int64   `dynamorm:"attr:avgCostPerDelivery" json:"avg_cost_per_delivery"`   // Average cost per delivery
	CostEfficiencyScore float64 `dynamorm:"attr:costEfficiencyScore" json:"cost_efficiency_score"`  // Cost vs. performance ratio
	DataTransferBytes   int64   `dynamorm:"attr:dataTransferBytes" json:"data_transfer_bytes"`      // Total bytes transferred
	AvgPayloadSize      int64   `dynamorm:"attr:avgPayloadSize" json:"avg_payload_size"`            // Average payload size

	// Retry analysis
	TotalRetries       int64   `dynamorm:"attr:totalRetries" json:"total_retries"`              // Total retry attempts
	AvgRetriesPerFail  float64 `dynamorm:"attr:avgRetriesPerFail" json:"avg_retries_per_fail"`  // Average retries before giving up
	RetrySuccessRate   float64 `dynamorm:"attr:retrySuccessRate" json:"retry_success_rate"`     // Success rate of retries
	AvgRetryDelayMs    int64   `dynamorm:"attr:avgRetryDelayMs" json:"avg_retry_delay_ms"`      // Average delay between retries
	RetryBackoffFactor float64 `dynamorm:"attr:retryBackoffFactor" json:"retry_backoff_factor"` // Exponential backoff factor

	// Circuit breaker state
	CircuitBreakerState string     `dynamorm:"attr:circuitBreakerState" json:"circuit_breaker_state"` // closed, open, half_open
	StateChangeTime     *time.Time `dynamorm:"attr:stateChangeTime" json:"state_change_time"`         // When state last changed
	NextRetryTime       *time.Time `dynamorm:"attr:nextRetryTime" json:"next_retry_time"`             // When to try again if open

	// Health scoring
	HealthScore      float64   `dynamorm:"attr:healthScore" json:"health_score"`           // Overall route health (0.0-1.0)
	ReliabilityScore float64   `dynamorm:"attr:reliabilityScore" json:"reliability_score"` // Reliability metric
	PerformanceScore float64   `dynamorm:"attr:performanceScore" json:"performance_score"` // Speed metric
	StabilityScore   float64   `dynamorm:"attr:stabilityScore" json:"stability_score"`     // Consistency metric
	HealthHistory    []float64 `dynamorm:"attr:healthHistory" json:"health_history"`       // Historical health scores
	LastHealthCheck  time.Time `dynamorm:"attr:lastHealthCheck" json:"last_health_check"`  // When health was last calculated
	HealthTrend      string    `dynamorm:"attr:healthTrend" json:"health_trend"`           // improving, stable, degrading

	// Load balancing metrics
	CurrentWeight      float64    `dynamorm:"attr:currentWeight" json:"current_weight"`            // Current load balancing weight
	OptimalWeight      float64    `dynamorm:"attr:optimalWeight" json:"optimal_weight"`            // Calculated optimal weight
	WeightAdjustments  int        `dynamorm:"attr:weightAdjustments" json:"weight_adjustments"`    // Number of weight changes
	LastWeightChange   *time.Time `dynamorm:"attr:lastWeightChange" json:"last_weight_change"`     // When weight was last adjusted
	LoadBalancingScore float64    `dynamorm:"attr:loadBalancingScore" json:"load_balancing_score"` // Effectiveness score

	// Geographic and network info
	DataCenter     string `dynamorm:"attr:dataCenter" json:"data_center"`         // Which DC the route goes through
	NetworkPath    string `dynamorm:"attr:networkPath" json:"network_path"`       // Network routing information
	IPAddress      string `dynamorm:"attr:ipAddress" json:"ip_address"`           // Resolved IP address
	ASN            string `dynamorm:"attr:asn" json:"asn"`                        // Autonomous System Number
	GeographicZone string `dynamorm:"attr:geographicZone" json:"geographic_zone"` // Geographic region
	CDNProvider    string `dynamorm:"attr:cdnProvider" json:"cdn_provider"`       // CDN if applicable

	// Aggregation period
	PeriodType  string    `dynamorm:"attr:periodType" json:"period_type"`   // hour, day, week, month
	PeriodStart time.Time `dynamorm:"attr:periodStart" json:"period_start"` // Start of aggregation period
	PeriodEnd   time.Time `dynamorm:"attr:periodEnd" json:"period_end"`     // End of aggregation period

	// Timestamps
	CreatedAt   time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
	FirstUsed   time.Time `dynamorm:"attr:firstUsed" json:"first_used"`     // When route was first used
	LastUsed    time.Time `dynamorm:"attr:lastUsed" json:"last_used"`       // When route was last used
	LastSuccess time.Time `dynamorm:"attr:lastSuccess" json:"last_success"` // When route last succeeded

	// TTL for automatic cleanup (90 days for route metrics)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys sets the primary keys for the FederationRouteMetrics model
func (f *FederationRouteMetrics) UpdateKeys() {
	dateStr := f.PeriodStart.Format(common.CompactDateFormat)
	timestampStr := f.PeriodStart.Format(common.CompactTimeFormat)

	f.PK = fmt.Sprintf("FED_ROUTE#%s#%s", f.RouteID, dateStr)
	f.SK = fmt.Sprintf("METRICS#%s", f.PeriodType)

	// GSI1 for time-based route queries
	f.GSI1PK = fmt.Sprintf("FED_ROUTES#%s", dateStr)
	f.GSI1SK = fmt.Sprintf("ROUTE#%s#%s", f.RouteID, timestampStr)

	// GSI2 for domain-route queries
	f.GSI2PK = fmt.Sprintf("FED_DOMAIN_ROUTES#%s", f.DestinationDomain)
	f.GSI2SK = fmt.Sprintf("ROUTE#%s#%s", f.RouteID, timestampStr)

	// GSI3 for performance queries - tier based on latency
	perfTier := f.determinePerformanceTier()
	f.GSI3PK = fmt.Sprintf("FED_ROUTE_PERF#%s", perfTier)
	f.GSI3SK = fmt.Sprintf("LATENCY#%06d#%s", f.AvgLatencyMs, f.RouteID)
}

// determinePerformanceTier categorizes route performance
func (f *FederationRouteMetrics) determinePerformanceTier() string {
	if f.AvgLatencyMs <= 100 {
		return "excellent"
	} else if f.AvgLatencyMs <= 300 {
		return "good"
	} else if f.AvgLatencyMs <= 1000 {
		return "fair"
	}
	return "poor"
}

// BeforeCreate is called before creating the record
func (f *FederationRouteMetrics) BeforeCreate() error {
	now := time.Now()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	if f.FirstUsed.IsZero() {
		f.FirstUsed = now
	}
	f.UpdatedAt = now

	// Initialize maps and slices
	if f.ErrorBreakdown == nil {
		f.ErrorBreakdown = make(map[string]int64)
	}
	if f.HealthHistory == nil {
		f.HealthHistory = make([]float64, 0)
	}

	// Calculate derived metrics
	f.calculateDerivedMetrics()

	// Set TTL to 90 days from creation
	f.TTL = now.AddDate(0, 0, 90).Unix()

	f.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (f *FederationRouteMetrics) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	f.calculateDerivedMetrics()
	f.UpdateKeys()
	return nil
}

// calculateDerivedMetrics calculates computed fields
func (f *FederationRouteMetrics) calculateDerivedMetrics() {
	// Calculate success rate
	if f.TotalAttempts > 0 {
		f.SuccessRate = float64(f.SuccessfulAttempts) / float64(f.TotalAttempts) * 100.0
	}

	// Calculate timeout rate
	if f.TotalAttempts > 0 {
		f.TimeoutRate = float64(f.TimeoutCount) / float64(f.TotalAttempts) * 100.0
	}

	// Calculate average cost per delivery
	if f.SuccessfulAttempts > 0 {
		f.AvgCostPerDelivery = f.TotalCostMicroCents / f.SuccessfulAttempts
	}

	// Calculate average retries per failure
	if f.FailedAttempts > 0 {
		f.AvgRetriesPerFail = float64(f.TotalRetries) / float64(f.FailedAttempts)
	}

	// Calculate retry success rate
	retryAttempts := f.TotalAttempts - (f.TotalAttempts - f.TotalRetries)
	if retryAttempts > 0 {
		f.RetrySuccessRate = float64(f.SuccessfulAttempts) / float64(retryAttempts) * 100.0
	}

	// Calculate cost efficiency score
	if f.AvgLatencyMs > 0 && f.AvgCostPerDelivery > 0 {
		// Lower cost and lower latency = higher efficiency
		costFactor := 1.0 / (float64(f.AvgCostPerDelivery)/1000000.0 + 1.0) // Normalize cost
		speedFactor := 1.0 / (float64(f.AvgLatencyMs)/100.0 + 1.0)          // Normalize latency
		f.CostEfficiencyScore = (costFactor + speedFactor) / 2.0 * f.SuccessRate / 100.0
	}

	// Calculate health scores
	f.calculateHealthScores()
}

// calculateHealthScores computes various health metrics
func (f *FederationRouteMetrics) calculateHealthScores() {
	// Reliability score based on success rate and consistency
	f.ReliabilityScore = f.SuccessRate / 100.0
	if f.MaxConsecutiveFails > 5 {
		f.ReliabilityScore *= 0.8 // Penalize for long failure streaks
	}

	// Performance score based on latency percentiles
	if f.P95LatencyMs > 0 {
		// Good performance = low latency
		f.PerformanceScore = 1.0 / (1.0 + float64(f.P95LatencyMs)/1000.0)
	}

	// Stability score based on latency variance and error consistency
	if f.MaxLatencyMs > f.MinLatencyMs && f.MinLatencyMs > 0 {
		latencyRatio := float64(f.MaxLatencyMs) / float64(f.MinLatencyMs)
		f.StabilityScore = 1.0 / (1.0 + latencyRatio/10.0) // Penalize high variance
	} else {
		f.StabilityScore = 1.0
	}

	// Overall health score (weighted average)
	weights := map[string]float64{
		"reliability": 0.5,
		"performance": 0.3,
		"stability":   0.2,
	}
	f.HealthScore = f.ReliabilityScore*weights["reliability"] +
		f.PerformanceScore*weights["performance"] +
		f.StabilityScore*weights["stability"]

	// Update health history
	f.HealthHistory = append(f.HealthHistory, f.HealthScore)
	if len(f.HealthHistory) > 24 { // Keep last 24 measurements
		f.HealthHistory = f.HealthHistory[1:]
	}

	// Determine health trend
	f.determineHealthTrend()

	// Calculate optimal weight for load balancing
	f.OptimalWeight = f.HealthScore * f.CostEfficiencyScore
}

// determineHealthTrend analyzes health score trends
func (f *FederationRouteMetrics) determineHealthTrend() {
	if len(f.HealthHistory) < 3 {
		f.HealthTrend = "stable"
		return
	}

	// Calculate trend over last few measurements
	recentCount := 5
	if len(f.HealthHistory) < recentCount {
		recentCount = len(f.HealthHistory)
	}

	recent := f.HealthHistory[len(f.HealthHistory)-recentCount:]

	// Simple linear regression on recent health scores
	n := float64(len(recent))
	var sumX, sumY, sumXY, sumX2 float64

	for i, score := range recent {
		x := float64(i)
		sumX += x
		sumY += score
		sumXY += x * score
		sumX2 += x * x
	}

	// Calculate slope
	denominator := n*sumX2 - sumX*sumX
	if denominator != 0 {
		slope := (n*sumXY - sumX*sumY) / denominator

		if slope > 0.02 {
			f.HealthTrend = "improving"
		} else if slope < -0.02 {
			f.HealthTrend = "degrading"
		} else {
			f.HealthTrend = "stable"
		}
	}
}

// AddDeliveryAttempt records a new delivery attempt
func (f *FederationRouteMetrics) AddDeliveryAttempt(success bool, latencyMs int64, errorCode string, errorMsg string, costMicroCents int64, payloadBytes int64) {
	f.TotalAttempts++
	f.LastUsed = time.Now()

	if success {
		f.SuccessfulAttempts++
		f.LastSuccess = time.Now()
		f.ConsecutiveFailures = 0

		// Update recovery time if we were in a failure state
		if f.CircuitBreakerState == CircuitBreakerStateOpen {
			now := time.Now()
			f.RecoveryTime = &now
			f.CircuitBreakerState = CircuitBreakerStateClosed
			f.StateChangeTime = &now
		}
	} else {
		f.FailedAttempts++
		f.ConsecutiveFailures++

		if f.ConsecutiveFailures > f.MaxConsecutiveFails {
			f.MaxConsecutiveFails = f.ConsecutiveFailures
		}

		// Update error tracking
		if errorCode != "" {
			if f.ErrorBreakdown == nil {
				f.ErrorBreakdown = make(map[string]int64)
			}
			f.ErrorBreakdown[errorCode]++
			f.LastErrorCode = errorCode
			f.LastErrorMessage = errorMsg
			now := time.Now()
			f.LastErrorTime = &now
		}

		// Update circuit breaker state
		f.updateCircuitBreakerState()
	}

	// Update latency statistics
	f.updateLatencyStats(latencyMs)

	// Update cost tracking
	f.TotalCostMicroCents += costMicroCents
	f.DataTransferBytes += payloadBytes
	if f.TotalAttempts > 0 {
		f.AvgPayloadSize = f.DataTransferBytes / f.TotalAttempts
	}

	// Recalculate derived metrics
	f.calculateDerivedMetrics()
}

// updateLatencyStats updates latency statistics with new measurement
func (f *FederationRouteMetrics) updateLatencyStats(latencyMs int64) {
	// Update min/max
	if f.MinLatencyMs == 0 || latencyMs < f.MinLatencyMs {
		f.MinLatencyMs = latencyMs
	}
	if latencyMs > f.MaxLatencyMs {
		f.MaxLatencyMs = latencyMs
	}

	// Update average (running average)
	if f.AvgLatencyMs == 0 {
		f.AvgLatencyMs = latencyMs
	} else {
		// Exponential moving average with alpha = 0.1
		alpha := 0.1
		f.AvgLatencyMs = int64(alpha*float64(latencyMs) + (1-alpha)*float64(f.AvgLatencyMs))
	}

	// For percentiles, we'd need to maintain a histogram or sample
	// For simplicity, estimate based on current data
	f.MedianLatencyMs = (f.MinLatencyMs + f.MaxLatencyMs) / 2
	f.P95LatencyMs = f.MinLatencyMs + int64(0.95*float64(f.MaxLatencyMs-f.MinLatencyMs))
	f.P99LatencyMs = f.MinLatencyMs + int64(0.99*float64(f.MaxLatencyMs-f.MinLatencyMs))
}

// updateCircuitBreakerState manages circuit breaker logic
func (f *FederationRouteMetrics) updateCircuitBreakerState() {
	now := time.Now()

	switch f.CircuitBreakerState {
	case CircuitBreakerStateClosed, "":
		// Move to open if too many consecutive failures
		if f.ConsecutiveFailures >= 5 {
			f.CircuitBreakerState = CircuitBreakerStateOpen
			f.StateChangeTime = &now
			// Next retry in 1 minute
			nextRetry := now.Add(1 * time.Minute)
			f.NextRetryTime = &nextRetry
		}

	case "open":
		// Check if it's time to try again
		if f.NextRetryTime != nil && now.After(*f.NextRetryTime) {
			f.CircuitBreakerState = "half_open"
			f.StateChangeTime = &now
		}

	case "half_open":
		// In half-open state, a single success closes the circuit
		// A failure opens it again (handled in AddDeliveryAttempt)
		break
	}
}

// AddRetryAttempt records a retry attempt
func (f *FederationRouteMetrics) AddRetryAttempt(delayMs int64) {
	f.TotalRetries++

	// Update average retry delay
	if f.AvgRetryDelayMs == 0 {
		f.AvgRetryDelayMs = delayMs
	} else {
		// Running average
		f.AvgRetryDelayMs = (f.AvgRetryDelayMs + delayMs) / 2
	}
}

// IsHealthy returns whether the route is considered healthy
func (f *FederationRouteMetrics) IsHealthy() bool {
	if f.CircuitBreakerState == CircuitBreakerStateOpen {
		return false
	}

	return f.HealthScore >= 0.7 && f.SuccessRate >= 90.0
}

// GetRouteSummary returns a summary of route performance
func (f *FederationRouteMetrics) GetRouteSummary() map[string]interface{} {
	return map[string]interface{}{
		"route_id":           f.RouteID,
		"destination_domain": f.DestinationDomain,
		"route_type":         f.RouteType,
		"health_score":       f.HealthScore,
		"success_rate":       f.SuccessRate,
		"avg_latency_ms":     f.AvgLatencyMs,
		"total_attempts":     f.TotalAttempts,
		"circuit_breaker":    f.CircuitBreakerState,
		"cost_efficiency":    f.CostEfficiencyScore,
		"health_trend":       f.HealthTrend,
		"is_healthy":         f.IsHealthy(),
	}
}

// GetOptimizationRecommendations provides route optimization suggestions
func (f *FederationRouteMetrics) GetOptimizationRecommendations() []RouteRecommendation {
	var recommendations []RouteRecommendation

	// High latency recommendation
	if f.AvgLatencyMs > 1000 {
		recommendations = append(recommendations, RouteRecommendation{
			Type:        "performance",
			Priority:    "high",
			Title:       "High Latency Detected",
			Description: fmt.Sprintf("Average latency of %dms exceeds threshold. Consider route optimization or alternative endpoints.", f.AvgLatencyMs),
			Action:      "optimize_route",
			Impact:      "high",
		})
	}

	// Low success rate recommendation
	if f.SuccessRate < 90.0 {
		recommendations = append(recommendations, RouteRecommendation{
			Type:        "reliability",
			Priority:    "high",
			Title:       "Low Success Rate",
			Description: fmt.Sprintf("Success rate of %.1f%% is below acceptable threshold. Investigate error patterns.", f.SuccessRate),
			Action:      "investigate_errors",
			Impact:      "high",
		})
	}

	// Circuit breaker recommendation
	if f.CircuitBreakerState == CircuitBreakerStateOpen {
		recommendations = append(recommendations, RouteRecommendation{
			Type:        "availability",
			Priority:    "critical",
			Title:       "Route Circuit Breaker Open",
			Description: "Route is currently blocked due to consecutive failures. Manual intervention may be required.",
			Action:      "check_endpoint_health",
			Impact:      "critical",
		})
	}

	// Cost efficiency recommendation
	if f.CostEfficiencyScore < 0.5 {
		recommendations = append(recommendations, RouteRecommendation{
			Type:        "cost",
			Priority:    "medium",
			Title:       "Poor Cost Efficiency",
			Description: "Route has low cost efficiency. Consider load balancing adjustments or endpoint optimization.",
			Action:      "adjust_load_balancing",
			Impact:      "medium",
		})
	}

	// Health trend recommendation
	if f.HealthTrend == "degrading" {
		recommendations = append(recommendations, RouteRecommendation{
			Type:        "maintenance",
			Priority:    "medium",
			Title:       "Degrading Performance Trend",
			Description: "Route performance is trending downward. Proactive maintenance recommended.",
			Action:      "schedule_maintenance",
			Impact:      "medium",
		})
	}

	return recommendations
}

// RouteRecommendation represents a route optimization recommendation
type RouteRecommendation struct {
	Type        string `json:"type"`        // performance, reliability, cost, etc.
	Priority    string `json:"priority"`    // critical, high, medium, low
	Title       string `json:"title"`       // Brief title
	Description string `json:"description"` // Detailed description
	Action      string `json:"action"`      // Recommended action
	Impact      string `json:"impact"`      // Expected impact level
}

// TableName returns the DynamoDB table backing RouteRecommendation.
func (RouteRecommendation) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing FederationRouteMetrics.
func (FederationRouteMetrics) TableName() string {
	return MainTableName
}

// FederationRouteAggregation represents aggregated route metrics across multiple time periods
type FederationRouteAggregation struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - route aggregation uses FED_ROUTE_AGG#{period}#{route_id} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for route comparison queries - FED_ROUTE_COMPARE#{period}, SCORE#{health_score}#{route_id}
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk"`

	// Aggregation metadata
	RouteID           string    `json:"route_id"`
	DestinationDomain string    `json:"destination_domain"`
	Period            string    `json:"period"` // hour, day, week, month
	PeriodStart       time.Time `json:"period_start"`
	PeriodEnd         time.Time `json:"period_end"`

	// Aggregated performance metrics
	AvgHealthScore     float64 `json:"avg_health_score"`
	MinHealthScore     float64 `json:"min_health_score"`
	MaxHealthScore     float64 `json:"max_health_score"`
	HealthScoreStdDev  float64 `json:"health_score_std_dev"`
	TotalAttempts      int64   `json:"total_attempts"`
	TotalSuccesses     int64   `json:"total_successes"`
	OverallSuccessRate float64 `json:"overall_success_rate"`
	AvgLatencyMs       int64   `json:"avg_latency_ms"`
	MedianLatencyMs    int64   `json:"median_latency_ms"`
	P95LatencyMs       int64   `json:"p95_latency_ms"`
	LatencyStdDev      float64 `json:"latency_std_dev"`

	// Cost aggregations
	TotalCostMicroCents int64   `json:"total_cost_micro_cents"`
	AvgCostPerDelivery  int64   `json:"avg_cost_per_delivery"`
	CostEfficiencyScore float64 `json:"cost_efficiency_score"`

	// Reliability metrics
	UptimePercentage       float64 `json:"uptime_percentage"`
	DowntimeMinutes        int64   `json:"downtime_minutes"`
	MTBF                   float64 `json:"mtbf"` // Mean Time Between Failures (hours)
	MTTR                   float64 `json:"mttr"` // Mean Time To Recovery (minutes)
	MaxConsecutiveFailures int64   `json:"max_consecutive_failures"`
	CircuitBreakerTrips    int64   `json:"circuit_breaker_trips"`

	// Ranking and comparison
	PerformanceRank    int     `json:"performance_rank"`     // Rank among all routes for this domain
	ReliabilityRank    int     `json:"reliability_rank"`     // Rank based on reliability
	CostEfficiencyRank int     `json:"cost_efficiency_rank"` // Rank based on cost efficiency
	OverallRank        int     `json:"overall_rank"`         // Overall ranking
	PercentileScore    float64 `json:"percentile_score"`     // Percentile among all routes

	// Trend analysis
	PerformanceTrend string  `json:"performance_trend"` // improving, stable, degrading
	ReliabilityTrend string  `json:"reliability_trend"` // improving, stable, degrading
	CostTrend        string  `json:"cost_trend"`        // improving, stable, degrading
	TrendConfidence  float64 `json:"trend_confidence"`  // Statistical confidence in trends

	// Top error codes and frequencies
	TopErrors []ErrorFrequency `json:"top_errors"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup (1 year for aggregated data)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing FederationRouteAggregation.
func (FederationRouteAggregation) TableName() string {
	return MainTableName
}

// ErrorFrequency represents error frequency data
type ErrorFrequency struct {
	ErrorCode  string  `json:"error_code"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// TableName returns the DynamoDB table backing ErrorFrequency.
func (ErrorFrequency) TableName() string {
	return MainTableName
}

// UpdateKeys sets the primary keys for the FederationRouteAggregation model
func (f *FederationRouteAggregation) UpdateKeys() {
	f.PK = fmt.Sprintf("FED_ROUTE_AGG#%s#%s", f.Period, f.RouteID)
	f.SK = f.PeriodStart.Format(common.CompactDateFormat)

	// GSI1 for route comparison
	f.GSI1PK = fmt.Sprintf("FED_ROUTE_COMPARE#%s", f.Period)
	f.GSI1SK = fmt.Sprintf("SCORE#%06.2f#%s", f.AvgHealthScore*100, f.RouteID)
}

// BeforeCreate is called before creating the aggregated record
func (f *FederationRouteAggregation) BeforeCreate() error {
	now := time.Now()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now

	// Calculate percentile score (simplified)
	if f.OverallRank > 0 {
		// Assume 100 total routes for percentile calculation
		totalRoutes := 100.0
		f.PercentileScore = (totalRoutes - float64(f.OverallRank)) / totalRoutes * 100.0
	}

	// Set TTL to 1 year from creation
	f.TTL = now.AddDate(1, 0, 0).Unix()

	f.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the aggregated record
func (f *FederationRouteAggregation) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	f.UpdateKeys()
	return nil
}

// GetRouteComparisonMetrics returns metrics for comparing routes
func (f *FederationRouteAggregation) GetRouteComparisonMetrics() map[string]interface{} {
	return map[string]interface{}{
		"route_id":             f.RouteID,
		"destination_domain":   f.DestinationDomain,
		"avg_health_score":     f.AvgHealthScore,
		"overall_success_rate": f.OverallSuccessRate,
		"avg_latency_ms":       f.AvgLatencyMs,
		"cost_efficiency":      f.CostEfficiencyScore,
		"uptime_percentage":    f.UptimePercentage,
		"overall_rank":         f.OverallRank,
		"percentile_score":     f.PercentileScore,
		"performance_trend":    f.PerformanceTrend,
		"mtbf_hours":           f.MTBF,
		"mttr_minutes":         f.MTTR,
	}
}
