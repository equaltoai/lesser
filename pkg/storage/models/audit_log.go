package models

import (
	"fmt"
	"time"
)

// AuthAuditLog represents an authentication audit log entry in DynamoDB
type AuthAuditLog struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Core fields
	ID        string    `theorydb:"attr:id" json:"id"`
	Timestamp time.Time `theorydb:"attr:timestamp" json:"timestamp"`
	EventType string    `theorydb:"attr:eventType" json:"event_type"`
	Severity  string    `theorydb:"attr:severity" json:"severity"`

	// User information
	Username   string `theorydb:"attr:username" json:"username,omitempty"`
	UserID     string `theorydb:"attr:userID" json:"user_id,omitempty"`
	IPAddress  string `theorydb:"attr:ipAddress" json:"ip_address"`
	UserAgent  string `theorydb:"attr:userAgent" json:"user_agent,omitempty"`
	DeviceName string `theorydb:"attr:deviceName" json:"device_name,omitempty"`

	// Session information
	SessionID string `theorydb:"attr:sessionID" json:"session_id,omitempty"`
	RequestID string `theorydb:"attr:requestID" json:"request_id,omitempty"`

	// Result information
	Success       bool   `theorydb:"attr:success" json:"success"`
	FailureReason string `theorydb:"attr:failureReason" json:"failure_reason,omitempty"`

	// Geographic information
	Country   string  `theorydb:"attr:country" json:"country,omitempty"`
	City      string  `theorydb:"attr:city" json:"city,omitempty"`
	Region    string  `theorydb:"attr:region" json:"region,omitempty"`
	Latitude  float64 `theorydb:"attr:latitude" json:"latitude,omitempty"`
	Longitude float64 `theorydb:"attr:longitude" json:"longitude,omitempty"`

	// Risk assessment
	RiskScore float64  `theorydb:"attr:riskScore" json:"risk_score,omitempty"`
	RiskFlags []string `theorydb:"attr:riskFlags" json:"risk_flags,omitempty"`

	// Additional metadata (stored as JSON string)
	Metadata string `theorydb:"attr:metadata" json:"metadata,omitempty"`

	// GSI keys for querying
	GSI1PK string `theorydb:"attr:gsi1PK" json:"-"` // USER#username
	GSI1SK string `theorydb:"attr:gsi1SK" json:"-"` // AUDIT#timestamp
	GSI2PK string `theorydb:"attr:gsi2PK" json:"-"` // IP#address
	GSI2SK string `theorydb:"attr:gsi2SK" json:"-"` // AUDIT#timestamp
	GSI3PK string `theorydb:"attr:gsi3PK" json:"-"` // SESSION#id
	GSI3SK string `theorydb:"attr:gsi3SK" json:"-"` // AUDIT#timestamp
	GSI4PK string `theorydb:"attr:gsi4PK" json:"-"` // SEVERITY#level
	GSI4SK string `theorydb:"attr:gsi4SK" json:"-"` // AUDIT#timestamp

	// TTL for automatic deletion
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	// Compliance fields
	DataRetentionDays int      `theorydb:"attr:dataRetentionDays" json:"data_retention_days,omitempty"`
	ComplianceFlags   []string `theorydb:"attr:complianceFlags" json:"compliance_flags,omitempty"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing AuthAuditLog.
func (AuthAuditLog) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys before saving
func (a *AuthAuditLog) UpdateKeys() error {
	// Primary key - partitioned by date for efficient querying
	if a.Timestamp.IsZero() {
		a.Timestamp = time.Now().UTC()
	}
	a.PK = fmt.Sprintf("AUDIT#%s", a.Timestamp.Format("2006-01-02"))
	a.SK = fmt.Sprintf("EVENT#%s#%d", a.ID, a.Timestamp.UnixNano())

	// GSI1 - Query by user
	if a.Username != "" {
		a.GSI1PK = fmt.Sprintf("USER#%s", a.Username)
		a.GSI1SK = fmt.Sprintf("AUDIT#%d", a.Timestamp.Unix())
	}

	// GSI2 - Query by IP address
	if a.IPAddress != "" {
		a.GSI2PK = fmt.Sprintf("IP#%s", a.IPAddress)
		a.GSI2SK = fmt.Sprintf("AUDIT#%d", a.Timestamp.Unix())
	}

	// GSI3 - Query by session
	if a.SessionID != "" {
		a.GSI3PK = fmt.Sprintf("SESSION#%s", a.SessionID)
		a.GSI3SK = fmt.Sprintf("AUDIT#%d", a.Timestamp.Unix())
	}

	// GSI4 - Query by severity
	if a.Severity != "" {
		a.GSI4PK = fmt.Sprintf("SEVERITY#%s", a.Severity)
		a.GSI4SK = fmt.Sprintf("AUDIT#%d", a.Timestamp.Unix())
	}

	// Set TTL if retention days specified
	if a.DataRetentionDays > 0 {
		a.TTL = time.Now().Add(time.Duration(a.DataRetentionDays) * 24 * time.Hour).Unix()
	}

	// Set created timestamp
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}

	return nil
}

// BeforeSave is called before saving to DynamoDB
func (a *AuthAuditLog) BeforeSave() error {
	return a.UpdateKeys()
}

// GetPK returns the partition key
func (a *AuthAuditLog) GetPK() string {
	return a.PK
}

// GetSK returns the sort key
func (a *AuthAuditLog) GetSK() string {
	return a.SK
}
