package models

import (
	"fmt"
	"time"
)

// UserLogin represents a login attempt record
type UserLogin struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"pk"` // USER#{username}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // LOGIN#{timestamp}

	Username  string    `theorydb:"attr:username" json:"username"`
	Timestamp time.Time `theorydb:"attr:timestamp" json:"timestamp"`
	Success   bool      `theorydb:"attr:success" json:"success"`
	IPAddress string    `theorydb:"attr:ipAddress" json:"ip_address,omitempty"`
	UserAgent string    `theorydb:"attr:userAgent" json:"user_agent,omitempty"`

	// TTL for automatic cleanup (e.g., 90 days)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (UserLogin) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the model before creation
func (l *UserLogin) BeforeCreate() error {
	l.PK = fmt.Sprintf(KeyPatternUser, l.Username)
	l.SK = fmt.Sprintf("LOGIN#%s", l.Timestamp.Format(time.RFC3339Nano))

	// Set TTL to 90 days from now
	l.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()

	return nil
}

// GetPK returns the partition key
func (l *UserLogin) GetPK() string {
	return l.PK
}

// GetSK returns the sort key
func (l *UserLogin) GetSK() string {
	return l.SK
}

// UpdateKeys updates the keys
func (l *UserLogin) UpdateKeys() error {
	// Validate required fields
	if l.Username == "" {
		return fmt.Errorf("username is required")
	}
	if l.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	// Set primary keys
	l.PK = fmt.Sprintf(KeyPatternUser, l.Username)
	l.SK = fmt.Sprintf("LOGIN#%s", l.Timestamp.Format(time.RFC3339Nano))

	return nil
}
