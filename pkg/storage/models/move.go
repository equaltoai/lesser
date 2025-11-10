package models

import (
	"fmt"
	"time"
)

// Move represents an account move/migration activity
type Move struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"PK"` // MOVE#ACTOR#{actor}
	SK string `dynamorm:"sk,attr:SK" json:"SK"` // TARGET#{target}

	// GSI1 for reverse lookups (moves to a target)
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"GSI1PK"` // MOVE#TARGET#{target}
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"GSI1SK"` // ACTOR#{actor}

	// Move data (nested in legacy)
	ID        string    `dynamorm:"attr:id" json:"ID"`               // The move activity ID
	Actor     string    `dynamorm:"attr:actor" json:"Actor"`         // The old account moving
	Target    string    `dynamorm:"attr:target" json:"Target"`       // The new account location
	Published time.Time `dynamorm:"attr:published" json:"Published"` // When the move was announced
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"CreatedAt"` // Database timestamp

	// Optional TTL for cleanup
	TTL *int64 `dynamorm:"ttl,attr:ttl" json:"TTL,omitempty"`
}

// TableName returns the DynamoDB table name
func (Move) TableName() string {
	return MainTableName
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
	m.GSI1SK = fmt.Sprintf(KeyPatternActor, m.Actor)
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
