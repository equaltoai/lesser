package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Mute represents a mute relationship between actors
type Mute struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `theorydb:"pk,attr:PK" json:"PK"` // MUTE#{username}
	SK string `theorydb:"sk,attr:SK" json:"SK"` // MUTED#{muted_username}

	// GSI1 for reverse lookup
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1PK"` // MUTED#{muted_username}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1SK"` // MUTER#{username}

	// Core fields from legacy
	Type              string    `theorydb:"attr:type" json:"Type"`                           // Always "Mute"
	Actor             string    `theorydb:"attr:actor" json:"Actor"`                         // Full actor ID who is muting
	Object            string    `theorydb:"attr:object" json:"Object"`                       // Full actor ID being muted
	ID                string    `theorydb:"attr:id" json:"ID"`                               // Mute activity ID
	HideNotifications bool      `theorydb:"attr:hideNotifications" json:"HideNotifications"` // Whether to hide notifications from this user
	Published         time.Time `theorydb:"attr:published" json:"Published"`                 // When the mute was published
	CreatedAt         time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`                 // When stored in DB
}

// TableName returns the DynamoDB table name
func (Mute) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the Mute for creation
func (m *Mute) BeforeCreate() error {
	// Set type
	m.Type = "Mute"

	// Set timestamps if not already set
	if m.Published.IsZero() {
		m.Published = time.Now().UTC()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("m.ID", m.ID); err != nil {
		m.ID = fmt.Sprintf("%s/activities/mute-%d", m.Actor, time.Now().Unix())
	}

	// Update keys based on actor usernames
	if err := m.UpdateKeys(); err != nil {
		return fmt.Errorf("%w: %w", ErrMuteUpdateKeysFailed, err)
	}

	return nil
}

// UpdateKeys sets the primary and GSI keys based on the actor usernames
// This implements the BaseModel interface requirement
func (m *Mute) UpdateKeys() error {
	// Extract usernames from actor IDs
	muterUsername := extractUsername(m.Actor)
	mutedUsername := extractUsername(m.Object)

	// Primary keys
	m.PK = fmt.Sprintf("MUTE#%s", muterUsername)
	m.SK = fmt.Sprintf("MUTED#%s", mutedUsername)

	// GSI1 for reverse lookup
	m.GSI1PK = fmt.Sprintf("MUTED#%s", mutedUsername)
	m.GSI1SK = fmt.Sprintf("MUTER#%s", muterUsername)

	return nil
}

// GetPK returns the partition key (implements BaseModel interface)
func (m *Mute) GetPK() string {
	return m.PK
}

// GetSK returns the sort key (implements BaseModel interface)
func (m *Mute) GetSK() string {
	return m.SK
}
