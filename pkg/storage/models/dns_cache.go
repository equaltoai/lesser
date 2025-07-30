package models

import (
	"fmt"
	"time"
)

// DNSCache represents a cached DNS lookup result in DynamoDB
type DNSCache struct {
	// Keys - EXACT pattern from legacy: PK=DNSCACHE#hostname, SK=ENTRY
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`
	
	// Business fields matching legacy implementation
	Hostname   string    `json:"hostname"`
	IPs        []string  `json:"ips"`
	ResolvedAt time.Time `json:"resolved_at"`
	TTL        int       `json:"ttl"`        // seconds
	ExpiresAt  int64     `json:"expires_at" dynamorm:"ttl"` // Unix timestamp for DynamoDB TTL
}

// UpdateKeys sets the composite key values for DynamoDB
func (d *DNSCache) UpdateKeys() {
	if d.Hostname != "" {
		d.PK = fmt.Sprintf("DNSCACHE#%s", d.Hostname)
		d.SK = "ENTRY"
	}
}

// TableName returns the DynamoDB table name for this model
func (d *DNSCache) TableName() string {
	return "LiftTable" // This will be overridden by the repository
}