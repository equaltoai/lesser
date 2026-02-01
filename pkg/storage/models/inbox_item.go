package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// InboxItem represents an activity delivered to an actor's inbox
type InboxItem struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// GSI fields for inbox queries
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"`

	// Business fields
	ActorID    string                `theorydb:"attr:actorID" json:"actor_id"`       // The recipient actor ID
	ActivityID string                `theorydb:"attr:activityID" json:"activity_id"` // The activity ID
	Activity   *activitypub.Activity `theorydb:"attr:activity" json:"activity"`      // The full activity object
	Timestamp  time.Time             `theorydb:"attr:timestamp" json:"timestamp"`    // When the activity was received
	CreatedAt  time.Time             `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing InboxItem.
func (InboxItem) TableName() string {
	return MainTableName
}

// UpdateKeys updates the composite keys based on the inbox item data
func (i *InboxItem) UpdateKeys() {
	// Primary key pattern: ACTOR#actorID, SK: ACTIVITY#timestamp#activityID
	i.PK = fmt.Sprintf(KeyPatternActor, i.ActorID)
	i.SK = fmt.Sprintf("ACTIVITY#%s#%s", i.Timestamp.Format(time.RFC3339Nano), i.ActivityID)

	// GSI1 for inbox queries
	i.GSI1PK = fmt.Sprintf("INBOX#%s", i.ActorID)
	i.GSI1SK = i.Timestamp.Format(time.RFC3339Nano)
}
