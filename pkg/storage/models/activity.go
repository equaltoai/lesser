package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// Activity represents an ActivityPub activity in DynamoDB
// This matches the legacy storage.ActivityRecord structure
type Activity struct {
	// Primary keys - MUST match legacy patterns exactly
	PK string `dynamorm:"pk" json:"PK"` // Format: "ACTOR#{username}"
	SK string `dynamorm:"sk" json:"SK"` // Format: "ACTIVITY#{timestamp}#{activity_id}"

	// GSI for inbox activities - MUST match legacy patterns exactly
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK,omitempty"` // Format: "INBOX#{username}" (only for inbox activities)
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK,omitempty"` // Format: timestamp (only for inbox activities)

	// Activity data
	Activity  *activitypub.Activity `json:"Activity"`
	CreatedAt time.Time             `json:"CreatedAt"`
}

// UpdateKeys updates the GSI keys for inbox activities
func (a *Activity) UpdateKeys() {
	// GSI keys are set conditionally based on whether it's an inbox activity
	// This is handled in the repository methods
}
