// Package trends provides trending content analysis and aggregation services.
package trends

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage" // Used for TrendingHashtag, TrendingStatus, TrendingLink types
	"github.com/equaltoai/lesser/pkg/storage/core"
)

// TrendingAlgorithm defines the interface for different trending algorithms
type TrendingAlgorithm interface {
	Calculate(items []TrendItem) []TrendScore
}

// TrendItem represents an item that can trend (hashtag, status, link)
type TrendItem struct {
	ID          string
	Type        TrendType
	Content     string
	UsageCount  int64
	UniqueUsers int64
	LastUsed    time.Time
	FirstSeen   time.Time
	Engagements int64   // likes, boosts, replies
	TrustScore  float64 // average trust score of users interacting
}

// TrendScore represents a calculated trend score
type TrendScore struct {
	Item  TrendItem
	Score float64
}

// TrendType represents the type of trending item
type TrendType string

const (
	// TrendTypeHashtag represents hashtag trend type
	TrendTypeHashtag TrendType = "hashtag"
	// TrendTypeStatus represents status/post trend type
	TrendTypeStatus TrendType = "status"
	// TrendTypeLink represents link/URL trend type
	TrendTypeLink TrendType = "link"
)

// Service provides trending functionality
type Service struct {
	storage   core.RepositoryStorage
	algorithm TrendingAlgorithm
}

// NewService creates a new trending service
func NewService(storage core.RepositoryStorage) *Service {
	return &Service{
		storage:   storage,
		algorithm: NewDefaultAlgorithm(),
	}
}

// GetTrends returns general trends (mix of all types)
func (s *Service) GetTrends(ctx context.Context, limit int) ([]Trend, error) {
	// Get trends from each category
	hashtags, err := s.GetTrendingHashtags(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get hashtag trends: %w", err)
	}

	statuses, err := s.GetTrendingStatuses(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get status trends: %w", err)
	}

	links, err := s.GetTrendingLinks(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get link trends: %w", err)
	}

	// Combine and sort by score
	allTrends := make([]Trend, 0, len(hashtags)+len(statuses)+len(links))

	for _, h := range hashtags {
		allTrends = append(allTrends, Trend{
			Type:  "hashtag",
			Value: h,
		})
	}

	for _, s := range statuses {
		allTrends = append(allTrends, Trend{
			Type:  "status",
			Value: s,
		})
	}

	for _, l := range links {
		allTrends = append(allTrends, Trend{
			Type:  "link",
			Value: l,
		})
	}

	// Limit to requested amount
	if len(allTrends) > limit {
		allTrends = allTrends[:limit]
	}

	return allTrends, nil
}

// GetTrendingHashtags returns trending hashtags
func (s *Service) GetTrendingHashtags(ctx context.Context, limit int) ([]HashtagTrend, error) {
	// Get hashtag usage data from storage
	var trends []*storage.TrendingHashtag
	trends, err := s.storage.Analytics().GetTrendingHashtags(ctx, time.Now().Add(-24*time.Hour), limit)
	if err != nil {
		return nil, err
	}

	result := make([]HashtagTrend, len(trends))
	for i, t := range trends {
		// Get historical data for sparkline
		history, _ := s.storage.Hashtag().GetHashtagUsageHistory(ctx, t.Name, 7)

		result[i] = HashtagTrend{
			Name:     t.Name,
			URL:      t.URL,
			History:  history,
			Uses:     t.UsageCount,
			Accounts: t.UniqueUsers,
		}
	}

	return result, nil
}

// GetTrendingStatuses returns trending statuses
func (s *Service) GetTrendingStatuses(ctx context.Context, limit int) ([]StatusTrend, error) {
	// Get trending statuses from storage
	var trends []*storage.TrendingStatus
	trends, err := s.storage.Analytics().GetTrendingStatuses(ctx, time.Now().Add(-24*time.Hour), limit)
	if err != nil {
		return nil, err
	}

	result := make([]StatusTrend, len(trends))
	for i, t := range trends {
		result[i] = StatusTrend{
			StatusID:    t.ID,
			URL:         t.URL,
			AuthorID:    t.AuthorID,
			Content:     t.Content,
			Engagements: t.Engagements,
			PublishedAt: t.PublishedAt,
		}
	}

	return result, nil
}

