package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// FederationStats represents federation activity statistics
type FederationStats struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // FEDERATION#STATS
	SK string `theorydb:"sk,attr:SK" json:"-"` // {date} (e.g., 2024-01-30)

	// Attributes from interface
	ActiveInstances int   `theorydb:"attr:activeInstances" json:"active_instances"`
	TotalMessages   int64 `theorydb:"attr:totalMessages" json:"total_messages"`
	TotalUsers      int   `theorydb:"attr:totalUsers" json:"total_users"`

	// Additional metadata
	Date      string    `theorydb:"attr:date" json:"date"`             // The date these stats are for
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`  // Last update time
	TTL       int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"` // 90 days retention
}

// TableName returns the DynamoDB table backing FederationStats.
func (FederationStats) TableName() string {
	return MainTableName
}

// UpdateKeys updates the partition and sort keys based on the date
func (f *FederationStats) UpdateKeys() {
	f.PK = FederationStatsPK
	f.SK = f.Date

	// Set TTL to 90 days from the stats date
	if f.Date != "" {
		if t, err := time.Parse(common.DateFormat, f.Date); err == nil {
			f.TTL = t.AddDate(0, 3, 0).Unix() // 3 months retention
		}
	}
}

// NewFederationStats creates a new FederationStats for a given date
func NewFederationStats(date string) *FederationStats {
	stats := &FederationStats{
		Date:      date,
		UpdatedAt: time.Now().UTC(),
	}
	stats.UpdateKeys()
	return stats
}

// GetFederationStatsKey returns the key for retrieving stats for a specific date
func GetFederationStatsKey(date string) (pk, sk string) {
	return "FEDERATION#STATS", date
}

// GetFederationStatsRangeKeys returns keys for querying a date range
func GetFederationStatsRangeKeys(startDate, endDate string) (pk, skStart, skEnd string) {
	return "FEDERATION#STATS", startDate, endDate
}

// IncrementStats adds values to the current stats
func (f *FederationStats) IncrementStats(activeInstances int, messages int64, users int) {
	f.ActiveInstances += activeInstances
	f.TotalMessages += messages
	f.TotalUsers += users
	f.UpdatedAt = time.Now().UTC()
}

// FormatStatsDate formats a time as a date string for use as sort key
func FormatStatsDate(t time.Time) string {
	return t.Format(common.DateFormat)
}
