package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// MediaPopularity tracks aggregated popularity metrics for media items
// This is maintained by streaming analytics ingestion for efficient trending queries
type MediaPopularity struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB Keys (STABLE - never change)
	PK string `theorydb:"pk,attr:PK" json:"pk"` // MEDIA_POPULARITY#{period}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // MEDIA#{mediaID}

	// GSI1 - Sorted by popularity (UPDATEABLE)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1pk"` // PERIOD#{period}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1sk"` // {inverted_view_count} for descending sort

	// GSI2 - Query by date
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"gsi2pk"` // DATE#{date}
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"gsi2sk"` // MEDIA#{mediaID}

	// Business fields
	MediaID         string    `theorydb:"attr:mediaID" json:"media_id"`
	ViewCount       int64     `theorydb:"attr:viewCount" json:"view_count"`
	UniqueViewers   int64     `theorydb:"attr:uniqueViewers" json:"unique_viewers"`
	CompletionCount int64     `theorydb:"attr:completionCount" json:"completion_count"`
	TotalWatchTime  int64     `theorydb:"attr:totalWatchTime" json:"total_watch_time"` // seconds
	BufferingEvents int64     `theorydb:"attr:bufferingEvents" json:"buffering_events"`
	LastViewed      time.Time `theorydb:"attr:lastViewed" json:"last_viewed"`
	FirstViewed     time.Time `theorydb:"attr:firstViewed" json:"first_viewed"`
	Period          string    `theorydb:"attr:period" json:"period"` // DAY, WEEK, MONTH
	Date            string    `theorydb:"attr:date" json:"date"`     // YYYY-MM-DD
	Timestamp       time.Time `theorydb:"attr:timestamp" json:"timestamp"`

	// Quality distribution
	QualityViews map[string]int64 `theorydb:"attr:qualityViews" json:"quality_views"` // quality -> view count

	// Popularity score (calculated)
	PopularityScore float64 `theorydb:"attr:popularityScore" json:"popularity_score"`
	TrendScore      float64 `theorydb:"attr:trendScore" json:"trend_score"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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

// TableName returns the DynamoDB table backing MediaPopularity.
func (MediaPopularity) TableName() string {
	return MainTableName
}
