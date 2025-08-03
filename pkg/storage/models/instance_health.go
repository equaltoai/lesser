package models

import (
	"fmt"
	"time"
)

// InstanceHealth represents health status for a federated instance
type InstanceHealth struct {
	// Keys - Using same pattern as legacy health checker
	PK string `dynamorm:"pk" json:"-"` // INSTANCE#domain
	SK string `dynamorm:"sk" json:"-"` // HEALTH#timestamp_nano

	// Core health data
	Domain       string        `json:"domain"`
	Timestamp    time.Time     `json:"timestamp"`
	Reachable    bool          `json:"reachable"`
	ResponseTime time.Duration `json:"response_time"`
	StatusCode   int           `json:"status_code"`
	ErrorMessage string        `json:"error_message,omitempty"`

	// Resource usage metrics
	CPUUsage    float64 `json:"cpu_usage,omitempty"`
	MemoryUsage float64 `json:"memory_usage,omitempty"`
	DiskUsage   float64 `json:"disk_usage,omitempty"`

	// Federation metrics
	InboxBacklog    int           `json:"inbox_backlog,omitempty"`
	ProcessingDelay time.Duration `json:"processing_delay,omitempty"`
	ErrorRate       float64       `json:"error_rate,omitempty"`

	// Additional metadata
	CheckerVersion string `json:"checker_version,omitempty"`
	UserAgent      string `json:"user_agent,omitempty"`

	// TTL for automatic cleanup (7 days)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the partition and sort keys
func (h *InstanceHealth) UpdateKeys() {
	h.PK = fmt.Sprintf("INSTANCE#%s", h.Domain)
	h.SK = fmt.Sprintf("HEALTH#%d", h.Timestamp.UnixNano())
	
	// Set TTL to 7 days from timestamp
	h.TTL = h.Timestamp.Add(7 * 24 * time.Hour).Unix()
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
	health.UpdateKeys()
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
		score -= min(penalty, 30.0)
	}

	// Error rate penalty
	score -= h.ErrorRate * 20.0

	// Backlog penalty (penalize anything over 1000 messages)
	if h.InboxBacklog > 1000 {
		penalty := float64(h.InboxBacklog-1000) / 1000.0 // -1 point per 1000 messages
		score -= min(penalty, 10.0)
	}

	return max(score, 0.0)
}

// InstanceHealthSummary represents aggregated health data for an instance
type InstanceHealthSummary struct {
	// Keys for summary data
	PK string `dynamorm:"pk" json:"-"` // INSTANCE#domain
	SK string `dynamorm:"sk" json:"-"` // SUMMARY#window (e.g., SUMMARY#1h, SUMMARY#24h)

	// Metadata
	Domain        string        `json:"domain"`
	Window        time.Duration `json:"window"`        // Time window for aggregation
	LastUpdated   time.Time     `json:"last_updated"`
	SampleCount   int           `json:"sample_count"`
	
	// Aggregated metrics
	Availability      float64       `json:"availability"`       // Percentage of successful checks
	AvgResponseTime   time.Duration `json:"avg_response_time"`
	MaxResponseTime   time.Duration `json:"max_response_time"`
	ErrorRate         float64       `json:"error_rate"`
	AvgInboxBacklog   int           `json:"avg_inbox_backlog"`
	MaxInboxBacklog   int           `json:"max_inbox_backlog"`
	HealthScore       float64       `json:"health_score"`       // 0-100

	// Status code distribution
	StatusCodeCounts map[string]int `json:"status_code_counts"` // JSON serialized map

	// TTL for cleanup (summaries kept longer - 30 days)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the partition and sort keys for health summary
func (s *InstanceHealthSummary) UpdateKeys() {
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
	summary.UpdateKeys()
	return summary
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}