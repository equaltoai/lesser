package models

import (
	"fmt"
	"time"
)

// UserLogin represents a login attempt record
type UserLogin struct {
	PK string `dynamorm:"pk" json:"pk"` // USER#{username}
	SK string `dynamorm:"sk" json:"sk"` // LOGIN#{timestamp}

	Username  string    `json:"username"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	IPAddress string    `json:"ip_address,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`

	// TTL for automatic cleanup (e.g., 90 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
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
	return l.BeforeCreate()
}
