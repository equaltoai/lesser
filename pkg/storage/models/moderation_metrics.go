package models

import (
	"fmt"
	"time"
)

// ModerationMetricsEntry represents a single metrics entry in DynamoDB
type ModerationMetricsEntry struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - metrics by date
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "METRICS#{YYYY-MM-DD}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "STATS#{hour}#{metric_type}"

	// GSI1 - Metric type queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk,omitempty"` // Format: "METRIC_TYPE#{metric_type}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk,omitempty"` // Format: "DATE#{YYYY-MM-DD}#{hour}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "METRIC_STATS"

	// Metric data
	MetricType string `dynamorm:"attr:metricType" json:"metric_type"` // e.g., "content_type:text", "decision:allow"
	Count      int64  `dynamorm:"attr:count" json:"count"`
	Hour       string `dynamorm:"attr:hour" json:"hour"`
	Date       string `dynamorm:"attr:date" json:"date"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	TTL       int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationMetricsEntry) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (m *ModerationMetricsEntry) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *ModerationMetricsEntry) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationMetricsEntry) UpdateKeys() error {
	// Primary key - metrics by date
	m.PK = fmt.Sprintf("METRICS#%s", m.Date)
	m.SK = fmt.Sprintf("STATS#%s#%s", m.Hour, m.MetricType)

	// GSI1 - metric type queries
	m.GSI1PK = fmt.Sprintf("METRIC_TYPE#%s", m.MetricType)
	m.GSI1SK = fmt.Sprintf("DATE#%s#%s", m.Date, m.Hour)

	// Set type marker
	m.Type = "METRIC_STATS"

	// Set TTL (90 days)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
	return nil
}

// ModerationFalsePositive represents a false positive record
type ModerationFalsePositive struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - false positives by date
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "METRICS#{YYYY-MM-DD}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "FP#{content_id}"

	// GSI1 - False positive queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk,omitempty"` // "FALSE_POSITIVES"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk,omitempty"` // Format: "DATE#{YYYY-MM-DD}#{timestamp}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "FALSE_POSITIVE"

	// False positive data
	ContentID        string  `dynamorm:"attr:contentID" json:"content_id"`
	OriginalDecision string  `dynamorm:"attr:originalDecision" json:"original_decision"`
	Confidence       float64 `dynamorm:"attr:confidence" json:"confidence"`
	Date             string  `dynamorm:"attr:date" json:"date"`

	// Timestamps
	Timestamp time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	TTL       int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationFalsePositive) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (m *ModerationFalsePositive) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *ModerationFalsePositive) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationFalsePositive) UpdateKeys() error {
	// Primary key - false positives by date
	m.PK = fmt.Sprintf("METRICS#%s", m.Date)
	m.SK = fmt.Sprintf("FP#%s", m.ContentID)

	// GSI1 - false positive queries
	m.GSI1PK = "FALSE_POSITIVES"
	m.GSI1SK = fmt.Sprintf("DATE#%s#%s", m.Date, m.Timestamp.Format(time.RFC3339))

	// Set type marker
	m.Type = "FALSE_POSITIVE"

	// Set TTL (90 days)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
	return nil
}

// ModerationDecisionSample represents a decision sample for analysis
type ModerationDecisionSample struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - samples by date
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "SAMPLES#{YYYY-MM-DD}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "{unix_nano}#{content_id}"

	// GSI1 - Decision type queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk,omitempty"` // Format: "DECISION#{decision}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk,omitempty"` // Format: "DATE#{YYYY-MM-DD}#{timestamp}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "DECISION_SAMPLE"

	// Sample data
	ContentID      string  `dynamorm:"attr:contentID" json:"content_id"`
	Decision       string  `dynamorm:"attr:decision" json:"decision"`
	Confidence     float64 `dynamorm:"attr:confidence" json:"confidence"`
	ProcessingTime int64   `dynamorm:"attr:processingTime" json:"processing_time"` // milliseconds
	ReasonCount    int     `dynamorm:"attr:reasonCount" json:"reason_count"`
	RequiresReview bool    `dynamorm:"attr:requiresReview" json:"requires_review"`
	Date           string  `dynamorm:"attr:date" json:"date"`

	// Timestamps
	Timestamp time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	TTL       int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationDecisionSample) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (m *ModerationDecisionSample) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *ModerationDecisionSample) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationDecisionSample) UpdateKeys() error {
	// Primary key - samples by date
	m.PK = fmt.Sprintf("SAMPLES#%s", m.Date)
	m.SK = fmt.Sprintf("%d#%s", m.Timestamp.UnixNano(), m.ContentID)

	// GSI1 - decision type queries
	m.GSI1PK = fmt.Sprintf("DECISION#%s", m.Decision)
	m.GSI1SK = fmt.Sprintf("DATE#%s#%s", m.Date, m.Timestamp.Format(time.RFC3339))

	// Set type marker
	m.Type = "DECISION_SAMPLE"

	// Set TTL (30 days)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
	return nil
}

