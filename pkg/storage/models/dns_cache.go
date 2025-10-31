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
	TTL        int       `json:"ttl"`                       // seconds
	ExpiresAt  int64     `json:"expires_at" dynamorm:"ttl"` // Unix timestamp for DynamoDB TTL
}

// UpdateKeys sets the composite key values for DynamoDB
func (d *DNSCache) UpdateKeys() error {
	if d.Hostname != "" {
		d.PK = fmt.Sprintf("DNSCACHE#%s", d.Hostname)
		d.SK = SKEntry
	}
	return nil
}

// GetPK returns the partition key
func (d *DNSCache) GetPK() string {
	return d.PK
}

// GetSK returns the sort key
func (d *DNSCache) GetSK() string {
	return d.SK
}

// TableName returns the DynamoDB table backing DNSCache.
func (DNSCache) TableName() string {
	return MainTableName
}
