package models

import (
	"fmt"
	"github.com/equaltoai/lesser/pkg/common"
	"time"
)

// Block represents a block relationship between actors
type Block struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // ACTOR#{blocker_username}#BLOCKS
	SK string `dynamorm:"sk" json:"SK"` // BLOCKED#{blocked_username}

	// GSI5 for reverse lookup
	GSI5PK string `dynamorm:"index:GSI5,pk" json:"GSI5PK"` // BLOCKED#{blocked_username}
	GSI5SK string `dynamorm:"index:GSI5,sk" json:"GSI5SK"` // BLOCKER#{blocker_username}

	// Core fields from legacy
	Type      string    `json:"Type"`      // Always "Block"
	Actor     string    `json:"Actor"`     // Full actor ID who is blocking
	Object    string    `json:"Object"`    // Full actor ID being blocked
	ID        string    `json:"ID"`        // Block activity ID
	Published time.Time `json:"Published"` // When the block was published
	CreatedAt time.Time `json:"CreatedAt"` // When stored in DB
}

// TableName returns the DynamoDB table name
func (Block) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the Block for creation
func (b *Block) BeforeCreate() error {
	// Set type
	b.Type = "Block"

	// Set timestamps if not already set
	if b.Published.IsZero() {
		b.Published = time.Now().UTC()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("b.ID", b.ID); err != nil {
		b.ID = fmt.Sprintf("%s/activities/block-%d", b.Actor, time.Now().Unix())
	}

	// Update keys based on actor usernames
	if err := b.UpdateKeys(); err != nil {
		return fmt.Errorf("%w: %w", ErrBlockUpdateKeysFailed, err)
	}

	return nil
}

// UpdateKeys sets the primary and GSI keys based on the actor usernames
// This implements the BaseModel interface requirement
func (b *Block) UpdateKeys() error {
	// Extract usernames from actor IDs
	blockerUsername := extractUsername(b.Actor)
	blockedUsername := extractUsername(b.Object)

	// Primary keys
	b.PK = fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)
	b.SK = fmt.Sprintf("BLOCKED#%s", blockedUsername)

	// GSI5 for reverse lookup
	b.GSI5PK = fmt.Sprintf("BLOCKED#%s", blockedUsername)
	b.GSI5SK = fmt.Sprintf("BLOCKER#%s", blockerUsername)

	return nil
}

// GetPK returns the partition key (implements BaseModel interface)
func (b *Block) GetPK() string {
	return b.PK
}

// GetSK returns the sort key (implements BaseModel interface)
func (b *Block) GetSK() string {
	return b.SK
}

// extractUsername extracts the username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsername(actorID string) string {
	// Split by forward slashes
	parts := []string{}
	current := ""
	for _, char := range actorID {
		if char == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	// Return the last part
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return actorID
}
