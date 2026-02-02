package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// OutboxItem represents an activity created by an actor (in their outbox)
type OutboxItem struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// GSI fields for public outbox queries
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"`

	// Business fields
	ActorID    string                `theorydb:"attr:actorID" json:"actor_id"`       // The actor who created the activity
	ActivityID string                `theorydb:"attr:activityID" json:"activity_id"` // The activity ID
	Activity   *activitypub.Activity `theorydb:"attr:activity" json:"activity"`      // The full activity object
	Timestamp  time.Time             `theorydb:"attr:timestamp" json:"timestamp"`    // When the activity was created
	CreatedAt  time.Time             `theorydb:"attr:createdAt" json:"created_at"`
	Public     bool                  `theorydb:"attr:public" json:"public"` // Whether this is a public activity
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
