package federation

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// AnalyticsAggregator implements the 5-minute primary aggregation pattern
// following the federation-analytics-guidance.md specifications
type AnalyticsAggregator struct {
	federationRepo analyticsFederationRepository
	logger         *zap.Logger
}

type analyticsFederationRepository interface {
	StoreDetailedFederationMetrics(ctx context.Context, metrics *models.FederationAnalyticsTimeSeries) error
	AggregateFederationMetrics(ctx context.Context, domain, fromPeriod, toPeriod string, timestamp time.Time) error
	GetDomainHealthScore(ctx context.Context, domain string) (float64, error)
	GetDetailedMetricsByPeriod(ctx context.Context, period string, startTime, endTime time.Time, limit int) ([]*models.FederationAnalyticsTimeSeries, error)
	GetUnhealthyDomains(ctx context.Context, healthThreshold float64) ([]*models.FederationAnalyticsTimeSeries, error)
}

// NewAnalyticsAggregator creates a new analytics aggregator
func NewAnalyticsAggregator(federationRepo *repositories.FederationRepository, logger *zap.Logger) *AnalyticsAggregator {
	return &AnalyticsAggregator{
		federationRepo: federationRepo,
		logger:         logger,
	}
}

// RecordMetric records a raw federation metric that will be aggregated
func (a *AnalyticsAggregator) RecordMetric(ctx context.Context, domain string, metric *Metric) error {
	// Create raw time series record
	now := time.Now()
	rawMetric := models.NewFederationAnalyticsTimeSeries(domain, "raw", now)

	// Populate from the metric
	rawMetric.ActivityCount = 1
	rawMetric.TotalInboundVolume = metric.InboundBytes
	rawMetric.TotalOutboundVolume = metric.OutboundBytes
	rawMetric.InboxDeliveryP95 = metric.ResponseTimeMs
	rawMetric.SignatureVerificationTime = metric.SignatureTimeMs
	if metric.Success {
		rawMetric.InstanceReachability = 1.0
		rawMetric.EndpointAvailability = 1.0
		rawMetric.ErrorRate = 0.0
	} else {
		rawMetric.InstanceReachability = 0.0
		rawMetric.EndpointAvailability = 0.0
		rawMetric.ErrorRate = 1.0
	}

	if metric.Success {
		rawMetric.SuccessfulActivities = 1
		rawMetric.FailedActivities = 0
	} else {
		rawMetric.SuccessfulActivities = 0
		rawMetric.FailedActivities = 1
		rawMetric.ConsecutiveFailures = 1
	}

	if metric.ErrorType != "" {
		switch metric.ErrorType {
		case "signature_failure":
			rawMetric.SignatureFailures = 1
		case "timeout":
			rawMetric.TimeoutRate = 1.0
		case "rate_limit":
			rawMetric.RateLimitHits = 1
		case "malformed":
			rawMetric.MalformedActivities = 1
		case "validation":
			rawMetric.ValidationFailures = 1
		}
	}

	// Set last successful contact if this was a success
	if metric.Success {
		rawMetric.LastSuccessfulContact = &now
	}

	// Calculate health score
	rawMetric.CalculateHealthScore()

	// Store the raw metric using detailed storage
	err := a.federationRepo.StoreDetailedFederationMetrics(ctx, rawMetric)
	if err != nil {
		a.logger.Error("failed to store federation metric", zap.Error(err), zap.String("domain", domain))
		return errors.Join(ErrFederationMetricStoreFailed, err)
	}

	// Trigger 5-minute aggregation if we're at a 5-minute boundary
	if now.Minute()%5 == 0 && now.Second() < 30 {
		go a.triggerAggregation(context.Background(), domain, now)
	}

	return nil
}

