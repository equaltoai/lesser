package models

import "time"

// ModerationAnalytics tracks moderation actions and statistics
type ModerationAnalytics struct {
	// Key fields - Pattern from legacy: PK=`MOD_ANALYTICS#date`, SK=`type#count`
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// GSI fields for querying by type
	GSI2PK string `dynamorm:"index:GSI2,pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk"`

	// Business fields from legacy
	Date                  string           `json:"date"` // YYYY-MM-DD format
	ReportType            string           `json:"report_type"`
	Count                 int64            `json:"count"`
	ResolvedCount         int64            `json:"resolved_count"`
	AverageResolutionTime float64          `json:"average_resolution_time"` // in hours
	ModeratorActions      map[string]int64 `json:"moderator_actions"`       // moderator -> action count
	UpdatedAt             time.Time        `json:"updated_at"`

	// Additional fields for pattern analytics (keeping compatibility)
	PatternID string    `json:"pattern_id,omitempty"`
	Matched   bool      `json:"matched,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	// TTL field - 90 days as per legacy
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table name
func (m *ModerationAnalytics) TableName() string {
	return DefaultTableName // Replace with actual table name
}

// UpdateKeys updates the primary and sort keys for DynamoDB
func (m *ModerationAnalytics) UpdateKeys() {
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
}
