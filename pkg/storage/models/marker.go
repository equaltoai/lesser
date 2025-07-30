package models

import (
	"fmt"
	"time"
)

// Marker represents a timeline position marker stored in DynamoDB using DynamORM
// Tracks user's last read positions for various timelines (home, notifications, etc.)
type Marker struct {
	// Primary key - matches legacy pattern exactly
	PK string `dynamorm:"pk" json:"pk"` // Format: "USER#{username}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "MARKER#{timeline}"

	// Timeline marker data - matches legacy MarkerRecord exactly
	LastReadID string    `json:"LastReadID"` // Preserve exact case from legacy
	UpdatedAt  time.Time `json:"UpdatedAt"`  // Preserve exact case from legacy
	Version    int       `json:"Version"`    // Preserve exact case from legacy

	// Internal fields for DynamORM operations
	Username string `json:"username"` // Extracted from PK for convenience
	Timeline string `json:"timeline"` // Extracted from SK for convenience
}

// TableName returns the DynamoDB table name for the Marker model
func (Marker) TableName() string {
	return "lesser-main" // Use the main table
}

// BeforeCreate sets up the model before creation
func (m *Marker) BeforeCreate() error {
	now := time.Now()
	m.UpdatedAt = now

	// Set up primary key using exact legacy pattern
	m.PK = fmt.Sprintf("USER#%s", m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)

	return nil
}

// BeforeUpdate sets up the model before update
func (m *Marker) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Ensure keys are set using exact legacy pattern
	m.PK = fmt.Sprintf("USER#%s", m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)

	return nil
}

// UpdateKeys updates the primary key fields based on username and timeline
// This method allows for key updates when username or timeline changes
func (m *Marker) UpdateKeys() {
	m.PK = fmt.Sprintf("USER#%s", m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)
}