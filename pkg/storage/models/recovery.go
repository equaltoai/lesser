package models

import (
	"fmt"
	"time"
)

// Trustee represents a trusted contact for social recovery
type Trustee struct {
	// Primary keys
	PK string `dynamorm:"pk"` // USER#username
	SK string `dynamorm:"sk"` // TRUSTEE#actorID

	// Business fields
	Username  string    `json:"username"` // Who owns this trustee relationship
	ActorID   string    `json:"actor_id"` // @friend@mastodon.social
	AddedAt   time.Time `json:"added_at"`
	Confirmed bool      `json:"confirmed"`
}

// UpdateKeys updates the primary and GSI keys based on the model's business fields
func (t *Trustee) UpdateKeys() {
	t.PK = fmt.Sprintf(KeyPatternUser, t.Username)
	t.SK = fmt.Sprintf("TRUSTEE#%s", t.ActorID)
}

// RecoveryRequest represents an active social recovery request
type RecoveryRequest struct {
	// Primary keys
	PK string `dynamorm:"pk"` // RECOVERY#id
	SK string `dynamorm:"sk"` // REQUEST

	// GSI1 for querying by username
	GSI1PK string `dynamorm:"index:GSI1,pk"` // USER#username
	GSI1SK string `dynamorm:"index:GSI1,sk"` // RECOVERY#timestamp

	// Business fields
	ID            string          `json:"id"`
	Username      string          `json:"username"`
	InitiatedAt   time.Time       `json:"initiated_at"`
	ExpiresAt     time.Time       `json:"expires_at"`
	RequiredVotes int             `json:"required_votes"`
	ReceivedVotes map[string]bool `json:"received_votes"` // trustee_id -> voted
	RecoveryToken string          `json:"recovery_token"`
	Status        string          `json:"status"` // pending, approved, expired, cancelled

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the primary and GSI keys based on the model's business fields
func (r *RecoveryRequest) UpdateKeys() {
	r.PK = fmt.Sprintf("RECOVERY#%s", r.ID)
	r.SK = "REQUEST"
	r.GSI1PK = fmt.Sprintf(KeyPatternUser, r.Username)
	r.GSI1SK = fmt.Sprintf("RECOVERY#%s", r.InitiatedAt.Format(time.RFC3339))
	if !r.ExpiresAt.IsZero() {
		r.TTL = r.ExpiresAt.Unix()
	}
}

// RecoveryCode represents a single recovery code
type RecoveryCode struct {
	// Primary keys
	PK string `dynamorm:"pk"` // USER#username
	SK string `dynamorm:"sk"` // RECOVERY_CODE#position

	// Business fields
	Username  string     `json:"username"`
	CodeHash  string     `json:"code_hash"` // bcrypt hash of the code
	CreatedAt time.Time  `json:"created_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	Position  int        `json:"position"` // Position in the list (0-7 typically)
}

// UpdateKeys updates the primary and GSI keys based on the model's business fields
func (c *RecoveryCode) UpdateKeys() {
	c.PK = fmt.Sprintf(KeyPatternUser, c.Username)
	c.SK = fmt.Sprintf("RECOVERY_CODE#%d", c.Position)
}

// RecoveryToken represents a generic recovery token with custom data
type RecoveryToken struct {
	// Primary keys
	PK string `dynamorm:"pk"` // The key parameter directly
	SK string `dynamorm:"sk"` // TOKEN

	// Business fields
	Data      map[string]any `json:"data"`
	CreatedAt time.Time      `json:"created_at"`

	// TTL for automatic cleanup (24 hours)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the primary keys and TTL based on the model's business fields
func (t *RecoveryToken) UpdateKeys() {
	// PK is set directly from the key parameter
	t.SK = SKToken
	// Set TTL to 24 hours from creation
	if !t.CreatedAt.IsZero() {
		t.TTL = t.CreatedAt.Add(24 * time.Hour).Unix()
	}
}
