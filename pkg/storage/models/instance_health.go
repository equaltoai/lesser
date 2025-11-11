package models

import (
	"fmt"
	"time"
)

// InstanceHealth represents health status for a federated instance
type InstanceHealth struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys - Using same pattern as legacy health checker
	PK string `dynamorm:"pk,attr:PK" json:"-"` // INSTANCE#domain
	SK string `dynamorm:"sk,attr:SK" json:"-"` // HEALTH#timestamp_nano

	// Core health data
	Domain       string        `dynamorm:"attr:domain" json:"domain"`
	Timestamp    time.Time     `dynamorm:"attr:timestamp" json:"timestamp"`
	Reachable    bool          `dynamorm:"attr:reachable" json:"reachable"`
	ResponseTime time.Duration `dynamorm:"attr:responseTime" json:"response_time"`
	StatusCode   int           `dynamorm:"attr:statusCode" json:"status_code"`
	ErrorMessage string        `dynamorm:"attr:errorMessage" json:"error_message,omitempty"`

	// Resource usage metrics
	CPUUsage    float64 `dynamorm:"attr:cpuUsage" json:"cpu_usage,omitempty"`
	MemoryUsage float64 `dynamorm:"attr:memoryUsage" json:"memory_usage,omitempty"`
	DiskUsage   float64 `dynamorm:"attr:diskUsage" json:"disk_usage,omitempty"`

	// Federation metrics
	InboxBacklog    int           `dynamorm:"attr:inboxBacklog" json:"inbox_backlog,omitempty"`
	ProcessingDelay time.Duration `dynamorm:"attr:processingDelay" json:"processing_delay,omitempty"`
	ErrorRate       float64       `dynamorm:"attr:errorRate" json:"error_rate,omitempty"`

	// Additional metadata
	CheckerVersion string `dynamorm:"attr:checkerVersion" json:"checker_version,omitempty"`
	UserAgent      string `dynamorm:"attr:userAgent" json:"user_agent,omitempty"`

	// TTL for automatic cleanup (7 days)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceHealth.
func (InstanceHealth) TableName() string {
	return MainTableName
}

