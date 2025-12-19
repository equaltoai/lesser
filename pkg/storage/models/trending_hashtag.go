package models

import (
	"fmt"
	"time"
)

// TrendingHashtag tracks trending hashtags with usage statistics
type TrendingHashtag struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Key fields - EXACT pattern from legacy: PK=`TRENDING#date`, SK=`HASHTAG#score#tag`
	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	// GSI fields for trending queries
	GSI8PK string `dynamorm:"index:gsi8,pk,attr:gsi8PK"`
	GSI8SK string `dynamorm:"index:gsi8,sk,attr:gsi8SK"`

	// Business fields from legacy
	Hashtag   string    `dynamorm:"attr:hashtag" json:"hashtag"`
	Date      string    `dynamorm:"attr:date" json:"date"`   // YYYY-MM-DD format
	Score     float64   `dynamorm:"attr:score" json:"score"` // trending score
	UseCount  int64     `dynamorm:"attr:useCount" json:"use_count"`
	UserCount int64     `dynamorm:"attr:userCount" json:"user_count"` // unique users
	History   []float64 `dynamorm:"attr:history" json:"history"`      // 7-day trend
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL field - 30 days as per legacy
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
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
