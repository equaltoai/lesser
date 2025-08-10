package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// Metrics represents system metrics data
type Metrics struct {
	// Primary key - using metric type as partition key with timestamp sort key
	PK string `dynamorm:"pk" json:"pk"` // Format: "metrics#{type}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "ts#{timestamp}#{id}"

	// GSI1 - Service queries
	GSI1PK string `dynamorm:"index:service-index,pk" json:"gsi1_pk"` // Format: "METRICS_SVC#{service}"
	GSI1SK string `dynamorm:"index:service-index,sk" json:"gsi1_sk"` // Format: "{timestamp}#{type}#{id}"

	// GSI2 - Aggregation queries
	GSI2PK string `dynamorm:"index:aggregate-index,pk" json:"gsi2_pk"` // Format: "METRICS_AGG#{period}#{type}"
	GSI2SK string `dynamorm:"index:aggregate-index,sk" json:"gsi2_sk"` // Format: "{timestamp}#{id}"

	// Core metrics data
	ID        string    `json:"id"`
	Type      string    `json:"type"`    // request, error, latency, throughput, etc.
	Service   string    `json:"service"` // api, auth, federation, etc.
	Timestamp time.Time `json:"timestamp"`
	Period    string    `json:"period"` // minute, hour, day

	// Metric values
	Value       float64            `json:"value"`
	Count       int64              `json:"count"`
	Sum         float64            `json:"sum"`
	Min         float64            `json:"min"`
	Max         float64            `json:"max"`
	Average     float64            `json:"average"`
	Percentiles map[string]float64 `json:"percentiles,omitempty"` // p50, p90, p95, p99

	// Dimensions for filtering
	Dimensions map[string]string `json:"dimensions,omitempty"`

	// Resource information
	ResourceID   string `json:"resource_id,omitempty"`   // Lambda function name, etc.
	ResourceType string `json:"resource_type,omitempty"` // lambda, dynamodb, etc.

	// Additional metadata
	Unit       string                 `json:"unit,omitempty"` // ms, count, bytes, etc.
	Tags       map[string]string      `json:"tags,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup (30 days for raw, 90 days for aggregated)
	ExpiresAt int64 `dynamorm:"ttl" json:"expires_at"` // Unix timestamp
}

// AggregatedMetrics represents pre-computed metrics aggregations
type AggregatedMetrics struct {
	// Primary key
	PK string `dynamorm:"pk" json:"pk"` // Format: "metrics_agg#{period}#{type}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "window#{windowStart}"

	// Aggregation details
	Period      string    `json:"period"`       // minute, hour, day, week, month
	Type        string    `json:"type"`         // Same as Metrics.Type
	Service     string    `json:"service"`      // Service name
	WindowStart time.Time `json:"window_start"` // Start of aggregation window
	WindowEnd   time.Time `json:"window_end"`   // End of aggregation window

	// Aggregated values
	TotalCount  int64              `json:"total_count"`
	TotalSum    float64            `json:"total_sum"`
	Average     float64            `json:"average"`
	Min         float64            `json:"min"`
	Max         float64            `json:"max"`
	StdDev      float64            `json:"std_dev"`
	Percentiles map[string]float64 `json:"percentiles"`

	// Breakdown by dimensions
	DimensionBreakdown map[string]DimensionStats `json:"dimension_breakdown,omitempty"`

	// Service-specific metrics
	ServiceMetrics map[string]interface{} `json:"service_metrics,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL (longer for aggregated data)
	ExpiresAt int64 `dynamorm:"ttl" json:"expires_at"`
}

