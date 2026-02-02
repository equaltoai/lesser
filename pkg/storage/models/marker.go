package models

import (
	"fmt"
	"time"
)

// Marker represents a timeline position marker stored in DynamoDB using DynamORM
// Tracks user's last read positions for various timelines (home, notifications, etc.)
type Marker struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - matches legacy pattern exactly
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "USER#{username}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "MARKER#{timeline}"

	// Timeline marker data - matches legacy MarkerRecord exactly
	LastReadID string    `theorydb:"attr:lastReadID" json:"LastReadID"` // Preserve exact case from legacy
	UpdatedAt  time.Time `theorydb:"attr:updatedAt" json:"UpdatedAt"`   // Preserve exact case from legacy
	Version    int       `theorydb:"attr:version" json:"Version"`       // Preserve exact case from legacy

	// Internal fields for DynamORM operations
	Username string `theorydb:"attr:username" json:"username"` // Extracted from PK for convenience
	Timeline string `theorydb:"attr:timeline" json:"timeline"` // Extracted from SK for convenience
}

// TableName returns the DynamoDB table name for the Marker model
func (Marker) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (m *Marker) BeforeCreate() error {
	now := time.Now()
	m.UpdatedAt = now

	// Set up primary key using exact legacy pattern
	m.PK = fmt.Sprintf(KeyPatternUser, m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)

	return nil
}

// BeforeUpdate sets up the model before update
func (m *Marker) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Ensure keys are set using exact legacy pattern
	m.PK = fmt.Sprintf(KeyPatternUser, m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)

	return nil
}

// UpdateKeys updates the primary key fields based on username and timeline
// This method allows for key updates when username or timeline changes
func (m *Marker) UpdateKeys() error {
	m.PK = fmt.Sprintf(KeyPatternUser, m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)
	return nil
}

// GetPK returns the partition key for BaseRepository interface
func (m *Marker) GetPK() string {
	return m.PK
}

// GetSK returns the sort key for BaseRepository interface
func (m *Marker) GetSK() string {
	return m.SK
}