// GetTrendingLinks returns trending links
func (s *Service) GetTrendingLinks(ctx context.Context, limit int) ([]LinkTrend, error) {
	// Get trending links from storage
	var trends []*storage.TrendingLink
	trends, err := s.storage.Analytics().GetTrendingLinks(ctx, time.Now().Add(-24*time.Hour), limit)
	if err != nil {
		return nil, err
	}

	result := make([]LinkTrend, len(trends))
	for i, t := range trends {
		result[i] = LinkTrend{
			URL:         t.URL,
			Title:       t.Title,
			Description: t.Description,
			Type:        t.Type,
			AuthorName:  t.AuthorName,
			Image:       t.Image,
			Shares:      t.ShareCount,
		}
	}

	return result, nil
}

// RecordHashtagUsage records hashtag usage for trending calculation
func (s *Service) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	return s.storage.Analytics().RecordHashtagUsage(ctx, hashtag, statusID, authorID)
}

// RecordStatusEngagement records engagement for trending calculation
func (s *Service) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	return s.storage.Analytics().RecordStatusEngagement(ctx, statusID, engagementType, userID)
}

// RecordLinkShare records link sharing for trending calculation
func (s *Service) RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error {
	return s.storage.Analytics().RecordLinkShare(ctx, url, statusID, authorID)
}

// GetStatusesByLink returns statuses that contain a specific link
func (s *Service) GetStatusesByLink(ctx context.Context, url string, limit int) ([]interface{}, error) {
	return s.storage.Analytics().GetStatusesByLink(ctx, url, limit)
}

// Trend represents a general trend
type Trend struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// HashtagTrend represents a trending hashtag
type HashtagTrend struct {
	Name     string  `json:"name"`
	URL      string  `json:"url"`
	History  []int64 `json:"history"`  // Usage count per day for last 7 days
	Uses     int64   `json:"uses"`     // Total uses in time period
	Accounts int64   `json:"accounts"` // Unique accounts using it
}

// StatusTrend represents a trending status
type StatusTrend struct {
	StatusID    string    `json:"id"`
	URL         string    `json:"url"`
	AuthorID    string    `json:"author_id"`
	Content     string    `json:"content"`
	Engagements int64     `json:"engagements"`
	PublishedAt time.Time `json:"published_at"`
}

// LinkTrend represents a trending link
type LinkTrend struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"` // link, photo, video
	AuthorName  string `json:"author_name"`
	Image       string `json:"image"`
	Shares      int64  `json:"shares"`
}

// DefaultAlgorithm implements a time-decay algorithm for trending
type DefaultAlgorithm struct {
	halfLife time.Duration
}

// NewDefaultAlgorithm creates the default trending algorithm
func NewDefaultAlgorithm() *DefaultAlgorithm {
	return &DefaultAlgorithm{
		halfLife: 2 * time.Hour,
	}
}

// Calculate implements TrendingAlgorithm
func (a *DefaultAlgorithm) Calculate(items []TrendItem) []TrendScore {
	now := time.Now()
	scores := make([]TrendScore, len(items))

	for i, item := range items {
		// Calculate age factor (exponential decay)
		age := now.Sub(item.LastUsed)
		ageFactor := 1.0 / (1 + age.Hours()/a.halfLife.Hours())

		// Calculate engagement factor
		engagementFactor := float64(item.Engagements) / float64(item.UsageCount+1)

		// Calculate diversity factor (unique users / total uses)
		diversityFactor := float64(item.UniqueUsers) / float64(item.UsageCount+1)

		// Trust factor (higher trust users count more)
		trustFactor := item.TrustScore

		// Combined score
		score := float64(item.UsageCount) * ageFactor * (1 + engagementFactor) * (1 + diversityFactor) * (1 + trustFactor)

		scores[i] = TrendScore{
			Item:  item,
			Score: score,
		}
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores
}