// DimensionStats represents statistics for a specific dimension value
type DimensionStats struct {
	Value   string  `json:"value"`
	Count   int64   `json:"count"`
	Sum     float64 `json:"sum"`
	Average float64 `json:"average"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

// TableName returns the DynamoDB table name
func (Metrics) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table name
func (AggregatedMetrics) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the model before creation
func (m *Metrics) BeforeCreate() error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now

	// Generate ID if not provided
	if m.ID == "" {
		m.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if m.Timestamp.IsZero() {
		m.Timestamp = now
	}

	// Calculate average if not set
	if m.Count > 0 && m.Average == 0 {
		m.Average = m.Sum / float64(m.Count)
	}

	// Set TTL based on period
	ttlDays := 30 // Default for raw metrics
	if m.Period == "hour" || m.Period == "day" {
		ttlDays = 90 // Keep aggregated data longer
	}
	m.ExpiresAt = now.Add(time.Duration(ttlDays) * 24 * time.Hour).Unix()

	// Set up primary key
	m.PK = fmt.Sprintf("metrics#%s", m.Type)
	timestamp := m.Timestamp.Format("20060102150405")
	m.SK = fmt.Sprintf("ts#%s#%s", timestamp, m.ID)

	// Set up GSI keys
	m.setupGSIKeys()

	return m.Validate()
}

// BeforeUpdate sets up the model before update
func (m *Metrics) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Recalculate average
	if m.Count > 0 {
		m.Average = m.Sum / float64(m.Count)
	}

	// Update GSI keys in case indexed fields changed
	m.setupGSIKeys()

	return m.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (m *Metrics) setupGSIKeys() {
	timestampStr := m.Timestamp.Format(time.RFC3339)

	// GSI1 - Service queries
	m.GSI1PK = fmt.Sprintf("METRICS_SVC#%s", m.Service)
	m.GSI1SK = fmt.Sprintf("%s#%s#%s", timestampStr, m.Type, m.ID)

	// GSI2 - Aggregation queries
	m.GSI2PK = fmt.Sprintf("METRICS_AGG#%s#%s", m.Period, m.Type)
	m.GSI2SK = fmt.Sprintf("%s#%s", timestampStr, m.ID)
}

// Validate performs validation on the Metrics
func (m *Metrics) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("ID is required")
	}
	if strings.TrimSpace(m.Type) == "" {
		return fmt.Errorf("type is required")
	}
	if strings.TrimSpace(m.Service) == "" {
		return fmt.Errorf("service is required")
	}
	if !isValidMetricType(m.Type) {
		return fmt.Errorf("invalid metric type: %s", m.Type)
	}
	if m.Period != "" && !isValidPeriod(m.Period) {
		return fmt.Errorf("invalid period: %s", m.Period)
	}

	return nil
}

// BeforeCreate for AggregatedMetrics
func (am *AggregatedMetrics) BeforeCreate() error {
	now := time.Now()
	am.CreatedAt = now
	am.UpdatedAt = now

	// Set TTL (keep aggregated data longer)
	ttlDays := 90
	if am.Period == "month" {
		ttlDays = 365 // Keep monthly data for a year
	}
	am.ExpiresAt = now.Add(time.Duration(ttlDays) * 24 * time.Hour).Unix()

	// Set up primary key
	am.PK = fmt.Sprintf("metrics_agg#%s#%s", am.Period, am.Type)
	am.SK = fmt.Sprintf("window#%s", am.WindowStart.Format(time.RFC3339))

	return am.Validate()
}

// BeforeUpdate for AggregatedMetrics
func (am *AggregatedMetrics) BeforeUpdate() error {
	am.UpdatedAt = time.Now()
	return am.Validate()
}

// Validate for AggregatedMetrics
func (am *AggregatedMetrics) Validate() error {
	if strings.TrimSpace(am.Type) == "" {
		return fmt.Errorf("type is required")
	}
	if strings.TrimSpace(am.Period) == "" {
		return fmt.Errorf("period is required")
	}
	if am.WindowStart.IsZero() {
		return fmt.Errorf("WindowStart is required")
	}
	if am.WindowEnd.IsZero() {
		return fmt.Errorf("WindowEnd is required")
	}
	if am.WindowEnd.Before(am.WindowStart) {
		return fmt.Errorf("WindowEnd must be after WindowStart")
	}

	return nil
}

// AddDimension adds a dimension to the metrics
func (m *Metrics) AddDimension(key, value string) {
	if m.Dimensions == nil {
		m.Dimensions = make(map[string]string)
	}
	m.Dimensions[key] = value
}

// AddTag adds a tag to the metrics
func (m *Metrics) AddTag(key, value string) {
	if m.Tags == nil {
		m.Tags = make(map[string]string)
	}
	m.Tags[key] = value
}

// SetProperty sets a custom property
func (m *Metrics) SetProperty(key string, value interface{}) {
	if m.Properties == nil {
		m.Properties = make(map[string]interface{})
	}
	m.Properties[key] = value
}

// GetProperty gets a custom property
func (m *Metrics) GetProperty(key string) (interface{}, bool) {
	if m.Properties == nil {
		return nil, false
	}
	value, exists := m.Properties[key]
	return value, exists
}

// SetPercentile sets a percentile value
func (m *Metrics) SetPercentile(percentile string, value float64) {
	if m.Percentiles == nil {
		m.Percentiles = make(map[string]float64)
	}
	m.Percentiles[percentile] = value
}

// isValidMetricType checks if the metric type is valid
func isValidMetricType(metricType string) bool {
	validTypes := map[string]bool{
		"request":            true,
		"error":              true,
		"latency":            true,
		"throughput":         true,
		"cpu":                true,
		"memory":             true,
		"disk":               true,
		"network":            true,
		"custom":             true,
		"database_read":      true,
		"database_write":     true,
		"cache_hit":          true,
		"cache_miss":         true,
		"queue_depth":        true,
		"concurrent_users":   true,
		"active_connections": true,
	}
	return validTypes[strings.ToLower(metricType)]
}

// isValidPeriod checks if the period is valid
func isValidPeriod(period string) bool {
	validPeriods := map[string]bool{
		"minute": true,
		"hour":   true,
		"day":    true,
		"week":   true,
		"month":  true,
	}
	return validPeriods[strings.ToLower(period)]
}

// MetricsBuilder helps create metrics
type MetricsBuilder struct {
	metrics *Metrics
}

// NewMetricsBuilder creates a new metrics builder
func NewMetricsBuilder() *MetricsBuilder {
	return &MetricsBuilder{
		metrics: &Metrics{
			Dimensions: make(map[string]string),
			Tags:       make(map[string]string),
			Properties: make(map[string]interface{}),
		},
	}
}

// ForService sets the service
func (mb *MetricsBuilder) ForService(service string) *MetricsBuilder {
	mb.metrics.Service = service
	return mb
}

// OfType sets the metric type
func (mb *MetricsBuilder) OfType(metricType string) *MetricsBuilder {
	mb.metrics.Type = metricType
	return mb
}

// WithValue sets the metric value
func (mb *MetricsBuilder) WithValue(value float64) *MetricsBuilder {
	mb.metrics.Value = value
	mb.metrics.Count = 1
	mb.metrics.Sum = value
	mb.metrics.Min = value
	mb.metrics.Max = value
	mb.metrics.Average = value
	return mb
}

// WithStats sets statistical values
func (mb *MetricsBuilder) WithStats(count int64, sum, minVal, maxVal float64) *MetricsBuilder {
	mb.metrics.Count = count
	mb.metrics.Sum = sum
	mb.metrics.Min = minVal
	mb.metrics.Max = maxVal
	if count > 0 {
		mb.metrics.Average = sum / float64(count)
	}
	return mb
}

// WithUnit sets the unit
func (mb *MetricsBuilder) WithUnit(unit string) *MetricsBuilder {
	mb.metrics.Unit = unit
	return mb
}

// WithPeriod sets the period
func (mb *MetricsBuilder) WithPeriod(period string) *MetricsBuilder {
	mb.metrics.Period = period
	return mb
}

// WithResource sets resource information
func (mb *MetricsBuilder) WithResource(resourceID, resourceType string) *MetricsBuilder {
	mb.metrics.ResourceID = resourceID
	mb.metrics.ResourceType = resourceType
	return mb
}

// WithDimension adds a dimension
func (mb *MetricsBuilder) WithDimension(key, value string) *MetricsBuilder {
	mb.metrics.AddDimension(key, value)
	return mb
}

// WithTag adds a tag
func (mb *MetricsBuilder) WithTag(key, value string) *MetricsBuilder {
	mb.metrics.AddTag(key, value)
	return mb
}

// Build creates the metrics
func (mb *MetricsBuilder) Build() *Metrics {
	return mb.metrics
}

// MetricRecord represents the new reporting table schema with extensive indexing
// Following Architecture Decisions pattern: METRICS#<type>#<timestamp>
type MetricRecord struct {
	// Primary key pattern: METRICS#<type>#<bucket>
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"` // timestamp ISO format

	// GSI1: Service-based queries - SERVICE#<name> / TIMESTAMP#<iso>
	GSI1PK string `dynamorm:"index:service-index,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:service-index,sk" json:"gsi1_sk"`

	// GSI2: Metric type queries - METRIC_TYPE#<type> / TIMESTAMP#<iso>
	GSI2PK string `dynamorm:"index:metric-type-index,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:metric-type-index,sk" json:"gsi2_sk"`

	// GSI3: Date-based queries - DATE#<yyyy-mm-dd> / SERVICE#<name>#<timestamp>
	GSI3PK string `dynamorm:"index:date-index,pk" json:"gsi3_pk"`
	GSI3SK string `dynamorm:"index:date-index,sk" json:"gsi3_sk"`

	// GSI4: Aggregation queries - AGGREGATION#<level> / TIMESTAMP#<iso>
	GSI4PK string `dynamorm:"index:aggregation-index,pk" json:"gsi4_pk"`
	GSI4SK string `dynamorm:"index:aggregation-index,sk" json:"gsi4_sk"`

	// Core fields
	MetricType  string    `json:"metric_type"`
	ServiceName string    `json:"service_name"`
	Timestamp   time.Time `json:"timestamp"`
	MetricID    string    `json:"metric_id"` // UUID for uniqueness

	// Metric Values (statistical aggregates)
	Count int64   `json:"count,omitempty"`
	Sum   float64 `json:"sum,omitempty"`
	Min   float64 `json:"min,omitempty"`
	Max   float64 `json:"max,omitempty"`
	P50   float64 `json:"p50,omitempty"`
	P95   float64 `json:"p95,omitempty"`
	P99   float64 `json:"p99,omitempty"`

	// Dimensions for filtering/grouping
	Dimensions map[string]string `json:"dimensions,omitempty"`

	// Metadata
	AggregationLevel string `json:"aggregation_level"` // raw, 5min, hourly, daily
	Unit             string `json:"unit,omitempty"`    // ms, count, bytes, etc.

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup (data retention per aggregation level)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"` // Unix timestamp
}

