package models

import (
	"fmt"
	"time"
)

// SearchSuggestion represents search autocomplete suggestions
type SearchSuggestion struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Fields
	Type      string    `theorydb:"attr:type" json:"type"`   // hashtag, account, username, display_name
	Term      string    `theorydb:"attr:term" json:"term"`   // the search term
	Score     float64   `theorydb:"attr:score" json:"score"` // popularity/relevance score
	LastUsed  time.Time `theorydb:"attr:lastUsed" json:"last_used"`
	UseCount  int       `theorydb:"attr:useCount" json:"use_count"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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
