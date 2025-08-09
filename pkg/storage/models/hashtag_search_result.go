package models

import (
	"time"
)

// HashtagSearchResult represents a hashtag search result
// This is a response type used for search results, not stored in DynamoDB
type HashtagSearchResult struct {
	Name      string             `json:"name"`
	URL       string             `json:"url"`
	History   []*TrendingHashtag `json:"history"`
	Following *bool              `json:"following,omitempty"`
}

// NewHashtagSearchResult creates a new hashtag search result
func NewHashtagSearchResult(name, url string) *HashtagSearchResult {
	return &HashtagSearchResult{
		Name:    name,
		URL:     url,
		History: make([]*TrendingHashtag, 0),
	}
}

// AddHistory adds a trending history entry
func (h *HashtagSearchResult) AddHistory(trend *TrendingHashtag) {
	if trend != nil {
		h.History = append(h.History, trend)
	}
}

// SetFollowing sets whether the user is following this hashtag
func (h *HashtagSearchResult) SetFollowing(following bool) {
	h.Following = &following
}

// IsFollowing returns whether the user is following this hashtag
func (h *HashtagSearchResult) IsFollowing() bool {
	if h.Following == nil {
		return false
	}
	return *h.Following
}

// GetLatestUsage returns the most recent usage count, or 0 if no history
func (h *HashtagSearchResult) GetLatestUsage() int64 {
	if len(h.History) == 0 {
		return 0
	}

	// Find the most recent entry based on UpdatedAt
	var latest *TrendingHashtag
	for _, trend := range h.History {
		if latest == nil || trend.UpdatedAt.After(latest.UpdatedAt) {
			latest = trend
		}
	}

	if latest != nil {
		return latest.UseCount
	}
	return 0
}

// GetTotalUsage returns the sum of all usage counts in history
func (h *HashtagSearchResult) GetTotalUsage() int64 {
	var total int64
	for _, trend := range h.History {
		total += trend.UseCount
	}
	return total
}

// HasRecentActivity returns true if the hashtag has been used in the last 24 hours
func (h *HashtagSearchResult) HasRecentActivity() bool {
	now := time.Now()
	for _, trend := range h.History {
		if now.Sub(trend.UpdatedAt) < 24*time.Hour {
			return true
		}
	}
	return false
}
