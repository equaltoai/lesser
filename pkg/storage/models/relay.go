package models

import (
	"fmt"
	"time"
)

// Relay represents a relay server for federation
type Relay struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI fields for querying active relays
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1sk,omitempty"`

	// GSI fields for querying by domain
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2pk,omitempty"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2sk,omitempty"`

	// Business fields matching storage.RelayInfo
	URL        string    `json:"url"`
	InboxURL   string    `json:"inbox_url"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Domain     string    `json:"domain,omitempty"`
	Status     string    `json:"status,omitempty"` // pending/active/rejected/error
	ErrorCount int       `json:"error_count,omitempty"`
	TTL        int64     `json:"ttl,omitempty" dynamorm:"ttl"` // For automatic cleanup
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
