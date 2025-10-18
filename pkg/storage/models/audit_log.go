package models

import (
	"fmt"
	"time"
)

// AuthAuditLog represents an authentication audit log entry in DynamoDB
type AuthAuditLog struct {
	// Primary key
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Core fields
	ID        string    `dynamorm:"id" json:"id"`
	Timestamp time.Time `dynamorm:"timestamp" json:"timestamp"`
	EventType string    `dynamorm:"event_type" json:"event_type"`
	Severity  string    `dynamorm:"severity" json:"severity"`

	// User information
	Username   string `dynamorm:"username,omitempty" json:"username,omitempty"`
	UserID     string `dynamorm:"user_id,omitempty" json:"user_id,omitempty"`
	IPAddress  string `dynamorm:"ip_address" json:"ip_address"`
	UserAgent  string `dynamorm:"user_agent,omitempty" json:"user_agent,omitempty"`
	DeviceName string `dynamorm:"device_name,omitempty" json:"device_name,omitempty"`

	// Session information
	SessionID string `dynamorm:"session_id,omitempty" json:"session_id,omitempty"`
	RequestID string `dynamorm:"request_id,omitempty" json:"request_id,omitempty"`

	// Result information
	Success       bool   `dynamorm:"success" json:"success"`
	FailureReason string `dynamorm:"failure_reason,omitempty" json:"failure_reason,omitempty"`

	// Geographic information
	Country   string  `dynamorm:"country,omitempty" json:"country,omitempty"`
	City      string  `dynamorm:"city,omitempty" json:"city,omitempty"`
	Region    string  `dynamorm:"region,omitempty" json:"region,omitempty"`
	Latitude  float64 `dynamorm:"latitude,omitempty" json:"latitude,omitempty"`
	Longitude float64 `dynamorm:"longitude,omitempty" json:"longitude,omitempty"`

	// Risk assessment
	RiskScore float64  `dynamorm:"risk_score,omitempty" json:"risk_score,omitempty"`
	RiskFlags []string `dynamorm:"risk_flags,omitempty" json:"risk_flags,omitempty"`

	// Additional metadata (stored as JSON string)
	Metadata string `dynamorm:"metadata,omitempty" json:"metadata,omitempty"`

	// GSI keys for querying
	GSI1PK string `dynamorm:"gsi1pk,omitempty" json:"-"` // USER#username
	GSI1SK string `dynamorm:"gsi1sk,omitempty" json:"-"` // AUDIT#timestamp
	GSI2PK string `dynamorm:"gsi2pk,omitempty" json:"-"` // IP#address
	GSI2SK string `dynamorm:"gsi2sk,omitempty" json:"-"` // AUDIT#timestamp
	GSI3PK string `dynamorm:"gsi3pk,omitempty" json:"-"` // SESSION#id
	GSI3SK string `dynamorm:"gsi3sk,omitempty" json:"-"` // AUDIT#timestamp
	GSI4PK string `dynamorm:"gsi4pk,omitempty" json:"-"` // SEVERITY#level
	GSI4SK string `dynamorm:"gsi4sk,omitempty" json:"-"` // AUDIT#timestamp

	// TTL for automatic deletion
	TTL int64 `dynamorm:"ttl,omitempty" json:"-"`

	// Compliance fields
	DataRetentionDays int      `dynamorm:"data_retention_days,omitempty" json:"data_retention_days,omitempty"`
	ComplianceFlags   []string `dynamorm:"compliance_flags,omitempty" json:"compliance_flags,omitempty"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
}

// TableName returns the DynamoDB table name
func (a *AuthAuditLog) TableName() string {
	return "" // Uses default table
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
