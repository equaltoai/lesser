package models

import (
	"time"
)

// Activity represents an ActivityPub activity in DynamoDB
type Activity struct {
	// Primary key - using composite key for activities
	PK string `dynamorm:"pk" json:"pk"` // Format: "activity#{activity_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "{direction}#{username}#{timestamp}"

	// GSI for username queries
	Username string `dynamorm:"index:username-index,pk" json:"username"`

	// GSI for timestamp sorting
	Timestamp string `dynamorm:"index:timestamp-index,sk" json:"timestamp"`

	// Activity data
	ActivityID  string     `json:"activity_id"`
	Activity    string     `json:"activity"`  // JSON string of the activity
	Direction   string     `json:"direction"` // "inbox" or "outbox"
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}
