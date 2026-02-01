package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// HashtagStats represents statistics for a hashtag
// Stored in DynamoDB with pattern:
// PK: HASHTAG#name
// SK: STATS
type HashtagStats struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	Name          string                `theorydb:"attr:name" json:"name"`
	UsageCount    int64                 `theorydb:"attr:usageCount" json:"usage_count"`
	UniqueUsers   int64                 `theorydb:"attr:uniqueUsers" json:"unique_users"`
	FirstSeen     time.Time             `theorydb:"attr:firstSeen" json:"first_seen"`
	LastUsed      time.Time             `theorydb:"attr:lastUsed" json:"last_used"`
	TrendingScore float64               `theorydb:"attr:trendingScore" json:"trending_score"`
	TotalUses     int64                 `theorydb:"attr:totalUses" json:"total_uses"`         // Total usage count
	TotalAccounts int64                 `theorydb:"attr:totalAccounts" json:"total_accounts"` // Total unique accounts
	History       []HashtagHistoryEntry `theorydb:"attr:history" json:"history"`              // Historical data
}

// UpdateKeys updates the DynamoDB keys based on the hashtag name
func (h *HashtagStats) UpdateKeys() {
	if h.Name != "" {
		h.PK = fmt.Sprintf(KeyPatternHashtag, h.Name)
		h.SK = SKStats
	}
}

// NewHashtagStats creates a new hashtag stats entry
func NewHashtagStats(name string) *HashtagStats {
	now := time.Now()
	stats := &HashtagStats{
		Name:          name,
		UsageCount:    0,
		UniqueUsers:   0,
		FirstSeen:     now,
		LastUsed:      now,
		TrendingScore: 0.0,
		TotalUses:     0,
		TotalAccounts: 0,
		History:       make([]HashtagHistoryEntry, 0),
	}
	stats.UpdateKeys()
	return stats
}

// IncrementUsage increments the usage count and updates last used time
func (h *HashtagStats) IncrementUsage() {
	h.UsageCount++
	h.TotalUses++
	h.LastUsed = time.Now()
}

// AddUniqueUser increments the unique user count
func (h *HashtagStats) AddUniqueUser() {
	h.UniqueUsers++
	h.TotalAccounts++
}

// UpdateTrendingScore updates the trending score based on recent activity
func (h *HashtagStats) UpdateTrendingScore() {
	// Simple trending algorithm: recent usage weighted by unique users
	hoursSinceLastUse := time.Since(h.LastUsed).Hours()
	if hoursSinceLastUse > 0 {
		// Score decreases over time
		h.TrendingScore = float64(h.UsageCount) * float64(h.UniqueUsers) / hoursSinceLastUse
	} else {
		// Maximum score for very recent usage
		h.TrendingScore = float64(h.UsageCount) * float64(h.UniqueUsers)
	}
}

// AddHistoryEntry adds a new history entry for tracking trends
func (h *HashtagStats) AddHistoryEntry(date time.Time, usageCount, userCount int64) {
	entry := HashtagHistoryEntry{
		Date:       date,
		UsageCount: usageCount,
		UserCount:  userCount,
	}
	h.History = append(h.History, entry)

	// Keep only last 30 days of history
	if len(h.History) > 30 {
		h.History = h.History[len(h.History)-30:]
	}
}

// GetAverageUsage calculates the average usage over the history period
func (h *HashtagStats) GetAverageUsage() float64 {
	if err := common.ValidateSliceNotEmpty("h.History", h.History); err != nil {
		return float64(h.UsageCount)
	}

	var total int64
	for _, entry := range h.History {
		total += entry.UsageCount
	}
	return float64(total) / float64(len(h.History))
}

// GetGrowthRate calculates the growth rate between first and last history entries
func (h *HashtagStats) GetGrowthRate() float64 {
	if len(h.History) < 2 {
		return 0
	}

	first := h.History[0]
	last := h.History[len(h.History)-1]

	if first.UsageCount == 0 {
		return 0
	}

	return float64(last.UsageCount-first.UsageCount) / float64(first.UsageCount)
}

// IsActive returns true if the hashtag has been used in the last 7 days
func (h *HashtagStats) IsActive() bool {
	return time.Since(h.LastUsed) < 7*24*time.Hour
}

// IsTrending returns true if the trending score is above a threshold
func (h *HashtagStats) IsTrending(threshold float64) bool {
	return h.TrendingScore > threshold
}

// TableName returns the DynamoDB table backing HashtagStats.
func (HashtagStats) TableName() string {
	return MainTableName
}
