package models

import "time"

// ModerationAnalytics tracks moderation actions and statistics
type ModerationAnalytics struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Key fields - Pattern from legacy: PK=`MOD_ANALYTICS#date`, SK=`type#count`
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// GSI fields for querying by type
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK"`

	// Business fields from legacy
	Date                  string           `theorydb:"attr:date" json:"date"` // YYYY-MM-DD format
	ReportType            string           `theorydb:"attr:reportType" json:"report_type"`
	Count                 int64            `theorydb:"attr:count" json:"count"`
	ResolvedCount         int64            `theorydb:"attr:resolvedCount" json:"resolved_count"`
	AverageResolutionTime float64          `theorydb:"attr:averageResolutionTime" json:"average_resolution_time"` // in hours
	ModeratorActions      map[string]int64 `theorydb:"attr:moderatorActions" json:"moderator_actions"`            // moderator -> action count
	UpdatedAt             time.Time        `theorydb:"attr:updatedAt" json:"updated_at"`

	// Additional fields for pattern analytics (keeping compatibility)
	PatternID string    `theorydb:"attr:patternID" json:"pattern_id,omitempty"`
	Matched   bool      `theorydb:"attr:matched" json:"matched,omitempty"`
	Timestamp time.Time `theorydb:"attr:timestamp" json:"timestamp,omitempty"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`

	// TTL field - 90 days as per legacy
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
