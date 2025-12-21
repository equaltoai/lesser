package models

import (
	"fmt"
	"time"
)

// StatusPin represents a pinned status on a user's profile
type StatusPin struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"PK"` // USER#{username}#PINS
	SK string `dynamorm:"sk,attr:SK" json:"SK"` // STATUS#{status_id}

	// Core fields from legacy
	Username  string    `dynamorm:"attr:username" json:"username"`    // Who pinned the status
	StatusID  string    `dynamorm:"attr:statusID" json:"status_id"`   // The status that was pinned
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"` // When it was pinned
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
	if err := s.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

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
