package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// MediaPopularity tracks aggregated popularity metrics for media items
// This is maintained by streaming analytics ingestion for efficient trending queries
type MediaPopularity struct {
	// DynamoDB Keys (STABLE - never change)
	PK string `dynamorm:"pk" json:"pk"` // MEDIA_POPULARITY#{period}
	SK string `dynamorm:"sk" json:"sk"` // MEDIA#{mediaID}

	// GSI1 - Sorted by popularity (UPDATEABLE)
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // PERIOD#{period}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // {inverted_view_count} for descending sort

	// GSI2 - Query by date
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2pk"` // DATE#{date}
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2sk"` // MEDIA#{mediaID}

	// Business fields
	MediaID         string    `json:"media_id"`
	ViewCount       int64     `json:"view_count"`
	UniqueViewers   int64     `json:"unique_viewers"`
	CompletionCount int64     `json:"completion_count"`
	TotalWatchTime  int64     `json:"total_watch_time"` // seconds
	BufferingEvents int64     `json:"buffering_events"`
	LastViewed      time.Time `json:"last_viewed"`
	FirstViewed     time.Time `json:"first_viewed"`
	Period          string    `json:"period"` // DAY, WEEK, MONTH
	Date            string    `json:"date"`   // YYYY-MM-DD
	Timestamp       time.Time `json:"timestamp"`

	// Quality distribution
	QualityViews map[string]int64 `json:"quality_views"` // quality -> view count

	// Popularity score (calculated)
	PopularityScore float64 `json:"popularity_score"`
	TrendScore      float64 `json:"trend_score"`

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the keys based on the current values
func (m *MediaPopularity) UpdateKeys() error {
	// Primary keys (STABLE - never change after creation)
	m.PK = fmt.Sprintf("MEDIA_POPULARITY#%s", m.Period)
	m.SK = fmt.Sprintf("MEDIA#%s", m.MediaID)

	// GSI1 - For sorted popularity queries (UPDATEABLE)
	// Invert view count for descending sort: high counts have low sort keys
	maxViewCount := int64(999999999999999999)
	invertedCount := maxViewCount - m.ViewCount
	m.GSI1PK = fmt.Sprintf("PERIOD#%s", m.Period)
	m.GSI1SK = fmt.Sprintf("%020d", invertedCount)

	// GSI2 - For date-based queries
	m.GSI2PK = fmt.Sprintf("DATE#%s", m.Date)
	m.GSI2SK = fmt.Sprintf("MEDIA#%s", m.MediaID)

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (m *MediaPopularity) GetPK() string {
	return m.PK
}

// GetSK returns the sort key for BaseModel interface
func (m *MediaPopularity) GetSK() string {
	return m.SK
}

// SetForPeriod configures this record for a specific time period
func (m *MediaPopularity) SetForPeriod(mediaID string, period string, viewCount int64) {
	m.MediaID = mediaID
	m.Period = period
	m.ViewCount = viewCount
	m.Timestamp = time.Now()
	m.Date = m.Timestamp.Format(common.DateFormat)

	// Initialize maps
	if m.QualityViews == nil {
		m.QualityViews = make(map[string]int64)
	}

	// Calculate popularity score (views with recency decay)
	m.PopularityScore = float64(viewCount)
	m.TrendScore = float64(viewCount) // Would be enhanced with velocity

	// Set TTL based on period
	switch period {
	case "DAY":
		m.TTL = time.Now().Add(7 * 24 * time.Hour).Unix() // Keep daily for 7 days
	case "WEEK":
		m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix() // Keep weekly for 30 days
	case "MONTH":
		m.TTL = time.Now().Add(90 * 24 * time.Hour).Unix() // Keep monthly for 90 days
	default:
		m.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	}

	_ = m.UpdateKeys()
}

// IncrementViews atomically increments the view count
func (m *MediaPopularity) IncrementViews(count int64) {
	m.ViewCount += count
	m.LastViewed = time.Now()
	_ = m.UpdateKeys() // Recalculate SK with new view count
}

// AddQualityView increments view count for a specific quality
func (m *MediaPopularity) AddQualityView(quality string, count int64) {
	if m.QualityViews == nil {
		m.QualityViews = make(map[string]int64)
	}
	m.QualityViews[quality] += count
}

// CalculateCompletionRate calculates the completion rate
func (m *MediaPopularity) CalculateCompletionRate() float64 {
	if m.ViewCount == 0 {
		return 0.0
	}
	return float64(m.CompletionCount) / float64(m.ViewCount)
}

// CalculateAvgWatchTime calculates average watch time in seconds
func (m *MediaPopularity) CalculateAvgWatchTime() float64 {
	if m.ViewCount == 0 {
		return 0.0
	}
	return float64(m.TotalWatchTime) / float64(m.ViewCount)
}