// triggerAggregation triggers the aggregation pipeline for a domain
func (a *AnalyticsAggregator) triggerAggregation(ctx context.Context, domain string, timestamp time.Time) {
	// 5-minute aggregation (primary period)
	if err := a.federationRepo.AggregateFederationMetrics(ctx, domain, "raw", "5min", timestamp); err != nil {
		a.logger.Error("Failed to aggregate to 5-minute",
			zap.String("domain", domain),
			zap.Time("timestamp", timestamp),
			zap.Error(err))
	}

	// Hourly aggregation (every hour)
	if timestamp.Minute() == 0 {
		if err := a.federationRepo.AggregateFederationMetrics(ctx, domain, "5min", "hourly", timestamp); err != nil {
			a.logger.Error("Failed to aggregate to hourly",
				zap.String("domain", domain),
				zap.Time("timestamp", timestamp),
				zap.Error(err))
		}
	}

	// Daily aggregation (at midnight)
	if timestamp.Hour() == 0 && timestamp.Minute() == 0 {
		if err := a.federationRepo.AggregateFederationMetrics(ctx, domain, "hourly", "daily", timestamp); err != nil {
			a.logger.Error("Failed to aggregate to daily",
				zap.String("domain", domain),
				zap.Time("timestamp", timestamp),
				zap.Error(err))
		}
	}

	// Monthly aggregation (first day of month)
	if timestamp.Day() == 1 && timestamp.Hour() == 0 && timestamp.Minute() == 0 {
		if err := a.federationRepo.AggregateFederationMetrics(ctx, domain, "daily", "monthly", timestamp); err != nil {
			a.logger.Error("Failed to aggregate to monthly",
				zap.String("domain", domain),
				zap.Time("timestamp", timestamp),
				zap.Error(err))
		}
	}
}

// GetDomainHealthStatus returns the current health status for a domain
func (a *AnalyticsAggregator) GetDomainHealthStatus(ctx context.Context, domain string) (*DomainHealthStatus, error) {
	// Get recent health score
	healthScore, err := a.federationRepo.GetDomainHealthScore(ctx, domain)
	if err != nil {
		a.logger.Error("failed to retrieve health score", zap.Error(err), zap.String("domain", domain))
		return nil, errors.Join(ErrHealthScoreRetrieveFailed, err)
	}

	// Get recent metrics for additional context
	endTime := time.Now()
	startTime := endTime.Add(-30 * time.Minute)

	recentMetrics, err := a.federationRepo.GetDetailedMetricsByPeriod(ctx, "5min", startTime, endTime, 100)
	if err != nil {
		a.logger.Error("failed to retrieve recent metrics", zap.Error(err), zap.String("domain", domain), zap.Duration("period", 30*time.Minute))
		return nil, errors.Join(ErrRecentMetricsRetrieveFailed, err)
	}

	status := &DomainHealthStatus{
		Domain:      domain,
		HealthScore: healthScore,
		CheckedAt:   time.Now(),
	}

	// Determine status
	if healthScore >= 80 {
		status.Status = "HEALTHY"
	} else if healthScore >= 60 {
		status.Status = "DEGRADED"
	} else if healthScore >= 40 {
		status.Status = "UNHEALTHY"
	} else {
		status.Status = "CRITICAL"
	}

	// Calculate recent statistics
	if len(recentMetrics) > 0 {
		var totalActivities, totalErrors int64
		var totalLatency int64
		var availabilitySum float64

		for _, metric := range recentMetrics {
			totalActivities += metric.ActivityCount
			totalErrors += metric.FailedActivities
			totalLatency += metric.InboxDeliveryP95
			availabilitySum += metric.InstanceReachability
		}

		status.RecentActivities = totalActivities
		status.RecentErrors = totalErrors
		if totalActivities > 0 {
			status.RecentErrorRate = float64(totalErrors) / float64(totalActivities)
		}
		if len(recentMetrics) > 0 {
			status.AvgLatencyMs = totalLatency / int64(len(recentMetrics))
			status.AvailabilityRate = availabilitySum / float64(len(recentMetrics))
		}
	}

	// Check for alert conditions
	shouldAlert, message := a.checkAlertConditions(status)
	status.ShouldAlert = shouldAlert
	status.AlertMessage = message

	return status, nil
}

