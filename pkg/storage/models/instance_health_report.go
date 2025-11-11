package models

import (
	"fmt"
	"time"
)

// InstanceHealthReport represents health metrics for a federated instance
type InstanceHealthReport struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK" json:"-"` // INSTANCE#domain
	SK string `dynamorm:"sk,attr:SK" json:"-"` // HEALTH#{timestamp}

	// Attributes from interface
	Domain          string    `dynamorm:"attr:domain" json:"domain"`
	Status          string    `dynamorm:"attr:status" json:"status"` // healthy/warning/critical
	ResponseTime    float64   `dynamorm:"attr:responseTime" json:"response_time"`
	ErrorRate       float64   `dynamorm:"attr:errorRate" json:"error_rate"`
	FederationDelay float64   `dynamorm:"attr:federationDelay" json:"federation_delay"`
	QueueDepth      int       `dynamorm:"attr:queueDepth" json:"queue_depth"`
	Issues          []string  `dynamorm:"attr:issues" json:"issues"`
	Recommendations []string  `dynamorm:"attr:recommendations" json:"recommendations"`
	LastChecked     time.Time `dynamorm:"attr:lastChecked" json:"last_checked"`

	// Additional metadata
	Timestamp string `dynamorm:"attr:timestamp" json:"timestamp"`                 // ISO timestamp for sorting
	TTL       int64  `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`               // 30 days retention
}

// TableName returns the DynamoDB table backing InstanceHealthReport.
func (InstanceHealthReport) TableName() string {
	return MainTableName
}

// UpdateKeys updates the partition and sort keys
func (h *InstanceHealthReport) UpdateKeys() {
	h.PK = fmt.Sprintf("INSTANCE#%s", h.Domain)
	h.SK = fmt.Sprintf("HEALTH#%s", h.Timestamp)

	// Set TTL to 30 days from last check
	h.TTL = h.LastChecked.AddDate(0, 0, 30).Unix()
}

// NewInstanceHealthReport creates a new health report
func NewInstanceHealthReport(domain string) *InstanceHealthReport {
	now := time.Now().UTC()
	report := &InstanceHealthReport{
		Domain:          domain,
		LastChecked:     now,
		Timestamp:       now.Format(time.RFC3339Nano),
		Status:          "unknown",
		Issues:          []string{},
		Recommendations: []string{},
	}
	report.UpdateKeys()
	return report
}

// GetHealthReportKey returns the key for retrieving a specific health report
func GetHealthReportKey(domain, timestamp string) (pk, sk string) {
	return fmt.Sprintf("INSTANCE#%s", domain), fmt.Sprintf("HEALTH#%s", timestamp)
}

// GetLatestHealthReportKeys returns keys for querying the latest report
func GetLatestHealthReportKeys(domain string) (pk, skPrefix string) {
	return fmt.Sprintf("INSTANCE#%s", domain), "HEALTH#"
}

// GetHealthReportRangeKeys returns keys for querying reports in a time range
func GetHealthReportRangeKeys(domain string, startTime, endTime time.Time) (pk, skStart, skEnd string) {
	pk = fmt.Sprintf("INSTANCE#%s", domain)
	skStart = fmt.Sprintf("HEALTH#%s", startTime.Format(time.RFC3339Nano))
	skEnd = fmt.Sprintf("HEALTH#%s", endTime.Format(time.RFC3339Nano))
	return
}

// SetHealthStatus updates the health status based on metrics
func (h *InstanceHealthReport) SetHealthStatus() {
	h.Issues = []string{}
	h.Recommendations = []string{}

	// Determine health status based on metrics
	if h.ErrorRate > 0.5 || h.ResponseTime > 5000 {
		h.Status = StatusCritical
		if h.ErrorRate > 0.5 {
			h.Issues = append(h.Issues, fmt.Sprintf("High error rate: %.1f%%", h.ErrorRate*100))
			h.Recommendations = append(h.Recommendations, "Check instance logs for errors")
		}
		if h.ResponseTime > 5000 {
			h.Issues = append(h.Issues, fmt.Sprintf("Very slow response time: %.0fms", h.ResponseTime))
			h.Recommendations = append(h.Recommendations, "Consider reducing federation frequency")
		}
	} else if h.ErrorRate > 0.1 || h.ResponseTime > 2000 || h.QueueDepth > 1000 {
		h.Status = StatusWarning
		if h.ErrorRate > 0.1 {
			h.Issues = append(h.Issues, fmt.Sprintf("Elevated error rate: %.1f%%", h.ErrorRate*100))
		}
		if h.ResponseTime > 2000 {
			h.Issues = append(h.Issues, fmt.Sprintf("Slow response time: %.0fms", h.ResponseTime))
		}
		if h.QueueDepth > 1000 {
			h.Issues = append(h.Issues, fmt.Sprintf("Large queue depth: %d", h.QueueDepth))
			h.Recommendations = append(h.Recommendations, "Monitor for processing delays")
		}
	} else {
		h.Status = "healthy"
	}

	if h.FederationDelay > 300 { // 5 minutes
		h.Issues = append(h.Issues, fmt.Sprintf("Federation delay: %.0f seconds", h.FederationDelay))
		h.Recommendations = append(h.Recommendations, "Check network connectivity")
	}
}

// IsHealthy returns true if the instance is healthy
func (h *InstanceHealthReport) IsHealthy() bool {
	return h.Status == "healthy"
}

// IsCritical returns true if the instance is in critical state
func (h *InstanceHealthReport) IsCritical() bool {
	return h.Status == "critical"
}
