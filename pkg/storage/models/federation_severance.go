package models

import (
	"fmt"
	"time"
)

// FederationSeverance represents a severed federation relationship
type FederationSeverance struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`

	UserID       string    `theorydb:"attr:userID" json:"user_id"`
	Domain       string    `theorydb:"attr:domain" json:"domain"`
	SeveredAt    time.Time `theorydb:"attr:severedAt" json:"severed_at"`
	Acknowledged bool      `theorydb:"attr:acknowledged" json:"acknowledged"`
	Reason       string    `theorydb:"attr:reason" json:"reason"`
	Type         string    `theorydb:"attr:type" json:"type"` // "domain_block", "suspension", "defederation"
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Domain      string    `theorydb:"attr:domain" json:"domain"`
	IssueType   string    `theorydb:"attr:issueType" json:"issue_type"` // "timeout", "error", "unreachable", "blocked"
	Timestamp   time.Time `theorydb:"attr:timestamp" json:"timestamp"`
	Description string    `theorydb:"attr:description" json:"description,omitempty"`
	Severity    string    `theorydb:"attr:severity" json:"severity"` // "low", "medium", "high", "critical"
	Resolved    bool      `theorydb:"attr:resolved" json:"resolved"`
	TTL         int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	UserID       string    `theorydb:"attr:userID" json:"user_id"`
	Domain       string    `theorydb:"attr:domain" json:"domain"`
	AttemptedAt  time.Time `theorydb:"attr:attemptedAt" json:"attempted_at"`
	Success      bool      `theorydb:"attr:success" json:"success"`
	ErrorMessage string    `theorydb:"attr:errorMessage" json:"error_message,omitempty"`
	Method       string    `theorydb:"attr:method" json:"method"` // "manual", "automatic", "scheduled"
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`

	Domain         string                 `theorydb:"attr:domain" json:"domain"`
	Period         string                 `theorydb:"attr:period" json:"period"` // "hourly", "daily", "weekly"
	Timestamp      time.Time              `theorydb:"attr:timestamp" json:"timestamp"`
	Metrics        map[string]interface{} `theorydb:"attr:metrics" json:"metrics"`
	ActivityVolume int64                  `theorydb:"attr:activityVolume" json:"activity_volume"`
	ErrorCount     int64                  `theorydb:"attr:errorCount" json:"error_count"`
	SuccessCount   int64                  `theorydb:"attr:successCount" json:"success_count"`
	TTL            int64                  `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
