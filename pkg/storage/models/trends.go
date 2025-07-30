package models

import (
	"fmt"
	"time"
)

// HashtagTrend represents a trending hashtag
type HashtagTrend struct {
	PK           string    `dynamorm:"pk"`
	SK           string    `dynamorm:"sk"`
	GSI8PK       string    `dynamorm:"index:gsi8,pk"`
	GSI8SK       string    `dynamorm:"index:gsi8,sk"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	UsageCount   int64     `json:"usage_count"`
	UniqueUsers  int64     `json:"unique_users"`
	LastUsed     time.Time `json:"last_used"`
	FirstSeen    time.Time `json:"first_seen"`
	TrendScore   float64   `json:"trend_score"`
	UpdatedAt    time.Time `json:"updated_at"`
	TTL          int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the GSI keys for hashtag trend
func (h *HashtagTrend) UpdateKeys() {
	timeBucket := h.UpdatedAt.Format("2006-01-02")
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
}

// StatusTrend represents a trending status
type StatusTrend struct {
	PK           string    `dynamorm:"pk"`
	SK           string    `dynamorm:"sk"`
	GSI8PK       string    `dynamorm:"index:gsi8,pk"`
	GSI8SK       string    `dynamorm:"index:gsi8,sk"`
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	AuthorID     string    `json:"author_id"`
	Content      string    `json:"content"`
	Engagements  int64     `json:"engagements"`
	PublishedAt  time.Time `json:"published_at"`
	TrendScore   float64   `json:"trend_score"`
	UpdatedAt    time.Time `json:"updated_at"`
	TTL          int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the GSI keys for status trend
func (s *StatusTrend) UpdateKeys() {
	timeBucket := s.UpdatedAt.Format("2006-01-02")
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
}

// LinkTrend represents a trending link
type LinkTrend struct {
	PK           string    `dynamorm:"pk"`
	SK           string    `dynamorm:"sk"`
	GSI8PK       string    `dynamorm:"index:gsi8,pk"`
	GSI8SK       string    `dynamorm:"index:gsi8,sk"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Type         string    `json:"type"` // "link", "video", "photo"
	AuthorName   string    `json:"author_name"`
	Image        string    `json:"image"`
	ShareCount   int64     `json:"share_count"`
	TrendScore   float64   `json:"trend_score"`
	UpdatedAt    time.Time `json:"updated_at"`
	TTL          int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the GSI keys for link trend
func (l *LinkTrend) UpdateKeys() {
	timeBucket := l.UpdatedAt.Format("2006-01-02")
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

// UpdateKeys updates the GSI keys for search query
func (s *SearchQuery) UpdateKeys() {
	// User search history
	s.PK = fmt.Sprintf("USER#%s", s.UserID)
	s.SK = fmt.Sprintf("SEARCH#%s", s.SearchedAt.Format(time.RFC3339Nano))
	
	// Set TTL to 30 days
	if s.TTL == 0 {
		s.TTL = s.SearchedAt.Add(30 * 24 * time.Hour).Unix()
	}
}