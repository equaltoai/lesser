package models

import (
	"fmt"
	"time"
)

// Relay represents a relay server for federation
type Relay struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// GSI fields for querying active relays
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1pk,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1sk,omitempty"`

	// GSI fields for querying by domain
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2pk,omitempty"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2sk,omitempty"`

	// GSI8 fields for listing all relays
	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty" json:"gsi8pk,omitempty"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty" json:"gsi8sk,omitempty"`

	// Business fields matching storage.RelayInfo
	URL        string    `theorydb:"attr:url" json:"url"`
	InboxURL   string    `theorydb:"attr:inboxURL" json:"inbox_url"`
	Active     bool      `theorydb:"attr:active" json:"active"`
	CreatedAt  time.Time `theorydb:"attr:createdAt" json:"created_at"`
	LastSeenAt time.Time `theorydb:"attr:lastSeenAt" json:"last_seen_at"`
	Domain     string    `theorydb:"attr:domain" json:"domain,omitempty"`
	Status     string    `theorydb:"attr:status" json:"status,omitempty"` // pending/active/rejected/error
	ErrorCount int       `theorydb:"attr:errorCount" json:"error_count,omitempty"`
	TTL        int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"` // For automatic cleanup
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
	} else {
		r.GSI2PK = ""
		r.GSI2SK = ""
	}

	// GSI8: For listing all relays (admin/debug)
	r.GSI8PK = "RELAYS"
	r.GSI8SK = fmt.Sprintf("URL#%s", r.URL)

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
