package models

import (
	"fmt"
	"time"
)

// UpdateHistory represents the edit history of an object
type UpdateHistory struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary Key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // OBJECT#<objectID>#HISTORY
	SK string `theorydb:"sk,attr:SK" json:"-"` // VERSION#00001 (padded for sorting)

	// Business fields matching storage.UpdateHistory
	ObjectID      string    `theorydb:"attr:objectID" json:"objectId"`           // The object that was updated
	Version       int       `theorydb:"attr:version" json:"version"`             // Version number (1 is original)
	UpdatedAt     time.Time `theorydb:"attr:updatedAt" json:"updatedAt"`         // When the update occurred
	UpdatedBy     string    `theorydb:"attr:updatedBy" json:"updatedBy"`         // Actor who made the update
	PreviousState string    `theorydb:"attr:previousState" json:"previousState"` // JSON of previous state
	Summary       string    `theorydb:"attr:summary" json:"summary,omitempty"`   // Edit summary

	// Metadata
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"-"`         // When this history record was created
	TTL       int64     `theorydb:"ttl,attr:ttl,omitempty" json:"-"` // Optional TTL for automatic cleanup
}

// UpdateKeys updates the primary key fields based on the business fields
func (h *UpdateHistory) UpdateKeys() {
	if h.ObjectID != "" {
		h.PK = fmt.Sprintf("OBJECT#%s#HISTORY", h.ObjectID)
		// Pad version number to 5 digits for proper sorting
		h.SK = fmt.Sprintf("VERSION#%05d", h.Version)
	}
}

// TableName returns the DynamoDB table backing UpdateHistory.
func (UpdateHistory) TableName() string {
	return MainTableName
}
