package models

import (
	"fmt"
	"time"
)

// FederationSeverance represents a severed federation relationship
type FederationSeverance struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK"`

	UserID       string    `dynamorm:"attr:userID" json:"user_id"`
	Domain       string    `dynamorm:"attr:domain" json:"domain"`
	SeveredAt    time.Time `dynamorm:"attr:severedAt" json:"severed_at"`
	Acknowledged bool      `dynamorm:"attr:acknowledged" json:"acknowledged"`
	Reason       string    `dynamorm:"attr:reason" json:"reason"`
	Type         string    `dynamorm:"attr:type" json:"type"` // "domain_block", "suspension", "defederation"
}

// TableName returns the DynamoDB table backing FederationSeverance.
func (FederationSeverance) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for federation severance
func (f *FederationSeverance) UpdateKeys() {
	f.PK = fmt.Sprintf(KeyPatternUser, f.UserID)
	f.SK = fmt.Sprintf("SEVERANCE#%s", f.Domain)

	// GSI1 for global severance tracking
	f.GSI1PK = fmt.Sprintf("SEVERANCE#%s", f.Domain)
	f.GSI1SK = fmt.Sprintf(KeyPatternUser, f.UserID)
}

// FederationIssue tracks federation issues for monitoring
type FederationIssue struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	Domain      string    `dynamorm:"attr:domain" json:"domain"`
	IssueType   string    `dynamorm:"attr:issueType" json:"issue_type"` // "timeout", "error", "unreachable", "blocked"
	Timestamp   time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	Description string    `dynamorm:"attr:description" json:"description,omitempty"`
	Severity    string    `dynamorm:"attr:severity" json:"severity"` // "low", "medium", "high", "critical"
	Resolved    bool      `dynamorm:"attr:resolved" json:"resolved"`
	TTL         int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing FederationIssue.
func (FederationIssue) TableName() string {
	return MainTableName
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	UserID       string    `dynamorm:"attr:userID" json:"user_id"`
	Domain       string    `dynamorm:"attr:domain" json:"domain"`
	AttemptedAt  time.Time `dynamorm:"attr:attemptedAt" json:"attempted_at"`
	Success      bool      `dynamorm:"attr:success" json:"success"`
	ErrorMessage string    `dynamorm:"attr:errorMessage" json:"error_message,omitempty"`
	Method       string    `dynamorm:"attr:method" json:"method"` // "manual", "automatic", "scheduled"
}

// TableName returns the DynamoDB table backing ReconnectionAttempt.
func (ReconnectionAttempt) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for reconnection attempt
func (r *ReconnectionAttempt) UpdateKeys() {
	r.PK = fmt.Sprintf("RECONNECTION#%s#%s", r.UserID, r.Domain)
	r.SK = fmt.Sprintf("ATTEMPT#%d", r.AttemptedAt.Unix())
}

// FederationTimeSeries stores time-series federation metrics
type FederationTimeSeries struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK"`

	Domain         string                 `dynamorm:"attr:domain" json:"domain"`
	Period         string                 `dynamorm:"attr:period" json:"period"` // "hourly", "daily", "weekly"
	Timestamp      time.Time              `dynamorm:"attr:timestamp" json:"timestamp"`
	Metrics        map[string]interface{} `dynamorm:"attr:metrics" json:"metrics"`
	ActivityVolume int64                  `dynamorm:"attr:activityVolume" json:"activity_volume"`
	ErrorCount     int64                  `dynamorm:"attr:errorCount" json:"error_count"`
	SuccessCount   int64                  `dynamorm:"attr:successCount" json:"success_count"`
	TTL            int64                  `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing FederationTimeSeries.
func (FederationTimeSeries) TableName() string {
	return MainTableName
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
		case PeriodHourly:
			f.TTL = f.Timestamp.Add(7 * 24 * time.Hour).Unix() // Keep hourly data for 7 days
		case PeriodDaily:
			f.TTL = f.Timestamp.Add(30 * 24 * time.Hour).Unix() // Keep daily data for 30 days
		case PeriodWeekly:
			f.TTL = f.Timestamp.Add(365 * 24 * time.Hour).Unix() // Keep weekly data for 1 year
		}
	}
}
