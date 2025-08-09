package models

import (
	"fmt"
	"time"
)

// UpdateHistory represents the edit history of an object
type UpdateHistory struct {
	// Primary Key fields
	PK string `dynamorm:"pk" json:"-"` // OBJECT#<objectID>#HISTORY
	SK string `dynamorm:"sk" json:"-"` // VERSION#00001 (padded for sorting)

	// Business fields matching storage.UpdateHistory
	ObjectID      string    `json:"objectId"`          // The object that was updated
	Version       int       `json:"version"`           // Version number (1 is original)
	UpdatedAt     time.Time `json:"updatedAt"`         // When the update occurred
	UpdatedBy     string    `json:"updatedBy"`         // Actor who made the update
	PreviousState string    `json:"previousState"`     // JSON of previous state
	Summary       string    `json:"summary,omitempty"` // Edit summary

	// Metadata
	CreatedAt time.Time `json:"-"`                          // When this history record was created
	TTL       int64     `json:"-" dynamorm:"ttl,omitempty"` // Optional TTL for automatic cleanup
}

// UpdateKeys updates the primary key fields based on the business fields
func (h *UpdateHistory) UpdateKeys() {
	if h.ObjectID != "" {
		h.PK = fmt.Sprintf("OBJECT#%s#HISTORY", h.ObjectID)
		// Pad version number to 5 digits for proper sorting
		h.SK = fmt.Sprintf("VERSION#%05d", h.Version)
	}
}
