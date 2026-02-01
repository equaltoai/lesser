package models

import (
	"fmt"
	"time"
)

// FeaturedTag represents a featured hashtag for a user
type FeaturedTag struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key: USER#username
	PK string `theorydb:"pk,attr:PK"`
	// Sort key: FEATURED_TAG#id
	SK string `theorydb:"sk,attr:SK"`

	// Core fields (must match legacy exactly)
	ID            string    `theorydb:"attr:id" json:"id"`
	Username      string    `theorydb:"attr:username" json:"username"`            // Who featured the tag
	Name          string    `theorydb:"attr:name" json:"name"`                    // The tag name (without #)
	URL           string    `theorydb:"attr:url" json:"url"`                      // URL to the tag
	StatusesCount int       `theorydb:"attr:statusesCount" json:"statuses_count"` // Number of statuses with this tag
	LastStatusAt  string    `theorydb:"attr:lastStatusAt" json:"last_status_at"`  // Last time the user posted with this tag
	CreatedAt     time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// UpdateKeys sets the PK and SK based on the username and ID
func (f *FeaturedTag) UpdateKeys() error {
	f.PK = fmt.Sprintf(KeyPatternUser, f.Username)
	f.SK = fmt.Sprintf("FEATURED_TAG#%s", f.ID)
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (f *FeaturedTag) GetPK() string {
	return f.PK
}

// GetSK returns the sort key for BaseModel interface
func (f *FeaturedTag) GetSK() string {
	return f.SK
}

// TableName returns the DynamoDB table backing FeaturedTag.
func (FeaturedTag) TableName() string {
	return MainTableName
}
