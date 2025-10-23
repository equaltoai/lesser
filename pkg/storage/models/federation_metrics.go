package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// FederationAnalyticsTimeSeries represents time series federation metrics with 5-minute primary aggregation
type FederationAnalyticsTimeSeries struct {
	// Primary keys - PK: FEDERATION_TIMESERIES#domain#period, SK: timestamp
	PK string `dynamorm:"pk" json:"-"` // FEDERATION_TIMESERIES#{domain}#{period}
	SK string `dynamorm:"sk" json:"-"` // {timestamp}

	// GSI1 - Domain-based queries: GSI1PK: DOMAIN#{domain}, GSI1SK: {period}#{timestamp}
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk"`

	// GSI2 - Period-based queries: GSI2PK: PERIOD#{period}, GSI2SK: {timestamp}#{domain}
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk"`

	// Core time series data
	Domain    string    `json:"domain"`
	Period    string    `json:"period"`    // 5min, hourly, daily, monthly
	Timestamp time.Time `json:"timestamp"` // Start time of the time window

	// Critical federation health metrics (following federation-analytics-guidance.md)

	// 1. Availability Metrics (40% weight in health scoring)
	InstanceReachability  float64    `json:"instance_reachability"` // % instances responding (0-1)
	EndpointAvailability  float64    `json:"endpoint_availability"` // % endpoints up (0-1)
	LastSuccessfulContact *time.Time `json:"last_successful_contact,omitempty"`
	ConsecutiveFailures   int64      `json:"consecutive_failures"`
	CircuitBreakerActive  bool       `json:"circuit_breaker_active"`

	// 2. Performance Metrics (30% weight)
	InboxDeliveryP50          int64 `json:"inbox_delivery_p50_ms"`     // ms
	InboxDeliveryP95          int64 `json:"inbox_delivery_p95_ms"`     // ms
	InboxDeliveryP99          int64 `json:"inbox_delivery_p99_ms"`     // ms
	OutboxProcessingTime      int64 `json:"outbox_processing_time_ms"` // ms
	SignatureVerificationTime int64 `json:"signature_verification_ms"` // ms
	MediaDeliveryTime         int64 `json:"media_delivery_time_ms"`    // ms

	// 3. Throughput Metrics (20% weight)
	IncomingActivitiesPerSec float64 `json:"incoming_activities_per_sec"`
	OutgoingActivitiesPerSec float64 `json:"outgoing_activities_per_sec"`
	QueueDepth               int64   `json:"queue_depth"`
	ProcessingBacklog        int64   `json:"processing_backlog_ms"`
	BurstCapacity            float64 `json:"burst_capacity"`

	// 4. Error Metrics (10% weight)
	SignatureFailures   int64   `json:"signature_failures"`
	TimeoutRate         float64 `json:"timeout_rate"` // 0-1
	RateLimitHits       int64   `json:"rate_limit_hits"`
	MalformedActivities int64   `json:"malformed_activities"`
	ValidationFailures  int64   `json:"validation_failures"`
	ErrorRate           float64 `json:"error_rate"` // Total error rate (0-1)

	// 5. Cost Efficiency Metrics
	PerActivityCost float64 `json:"per_activity_cost_usd"`
	BandwidthCost   float64 `json:"bandwidth_cost_usd"`
	ComputeCost     float64 `json:"compute_cost_usd"`
	StorageCost     float64 `json:"storage_cost_usd"`
	EgressCost      float64 `json:"egress_cost_usd"`

	// Volume metrics for aggregation
	TotalInboundVolume   int64 `json:"total_inbound_volume"`  // bytes
	TotalOutboundVolume  int64 `json:"total_outbound_volume"` // bytes
	ActivityCount        int64 `json:"activity_count"`
	SuccessfulActivities int64 `json:"successful_activities"`
	FailedActivities     int64 `json:"failed_activities"`

	// Health Score (calculated field)
	HealthScore float64 `json:"health_score"` // 0-100

	// Aggregation metadata
	WindowStart      time.Time `json:"window_start"`
	WindowEnd        time.Time `json:"window_end"`
	SampleCount      int64     `json:"sample_count"`              // Number of raw samples aggregated
	AggregationLevel string    `json:"aggregation_level"`         // raw, 5min, hourly, daily, monthly
	CompressedData   []byte    `json:"compressed_data,omitempty"` // Compressed historical data

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup based on aggregation level
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"` // Unix timestamp
}

// TableName returns the DynamoDB table backing FederationAnalyticsTimeSeries.
func (FederationAnalyticsTimeSeries) TableName() string {
	return MainTableName
}