// TableName returns the DynamoDB table name
func (MetricRecord) TableName() string {
	return MainTableName
}

// UpdateKeys implements BaseModel interface and populates ALL GSI keys based on record data
func (m *MetricRecord) UpdateKeys() error {
	// Validation
	if strings.TrimSpace(m.MetricType) == "" {
		return fmt.Errorf("MetricType is required")
	}
	if strings.TrimSpace(m.ServiceName) == "" {
		return fmt.Errorf("ServiceName is required")
	}
	if m.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if strings.TrimSpace(m.AggregationLevel) == "" {
		return fmt.Errorf("AggregationLevel is required")
	}

	// Generate ID if not provided
	if m.MetricID == "" {
		m.MetricID = uuid.New().String()
	}

	// PK format: METRICS#<type>#<bucket>
	bucket := m.getBucketString()
	m.PK = fmt.Sprintf("METRICS#%s#%s", m.MetricType, bucket)

	// SK: timestamp ISO format
	m.SK = m.Timestamp.Format(time.RFC3339)

	// GSI1: Service queries
	m.GSI1PK = fmt.Sprintf("SERVICE#%s", m.ServiceName)
	m.GSI1SK = fmt.Sprintf("TIMESTAMP#%s", m.Timestamp.Format(time.RFC3339))

	// GSI2: Metric type queries
	m.GSI2PK = fmt.Sprintf("METRIC_TYPE#%s", m.MetricType)
	m.GSI2SK = fmt.Sprintf("TIMESTAMP#%s", m.Timestamp.Format(time.RFC3339))

	// GSI3: Date queries
	m.GSI3PK = fmt.Sprintf("DATE#%s", m.Timestamp.Format(common.DateFormat))
	m.GSI3SK = fmt.Sprintf("SERVICE#%s#%s", m.ServiceName, m.Timestamp.Format(time.RFC3339))

	// GSI4: Aggregation queries
	m.GSI4PK = fmt.Sprintf("AGGREGATION#%s", m.AggregationLevel)
	m.GSI4SK = fmt.Sprintf("TIMESTAMP#%s", m.Timestamp.Format(time.RFC3339))

	// Set TTL based on aggregation level
	m.setTTL()

	return nil
}

