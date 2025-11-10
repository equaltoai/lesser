package models

import (
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// Activity represents an ActivityPub activity in DynamoDB
// This matches the legacy storage.ActivityRecord structure
type Activity struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - MUST match legacy patterns exactly
	PK string `dynamorm:"pk,attr:PK" json:"PK"` // Format: "ACTOR#{username}"
	SK string `dynamorm:"sk,attr:SK" json:"SK"` // Format: "ACTIVITY#{timestamp}#{activity_id}"

	// GSI for inbox activities - MUST match legacy patterns exactly
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"GSI1PK,omitempty"` // Format: "INBOX#{username}" (only for inbox activities)
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"GSI1SK,omitempty"` // Format: timestamp (only for inbox activities)

	// Activity data
	Activity  *activitypub.Activity `dynamorm:"attr:activity" json:"Activity"`
	CreatedAt time.Time             `dynamorm:"attr:createdAt" json:"CreatedAt"`
}

// TableName returns the DynamoDB table backing Activity.
func (Activity) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (a *Activity) GetPK() string {
	return a.PK
}

// GetSK returns the sort key
func (a *Activity) GetSK() string {
	return a.SK
}

// UpdateKeys updates the PK/SK and GSI keys for activities
// This method sets up the key structure for ActivityPub activities
// Key patterns MUST match legacy exactly:
// - PK: "ACTOR#{username}"
// - SK: "ACTIVITY#{timestamp}#{activity_id}"
// - GSI1PK: "INBOX#{username}" (only for inbox activities)
// - GSI1SK: timestamp (only for inbox activities)
func (a *Activity) UpdateKeys() error {
	// Extract username from SK or activity data if not already set in PK
	if a.PK == "" && a.Activity != nil {
		// Extract username from actor ID (e.g., "https://example.com/users/alice" -> "alice")
		actorParts := strings.Split(a.Activity.Actor, "/")
		if len(actorParts) >= 2 {
			username := actorParts[len(actorParts)-1]
			a.PK = "ACTOR#" + username
		}
	}

	// Set SK if not already set
	if a.SK == "" && a.Activity != nil {
		timestamp := a.CreatedAt.Format(time.RFC3339Nano)
		a.SK = "ACTIVITY#" + timestamp + "#" + a.Activity.ID
	}

	// GSI keys are set conditionally based on whether it's an inbox activity
	// This logic is handled in the repository methods since it requires domain knowledge
	// about whether the activity is from a local user (outbox) or external user (inbox)

	return nil
}
