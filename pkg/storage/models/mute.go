package models

import (
	"fmt"
	"time"
)

// Mute represents a mute relationship between actors
type Mute struct {
	// Primary keys - MUST match legacy exactly  
	PK string `dynamorm:"pk" json:"PK"` // MUTE#{username}
	SK string `dynamorm:"sk" json:"SK"` // MUTED#{muted_username}
	
	// GSI1 for reverse lookup
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK"` // MUTED#{muted_username}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK"` // MUTER#{username}
	
	// Core fields from legacy (embedded storage.Mute)
	Actor             string    `json:"actor"`              // The actor doing the muting
	Object            string    `json:"object"`             // The actor being muted
	ID                string    `json:"id"`                 // The mute activity ID
	HideNotifications bool      `json:"hide_notifications"` // Whether to hide notifications from this user
	Published         time.Time `json:"published"`          // When the mute was created
	CreatedAt         time.Time `json:"created_at"`         // Database timestamp
}

// TableName returns the DynamoDB table name
func (Mute) TableName() string {
	return "lesser-main"
}

// BeforeCreate prepares the Mute for creation
func (m *Mute) BeforeCreate() error {
	// Set timestamps if not already set
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.Published.IsZero() {
		m.Published = time.Now()
	}
	
	// Update keys based on actor usernames
	m.UpdateKeys()
	
	return nil
}

// UpdateKeys sets the primary and GSI keys based on the actor usernames
func (m *Mute) UpdateKeys() {
	// Primary keys
	m.PK = fmt.Sprintf("MUTE#%s", m.Actor)
	m.SK = fmt.Sprintf("MUTED#%s", m.Object)
	
	// GSI1 for reverse lookup
	m.GSI1PK = fmt.Sprintf("MUTED#%s", m.Object)
	m.GSI1SK = fmt.Sprintf("MUTER#%s", m.Actor)
}