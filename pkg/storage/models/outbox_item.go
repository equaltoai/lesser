package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// OutboxItem represents an activity created by an actor (in their outbox)
type OutboxItem struct {
	// Primary key fields
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	// GSI fields for public outbox queries
	GSI1PK string `dynamorm:"index:gsi1,pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk"`

	// Business fields
	ActorID    string                `json:"actor_id"`    // The actor who created the activity
	ActivityID string                `json:"activity_id"` // The activity ID
	Activity   *activitypub.Activity `json:"activity"`    // The full activity object
	Timestamp  time.Time             `json:"timestamp"`   // When the activity was created
	CreatedAt  time.Time             `json:"created_at"`
	Public     bool                  `json:"public"` // Whether this is a public activity
}

// TableName returns the DynamoDB table backing OutboxItem.
func (OutboxItem) TableName() string {
	return MainTableName
}

// UpdateKeys updates the composite keys based on the outbox item data
func (o *OutboxItem) UpdateKeys() {
	// Primary key pattern: ACTOR#actorID, SK: ACTIVITY#timestamp#activityID
	// Note: Same pattern as inbox, distinguished by query patterns
	o.PK = fmt.Sprintf(KeyPatternActor, o.ActorID)
	o.SK = fmt.Sprintf("ACTIVITY#%s#%s", o.Timestamp.Format(time.RFC3339Nano), o.ActivityID)

	// GSI1 for public outbox queries
	if o.Public {
		o.GSI1PK = fmt.Sprintf("PUBLIC_OUTBOX#%s", o.ActorID)
		o.GSI1SK = o.Timestamp.Format(time.RFC3339Nano)
	} else {
		// Clear GSI1 keys for non-public activities
		o.GSI1PK = ""
		o.GSI1SK = ""
	}
}
