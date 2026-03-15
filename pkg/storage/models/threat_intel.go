package models

import (
	"fmt"
	"time"
)

// ThreatIntel represents threat intelligence data in DynamoDB
type ThreatIntel struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields - EXACT pattern from legacy
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// GSI keys for querying
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"`

	// Threat data fields
	ID          string    `theorydb:"attr:id" json:"id"`
	ThreatType  string    `theorydb:"attr:threatType" json:"threat_type"`
	Severity    string    `theorydb:"attr:severity" json:"severity"`
	Description string    `theorydb:"attr:description" json:"description"`
	Indicators  []string  `theorydb:"attr:indicators" json:"indicators"`
	FirstSeen   time.Time `theorydb:"attr:firstSeen" json:"first_seen"`
	LastSeen    time.Time `theorydb:"attr:lastSeen" json:"last_seen"`
	HitCount    int64     `theorydb:"attr:hitCount" json:"hit_count"`
	Confidence  float64   `theorydb:"attr:confidence" json:"confidence"`

	// Source tracking
	SourceDomain string `theorydb:"attr:sourceDomain" json:"source_domain"`

	// TTL for automatic expiration
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the PK, SK, and GSI keys based on threat data
func (t *ThreatIntel) UpdateKeys() error {
	// Primary key: PK=THREAT#{id}, SK=METADATA (exact legacy pattern)
	t.PK = fmt.Sprintf("THREAT#%s", t.ID)
	t.SK = SKMetadata

	// GSI1 for querying by type: PK=TYPE#{threatType}, SK=THREAT#{id}
	t.GSI1PK = fmt.Sprintf("TYPE#%s", t.ThreatType)
	t.GSI1SK = fmt.Sprintf("THREAT#%s", t.ID)

	// GSI2 for querying by time: PK=THREATS, SK={timestamp}#{id}
	t.GSI2PK = "THREATS"
	t.GSI2SK = fmt.Sprintf("%d#%s", t.LastSeen.Unix(), t.ID)
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (t *ThreatIntel) GetPK() string {
	return t.PK
}

// GetSK returns the sort key for BaseModel interface
func (t *ThreatIntel) GetSK() string {
	return t.SK
}

// TableName returns the DynamoDB table backing ThreatIntel.
func (ThreatIntel) TableName() string {
	return MainTableName
}

// ThreatIndicator represents an indicator mapping for fast lookup
type ThreatIndicator struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// Indicator data
	ThreatID string `theorydb:"attr:threatID" json:"threat_id"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the PK and SK for indicator lookup
func (ti *ThreatIndicator) UpdateKeys(indicator, threatID string) error {
	// Primary key: PK=INDICATOR#{indicator}, SK=THREAT#{threatID}
	ti.PK = fmt.Sprintf("INDICATOR#%s", indicator)
	ti.SK = fmt.Sprintf("THREAT#%s", threatID)
	ti.ThreatID = threatID

	// Set TTL for 30 days (same as legacy)
	ti.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (ti *ThreatIndicator) GetPK() string {
	return ti.PK
}

// GetSK returns the sort key for BaseModel interface
func (ti *ThreatIndicator) GetSK() string {
	return ti.SK
}

// TableName returns the DynamoDB table backing ThreatIndicator.
func (ThreatIndicator) TableName() string {
	return MainTableName
}
