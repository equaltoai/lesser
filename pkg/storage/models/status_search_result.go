package models

import (
	"time"
)

// StatusSearchResult represents a status search result
// This is a response type used for search results, not stored in DynamoDB
type StatusSearchResult struct {
	StatusID       string            `json:"status_id"`
	Content        string            `json:"content"`
	URL            string            `json:"url"`
	AuthorID       string            `json:"author_id"`
	AuthorUsername string            `json:"author_username"`
	Published      time.Time         `json:"published"`
	Score          float64           `json:"score"`
	Highlights     map[string]string `json:"highlights"`
}

// NewStatusSearchResult creates a new status search result
func NewStatusSearchResult(statusID, content, url, authorID, authorUsername string, published time.Time) *StatusSearchResult {
	return &StatusSearchResult{
		StatusID:       statusID,
		Content:        content,
		URL:            url,
		AuthorID:       authorID,
		AuthorUsername: authorUsername,
		Published:      published,
		Score:          0.0,
		Highlights:     make(map[string]string),
	}
}

// SetScore sets the relevance score for the search result
func (s *StatusSearchResult) SetScore(score float64) {
	s.Score = score
}

// AddHighlight adds a highlighted snippet for a field
func (s *StatusSearchResult) AddHighlight(field, highlight string) {
	if s.Highlights == nil {
		s.Highlights = make(map[string]string)
	}
	s.Highlights[field] = highlight
}

// IsRecent returns true if the status was published within the last 24 hours
func (s *StatusSearchResult) IsRecent() bool {
	return time.Since(s.Published) < 24*time.Hour
}
