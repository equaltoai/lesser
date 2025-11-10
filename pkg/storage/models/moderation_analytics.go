package models

import "time"

// ModerationAnalytics tracks moderation actions and statistics
type ModerationAnalytics struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Key fields - Pattern from legacy: PK=`MOD_ANALYTICS#date`, SK=`type#count`
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// GSI fields for querying by type
	GSI2PK string `dynamorm:"index:GSI2,pk,attr:gsi2PK"`
	GSI2SK string `dynamorm:"index:GSI2,sk,attr:gsi2SK"`

	// Business fields from legacy
	Date                  string           `dynamorm:"attr:date" json:"date"` // YYYY-MM-DD format
	ReportType            string           `dynamorm:"attr:reportType" json:"report_type"`
	Count                 int64            `dynamorm:"attr:count" json:"count"`
	ResolvedCount         int64            `dynamorm:"attr:resolvedCount" json:"resolved_count"`
	AverageResolutionTime float64          `dynamorm:"attr:averageResolutionTime" json:"average_resolution_time"` // in hours
	ModeratorActions      map[string]int64 `dynamorm:"attr:moderatorActions" json:"moderator_actions"`            // moderator -> action count
	UpdatedAt             time.Time        `dynamorm:"attr:updatedAt" json:"updated_at"`

	// Additional fields for pattern analytics (keeping compatibility)
	PatternID string    `dynamorm:"attr:patternID" json:"pattern_id,omitempty"`
	Matched   bool      `dynamorm:"attr:matched" json:"matched,omitempty"`
	Timestamp time.Time `dynamorm:"attr:timestamp" json:"timestamp,omitempty"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`

	// TTL field - 90 days as per legacy
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing ModerationAnalytics.
func (ModerationAnalytics) TableName() string {
	return MainTableName
}

// UpdateKeys updates the primary and sort keys for DynamoDB
func (m *ModerationAnalytics) UpdateKeys() error {
	// Support both patterns
	if m.PatternID != "" {
		// Pattern analytics mode
		m.PK = "PATTERN#" + m.PatternID
		m.SK = "ANALYTICS#" + m.Timestamp.Format(time.RFC3339Nano)
	} else {
		// Moderation analytics mode
		m.PK = "MOD_ANALYTICS#" + m.Date
		m.SK = "type#" + m.ReportType
	}

	// GSI2 for querying by report type
	m.GSI2PK = "MOD_ANALYTICS#" + m.ReportType
	m.GSI2SK = m.Date
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (m *ModerationAnalytics) GetPK() string {
	return m.PK
}

// GetSK returns the sort key for BaseModel interface
func (m *ModerationAnalytics) GetSK() string {
	return m.SK
}