// UpdateKeys sets all primary and GSI keys for the time series record
func (f *FederationAnalyticsTimeSeries) UpdateKeys() {
	// Primary key pattern: FEDERATION_TIMESERIES#{domain}#{period}
	f.PK = fmt.Sprintf("FEDERATION_TIMESERIES#%s#%s", f.Domain, f.Period)
	f.SK = f.Timestamp.Format(time.RFC3339)

	// GSI1 - Domain-based queries
	f.GSI1PK = fmt.Sprintf("DOMAIN#%s", f.Domain)
	f.GSI1SK = fmt.Sprintf("%s#%s", f.Period, f.Timestamp.Format(time.RFC3339))

	// GSI2 - Period-based queries (for cross-domain analysis)
	f.GSI2PK = fmt.Sprintf("PERIOD#%s", f.Period)
	f.GSI2SK = fmt.Sprintf("%s#%s", f.Timestamp.Format(time.RFC3339), f.Domain)

	// Set TTL based on aggregation level following guidance
	f.setTTL()
}

// setTTL sets TTL based on aggregation level for progressive data retention
func (f *FederationAnalyticsTimeSeries) setTTL() {
	var retentionDuration time.Duration

	switch f.Period {
	case PeriodRaw:
		retentionDuration = 1 * time.Hour // Real-time: 1 hour retention
	case Period5Min:
		retentionDuration = 24 * time.Hour // Near-time: 24 hours retention
	case PeriodHourly:
		retentionDuration = 7 * 24 * time.Hour // Hourly: 7 days retention
	case PeriodDaily:
		retentionDuration = 90 * 24 * time.Hour // Daily: 90 days retention
	case PeriodMonthly:
		retentionDuration = 2 * 365 * 24 * time.Hour // Monthly: 2 years retention
	default:
		retentionDuration = 24 * time.Hour // Default to 24 hours
	}

	f.TTL = time.Now().Add(retentionDuration).Unix()
}

// CalculateHealthScore calculates instance health score based on weighted metrics
// Following guidance: 40% availability, 30% performance, 20% reliability, 10% activity
func (f *FederationAnalyticsTimeSeries) CalculateHealthScore() {
	score := 0.0

	// Availability: 40% weight
	score += f.InstanceReachability * 40.0

	// Performance: 30% weight
	performanceScore := 0.0
	if f.InboxDeliveryP95 > 0 {
		if f.InboxDeliveryP95 < 2000 { // < 2s
			performanceScore = 30.0
		} else if f.InboxDeliveryP95 < 5000 { // < 5s
			performanceScore = 20.0
		} else if f.InboxDeliveryP95 < 10000 { // < 10s
			performanceScore = 10.0
		}
	}
	score += performanceScore

	// Reliability: 20% weight (based on error rate)
	reliabilityScore := (1.0 - f.ErrorRate) * 20.0
	score += reliabilityScore

	// Activity: 10% weight (based on recent activity)
	activityScore := 0.0
	if f.LastSuccessfulContact != nil {
		hoursSinceLastContact := time.Since(*f.LastSuccessfulContact).Hours()
		if hoursSinceLastContact < 1 {
			activityScore = 10.0
		} else if hoursSinceLastContact < 24 {
			activityScore = 5.0
		}
	}
	score += activityScore

	f.HealthScore = score
}

// GetTimeBucket returns the time bucket for the given aggregation period
func GetTimeBucket(timestamp time.Time, period string) time.Time {
	switch period {
	case PeriodRaw:
		return timestamp.Truncate(time.Minute)
	case Period5Min:
		// Truncate to 5-minute boundaries
		return timestamp.Truncate(5 * time.Minute)
	case PeriodHourly:
		return timestamp.Truncate(time.Hour)
	case PeriodDaily:
		return time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())
	case PeriodMonthly:
		return time.Date(timestamp.Year(), timestamp.Month(), 1, 0, 0, 0, 0, timestamp.Location())
	default:
		return timestamp.Truncate(5 * time.Minute) // Default to 5-minute buckets
	}
}

// NewFederationAnalyticsTimeSeries creates a new time series record with proper defaults
func NewFederationAnalyticsTimeSeries(domain, period string, timestamp time.Time) *FederationAnalyticsTimeSeries {
	now := time.Now()

	// Get the proper time bucket
	bucketTime := GetTimeBucket(timestamp, period)

	fs := &FederationAnalyticsTimeSeries{
		Domain:           domain,
		Period:           period,
		Timestamp:        bucketTime,
		WindowStart:      bucketTime,
		WindowEnd:        getWindowEnd(bucketTime, period),
		AggregationLevel: period,
		CreatedAt:        now,
		UpdatedAt:        now,
		SampleCount:      0,
	}

	// Set keys and TTL
	fs.UpdateKeys()

	return fs
}

// getWindowEnd calculates the end time of the aggregation window
func getWindowEnd(windowStart time.Time, period string) time.Time {
	switch period {
	case PeriodRaw:
		return windowStart.Add(time.Minute)
	case Period5Min:
		return windowStart.Add(5 * time.Minute)
	case PeriodHourly:
		return windowStart.Add(time.Hour)
	case PeriodDaily:
		return windowStart.AddDate(0, 0, 1)
	case PeriodMonthly:
		return windowStart.AddDate(0, 1, 0)
	default:
		return windowStart.Add(5 * time.Minute)
	}
}

