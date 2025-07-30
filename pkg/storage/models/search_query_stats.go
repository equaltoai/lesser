package models

import (
	"fmt"
	"time"
)

// SearchQueryStats represents popular query statistics
type SearchQueryStats struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Fields
	Query       string    `json:"query"`
	QueryCount  int64     `json:"query_count"`
	LastQueried time.Time `json:"last_queried"`
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (s *SearchQueryStats) UpdateKeys() {
	s.PK = "POPULAR_QUERIES"
	s.SK = fmt.Sprintf("QUERY#%s", s.Query)
}