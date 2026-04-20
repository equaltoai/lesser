package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// HashtagTrend represents a trending hashtag
type HashtagTrend struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK          string    `theorydb:"pk,attr:PK"`
	SK          string    `theorydb:"sk,attr:SK"`
	GSI8PK      string    `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty"`
	GSI8SK      string    `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty"`
	Name        string    `theorydb:"attr:name" json:"name"`
	URL         string    `theorydb:"attr:url" json:"url"`
	UsageCount  int64     `theorydb:"attr:usageCount" json:"usage_count"`
	UniqueUsers int64     `theorydb:"attr:uniqueUsers" json:"unique_users"`
	LastUsed    time.Time `theorydb:"attr:lastUsed" json:"last_used"`
	FirstSeen   time.Time `theorydb:"attr:firstSeen" json:"first_seen"`
	TrendScore  float64   `theorydb:"attr:trendScore" json:"trend_score"`
	UpdatedAt   time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL         int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK          string    `theorydb:"pk,attr:PK"`
	SK          string    `theorydb:"sk,attr:SK"`
	GSI8PK      string    `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty"`
	GSI8SK      string    `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty"`
	ID          string    `theorydb:"attr:id" json:"id"`
	URL         string    `theorydb:"attr:url" json:"url"`
	AuthorID    string    `theorydb:"attr:authorID" json:"author_id"`
	Content     string    `theorydb:"attr:content" json:"content"`
	Engagements int64     `theorydb:"attr:engagements" json:"engagements"`
	PublishedAt time.Time `theorydb:"attr:publishedAt" json:"published_at"`
	TrendScore  float64   `theorydb:"attr:trendScore" json:"trend_score"`
	UpdatedAt   time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL         int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK          string    `theorydb:"pk,attr:PK"`
	SK          string    `theorydb:"sk,attr:SK"`
	GSI8PK      string    `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty"`
	GSI8SK      string    `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty"`
	URL         string    `theorydb:"attr:url" json:"url"`
	Title       string    `theorydb:"attr:title" json:"title"`
	Description string    `theorydb:"attr:description" json:"description"`
	Type        string    `theorydb:"attr:type" json:"type"` // "link", "video", "photo"
	AuthorName  string    `theorydb:"attr:authorName" json:"author_name"`
	Image       string    `theorydb:"attr:image" json:"image"`
	ShareCount  int64     `theorydb:"attr:shareCount" json:"share_count"`
	TrendScore  float64   `theorydb:"attr:trendScore" json:"trend_score"`
	UpdatedAt   time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL         int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK          string    `theorydb:"pk,attr:PK"`
	SK          string    `theorydb:"sk,attr:SK"`
	Query       string    `theorydb:"attr:query" json:"query"`
	UserID      string    `theorydb:"attr:userID" json:"user_id"`
	ResultCount int       `theorydb:"attr:resultCount" json:"result_count"`
	SearchedAt  time.Time `theorydb:"attr:searchedAt" json:"searched_at"`
	TTL         int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Key fields for atomic counter operations
	PK     string `theorydb:"pk,attr:PK"`                          // POPULAR_QUERY#query_hash
	SK     string `theorydb:"sk,attr:SK"`                          // COUNTER#time_bucket (daily, weekly, monthly)
	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty"` // For time-based queries
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty"` // For ranking by count

	// Business fields
	QueryHash    string    `theorydb:"attr:queryHash" json:"query_hash"`       // Hashed query for privacy
	Query        string    `theorydb:"attr:query" json:"query"`                // Original query (if not sensitive)
	TimeBucket   string    `theorydb:"attr:timeBucket" json:"time_bucket"`     // daily, weekly, monthly
	Date         string    `theorydb:"attr:date" json:"date"`                  // YYYY-MM-DD format
	Count        int64     `theorydb:"attr:count" json:"count"`                // Atomic counter value
	UserCount    int64     `theorydb:"attr:userCount" json:"user_count"`       // Unique users who searched
	AvgResults   float64   `theorydb:"attr:avgResults" json:"avg_results"`     // Average result count
	LastQueried  time.Time `theorydb:"attr:lastQueried" json:"last_queried"`   // Most recent query time
	FirstQueried time.Time `theorydb:"attr:firstQueried" json:"first_queried"` // First time this query was seen
	UpdatedAt    time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL          int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
