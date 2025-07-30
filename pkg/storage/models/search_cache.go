package models

import (
	"fmt"
	"time"
)

// SearchCache represents cached search results
type SearchCache struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Fields
	Query     string                 `json:"query"`      // original query
	Results   map[string]interface{} `json:"results"`    // cached search results
	CreatedAt time.Time              `json:"created_at"`
	TTL       int64                  `json:"ttl,omitempty" dynamorm:"ttl"` // Unix timestamp
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (s *SearchCache) UpdateKeys() {
	s.PK = fmt.Sprintf("SEARCH_CACHE#%s", s.Query)
	s.SK = "RESULTS"
}