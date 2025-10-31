package models

import (
	"fmt"
	"time"
)

// TrendingHashtag tracks trending hashtags with usage statistics
type TrendingHashtag struct {
	// Key fields - EXACT pattern from legacy: PK=`TRENDING#date`, SK=`HASHTAG#score#tag`
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	// GSI fields for trending queries
	GSI8PK string `dynamorm:"index:GSI8,pk"`
	GSI8SK string `dynamorm:"index:GSI8,sk"`

	// Business fields from legacy
	Hashtag   string    `json:"hashtag"`
	Date      string    `json:"date"`  // YYYY-MM-DD format
	Score     float64   `json:"score"` // trending score
	UseCount  int64     `json:"use_count"`
	UserCount int64     `json:"user_count"` // unique users
	History   []float64 `json:"history"`    // 7-day trend
	UpdatedAt time.Time `json:"updated_at"`

	// TTL field - 30 days as per legacy
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing TrendingHashtag.
func (TrendingHashtag) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys when the primary keys change
func (t *TrendingHashtag) UpdateKeys() error {
	// Validate required fields
	if t.Date == "" {
		return fmt.Errorf("date is required")
	}
	if t.Hashtag == "" {
		return fmt.Errorf("hashtag is required")
	}

	// Set primary keys
	t.PK = fmt.Sprintf("TRENDING#%s", t.Date)
	t.SK = fmt.Sprintf("HASHTAG#%f#%s", t.Score, t.Hashtag)

	// GSI8 is used for trending queries
	t.GSI8PK = t.PK
	t.GSI8SK = t.SK
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (t *TrendingHashtag) GetPK() string {
	return t.PK
}

// GetSK returns the sort key for BaseModel interface
func (t *TrendingHashtag) GetSK() string {
	return t.SK
}
