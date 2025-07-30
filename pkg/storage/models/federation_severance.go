package models

import (
	"fmt"
	"time"
)

// FederationSeverance represents a severed federation relationship
type FederationSeverance struct {
	PK           string    `dynamorm:"pk"`
	SK           string    `dynamorm:"sk"`
	GSI1PK       string    `dynamorm:"index:gsi1,pk"`
	GSI1SK       string    `dynamorm:"index:gsi1,sk"`
	UserID       string    `json:"user_id"`
	Domain       string    `json:"domain"`
	SeveredAt    time.Time `json:"severed_at"`
	Acknowledged bool      `json:"acknowledged"`
	Reason       string    `json:"reason"`
	Type         string    `json:"type"` // "domain_block", "suspension", "defederation"
}

// UpdateKeys updates the GSI keys for federation severance
func (f *FederationSeverance) UpdateKeys() {
	f.PK = fmt.Sprintf("USER#%s", f.UserID)
	f.SK = fmt.Sprintf("SEVERANCE#%s", f.Domain)
	
	// GSI1 for global severance tracking
	f.GSI1PK = fmt.Sprintf("SEVERANCE#%s", f.Domain)
	f.GSI1SK = fmt.Sprintf("USER#%s", f.UserID)
}

// FederationIssue tracks federation issues for monitoring
type FederationIssue struct {
	PK          string    `dynamorm:"pk"`
	SK          string    `dynamorm:"sk"`
	Domain      string    `json:"domain"`
	IssueType   string    `json:"issue_type"`   // "timeout", "error", "unreachable", "blocked"
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description,omitempty"`
	Severity    string    `json:"severity"`     // "low", "medium", "high", "critical"
	Resolved    bool      `json:"resolved"`
	TTL         int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the GSI keys for federation issue
func (f *FederationIssue) UpdateKeys() {
	f.PK = fmt.Sprintf("FEDERATION_ISSUE#%s", f.Domain)
	f.SK = fmt.Sprintf("TIMESTAMP#%d", f.Timestamp.Unix())
	
	// Set TTL to 90 days
	if f.TTL == 0 {
		f.TTL = f.Timestamp.Add(90 * 24 * time.Hour).Unix()
	}
}

// ReconnectionAttempt tracks attempts to reconnect to severed domains
type ReconnectionAttempt struct {
	PK           string    `dynamorm:"pk"`
	SK           string    `dynamorm:"sk"`
	UserID       string    `json:"user_id"`
	Domain       string    `json:"domain"`
	AttemptedAt  time.Time `json:"attempted_at"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
	Method       string    `json:"method"` // "manual", "automatic", "scheduled"
}

// UpdateKeys updates the GSI keys for reconnection attempt
func (r *ReconnectionAttempt) UpdateKeys() {
	r.PK = fmt.Sprintf("RECONNECTION#%s#%s", r.UserID, r.Domain)
	r.SK = fmt.Sprintf("ATTEMPT#%d", r.AttemptedAt.Unix())
}

// FederationTimeSeries stores time-series federation metrics
type FederationTimeSeries struct {
	PK             string                 `dynamorm:"pk"`
	SK             string                 `dynamorm:"sk"`
	GSI1PK         string                 `dynamorm:"index:gsi1,pk"`
	GSI1SK         string                 `dynamorm:"index:gsi1,sk"`
	Domain         string                 `json:"domain"`
	Period         string                 `json:"period"`     // "hourly", "daily", "weekly"
	Timestamp      time.Time              `json:"timestamp"`
	Metrics        map[string]interface{} `json:"metrics"`
	ActivityVolume int64                  `json:"activity_volume"`
	ErrorCount     int64                  `json:"error_count"`
	SuccessCount   int64                  `json:"success_count"`
	TTL            int64                  `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the GSI keys for federation time series
func (f *FederationTimeSeries) UpdateKeys() {
	f.PK = fmt.Sprintf("TIMESERIES#%s#%s", f.Domain, f.Period)
	f.SK = f.Timestamp.Format(time.RFC3339)
	
	// GSI1 for period-based queries
	f.GSI1PK = fmt.Sprintf("TIMESERIES#%s", f.Period)
	f.GSI1SK = fmt.Sprintf("%s#%s", f.Timestamp.Format(time.RFC3339), f.Domain)
	
	// Set TTL based on period
	if f.TTL == 0 {
		switch f.Period {
		case "hourly":
			f.TTL = f.Timestamp.Add(7 * 24 * time.Hour).Unix() // Keep hourly data for 7 days
		case "daily":
			f.TTL = f.Timestamp.Add(30 * 24 * time.Hour).Unix() // Keep daily data for 30 days
		case "weekly":
			f.TTL = f.Timestamp.Add(365 * 24 * time.Hour).Unix() // Keep weekly data for 1 year
		}
	}
}