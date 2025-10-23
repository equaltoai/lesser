package models

import (
	"fmt"
	"time"
)

// SearchSuggestion represents search autocomplete suggestions
type SearchSuggestion struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Fields
	Type      string    `json:"type"`  // hashtag, account, username, display_name
	Term      string    `json:"term"`  // the search term
	Score     float64   `json:"score"` // popularity/relevance score
	LastUsed  time.Time `json:"last_used"`
	UseCount  int       `json:"use_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table backing SearchSuggestion.
func (SearchSuggestion) TableName() string {
	return MainTableName
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (s *SearchSuggestion) UpdateKeys() error {
	s.PK = fmt.Sprintf("SEARCH_SUGGEST#%s", s.Type)
	s.SK = s.Term
	return nil
}

// GetPK returns the partition key
func (s *SearchSuggestion) GetPK() string {
	return s.PK
}

// GetSK returns the sort key
func (s *SearchSuggestion) GetSK() string {
	return s.SK
}