// UpdateKeys updates the partition and sort keys
func (h *InstanceHealth) UpdateKeys() error {
	h.PK = fmt.Sprintf("INSTANCE#%s", h.Domain)
	h.SK = fmt.Sprintf("HEALTH#%d", h.Timestamp.UnixNano())

	// Set TTL to 7 days from timestamp
	h.TTL = h.Timestamp.Add(7 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (h *InstanceHealth) GetPK() string {
	return h.PK
}

// GetSK returns the sort key for BaseModel interface
func (h *InstanceHealth) GetSK() string {
	return h.SK
}

// NewInstanceHealth creates a new instance health record
func NewInstanceHealth(domain string) *InstanceHealth {
	now := time.Now().UTC()
	health := &InstanceHealth{
		Domain:         domain,
		Timestamp:      now,
		Reachable:      false,
		ResponseTime:   0,
		StatusCode:     0,
		CheckerVersion: "serverless-v1",
		UserAgent:      "Lesser/1.0 (Federation Health Check)",
	}
	// UpdateKeys() is safe to ignore error here as it only does string formatting
	_ = health.UpdateKeys()
	return health
}

// IsHealthy returns true if the instance is considered healthy
func (h *InstanceHealth) IsHealthy() bool {
	return h.Reachable && h.StatusCode >= 200 && h.StatusCode < 400 && h.ErrorRate < 0.1
}

// IsCritical returns true if the instance is in critical state
func (h *InstanceHealth) IsCritical() bool {
	return !h.Reachable || h.StatusCode >= 500 || h.ErrorRate > 0.5
}

// GetHealthScore calculates a health score from 0-100
func (h *InstanceHealth) GetHealthScore() float64 {
	if !h.Reachable {
		return 0.0
	}

	score := 100.0

	// Status code penalty
	if h.StatusCode >= 500 {
		score -= 40.0
	} else if h.StatusCode >= 400 {
		score -= 20.0
	}

	// Response time penalty (penalize anything over 1 second)
	if h.ResponseTime > time.Second {
		penalty := float64(h.ResponseTime.Milliseconds()-1000) / 100.0 // -1 point per 100ms
		score -= mathMin(penalty, 30.0)
	}

	// Error rate penalty
	score -= h.ErrorRate * 20.0

	// Backlog penalty (penalize anything over 1000 messages)
	if h.InboxBacklog > 1000 {
		penalty := float64(h.InboxBacklog-1000) / 1000.0 // -1 point per 1000 messages
		score -= mathMin(penalty, 10.0)
	}

	return mathMax(score, 0.0)
}

// InstanceHealthSummary represents aggregated health data for an instance
type InstanceHealthSummary struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys for summary data
	PK string `dynamorm:"pk,attr:PK" json:"-"` // INSTANCE#domain
	SK string `dynamorm:"sk,attr:SK" json:"-"` // SUMMARY#window (e.g., SUMMARY#1h, SUMMARY#24h)

	// Metadata
	Domain      string        `dynamorm:"attr:domain" json:"domain"`
	Window      time.Duration `dynamorm:"attr:window" json:"window"` // Time window for aggregation
	LastUpdated time.Time     `dynamorm:"attr:lastUpdated" json:"last_updated"`
	SampleCount int           `dynamorm:"attr:sampleCount" json:"sample_count"`

	// Aggregated metrics
	Availability    float64       `dynamorm:"attr:availability" json:"availability"` // Percentage of successful checks
	AvgResponseTime time.Duration `dynamorm:"attr:avgResponseTime" json:"avg_response_time"`
	MaxResponseTime time.Duration `dynamorm:"attr:maxResponseTime" json:"max_response_time"`
	ErrorRate       float64       `dynamorm:"attr:errorRate" json:"error_rate"`
	AvgInboxBacklog int           `dynamorm:"attr:avgInboxBacklog" json:"avg_inbox_backlog"`
	MaxInboxBacklog int           `dynamorm:"attr:maxInboxBacklog" json:"max_inbox_backlog"`
	HealthScore     float64       `dynamorm:"attr:healthScore" json:"health_score"` // 0-100

	// Status code distribution
	StatusCodeCounts map[string]int `dynamorm:"attr:statusCodeCounts" json:"status_code_counts"` // JSON serialized map

	// TTL for cleanup (summaries kept longer - 30 days)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceHealthSummary.
func (InstanceHealthSummary) TableName() string {
	return MainTableName
}

// UpdateKeys updates the partition and sort keys for health summary
func (s *InstanceHealthSummary) UpdateKeys() error {
	s.PK = fmt.Sprintf("INSTANCE#%s", s.Domain)

	// Convert window to string identifier
	var windowStr string
	switch s.Window {
	case time.Hour:
		windowStr = "1h"
	case 24 * time.Hour:
		windowStr = "24h"
	case 7 * 24 * time.Hour:
		windowStr = "7d"
	default:
		windowStr = fmt.Sprintf("%ds", int(s.Window.Seconds()))
	}

	s.SK = fmt.Sprintf("SUMMARY#%s", windowStr)

	// Set TTL to 30 days from last update
	s.TTL = s.LastUpdated.Add(30 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (s *InstanceHealthSummary) GetPK() string {
	return s.PK
}

// GetSK returns the sort key for BaseModel interface
func (s *InstanceHealthSummary) GetSK() string {
	return s.SK
}

// NewInstanceHealthSummary creates a new health summary
func NewInstanceHealthSummary(domain string, window time.Duration) *InstanceHealthSummary {
	summary := &InstanceHealthSummary{
		Domain:           domain,
		Window:           window,
		LastUpdated:      time.Now().UTC(),
		SampleCount:      0,
		StatusCodeCounts: make(map[string]int),
	}
	// UpdateKeys() is safe to ignore error here as it only does string formatting
	_ = summary.UpdateKeys()
	return summary
}

// Helper functions
func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
