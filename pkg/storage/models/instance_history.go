package models

import (
	"fmt"
	"time"
)

// InstanceHistory stores time-series data for instance metrics
// Pattern: PK=INSTANCE#HISTORY, SK=DAILY#{YYYY-MM-DD} or MONTHLY#{YYYY-MM} or WEEKLY#{YYYY-WW}
// GSI1: PK=METRIC#{metric_type}, SK=DATE#{YYYY-MM-DD} for efficient time range queries
type InstanceHistory struct {
	// Primary key fields
	PK string `dynamorm:"pk"` // INSTANCE#HISTORY
	SK string `dynamorm:"sk"` // DAILY#{date} or MONTHLY#{date} or WEEKLY#{date}

	// GSI1 for metric-specific time queries
	GSI1PK string `dynamorm:"index:GSI1,pk"` // METRIC#{metric_type}
	GSI1SK string `dynamorm:"index:GSI1,sk"` // DATE#{YYYY-MM-DD}

	// Business fields
	Date        string    `json:"date"`         // YYYY-MM-DD format
	MetricType  string    `json:"metric_type"`  // user_count, storage_bytes, post_count
	Granularity string    `json:"granularity"`  // daily, weekly, monthly
	Value       int64     `json:"value"`        // Current value
	Delta       int64     `json:"delta"`        // Change from previous period
	RecordedAt  time.Time `json:"recorded_at"`  // When this metric was recorded

	// Specific metric fields for detailed tracking
	TotalUsers      int64 `json:"total_users,omitempty"`      // Total registered users
	ActiveUsers     int64 `json:"active_users,omitempty"`     // Users active in period
	NewUsers        int64 `json:"new_users,omitempty"`        // New registrations in period
	TotalPosts      int64 `json:"total_posts,omitempty"`      // Total posts/statuses
	NewPosts        int64 `json:"new_posts,omitempty"`        // New posts in period
	StorageBytes    int64 `json:"storage_bytes,omitempty"`    // Total storage used
	MediaBytes      int64 `json:"media_bytes,omitempty"`      // Media storage used
	DatabaseBytes   int64 `json:"database_bytes,omitempty"`   // Database storage used
	FederatedPosts  int64 `json:"federated_posts,omitempty"`  // Posts from other instances
	LocalPosts      int64 `json:"local_posts,omitempty"`      // Posts from local users
	KnownInstances  int64 `json:"known_instances,omitempty"`  // Number of federated instances
	ActiveInstances int64 `json:"active_instances,omitempty"` // Active federation partners

	// TTL field for automatic cleanup (90 days for daily, keep monthly forever)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table name
func (i *InstanceHistory) TableName() string {
	return DefaultTableName
}

// UpdateKeys updates the GSI keys when the primary keys change
func (i *InstanceHistory) UpdateKeys() {
	// GSI1 is used for metric-specific time range queries
	i.GSI1PK = fmt.Sprintf("METRIC#%s", i.MetricType)
	i.GSI1SK = fmt.Sprintf("DATE#%s", i.Date)
}

// NewDailyInstanceHistory creates a new daily history record
func NewDailyInstanceHistory(date string, metricType string) *InstanceHistory {
	now := time.Now()
	history := &InstanceHistory{
		PK:          "INSTANCE#HISTORY",
		SK:          fmt.Sprintf("DAILY#%s#%s", date, metricType),
		Date:        date,
		MetricType:  metricType,
		Granularity: "daily",
		RecordedAt:  now,
		// Set TTL to 90 days from now (90 * 24 * 60 * 60 = 7776000 seconds)
		TTL: now.Unix() + 7776000,
	}
	history.UpdateKeys()
	return history
}

// NewWeeklyInstanceHistory creates a new weekly history record
func NewWeeklyInstanceHistory(weekStart string, metricType string) *InstanceHistory {
	now := time.Now()
	history := &InstanceHistory{
		PK:          "INSTANCE#HISTORY",
		SK:          fmt.Sprintf("WEEKLY#%s#%s", weekStart, metricType),
		Date:        weekStart,
		MetricType:  metricType,
		Granularity: "weekly",
		RecordedAt:  now,
		// Set TTL to 365 days from now (365 * 24 * 60 * 60 = 31536000 seconds)
		TTL: now.Unix() + 31536000,
	}
	history.UpdateKeys()
	return history
}

// NewMonthlyInstanceHistory creates a new monthly history record (no TTL - keep forever)
func NewMonthlyInstanceHistory(monthStart string, metricType string) *InstanceHistory {
	history := &InstanceHistory{
		PK:          "INSTANCE#HISTORY",
		SK:          fmt.Sprintf("MONTHLY#%s#%s", monthStart, metricType),
		Date:        monthStart,
		MetricType:  metricType,
		Granularity: "monthly",
		RecordedAt:  time.Now(),
		// No TTL - keep monthly data forever
	}
	history.UpdateKeys()
	return history
}

// SetUserMetrics sets user-related metrics
func (i *InstanceHistory) SetUserMetrics(total, active, newUsers int64) {
	i.TotalUsers = total
	i.ActiveUsers = active
	i.NewUsers = newUsers
	i.Value = total // Primary value is total users
}

// SetStorageMetrics sets storage-related metrics
func (i *InstanceHistory) SetStorageMetrics(totalBytes, mediaBytes, dbBytes int64) {
	i.StorageBytes = totalBytes
	i.MediaBytes = mediaBytes
	i.DatabaseBytes = dbBytes
	i.Value = totalBytes // Primary value is total storage
}

// SetPostMetrics sets post-related metrics
func (i *InstanceHistory) SetPostMetrics(total, newPosts, local, federated int64) {
	i.TotalPosts = total
	i.NewPosts = newPosts
	i.LocalPosts = local
	i.FederatedPosts = federated
	i.Value = total // Primary value is total posts
}

// SetFederationMetrics sets federation-related metrics
func (i *InstanceHistory) SetFederationMetrics(knownInstances, activeInstances int64) {
	i.KnownInstances = knownInstances
	i.ActiveInstances = activeInstances
	i.Value = knownInstances // Primary value is known instances
}

// CalculateDelta calculates the delta from a previous period's value
func (i *InstanceHistory) CalculateDelta(previousValue int64) {
	i.Delta = i.Value - previousValue
}