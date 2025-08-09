package models

import (
	"fmt"
	"time"
)

// HealthCheckResult represents a stored health check result in DynamoDB
type HealthCheckResult struct {
	PK            string                 `dynamorm:"pk" json:"pk"`                 // HEALTH_CHECK#timestamp
	SK            string                 `dynamorm:"sk" json:"sk"`                 // RESULT#component_type#identifier
	GSI1PK        string                 `dynamorm:"index:gsi1,pk" json:"gsi1_pk"` // COMPONENT#component_type#identifier
	GSI1SK        string                 `dynamorm:"index:gsi1,sk" json:"gsi1_sk"` // timestamp
	Component     string                 `json:"component"`                        // component identifier
	ComponentType string                 `json:"component_type"`                   // "dynamodb", "lambda", "sqs"
	Status        string                 `json:"status"`                           // "healthy", "warning", "critical", "unknown"
	CheckTime     time.Time              `json:"check_time"`
	LatencyMs     int64                  `json:"latency_ms"`
	Error         string                 `json:"error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	RequestID     string                 `json:"request_id"`
	TTL           int64                  `dynamorm:"ttl" json:"ttl"` // Auto-expire after 30 days
}

// UpdateKeys updates the partition and sort keys for the health check result
func (h *HealthCheckResult) UpdateKeys() {
	timestamp := h.CheckTime.Format("2006-01-02T15:04:05Z")

	// Primary key: HEALTH_CHECK#{timestamp}
	h.PK = fmt.Sprintf("HEALTH_CHECK#%s", timestamp)

	// Sort key: RESULT#{component_type}#{component}
	h.SK = fmt.Sprintf("RESULT#%s#%s", h.ComponentType, h.Component)

	// GSI1 for querying by component
	h.GSI1PK = fmt.Sprintf("COMPONENT#%s#%s", h.ComponentType, h.Component)
	h.GSI1SK = timestamp

	// Set TTL to 30 days from check time
	h.TTL = h.CheckTime.Add(30 * 24 * time.Hour).Unix()
}

// NewHealthCheckResult creates a new health check result
func NewHealthCheckResult(componentType, component, status, requestID string, checkTime time.Time, latencyMs int64) *HealthCheckResult {
	result := &HealthCheckResult{
		Component:     component,
		ComponentType: componentType,
		Status:        status,
		CheckTime:     checkTime,
		LatencyMs:     latencyMs,
		RequestID:     requestID,
		Metadata:      make(map[string]interface{}),
	}

	result.UpdateKeys()
	return result
}

// HealthCheckSummaryResult represents aggregated health check data
type HealthCheckSummaryResult struct {
	PK             string    `dynamorm:"pk" json:"pk"`                 // HEALTH_SUMMARY#date
	SK             string    `dynamorm:"sk" json:"sk"`                 // SUMMARY#hour
	GSI1PK         string    `dynamorm:"index:gsi1,pk" json:"gsi1_pk"` // DATE#date
	GSI1SK         string    `dynamorm:"index:gsi1,sk" json:"gsi1_sk"` // HOUR#hour
	Date           string    `json:"date"`                             // YYYY-MM-DD
	Hour           int       `json:"hour"`                             // 0-23
	TotalChecks    int       `json:"total_checks"`
	HealthyChecks  int       `json:"healthy_checks"`
	WarningChecks  int       `json:"warning_checks"`
	CriticalChecks int       `json:"critical_checks"`
	UnknownChecks  int       `json:"unknown_checks"`
	AvgLatencyMs   float64   `json:"avg_latency_ms"`
	MaxLatencyMs   int64     `json:"max_latency_ms"`
	MinLatencyMs   int64     `json:"min_latency_ms"`
	LastUpdated    time.Time `json:"last_updated"`
	TTL            int64     `dynamorm:"ttl" json:"ttl"` // Auto-expire after 90 days
}

// UpdateKeys updates the partition and sort keys for the health check summary
func (h *HealthCheckSummaryResult) UpdateKeys() {
	// Primary key: HEALTH_SUMMARY#{date}
	h.PK = fmt.Sprintf("HEALTH_SUMMARY#%s", h.Date)

	// Sort key: SUMMARY#{hour}
	h.SK = fmt.Sprintf("SUMMARY#%02d", h.Hour)

	// GSI1 for time-based queries
	h.GSI1PK = fmt.Sprintf("DATE#%s", h.Date)
	h.GSI1SK = fmt.Sprintf("HOUR#%02d", h.Hour)

	// Set TTL to 90 days from last update
	h.TTL = h.LastUpdated.Add(90 * 24 * time.Hour).Unix()
}

// NewHealthCheckSummaryResult creates a new health check summary result
func NewHealthCheckSummaryResult(date string, hour int) *HealthCheckSummaryResult {
	summary := &HealthCheckSummaryResult{
		Date:         date,
		Hour:         hour,
		LastUpdated:  time.Now(),
		MinLatencyMs: -1, // Initialize to -1 to indicate no data yet
	}

	summary.UpdateKeys()
	return summary
}

// AddCheckResult adds a check result to the summary statistics
func (h *HealthCheckSummaryResult) AddCheckResult(status string, latencyMs int64) {
	h.TotalChecks++

	switch status {
	case StatusHealthy:
		h.HealthyChecks++
	case StatusWarning:
		h.WarningChecks++
	case StatusCritical:
		h.CriticalChecks++
	default:
		h.UnknownChecks++
	}

	// Update latency statistics
	if h.MinLatencyMs == -1 || latencyMs < h.MinLatencyMs {
		h.MinLatencyMs = latencyMs
	}
	if latencyMs > h.MaxLatencyMs {
		h.MaxLatencyMs = latencyMs
	}

	// Recalculate average latency
	totalLatency := h.AvgLatencyMs * float64(h.TotalChecks-1)
	h.AvgLatencyMs = (totalLatency + float64(latencyMs)) / float64(h.TotalChecks)

	h.LastUpdated = time.Now()
	h.UpdateKeys() // Update TTL
}

// ComponentHealthHistory represents historical health data for a specific component
type ComponentHealthHistory struct {
	PK            string    `dynamorm:"pk" json:"pk"` // COMPONENT_HISTORY#{component_type}#{component}
	SK            string    `dynamorm:"sk" json:"sk"` // HISTORY#{timestamp}
	Component     string    `json:"component"`
	ComponentType string    `json:"component_type"`
	Status        string    `json:"status"`
	CheckTime     time.Time `json:"check_time"`
	LatencyMs     int64     `json:"latency_ms"`
	Error         string    `json:"error,omitempty"`
	TTL           int64     `dynamorm:"ttl" json:"ttl"` // Auto-expire after 7 days
}

// UpdateKeys updates the partition and sort keys for component health history
func (c *ComponentHealthHistory) UpdateKeys() {
	// Primary key: COMPONENT_HISTORY#{component_type}#{component}
	c.PK = fmt.Sprintf("COMPONENT_HISTORY#%s#%s", c.ComponentType, c.Component)

	// Sort key: HISTORY#{timestamp}
	c.SK = fmt.Sprintf("HISTORY#%s", c.CheckTime.Format("2006-01-02T15:04:05Z"))

	// Set TTL to 7 days from check time
	c.TTL = c.CheckTime.Add(7 * 24 * time.Hour).Unix()
}

// NewComponentHealthHistory creates a new component health history entry
func NewComponentHealthHistory(componentType, component, status string, checkTime time.Time, latencyMs int64, err string) *ComponentHealthHistory {
	history := &ComponentHealthHistory{
		Component:     component,
		ComponentType: componentType,
		Status:        status,
		CheckTime:     checkTime,
		LatencyMs:     latencyMs,
		Error:         err,
	}

	history.UpdateKeys()
	return history
}
