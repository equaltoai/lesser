package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// SearchAnalytics represents search event analytics
type SearchAnalytics struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Fields
	Query         string    `json:"query"`
	ResultCount   int       `json:"result_count"`
	SearchTime    int64     `json:"search_time"`       // milliseconds
	UserID        *string   `json:"user_id,omitempty"` // optional
	Timestamp     time.Time `json:"timestamp"`
	ClickedResult *string   `json:"clicked_result,omitempty"`     // optional
	SearchType    string    `json:"search_type"`                  // accounts, statuses, hashtags
	TTL           int64     `json:"ttl,omitempty" dynamorm:"ttl"` // 90 days expiration
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (s *SearchAnalytics) UpdateKeys() {
	date := s.Timestamp.Format(common.DateFormat)
	s.PK = fmt.Sprintf("SEARCH_LOG#%s", date)
	s.SK = fmt.Sprintf("%d#%s#%s", s.Timestamp.Unix(), s.SearchType, s.Query)

	// Set TTL to 90 days from creation
	if s.TTL == 0 {
		s.TTL = s.Timestamp.Add(90 * 24 * time.Hour).Unix()
	}
}