// GetPK implements BaseModel interface
func (m *MetricRecord) GetPK() string {
	return m.PK
}

// GetSK implements BaseModel interface
func (m *MetricRecord) GetSK() string {
	return m.SK
}

// getBucketString returns the bucket string based on aggregation level for PK construction
func (m *MetricRecord) getBucketString() string {
	switch m.AggregationLevel {
	case "raw":
		return m.Timestamp.Format("2006-01-02T15:04") // Minute buckets
	case "5min":
		return m.Timestamp.Truncate(5 * time.Minute).Format("2006-01-02T15:04") // 5-min buckets
	case PeriodHourly:
		return m.Timestamp.Format("2006-01-02T15") // Hour buckets
	case PeriodDaily:
		return m.Timestamp.Format(common.DateFormat) // Day buckets
	default:
		return m.Timestamp.Format("2006-01-02T15:04")
	}
}

// setTTL sets TTL based on aggregation level for data retention
func (m *MetricRecord) setTTL() {
	var ttlDuration time.Duration

	switch m.AggregationLevel {
	case "raw":
		ttlDuration = 30 * 24 * time.Hour // 30 days
	case "5min":
		ttlDuration = 90 * 24 * time.Hour // 90 days
	case PeriodHourly:
		ttlDuration = 365 * 24 * time.Hour // 1 year
	case PeriodDaily:
		ttlDuration = 365 * 24 * time.Hour // 1 year
	default:
		ttlDuration = 30 * 24 * time.Hour
	}

	m.TTL = time.Now().Add(ttlDuration).Unix()
}

