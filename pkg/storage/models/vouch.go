package models

import (
	"fmt"
	"time"
)

// Vouch represents a reputation vouch between actors
type Vouch struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys
	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	// GSI1 for vouches given by an actor
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`

	// GSI2 for vouches received by an actor
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK"`
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK"`

	// Data fields
	VouchData string    `dynamorm:"attr:vouchData" json:"vouch_data"` // JSON encoded vouch
	Active    bool      `dynamorm:"attr:active" json:"active"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	ExpiresAt int64     `dynamorm:"ttl,attr:ttl" json:"ttl"` // Unix timestamp for TTL
}

// UpdateKeys sets all the DynamoDB keys based on the vouch data
func (v *Vouch) UpdateKeys(vouchID, fromActorID, toActorID string, active bool, createdAt, expiresAt time.Time) {
	// Set primary keys
	v.PK = fmt.Sprintf("VOUCH#%s", vouchID)
	v.SK = SKMetadata

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

// TableName returns the DynamoDB table backing Vouch.
func (Vouch) TableName() string {
	return MainTableName
}
