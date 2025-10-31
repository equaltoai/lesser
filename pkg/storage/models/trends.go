package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// HashtagTrend represents a trending hashtag
type HashtagTrend struct {
	PK          string    `dynamorm:"pk"`
	SK          string    `dynamorm:"sk"`
	GSI8PK      string    `dynamorm:"index:gsi8,pk"`
	GSI8SK      string    `dynamorm:"index:gsi8,sk"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	UsageCount  int64     `json:"usage_count"`
	UniqueUsers int64     `json:"unique_users"`
	LastUsed    time.Time `json:"last_used"`
	FirstSeen   time.Time `json:"first_seen"`
	TrendScore  float64   `json:"trend_score"`
	UpdatedAt   time.Time `json:"updated_at"`
	TTL         int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing HashtagTrend.
func (HashtagTrend) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for hashtag trend
func (h *HashtagTrend) UpdateKeys() error {
	timeBucket := h.UpdatedAt.Format(common.DateFormat)
	paddedScore := fmt.Sprintf("%010.0f", h.TrendScore*1000)

	h.PK = fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket)
	h.SK = fmt.Sprintf("SCORE#%s#%s", paddedScore, h.Name)

	// GSI8 for trend queries
	h.GSI8PK = fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket)
	h.GSI8SK = fmt.Sprintf("SCORE#%s#%s", paddedScore, h.Name)

	// Set TTL if not already set
	if h.TTL == 0 {
		h.TTL = h.UpdatedAt.Add(7 * 24 * time.Hour).Unix()
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (h *HashtagTrend) GetPK() string {
	return h.PK
}

// GetSK returns the sort key for BaseModel interface
func (h *HashtagTrend) GetSK() string {
	return h.SK
}

// StatusTrend represents a trending status
type StatusTrend struct {
	PK          string    `dynamorm:"pk"`
	SK          string    `dynamorm:"sk"`
	GSI8PK      string    `dynamorm:"index:gsi8,pk"`
	GSI8SK      string    `dynamorm:"index:gsi8,sk"`
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	AuthorID    string    `json:"author_id"`
	Content     string    `json:"content"`
	Engagements int64     `json:"engagements"`
	PublishedAt time.Time `json:"published_at"`
	TrendScore  float64   `json:"trend_score"`
	UpdatedAt   time.Time `json:"updated_at"`
	TTL         int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing StatusTrend.
func (StatusTrend) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for status trend
func (s *StatusTrend) UpdateKeys() error {
	timeBucket := s.UpdatedAt.Format(common.DateFormat)
	paddedScore := fmt.Sprintf("%010.0f", s.TrendScore*1000)

	s.PK = fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket)
	s.SK = fmt.Sprintf("SCORE#%s#%s", paddedScore, s.ID)

	// GSI8 for trend queries
	s.GSI8PK = fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket)
	s.GSI8SK = fmt.Sprintf("SCORE#%s#%s", paddedScore, s.ID)

	// Set TTL if not already set
	if s.TTL == 0 {
		s.TTL = s.UpdatedAt.Add(7 * 24 * time.Hour).Unix()
	}

	return nil
}

// LinkTrend represents a trending link
type LinkTrend struct {
	PK          string    `dynamorm:"pk"`
	SK          string    `dynamorm:"sk"`
	GSI8PK      string    `dynamorm:"index:gsi8,pk"`
	GSI8SK      string    `dynamorm:"index:gsi8,sk"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // "link", "video", "photo"
	AuthorName  string    `json:"author_name"`
	Image       string    `json:"image"`
	ShareCount  int64     `json:"share_count"`
	TrendScore  float64   `json:"trend_score"`
	UpdatedAt   time.Time `json:"updated_at"`
	TTL         int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing LinkTrend.
func (LinkTrend) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for link trend
func (l *LinkTrend) UpdateKeys() error {
	timeBucket := l.UpdatedAt.Format(common.DateFormat)
	paddedScore := fmt.Sprintf("%010.0f", l.TrendScore*1000)

	l.PK = fmt.Sprintf("TREND_TYPE#LINK#%s", timeBucket)
	l.SK = fmt.Sprintf("SCORE#%s#%s", paddedScore, l.URL)

	// GSI8 for trend queries
	l.GSI8PK = fmt.Sprintf("TREND_TYPE#LINK#%s", timeBucket)
	l.GSI8SK = fmt.Sprintf("SCORE#%s#%s", paddedScore, l.URL)

	// Set TTL if not already set
	if l.TTL == 0 {
		l.TTL = l.UpdatedAt.Add(7 * 24 * time.Hour).Unix()
	}

	return nil
}

// SearchQuery represents a search query for analytics
type SearchQuery struct {
	PK          string    `dynamorm:"pk"`
	SK          string    `dynamorm:"sk"`
	Query       string    `json:"query"`
	UserID      string    `json:"user_id"`
	ResultCount int       `json:"result_count"`
	SearchedAt  time.Time `json:"searched_at"`
	TTL         int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing SearchQuery.
func (SearchQuery) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for search query
func (s *SearchQuery) UpdateKeys() error {
	// User search history
	s.PK = fmt.Sprintf(KeyPatternUser, s.UserID)
	s.SK = fmt.Sprintf("SEARCH#%s", s.SearchedAt.Format(time.RFC3339Nano))

	// Set TTL to 30 days
	if s.TTL == 0 {
		s.TTL = s.SearchedAt.Add(30 * 24 * time.Hour).Unix()
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (s *SearchQuery) GetPK() string {
	return s.PK
}

// GetSK returns the sort key for BaseModel interface
func (s *SearchQuery) GetSK() string {
	return s.SK
}

// PopularQueryCounter represents an atomic counter for popular search queries
type PopularQueryCounter struct {
	// Key fields for atomic counter operations
	PK     string `dynamorm:"pk"`            // POPULAR_QUERY#query_hash
	SK     string `dynamorm:"sk"`            // COUNTER#time_bucket (daily, weekly, monthly)
	GSI8PK string `dynamorm:"index:gsi8,pk"` // For time-based queries
	GSI8SK string `dynamorm:"index:gsi8,sk"` // For ranking by count

	// Business fields
	QueryHash    string    `json:"query_hash"`    // Hashed query for privacy
	Query        string    `json:"query"`         // Original query (if not sensitive)
	TimeBucket   string    `json:"time_bucket"`   // daily, weekly, monthly
	Date         string    `json:"date"`          // YYYY-MM-DD format
	Count        int64     `json:"count"`         // Atomic counter value
	UserCount    int64     `json:"user_count"`    // Unique users who searched
	AvgResults   float64   `json:"avg_results"`   // Average result count
	LastQueried  time.Time `json:"last_queried"`  // Most recent query time
	FirstQueried time.Time `json:"first_queried"` // First time this query was seen
	UpdatedAt    time.Time `json:"updated_at"`
	TTL          int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing PopularQueryCounter.
func (PopularQueryCounter) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for popular query counter
func (p *PopularQueryCounter) UpdateKeys() error {
	// Primary keys for atomic operations
	p.PK = fmt.Sprintf("POPULAR_QUERY#%s", p.QueryHash)
	p.SK = fmt.Sprintf("COUNTER#%s", p.TimeBucket)

	// GSI8 for time-based and ranking queries
	// Format: POPULAR#bucket#date for time queries
	p.GSI8PK = fmt.Sprintf("POPULAR#%s#%s", p.TimeBucket, p.Date)
	// Padded count for proper sorting (highest first)
	paddedCount := fmt.Sprintf("%010d", p.Count)
	p.GSI8SK = fmt.Sprintf("COUNT#%s#%s", paddedCount, p.QueryHash)

	// Set TTL based on bucket type
	if p.TTL == 0 {
		switch p.TimeBucket {
		case "daily":
			p.TTL = p.UpdatedAt.Add(30 * 24 * time.Hour).Unix() // 30 days
		case "weekly":
			p.TTL = p.UpdatedAt.Add(90 * 24 * time.Hour).Unix() // 90 days
		case "monthly":
			p.TTL = p.UpdatedAt.Add(365 * 24 * time.Hour).Unix() // 1 year
		default:
			p.TTL = p.UpdatedAt.Add(30 * 24 * time.Hour).Unix() // Default 30 days
		}
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (p *PopularQueryCounter) GetPK() string {
	return p.PK
}

// GetSK returns the sort key for BaseModel interface
func (p *PopularQueryCounter) GetSK() string {
	return p.SK
}
