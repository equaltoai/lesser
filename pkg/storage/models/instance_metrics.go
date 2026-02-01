package models

import (
	"fmt"
	"time"
)

// InstanceMetrics tracks platform-wide metrics and statistics
type InstanceMetrics struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Key fields - EXACT pattern from legacy: PK=`INSTANCE_METRICS#date`, SK=`METRIC#type`
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// GSI fields for time-based queries
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"`

	// Business fields from legacy
	Date       string    `theorydb:"attr:date" json:"date"`              // YYYY-MM-DD format
	MetricType string    `theorydb:"attr:metricType" json:"metric_type"` // total_users, active_users_daily, etc.
	Value      int64     `theorydb:"attr:value" json:"value"`
	Delta      int64     `theorydb:"attr:delta" json:"delta"` // change from previous period
	UpdatedAt  time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// Additional metric types from legacy
	TotalUsers         int64 `theorydb:"attr:totalUsers" json:"total_users,omitempty"`
	ActiveUsersDaily   int64 `theorydb:"attr:activeUsersDaily" json:"active_users_daily,omitempty"`
	ActiveUsersWeekly  int64 `theorydb:"attr:activeUsersWeekly" json:"active_users_weekly,omitempty"`
	TotalStatuses      int64 `theorydb:"attr:totalStatuses" json:"total_statuses,omitempty"`
	TotalMedia         int64 `theorydb:"attr:totalMedia" json:"total_media,omitempty"`
	FederationInbound  int64 `theorydb:"attr:federationInbound" json:"federation_inbound,omitempty"`
	FederationOutbound int64 `theorydb:"attr:federationOutbound" json:"federation_outbound,omitempty"`

	// Weekly activity tracking
	Week          int64 `theorydb:"attr:week" json:"week,omitempty"`                   // Unix timestamp of week start
	Statuses      int32 `theorydb:"attr:statuses" json:"statuses,omitempty"`           // Status count for the week
	Logins        int32 `theorydb:"attr:logins" json:"logins,omitempty"`               // Login count for the week
	Registrations int32 `theorydb:"attr:registrations" json:"registrations,omitempty"` // Registration count for the week

	// TTL field
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
