package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Announce represents a reblog/boost activity
type Announce struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // OBJECT#{object_id}#ANNOUNCES
	SK string `dynamorm:"sk" json:"SK"` // ACTOR#{actor_id}

	// GSI4 for actor lookups
	GSI4PK string `dynamorm:"index:GSI4,pk" json:"GSI4PK"` // ACTOR#{actor_id}#ANNOUNCES
	GSI4SK string `dynamorm:"index:GSI4,sk" json:"GSI4SK"` // PUBLISHED#{timestamp}#OBJECT#{object_id}

	// Core fields from legacy (embedded storage.Announce)
	Actor     string    `json:"actor"`        // Who announced
	Object    string    `json:"object"`       // What was announced
	ID        string    `json:"id"`           // Announce activity ID
	Published time.Time `json:"published"`    // When it was announced
	CreatedAt time.Time `json:"created_at"`   // When stored in DB
	To        []string  `json:"to,omitempty"` // Audience
	CC        []string  `json:"cc,omitempty"` // CC audience
}

// TableName returns the DynamoDB table name
func (Announce) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the Announce for creation
func (a *Announce) BeforeCreate() error {
	// Generate activity ID if not provided
	if a.ID == "" {
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
	a.UpdateKeys()

	return nil
}

// UpdateKeys sets the primary and GSI keys
func (a *Announce) UpdateKeys() {
	// Primary keys
	a.PK = fmt.Sprintf("OBJECT#%s#ANNOUNCES", a.Object)
	a.SK = fmt.Sprintf(KeyPatternActor, a.Actor)

	// GSI4 for actor's announces
	a.GSI4PK = fmt.Sprintf("ACTOR#%s#ANNOUNCES", a.Actor)
	a.GSI4SK = fmt.Sprintf("PUBLISHED#%s#OBJECT#%s", a.Published.Format(time.RFC3339), a.Object)
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
