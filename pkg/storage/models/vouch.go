package models

import (
	"fmt"
	"time"
)

// Vouch represents a reputation vouch between actors
type Vouch struct {
	// Primary keys
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	// GSI1 for vouches given by an actor
	GSI1PK string `dynamorm:"index:gsi1-index,pk"`
	GSI1SK string `dynamorm:"index:gsi1-index,sk"`

	// GSI2 for vouches received by an actor
	GSI2PK string `dynamorm:"index:gsi2-index,pk"`
	GSI2SK string `dynamorm:"index:gsi2-index,sk"`

	// Data fields
	VouchData string    `json:"vouch_data"`              // JSON encoded vouch
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt int64     `json:"ttl" dynamorm:"ttl"`      // Unix timestamp for TTL
}

// UpdateKeys sets all the DynamoDB keys based on the vouch data
func (v *Vouch) UpdateKeys(vouchID, fromActorID, toActorID string, active bool, createdAt, expiresAt time.Time) {
	// Set primary keys
	v.PK = fmt.Sprintf("VOUCH#%s", vouchID)
	v.SK = "METADATA"

	// Set GSI1 keys (vouches given by an actor)
	v.GSI1PK = fmt.Sprintf("VOUCHER#%s", fromActorID)
	v.GSI1SK = fmt.Sprintf("TO#%s", toActorID)

	// Set GSI2 keys (vouches received by an actor) 
	v.GSI2PK = fmt.Sprintf("VOUCHEE#%s", toActorID)
	v.GSI2SK = fmt.Sprintf("FROM#%s", fromActorID)

	// Set other fields
	v.Active = active
	v.CreatedAt = createdAt
	v.ExpiresAt = expiresAt.Unix()
}