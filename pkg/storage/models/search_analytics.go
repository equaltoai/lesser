package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// SearchAnalytics represents search event analytics
type SearchAnalytics struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Fields
	Query         string    `theorydb:"attr:query" json:"query"`
	ResultCount   int       `theorydb:"attr:resultCount" json:"result_count"`
	SearchTime    int64     `theorydb:"attr:searchTime" json:"search_time"`   // milliseconds
	UserID        *string   `theorydb:"attr:userID" json:"user_id,omitempty"` // optional
	Timestamp     time.Time `theorydb:"attr:timestamp" json:"timestamp"`
	ClickedResult *string   `theorydb:"attr:clickedResult" json:"clicked_result,omitempty"` // optional
	SearchType    string    `theorydb:"attr:searchType" json:"search_type"`                 // accounts, statuses, hashtags
	TTL           int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`                  // 90 days expiration
}

// TableName returns the DynamoDB table backing SearchAnalytics.
func (SearchAnalytics) TableName() string {
	return MainTableName
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
