package models

import (
	"fmt"
	"time"
)

// DNSCache represents a cached DNS lookup result in DynamoDB
type DNSCache struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys - EXACT pattern from legacy: PK=DNSCACHE#hostname, SK=ENTRY
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Business fields matching legacy implementation
	Hostname   string    `theorydb:"attr:hostname" json:"hostname"`
	IPs        []string  `theorydb:"attr:ips" json:"ips"`
	ResolvedAt time.Time `theorydb:"attr:resolvedAt" json:"resolved_at"`
	TTL        int       `theorydb:"attr:ttl" json:"ttl"`            // seconds
	ExpiresAt  int64     `theorydb:"ttl,attr:ttl" json:"expires_at"` // Unix timestamp for DynamoDB TTL
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
