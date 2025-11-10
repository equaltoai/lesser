package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Announce represents a reblog/boost activity
type Announce struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"PK"` // OBJECT#{object_id}#ANNOUNCES
	SK string `dynamorm:"sk,attr:SK" json:"SK"` // ACTOR#{actor_id}

	// GSI4 for actor lookups
	GSI4PK string `dynamorm:"index:GSI4,pk,attr:gsI4PK" json:"GSI4PK"` // ACTOR#{actor_id}#ANNOUNCES
	GSI4SK string `dynamorm:"index:GSI4,sk,attr:gsI4SK" json:"GSI4SK"` // PUBLISHED#{timestamp}#OBJECT#{object_id}

	// Core fields from legacy (embedded storage.Announce)
	Actor     string    `dynamorm:"attr:actor" json:"actor"`          // Who announced
	Object    string    `dynamorm:"attr:object" json:"object"`        // What was announced
	ID        string    `dynamorm:"attr:id" json:"id"`                // Announce activity ID
	Published time.Time `dynamorm:"attr:published" json:"published"`  // When it was announced
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"` // When stored in DB
	To        []string  `dynamorm:"attr:to" json:"to,omitempty"`      // Audience
	CC        []string  `dynamorm:"attr:cc" json:"cc,omitempty"`      // CC audience
}

// TableName returns the DynamoDB table name
func (Announce) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the Announce for creation
func (a *Announce) BeforeCreate() error {
	// Generate activity ID if not provided
	if err := common.ValidateRequiredParam("a.ID", a.ID); err != nil {
		a.ID = fmt.Sprintf("%s/activities/announce-%d-%s",
			a.Actor,
			time.Now().Unix(),
			generateRandomID(8))
	}

	// Set timestamps if not already set
	if a.Published.IsZero() {
		a.Published = time.Now()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}

	// Update keys
	if err := a.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return nil
}

// UpdateKeys sets the primary and GSI keys
func (a *Announce) UpdateKeys() error {
	// Primary keys
	a.PK = fmt.Sprintf("OBJECT#%s#ANNOUNCES", a.Object)
	a.SK = fmt.Sprintf(KeyPatternActor, a.Actor)

	// GSI4 for actor's announces
	a.GSI4PK = fmt.Sprintf("ACTOR#%s#ANNOUNCES", a.Actor)
	a.GSI4SK = fmt.Sprintf("PUBLISHED#%s#OBJECT#%s", a.Published.Format(time.RFC3339), a.Object)
	return nil
}

// GetPK returns the primary key for BaseRepository interface
func (a *Announce) GetPK() string {
	return a.PK
}

// GetSK returns the sort key for BaseRepository interface
func (a *Announce) GetSK() string {
	return a.SK
}

// generateRandomID generates a random hex string of specified length
func generateRandomID(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