// ModerationPatternStats represents pattern matching statistics
type ModerationPatternStats struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - pattern stats by ID
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "PATTERN_STATS#{pattern_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "STATS"

	// GSI1 - Hit count ranking
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk,omitempty"` // "PATTERN_HITS"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk,omitempty"` // Format: "{hit_count_padded}#{pattern_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "PATTERN_STATS"

	// Pattern stats data
	PatternID   string    `dynamorm:"attr:patternID" json:"pattern_id"`
	PatternName string    `dynamorm:"attr:patternName" json:"pattern_name"`
	HitCount    int64     `dynamorm:"attr:hitCount" json:"hit_count"`
	LastHit     time.Time `dynamorm:"attr:lastHit" json:"last_hit"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
	TTL       int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationPatternStats) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (m *ModerationPatternStats) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *ModerationPatternStats) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationPatternStats) UpdateKeys() error {
	// Primary key - pattern stats by ID
	m.PK = fmt.Sprintf("PATTERN_STATS#%s", m.PatternID)
	m.SK = SKStats

	// GSI1 - hit count ranking (pad hit count for lexicographic sorting)
	m.GSI1PK = "PATTERN_HITS"
	m.GSI1SK = fmt.Sprintf("%020d#%s", m.HitCount, m.PatternID)

	// Set type marker
	m.Type = "PATTERN_STATS"

	// Set TTL (90 days)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
	return nil
}

// Helper types for moderation metrics

// AdvancedModerationAction represents advanced moderation action types - different from legacy ModerationAction
type AdvancedModerationAction string

const (
	// AdvancedModerationActionAllow represents allowing content
	AdvancedModerationActionAllow AdvancedModerationAction = "allow"
	// AdvancedModerationActionFlag represents flagging content
	AdvancedModerationActionFlag AdvancedModerationAction = "flag"
	// AdvancedModerationActionQuarantine represents quarantining content
	AdvancedModerationActionQuarantine AdvancedModerationAction = "quarantine"
	// AdvancedModerationActionRemove represents removing content
	AdvancedModerationActionRemove AdvancedModerationAction = "remove"
	// AdvancedModerationActionShadowBan represents shadow banning
	AdvancedModerationActionShadowBan AdvancedModerationAction = "shadow_ban"
	// AdvancedModerationActionReportToAuth represents reporting to authorities
	AdvancedModerationActionReportToAuth AdvancedModerationAction = "report_to_authorities"
)

// AdvancedSeverity represents the severity level of a moderation issue
type AdvancedSeverity string

const (
	// AdvancedSeverityLow represents low severity
	AdvancedSeverityLow AdvancedSeverity = "low"
	// AdvancedSeverityMedium represents medium severity
	AdvancedSeverityMedium AdvancedSeverity = "medium"
	// AdvancedSeverityHigh represents high severity
	AdvancedSeverityHigh AdvancedSeverity = "high"
	// AdvancedSeverityCritical represents critical severity
	AdvancedSeverityCritical AdvancedSeverity = "critical"
)

// ModerationMetricsTimeRange represents a time range for metrics queries
type ModerationMetricsTimeRange struct {
	Start time.Time
	End   time.Time
}

// TableName returns the DynamoDB table backing ModerationMetricsTimeRange.
func (ModerationMetricsTimeRange) TableName() string {
	return MainTableName
}

// ModerationMetricsStats represents aggregated moderation statistics
type ModerationMetricsStats struct {
	TimeRange         ModerationMetricsTimeRange
	TotalAnalyzed     int64
	ActionCounts      map[AdvancedModerationAction]int64
	CategoryCounts    map[string]int64
	SeverityCounts    map[AdvancedSeverity]int64
	AverageConfidence float64
	FalsePositives    int64
	TruePositives     int64
	ResponseTime      time.Duration
}

// TableName returns the DynamoDB table backing ModerationMetricsStats.
func (ModerationMetricsStats) TableName() string {
	return MainTableName
}

// RealtimeStats represents current real-time statistics
type RealtimeStats struct {
	Uptime          time.Duration
	TotalAnalyzed   int64
	AnalysisRate    float64 // per second
	AllowRate       float64
	FlagRate        float64
	RemoveRate      float64
	QuarantineRate  float64
	AvgResponseTime time.Duration
	P95ResponseTime time.Duration
}

// TableName returns the DynamoDB table backing RealtimeStats.
func (RealtimeStats) TableName() string {
	return MainTableName
}

// PatternStats represents pattern matching statistics
type PatternStats struct {
	PatternID   string
	PatternName string
	HitCount    int64
	LastHit     time.Time
}

// TableName returns the DynamoDB table backing PatternStats.
func (PatternStats) TableName() string {
	return MainTableName
}
