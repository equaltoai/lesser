package models

import (
	"fmt"
	"time"
)

// Bookmark represents a user's bookmark of a status/object
type Bookmark struct {
	// DynamoDB keys
	PK string `dynamorm:"pk" json:"-"`        // BOOKMARK#username
	SK string `dynamorm:"sk" json:"-"`        // timestamp#objectID

	// Core fields
	Username  string    `json:"username"`
	ObjectID  string    `json:"object_id"`
	CreatedAt time.Time `json:"created_at"`
	
	// TTL field for automatic cleanup (optional)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the DynamoDB partition and sort keys for the bookmark
func (b *Bookmark) UpdateKeys() {
	// PK: BOOKMARK#username (matches legacy pattern exactly)
	b.PK = fmt.Sprintf("BOOKMARK#%s", b.Username)
	
	// SK: timestamp#objectID (matches legacy pattern exactly)
	// Use RFC3339Nano for timestamp precision matching legacy
	b.SK = fmt.Sprintf("%s#%s", b.CreatedAt.Format(time.RFC3339Nano), b.ObjectID)
}