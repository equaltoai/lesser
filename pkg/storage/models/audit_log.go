package models

import (
	"fmt"
	"time"
)

// AuthAuditLog represents an authentication audit log entry in DynamoDB
type AuthAuditLog struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Core fields
	ID        string    `dynamorm:"attr:id" json:"id"`
	Timestamp time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	EventType string    `dynamorm:"attr:eventType" json:"event_type"`
	Severity  string    `dynamorm:"attr:severity" json:"severity"`

	// User information
	Username   string `dynamorm:"attr:username" json:"username,omitempty"`
	UserID     string `dynamorm:"attr:userID" json:"user_id,omitempty"`
	IPAddress  string `dynamorm:"attr:ipAddress" json:"ip_address"`
	UserAgent  string `dynamorm:"attr:userAgent" json:"user_agent,omitempty"`
	DeviceName string `dynamorm:"attr:deviceName" json:"device_name,omitempty"`

	// Session information
	SessionID string `dynamorm:"attr:sessionID" json:"session_id,omitempty"`
	RequestID string `dynamorm:"attr:requestID" json:"request_id,omitempty"`

	// Result information
	Success       bool   `dynamorm:"attr:success" json:"success"`
	FailureReason string `dynamorm:"attr:failureReason" json:"failure_reason,omitempty"`

	// Geographic information
	Country   string  `dynamorm:"attr:country" json:"country,omitempty"`
	City      string  `dynamorm:"attr:city" json:"city,omitempty"`
	Region    string  `dynamorm:"attr:region" json:"region,omitempty"`
	Latitude  float64 `dynamorm:"attr:latitude" json:"latitude,omitempty"`
	Longitude float64 `dynamorm:"attr:longitude" json:"longitude,omitempty"`

	// Risk assessment
	RiskScore float64  `dynamorm:"attr:riskScore" json:"risk_score,omitempty"`
	RiskFlags []string `dynamorm:"attr:riskFlags" json:"risk_flags,omitempty"`

	// Additional metadata (stored as JSON string)
	Metadata string `dynamorm:"attr:metadata" json:"metadata,omitempty"`

	// GSI keys for querying
	GSI1PK string `dynamorm:"attr:gsi1PK" json:"-"` // USER#username
	GSI1SK string `dynamorm:"attr:gsi1SK" json:"-"` // AUDIT#timestamp
	GSI2PK string `dynamorm:"attr:gsi2PK" json:"-"` // IP#address
	GSI2SK string `dynamorm:"attr:gsi2SK" json:"-"` // AUDIT#timestamp
	GSI3PK string `dynamorm:"attr:gsi3PK" json:"-"` // SESSION#id
	GSI3SK string `dynamorm:"attr:gsi3SK" json:"-"` // AUDIT#timestamp
	GSI4PK string `dynamorm:"attr:gsi4PK" json:"-"` // SEVERITY#level
	GSI4SK string `dynamorm:"attr:gsi4SK" json:"-"` // AUDIT#timestamp

	// TTL for automatic deletion
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"-"`

	// Compliance fields
	DataRetentionDays int      `dynamorm:"attr:dataRetentionDays" json:"data_retention_days,omitempty"`
	ComplianceFlags   []string `dynamorm:"attr:complianceFlags" json:"compliance_flags,omitempty"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
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
