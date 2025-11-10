package models

import (
	"fmt"
	"time"
)

// InstanceMetrics tracks platform-wide metrics and statistics
type InstanceMetrics struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Key fields - EXACT pattern from legacy: PK=`INSTANCE_METRICS#date`, SK=`METRIC#type`
	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	// GSI fields for time-based queries
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK"`
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK"`

	// Business fields from legacy
	Date       string    `dynamorm:"attr:date" json:"date"`               // YYYY-MM-DD format
	MetricType string    `dynamorm:"attr:metricType" json:"metric_type"`  // total_users, active_users_daily, etc.
	Value      int64     `dynamorm:"attr:value" json:"value"`
	Delta      int64     `dynamorm:"attr:delta" json:"delta"` // change from previous period
	UpdatedAt  time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// Additional metric types from legacy
	TotalUsers         int64 `dynamorm:"attr:totalUsers" json:"total_users,omitempty"`
	ActiveUsersDaily   int64 `dynamorm:"attr:activeUsersDaily" json:"active_users_daily,omitempty"`
	ActiveUsersWeekly  int64 `dynamorm:"attr:activeUsersWeekly" json:"active_users_weekly,omitempty"`
	TotalStatuses      int64 `dynamorm:"attr:totalStatuses" json:"total_statuses,omitempty"`
	TotalMedia         int64 `dynamorm:"attr:totalMedia" json:"total_media,omitempty"`
	FederationInbound  int64 `dynamorm:"attr:federationInbound" json:"federation_inbound,omitempty"`
	FederationOutbound int64 `dynamorm:"attr:federationOutbound" json:"federation_outbound,omitempty"`

	// Weekly activity tracking
	Week          int64 `dynamorm:"attr:week" json:"week,omitempty"`                   // Unix timestamp of week start
	Statuses      int32 `dynamorm:"attr:statuses" json:"statuses,omitempty"`           // Status count for the week
	Logins        int32 `dynamorm:"attr:logins" json:"logins,omitempty"`               // Login count for the week
	Registrations int32 `dynamorm:"attr:registrations" json:"registrations,omitempty"` // Registration count for the week

	// TTL field
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceMetrics.
func (i *InstanceMetrics) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys when the primary keys change
func (i *InstanceMetrics) UpdateKeys() error {
	// Validate required fields
	if i.Date == "" {
		return fmt.Errorf("date is required")
	}
	if i.MetricType == "" {
		return fmt.Errorf("metric type is required")
	}

	// Set primary keys
	i.PK = fmt.Sprintf("INSTANCE_METRICS#%s", i.Date)
	i.SK = fmt.Sprintf("METRIC#%s", i.MetricType)

	// GSI1 is used for time-based queries
	i.GSI1PK = "INSTANCE_METRICS"
	i.GSI1SK = i.Date
	return nil
}

// GetPK returns the partition key
func (i *InstanceMetrics) GetPK() string {
	return i.PK
}

// GetSK returns the sort key
func (i *InstanceMetrics) GetSK() string {
	return i.SK
}
