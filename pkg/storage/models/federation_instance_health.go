package models

import (
	"fmt"
	"time"
)

// FederationInstanceHealthTracking tracks health metrics for federated instances
type FederationInstanceHealthTracking struct {
	// Primary keys - INSTANCE#{domain}, HEALTH
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`
	
	// GSI1 for time-based queries - HEALTH_CHECK#{date}, TS#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`
	
	// GSI2 for unhealthy instances - UNHEALTHY (if unhealthy), SCORE#{health_score}#{domain}
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2_sk"`
	
	// Instance identification
	Domain string `json:"domain"` // Remote instance domain
	
	// Health metrics
	HealthScore      float64 `json:"health_score"`       // 0.0 to 1.0 (1.0 = perfect health)
	ResponseTimeP95  int64   `json:"response_time_p95"`  // 95th percentile response time in ms
	SuccessRate      float64 `json:"success_rate"`       // 0.0 to 1.0 (1.0 = 100% success)
	ConsecutiveFails int     `json:"consecutive_fails"`  // Number of consecutive failed requests
	IsHealthy        bool    `json:"is_healthy"`         // Overall health status
	
	// Performance metrics
	AverageResponseTime int64 `json:"average_response_time"` // Average response time in ms
	TotalRequests       int64 `json:"total_requests"`        // Total requests in measurement period
	FailedRequests      int64 `json:"failed_requests"`       // Failed requests in measurement period
	
	// Capacity metrics
	RateLimitRemaining int    `json:"rate_limit_remaining"` // Remaining rate limit capacity
	RateLimitReset     int64  `json:"rate_limit_reset"`      // Unix timestamp when rate limit resets
	BackoffUntil       *int64 `json:"backoff_until"`         // Unix timestamp to back off until (if applicable)
	
	// Timestamps
	LastHealthCheck time.Time `json:"last_health_check"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	
	// TTL for automatic cleanup (7 days for health records)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the primary and GSI keys for the health tracking model
func (f *FederationInstanceHealthTracking) UpdateKeys() {
	f.PK = fmt.Sprintf("INSTANCE#%s", f.Domain)
	f.SK = "HEALTH"
	
	// GSI1 for time-based queries
	dateStr := f.LastHealthCheck.Format("20060102")
	timestampStr := f.LastHealthCheck.Format("20060102150405")
	f.GSI1PK = fmt.Sprintf("HEALTH_CHECK#%s", dateStr)
	f.GSI1SK = fmt.Sprintf("TS#%s#%s", timestampStr, f.Domain)
	
	// GSI2 for unhealthy instances (only set if unhealthy)
	if !f.IsHealthy {
		f.GSI2PK = "UNHEALTHY"
		// Use inverted health score so lowest scores sort first
		invertedScore := 1.0 - f.HealthScore
		f.GSI2SK = fmt.Sprintf("SCORE#%06.4f#%s", invertedScore, f.Domain)
	} else {
		f.GSI2PK = ""
		f.GSI2SK = ""
	}
}

// BeforeCreate is called before creating the record
func (f *FederationInstanceHealthTracking) BeforeCreate() error {
	now := time.Now()
	if f.LastHealthCheck.IsZero() {
		f.LastHealthCheck = now
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	
	// Set TTL to 7 days from creation
	f.TTL = now.AddDate(0, 0, 7).Unix()
	
	f.UpdateKeys()
	f.CalculateHealthScore()
	return nil
}

// BeforeUpdate is called before updating the record
func (f *FederationInstanceHealthTracking) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	f.UpdateKeys()
	f.CalculateHealthScore()
	return nil
}

// CalculateHealthScore calculates the overall health score based on metrics
func (f *FederationInstanceHealthTracking) CalculateHealthScore() {
	// Weight different factors
	// - Success rate: 40%
	// - Response time: 30%
	// - Consecutive fails: 30%
	
	successScore := f.SuccessRate * 0.4
	
	// Response time score (lower is better)
	// Excellent: < 500ms = 1.0
	// Good: < 1000ms = 0.8
	// Fair: < 2000ms = 0.6
	// Poor: < 5000ms = 0.4
	// Bad: >= 5000ms = 0.2
	responseScore := 0.2
	if f.ResponseTimeP95 < 500 {
		responseScore = 1.0
	} else if f.ResponseTimeP95 < 1000 {
		responseScore = 0.8
	} else if f.ResponseTimeP95 < 2000 {
		responseScore = 0.6
	} else if f.ResponseTimeP95 < 5000 {
		responseScore = 0.4
	}
	responseScore *= 0.3
	
	// Consecutive fails score
	// 0 fails = 1.0
	// 1 fail = 0.8
	// 2 fails = 0.6
	// 3 fails = 0.4
	// 4 fails = 0.2
	// 5+ fails = 0.0
	failScore := 1.0
	if f.ConsecutiveFails > 0 {
		failScore = 1.0 - (float64(f.ConsecutiveFails) * 0.2)
		if failScore < 0 {
			failScore = 0
		}
	}
	failScore *= 0.3
	
	f.HealthScore = successScore + responseScore + failScore
	
	// Determine overall health status
	// Healthy: score >= 0.7
	// Unhealthy: score < 0.7 OR consecutive fails >= 3
	f.IsHealthy = f.HealthScore >= 0.7 && f.ConsecutiveFails < 3
}

// TableName returns the DynamoDB table name
func (f *FederationInstanceHealthTracking) TableName() string {
	return "lesser-main"
}