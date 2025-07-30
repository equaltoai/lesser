package models

import (
	"fmt"
	"time"
)

// Move represents an account move/migration activity
type Move struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // MOVE#ACTOR#{actor}
	SK string `dynamorm:"sk" json:"SK"` // TARGET#{target}
	
	// GSI1 for reverse lookups (moves to a target)
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK"` // MOVE#TARGET#{target}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK"` // ACTOR#{actor}
	
	// Move data (nested in legacy)
	ID        string    `json:"ID"`        // The move activity ID
	Actor     string    `json:"Actor"`     // The old account moving
	Target    string    `json:"Target"`    // The new account location
	Published time.Time `json:"Published"` // When the move was announced
	CreatedAt time.Time `json:"CreatedAt"` // Database timestamp
	
	// Optional TTL for cleanup
	TTL *int64 `dynamorm:"ttl" json:"TTL,omitempty"`
}

// TableName returns the DynamoDB table name
func (Move) TableName() string {
	return "lesser-main"
}

// BeforeCreate sets up the record before creation
func (m *Move) BeforeCreate() error {
	m.CreatedAt = time.Now()
	m.UpdateKeys()
	return nil
}

// UpdateKeys updates GSI keys based on actor and target
func (m *Move) UpdateKeys() {
	m.PK = fmt.Sprintf("MOVE#ACTOR#%s", m.Actor)
	m.SK = fmt.Sprintf("TARGET#%s", m.Target)
	m.GSI1PK = fmt.Sprintf("MOVE#TARGET#%s", m.Target)
	m.GSI1SK = fmt.Sprintf("ACTOR#%s", m.Actor)
}

// NewMove creates a new move record
func NewMove(id, actor, target string) *Move {
	now := time.Now()
	move := &Move{
		ID:        id,
		Actor:     actor,
		Target:    target,
		Published: now,
		CreatedAt: now,
	}
	move.UpdateKeys()
	return move
}

// SetTTL sets the TTL for the move record (in Unix epoch seconds)
func (m *Move) SetTTL(ttl time.Time) {
	ttlUnix := ttl.Unix()
	m.TTL = &ttlUnix
}

// ExtractActor extracts the actor from PK
func (m *Move) ExtractActor() string {
	prefix := "MOVE#ACTOR#"
	if len(m.PK) > len(prefix) {
		return m.PK[len(prefix):]
	}
	return ""
}

// ExtractTarget extracts the target from SK
func (m *Move) ExtractTarget() string {
	prefix := "TARGET#"
	if len(m.SK) > len(prefix) {
		return m.SK[len(prefix):]
	}
	return ""
}