package models

import (
	"fmt"
	"time"
)

// AccountPin represents a pinned/endorsed account in DynamoDB
// Key pattern: PK=ACCOUNT_PIN#{username}, SK=PIN#{pinned_actor_id}
type AccountPin struct {
	// Primary key components
	PK string `dynamorm:"pk" json:"pk"` // Format: "ACCOUNT_PIN#{username}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "PIN#{pinned_actor_id}"

	// Pin data - matches storage.AccountPin fields
	Username       string    `json:"username"`        // Who pinned the account
	PinnedActorID  string    `json:"pinned_actor_id"` // The actor ID that was pinned
	PinnedUsername string    `json:"pinned_username"` // The username that was pinned
	CreatedAt      time.Time `json:"created_at"`
}

// TableName returns the DynamoDB table name
func (AccountPin) TableName() string {
	return "lesser-main"
}

// BeforeCreate prepares the AccountPin for creation
func (p *AccountPin) BeforeCreate() error {
	// Set timestamp if not already set
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	
	// Update keys
	p.UpdateKeys()
	
	return nil
}

// UpdateKeys updates the primary key fields based on the current data
func (p *AccountPin) UpdateKeys() {
	p.PK = fmt.Sprintf("ACCOUNT_PIN#%s", p.Username)
	p.SK = fmt.Sprintf("PIN#%s", p.PinnedActorID)
}

// AccountNote represents a private note on an account in DynamoDB
// Key pattern: PK=ACCOUNT_NOTE#{username}, SK=NOTE#{target_actor_id}
type AccountNote struct {
	// Primary key components
	PK string `dynamorm:"pk" json:"pk"` // Format: "ACCOUNT_NOTE#{username}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "NOTE#{target_actor_id}"

	// Note data - matches storage.AccountNote fields
	Username      string    `json:"username"`        // Who wrote the note
	TargetActorID string    `json:"target_actor_id"` // The actor the note is about
	Note          string    `json:"note"`            // The note content
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (AccountNote) TableName() string {
	return "lesser-main"
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
	n.UpdateKeys()
	
	return nil
}

// UpdateKeys updates the primary key fields based on the current data
func (n *AccountNote) UpdateKeys() {
	n.PK = fmt.Sprintf("ACCOUNT_NOTE#%s", n.Username)
	n.SK = fmt.Sprintf("NOTE#%s", n.TargetActorID)
}