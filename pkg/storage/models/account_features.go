// Package models provides DynamORM data models for account features and relationship management.
package models

import (
	"fmt"
	"time"
)

// AccountPin represents a pinned/endorsed account in DynamoDB
// Key pattern: PK=ACCOUNT_PIN#{username}, SK=PIN#{pinned_actor_id}
type AccountPin struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key components
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "ACCOUNT_PIN#{username}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "PIN#{pinned_actor_id}"

	// Pin data - matches storage.AccountPin fields
	Username       string    `theorydb:"attr:username" json:"username"`              // Who pinned the account
	PinnedActorID  string    `theorydb:"attr:pinnedActorID" json:"pinned_actor_id"`  // The actor ID that was pinned
	PinnedUsername string    `theorydb:"attr:pinnedUsername" json:"pinned_username"` // The username that was pinned
	CreatedAt      time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table name
func (AccountPin) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the AccountPin for creation
func (p *AccountPin) BeforeCreate() error {
	// Set timestamp if not already set
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}

	// Update keys
	if err := p.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return nil
}

// UpdateKeys updates the primary key fields based on the current data
func (p *AccountPin) UpdateKeys() error {
	p.PK = fmt.Sprintf("ACCOUNT_PIN#%s", p.Username)
	p.SK = fmt.Sprintf("PIN#%s", p.PinnedActorID)
	return nil
}

// GetPK returns the partition key (implements BaseModel interface)
func (p *AccountPin) GetPK() string {
	return p.PK
}

// GetSK returns the sort key (implements BaseModel interface)
func (p *AccountPin) GetSK() string {
	return p.SK
}

// AccountNote represents a private note on an account in DynamoDB
// Key pattern: PK=ACCOUNT_NOTE#{username}, SK=NOTE#{target_actor_id}
type AccountNote struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key components
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "ACCOUNT_NOTE#{username}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "NOTE#{target_actor_id}"

	// Note data - matches storage.AccountNote fields
	Username      string    `theorydb:"attr:username" json:"username"`             // Who wrote the note
	TargetActorID string    `theorydb:"attr:targetActorID" json:"target_actor_id"` // The actor the note is about
	Note          string    `theorydb:"attr:note" json:"note"`                     // The note content
	CreatedAt     time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt     time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (AccountNote) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the AccountNote for creation
func (n *AccountNote) BeforeCreate() error {
	// Set timestamps if not already set
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = n.CreatedAt
	}

	// Update keys
	if err := n.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return nil
}

// UpdateKeys updates the primary key fields based on the current data
func (n *AccountNote) UpdateKeys() error {
	n.PK = fmt.Sprintf("ACCOUNT_NOTE#%s", n.Username)
	n.SK = fmt.Sprintf(KeyPatternNote, n.TargetActorID)
	return nil
}

// GetPK returns the partition key (implements BaseModel interface)
func (n *AccountNote) GetPK() string {
	return n.PK
}

// GetSK returns the sort key (implements BaseModel interface)
func (n *AccountNote) GetSK() string {
	return n.SK
}