// IsHealthy returns true if the instance is considered healthy based on thresholds
func (f *FederationAnalyticsTimeSeries) IsHealthy() bool {
	return f.HealthScore >= 80.0 // Healthy threshold from guidance
}

// IsDegraded returns true if the instance is in a degraded state
func (f *FederationAnalyticsTimeSeries) IsDegraded() bool {
	return f.HealthScore >= 60.0 && f.HealthScore < 80.0
}

// IsUnhealthy returns true if the instance is unhealthy
func (f *FederationAnalyticsTimeSeries) IsUnhealthy() bool {
	return f.HealthScore >= 40.0 && f.HealthScore < 60.0
}

// IsCritical returns true if the instance is in critical state
func (f *FederationAnalyticsTimeSeries) IsCritical() bool {
	return f.HealthScore < 40.0
}

// ShouldTriggerAlert checks if this metric should trigger an alert
func (f *FederationAnalyticsTimeSeries) ShouldTriggerAlert() (bool, string) {
	// Critical alerts (immediate page)
	if f.InstanceReachability < 0.5 {
		return true, "CRITICAL: Instance reachability below 50%"
	}
	if f.SignatureFailures > 100 {
		return true, "CRITICAL: Signature failures exceed 100/period"
	}

	// Warning alerts
	if f.InboxDeliveryP95 > 5000 {
		return true, "WARNING: P95 latency exceeds 5 seconds"
	}
	if f.QueueDepth > 10000 {
		return true, "WARNING: Queue depth exceeds 10,000"
	}

	return false, ""
}

// Aggregate aggregates raw metrics into this time series record
func (f *FederationAnalyticsTimeSeries) Aggregate(rawMetrics []*FederationAnalyticsTimeSeries) {
	if err := common.ValidateSliceNotEmpty("rawMetrics", rawMetrics); err != nil {
		return
	}

	// Initialize aggregation variables
	var totalInbound, totalOutbound int64
	var totalActivities, successfulActivities, failedActivities int64
	var totalP50, totalP95, totalP99, totalSigVerif time.Duration
	var totalErrors, totalTimeouts, totalRateLimits int64
	var reachabilitySum, availabilitySum float64

	count := int64(len(rawMetrics))

	// Aggregate all metrics
	for _, raw := range rawMetrics {
		totalInbound += raw.TotalInboundVolume
		totalOutbound += raw.TotalOutboundVolume
		totalActivities += raw.ActivityCount
		successfulActivities += raw.SuccessfulActivities
		failedActivities += raw.FailedActivities

		totalP50 += time.Duration(raw.InboxDeliveryP50) * time.Millisecond
		totalP95 += time.Duration(raw.InboxDeliveryP95) * time.Millisecond
		totalP99 += time.Duration(raw.InboxDeliveryP99) * time.Millisecond
		totalSigVerif += time.Duration(raw.SignatureVerificationTime) * time.Millisecond

		totalErrors += raw.SignatureFailures + raw.ValidationFailures + raw.MalformedActivities
		totalTimeouts += raw.SignatureFailures // Approximation
		totalRateLimits += raw.RateLimitHits

		reachabilitySum += raw.InstanceReachability
		availabilitySum += raw.EndpointAvailability
	}

	// Set aggregated values
	f.TotalInboundVolume = totalInbound
	f.TotalOutboundVolume = totalOutbound
	f.ActivityCount = totalActivities
	f.SuccessfulActivities = successfulActivities
	f.FailedActivities = failedActivities
	f.SampleCount = count

	// Calculate averages
	if count > 0 {
		f.InboxDeliveryP50 = int64(totalP50.Milliseconds() / count)
		f.InboxDeliveryP95 = int64(totalP95.Milliseconds() / count)
		f.InboxDeliveryP99 = int64(totalP99.Milliseconds() / count)
		f.SignatureVerificationTime = int64(totalSigVerif.Milliseconds() / count)

		f.InstanceReachability = reachabilitySum / float64(count)
		f.EndpointAvailability = availabilitySum / float64(count)

		// Calculate error rate
		if totalActivities > 0 {
			f.ErrorRate = float64(failedActivities) / float64(totalActivities)
		}
	}

	// Calculate health score
	f.CalculateHealthScore()

	// Update timestamp
	f.UpdatedAt = time.Now()
}

// FederationAlert represents an alert condition for federation monitoring
type FederationAlert struct {
	Domain      string    `json:"domain"`
	Level       string    `json:"level"` // CRITICAL, WARNING, INFO
	Message     string    `json:"message"`
	HealthScore float64   `json:"health_score"`
	Timestamp   time.Time `json:"timestamp"`
}

// TableName returns the DynamoDB table backing FederationAlert.
func (FederationAlert) TableName() string {
	return MainTableName
}
