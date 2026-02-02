package models

import (
	"fmt"
	"time"
)

// TimelineMarker represents a user's position in a timeline
type TimelineMarker struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"pk"` // USER#{username}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // MARKER#{timeline}

	Username   string    `theorydb:"attr:username" json:"username"`
	Timeline   string    `theorydb:"attr:timeline" json:"timeline"` // home, notifications, etc.
	LastReadID string    `theorydb:"attr:lastReadID" json:"last_read_id"`
	UpdatedAt  time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	Version    int       `theorydb:"version,attr:version" json:"version"`
}

// TableName returns the DynamoDB table name
func (TimelineMarker) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the model before creation
func (m *TimelineMarker) BeforeCreate() error {
	m.PK = fmt.Sprintf(KeyPatternUser, m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)

	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}

	return nil
}

// BeforeUpdate updates the timestamp
func (m *TimelineMarker) BeforeUpdate() error {
	m.UpdatedAt = time.Now()
	return nil
}

// GetPK returns the partition key
func (m *TimelineMarker) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *TimelineMarker) GetSK() string {
	return m.SK
}

// UpdateKeys updates the keys
func (m *TimelineMarker) UpdateKeys() error {
	// Validate required fields
	if m.Username == "" {
		return fmt.Errorf("username is required")
	}
	if m.Timeline == "" {
		return fmt.Errorf("timeline is required")
	}

	// Set primary keys
	m.PK = fmt.Sprintf(KeyPatternUser, m.Username)
	m.SK = fmt.Sprintf("MARKER#%s", m.Timeline)

	return nil
}