// checkAlertConditions checks if a domain should trigger alerts
func (a *AnalyticsAggregator) checkAlertConditions(status *DomainHealthStatus) (bool, string) {
	// Critical conditions
	if status.AvailabilityRate < 0.5 {
		return true, "CRITICAL: Instance reachability below 50%"
	}

	if status.HealthScore < 40 {
		return true, "CRITICAL: Health score below critical threshold"
	}

	// Warning conditions
	if status.AvgLatencyMs > 5000 {
		return true, "WARNING: P95 latency exceeds 5 seconds"
	}

	if status.RecentErrorRate > 0.1 {
		return true, "WARNING: Error rate exceeds 10%"
	}

	return false, ""
}

// GetUnhealthyDomains returns a list of domains that need attention
func (a *AnalyticsAggregator) GetUnhealthyDomains(ctx context.Context, threshold float64) ([]*DomainHealthStatus, error) {
	if threshold <= 0 {
		threshold = 60.0 // Default to degraded threshold
	}

	unhealthyMetrics, err := a.federationRepo.GetUnhealthyDomains(ctx, threshold)
	if err != nil {
		a.logger.Error("failed to retrieve unhealthy domains", zap.Error(err), zap.Float64("threshold", threshold))
		return nil, errors.Join(ErrUnhealthyDomainsRetrieveFailed, err)
	}

	var result []*DomainHealthStatus
	for _, metric := range unhealthyMetrics {
		status := &DomainHealthStatus{
			Domain:           metric.Domain,
			HealthScore:      metric.HealthScore,
			CheckedAt:        metric.Timestamp,
			RecentActivities: metric.ActivityCount,
			RecentErrors:     metric.FailedActivities,
			RecentErrorRate:  metric.ErrorRate,
			AvgLatencyMs:     metric.InboxDeliveryP95,
			AvailabilityRate: metric.InstanceReachability,
		}

		// Set status based on health score
		if metric.HealthScore >= 60 {
			status.Status = "DEGRADED"
		} else if metric.HealthScore >= 40 {
			status.Status = "UNHEALTHY"
		} else {
			status.Status = "CRITICAL"
		}

		// Check alert conditions
		shouldAlert, message := a.checkAlertConditions(status)
		status.ShouldAlert = shouldAlert
		status.AlertMessage = message

		result = append(result, status)
	}

	return result, nil
}

// Metric represents a single federation event to be recorded
type Metric struct {
	InboundBytes    int64  `json:"inbound_bytes"`
	OutboundBytes   int64  `json:"outbound_bytes"`
	ResponseTimeMs  int64  `json:"response_time_ms"`
	SignatureTimeMs int64  `json:"signature_time_ms"`
	Success         bool   `json:"success"`
	ErrorType       string `json:"error_type,omitempty"` // signature_failure, timeout, rate_limit, etc.
	ActivityType    string `json:"activity_type"`        // follow, like, announce, etc.
}

// DomainHealthStatus represents the current health status of a federated domain
type DomainHealthStatus struct {
	Domain           string    `json:"domain"`
	Status           string    `json:"status"`       // HEALTHY, DEGRADED, UNHEALTHY, CRITICAL
	HealthScore      float64   `json:"health_score"` // 0-100
	CheckedAt        time.Time `json:"checked_at"`
	RecentActivities int64     `json:"recent_activities"`
	RecentErrors     int64     `json:"recent_errors"`
	RecentErrorRate  float64   `json:"recent_error_rate"`
	AvgLatencyMs     int64     `json:"avg_latency_ms"`
	AvailabilityRate float64   `json:"availability_rate"`
	ShouldAlert      bool      `json:"should_alert"`
	AlertMessage     string    `json:"alert_message,omitempty"`
}
