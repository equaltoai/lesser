package models

import (
	"fmt"
	"time"
)

// Relay represents a relay server for federation
type Relay struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI fields for querying active relays
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsI1PK" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsI1SK" json:"gsi1sk,omitempty"`

	// GSI fields for querying by domain
	GSI2PK string `dynamorm:"index:GSI2,pk,attr:gsI2PK" json:"gsi2pk,omitempty"`
	GSI2SK string `dynamorm:"index:GSI2,sk,attr:gsI2SK" json:"gsi2sk,omitempty"`

	// Business fields matching storage.RelayInfo
	URL        string    `dynamorm:"attr:url" json:"url"`
	InboxURL   string    `dynamorm:"attr:inboxURL" json:"inbox_url"`
	Active     bool      `dynamorm:"attr:active" json:"active"`
	CreatedAt  time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	LastSeenAt time.Time `dynamorm:"attr:lastSeenAt" json:"last_seen_at"`
	Domain     string    `dynamorm:"attr:domain" json:"domain,omitempty"`
	Status     string    `dynamorm:"attr:status" json:"status,omitempty"` // pending/active/rejected/error
	ErrorCount int       `dynamorm:"attr:errorCount" json:"error_count,omitempty"`
	TTL        int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"` // For automatic cleanup
}

// TableName returns the DynamoDB table backing Relay.
func (Relay) TableName() string {
	return MainTableName
}

// UpdateKeys updates the composite keys based on the relay data
func (r *Relay) UpdateKeys() error {
	// Primary key: RELAY#url
	r.PK = fmt.Sprintf("RELAY#%s", r.URL)
	r.SK = SKInfo

	// GSI1: For querying active relays
	if r.Active {
		r.GSI1PK = "ACTIVE_RELAYS"
		r.GSI1SK = r.URL
	} else {
		// Clear GSI1 keys when not active
		r.GSI1PK = ""
		r.GSI1SK = ""
	}

	// GSI2: For querying by domain
	if r.Domain != "" {
		r.GSI2PK = fmt.Sprintf("RELAY_DOMAIN#%s", r.Domain)
		r.GSI2SK = r.URL
	}

	return nil
}

// GetPK returns the partition key
func (r *Relay) GetPK() string {
	return r.PK
}

// GetSK returns the sort key
func (r *Relay) GetSK() string {
	return r.SK
}
