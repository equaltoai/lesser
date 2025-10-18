package models

import (
	"time"
)

// HashtagHistoryEntry represents a historical data point for hashtag usage
// This is typically embedded in HashtagStats, not stored separately
type HashtagHistoryEntry struct {
	Date       time.Time `json:"date"`
	UsageCount int64     `json:"usage_count"`
	UserCount  int64     `json:"user_count"`
}

// NewHashtagHistoryEntry creates a new history entry
func NewHashtagHistoryEntry(date time.Time, usageCount, userCount int64) HashtagHistoryEntry {
	return HashtagHistoryEntry{
		Date:       date,
		UsageCount: usageCount,
		UserCount:  userCount,
	}
}

// GetEngagement calculates a simple engagement metric
func (h HashtagHistoryEntry) GetEngagement() float64 {
	if h.UserCount == 0 {
		return 0
	}
	return float64(h.UsageCount) / float64(h.UserCount)
}

// IsHighActivity returns true if usage exceeds the threshold
func (h HashtagHistoryEntry) IsHighActivity(threshold int64) bool {
	return h.UsageCount > threshold
}

// DaysSince returns the number of days since this entry
func (h HashtagHistoryEntry) DaysSince() int {
	return int(time.Since(h.Date).Hours() / 24)
}

// CompareWith compares this entry with another and returns the percentage change
func (h HashtagHistoryEntry) CompareWith(other HashtagHistoryEntry) float64 {
	if h.UsageCount == 0 {
		return 0
	}
	return float64(other.UsageCount-h.UsageCount) / float64(h.UsageCount) * 100
}
