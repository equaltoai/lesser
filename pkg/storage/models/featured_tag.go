package models

import (
	"fmt"
	"time"
)

// FeaturedTag represents a featured hashtag for a user
type FeaturedTag struct {
	// Primary key: USER#username
	PK string `dynamorm:"pk"`
	// Sort key: FEATURED_TAG#id
	SK string `dynamorm:"sk"`

	// Core fields (must match legacy exactly)
	ID            string    `json:"id"`
	Username      string    `json:"username"`       // Who featured the tag
	Name          string    `json:"name"`           // The tag name (without #)
	URL           string    `json:"url"`            // URL to the tag
	StatusesCount int       `json:"statuses_count"` // Number of statuses with this tag
	LastStatusAt  string    `json:"last_status_at"` // Last time the user posted with this tag
	CreatedAt     time.Time `json:"created_at"`
}

// UpdateKeys sets the PK and SK based on the username and ID
func (f *FeaturedTag) UpdateKeys() {
	f.PK = fmt.Sprintf("USER#%s", f.Username)
	f.SK = fmt.Sprintf("FEATURED_TAG#%s", f.ID)
}