package models

import (
	"fmt"
	"time"
)

// StatusPin represents a pinned status on a user's profile
type StatusPin struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // USER#{username}#PINS
	SK string `dynamorm:"sk" json:"SK"` // STATUS#{status_id}

	// Core fields from legacy
	Username  string    `json:"username"`   // Who pinned the status
	StatusID  string    `json:"status_id"`  // The status that was pinned
	CreatedAt time.Time `json:"created_at"` // When it was pinned
}

// TableName returns the DynamoDB table name
func (StatusPin) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the StatusPin for creation
func (s *StatusPin) BeforeCreate() error {
	// Set timestamp if not already set
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}

	// Update keys
	s.UpdateKeys()

	return nil
}

// UpdateKeys sets the primary keys based on the current data
func (s *StatusPin) UpdateKeys() error {
	s.PK = fmt.Sprintf("USER#%s#PINS", s.Username)
	s.SK = fmt.Sprintf(KeyPatternStatus, s.StatusID)
	return nil
}

// GetPK returns the partition key (implements BaseModel interface)
func (s *StatusPin) GetPK() string {
	return s.PK
}

// GetSK returns the sort key (implements BaseModel interface)
func (s *StatusPin) GetSK() string {
	return s.SK
}
