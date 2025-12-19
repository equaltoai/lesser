package models

import (
	"fmt"
	"time"
)

// DNSCache represents a cached DNS lookup result in DynamoDB
type DNSCache struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys - EXACT pattern from legacy: PK=DNSCACHE#hostname, SK=ENTRY
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Business fields matching legacy implementation
	Hostname   string    `dynamorm:"attr:hostname" json:"hostname"`
	IPs        []string  `dynamorm:"attr:ips" json:"ips"`
	ResolvedAt time.Time `dynamorm:"attr:resolvedAt" json:"resolved_at"`
	TTL        int       `dynamorm:"attr:ttl" json:"ttl"`            // seconds
	ExpiresAt  int64     `dynamorm:"ttl,attr:ttl" json:"expires_at"` // Unix timestamp for DynamoDB TTL
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