// BeforeCreate sets up the model before creation
func (m *MetricRecord) BeforeCreate() error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now

	// Set timestamp if not provided
	if m.Timestamp.IsZero() {
		m.Timestamp = now
	}

	// Update all keys
	if err := m.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return m.Validate()
}

// BeforeUpdate sets up the model before update
func (m *MetricRecord) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Update GSI keys in case indexed fields changed
	if err := m.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return m.Validate()
}

// Validate performs validation on the MetricRecord
func (m *MetricRecord) Validate() error {
	if strings.TrimSpace(m.MetricType) == "" {
		return fmt.Errorf("MetricType is required")
	}
	if strings.TrimSpace(m.ServiceName) == "" {
		return fmt.Errorf("ServiceName is required")
	}
	if m.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if !isValidAggregationLevel(m.AggregationLevel) {
		return fmt.Errorf("invalid aggregation level: %s", m.AggregationLevel)
	}

	return nil
}

// isValidAggregationLevel checks if the aggregation level is valid
func isValidAggregationLevel(level string) bool {
	validLevels := map[string]bool{
		"raw":       true,
		"5min":      true,
		"hourly":    true,
		PeriodDaily: true,
	}
	return validLevels[strings.ToLower(level)]
}

// AddDimension adds a dimension to the metric record
func (m *MetricRecord) AddDimension(key, value string) {
	if m.Dimensions == nil {
		m.Dimensions = make(map[string]string)
	}
	m.Dimensions[key] = value
}

// MetricRecordBuilder helps create metric records
type MetricRecordBuilder struct {
	record *MetricRecord
}

// NewMetricRecordBuilder creates a new metric record builder
func NewMetricRecordBuilder() *MetricRecordBuilder {
	return &MetricRecordBuilder{
		record: &MetricRecord{
			Dimensions: make(map[string]string),
		},
	}
}

// ForService sets the service name
func (mb *MetricRecordBuilder) ForService(serviceName string) *MetricRecordBuilder {
	mb.record.ServiceName = serviceName
	return mb
}

// OfType sets the metric type
func (mb *MetricRecordBuilder) OfType(metricType string) *MetricRecordBuilder {
	mb.record.MetricType = metricType
	return mb
}

// WithAggregationLevel sets the aggregation level
func (mb *MetricRecordBuilder) WithAggregationLevel(level string) *MetricRecordBuilder {
	mb.record.AggregationLevel = level
	return mb
}

// WithTimestamp sets the timestamp
func (mb *MetricRecordBuilder) WithTimestamp(timestamp time.Time) *MetricRecordBuilder {
	mb.record.Timestamp = timestamp
	return mb
}

// WithStats sets statistical values
func (mb *MetricRecordBuilder) WithStats(count int64, sum, minVal, maxVal, p50, p95, p99 float64) *MetricRecordBuilder {
	mb.record.Count = count
	mb.record.Sum = sum
	mb.record.Min = minVal
	mb.record.Max = maxVal
	mb.record.P50 = p50
	mb.record.P95 = p95
	mb.record.P99 = p99
	return mb
}

// WithUnit sets the unit
func (mb *MetricRecordBuilder) WithUnit(unit string) *MetricRecordBuilder {
	mb.record.Unit = unit
	return mb
}

// WithDimension adds a dimension
func (mb *MetricRecordBuilder) WithDimension(key, value string) *MetricRecordBuilder {
	mb.record.AddDimension(key, value)
	return mb
}

// Build creates the metric record
func (mb *MetricRecordBuilder) Build() *MetricRecord {
	return mb.record
}
