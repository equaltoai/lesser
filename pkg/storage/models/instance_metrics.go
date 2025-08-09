package models

import (
	"time"
)

// InstanceMetrics tracks platform-wide metrics and statistics
type InstanceMetrics struct {
	// Key fields - EXACT pattern from legacy: PK=`INSTANCE_METRICS#date`, SK=`METRIC#type`
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	// GSI fields for time-based queries
	GSI1PK string `dynamorm:"index:GSI1,pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk"`

	// Business fields from legacy
	Date       string    `json:"date"`        // YYYY-MM-DD format
	MetricType string    `json:"metric_type"` // total_users, active_users_daily, etc.
	Value      int64     `json:"value"`
	Delta      int64     `json:"delta"` // change from previous period
	UpdatedAt  time.Time `json:"updated_at"`

	// Additional metric types from legacy
	TotalUsers         int64 `json:"total_users,omitempty"`
	ActiveUsersDaily   int64 `json:"active_users_daily,omitempty"`
	ActiveUsersWeekly  int64 `json:"active_users_weekly,omitempty"`
	TotalStatuses      int64 `json:"total_statuses,omitempty"`
	TotalMedia         int64 `json:"total_media,omitempty"`
	FederationInbound  int64 `json:"federation_inbound,omitempty"`
	FederationOutbound int64 `json:"federation_outbound,omitempty"`

	// Weekly activity tracking
	Week          int64 `json:"week,omitempty"`          // Unix timestamp of week start
	Statuses      int32 `json:"statuses,omitempty"`      // Status count for the week
	Logins        int32 `json:"logins,omitempty"`        // Login count for the week
	Registrations int32 `json:"registrations,omitempty"` // Registration count for the week

	// TTL field
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table name
func (i *InstanceMetrics) TableName() string {
	return DefaultTableName // Replace with actual table name
}

// UpdateKeys updates the GSI keys when the primary keys change
func (i *InstanceMetrics) UpdateKeys() {
	// GSI1 is used for time-based queries
	i.GSI1PK = "INSTANCE_METRICS"
	i.GSI1SK = i.Date
}
