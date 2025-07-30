package models

import (
	"fmt"
	"time"
	
	"github.com/equaltoai/lesser/pkg/activitypub"
)

// InboxItem represents an activity delivered to an actor's inbox
type InboxItem struct {
	// Primary key fields
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`
	
	// GSI fields for inbox queries
	GSI1PK string `dynamorm:"index:gsi1,pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk"`
	
	// Business fields
	ActorID    string                `json:"actor_id"`    // The recipient actor ID
	ActivityID string                `json:"activity_id"` // The activity ID
	Activity   *activitypub.Activity `json:"activity"`    // The full activity object
	Timestamp  time.Time             `json:"timestamp"`   // When the activity was received
	CreatedAt  time.Time             `json:"created_at"`
}

// UpdateKeys updates the composite keys based on the inbox item data
func (i *InboxItem) UpdateKeys() {
	// Primary key pattern: ACTOR#actorID, SK: ACTIVITY#timestamp#activityID
	i.PK = fmt.Sprintf("ACTOR#%s", i.ActorID)
	i.SK = fmt.Sprintf("ACTIVITY#%s#%s", i.Timestamp.Format(time.RFC3339Nano), i.ActivityID)
	
	// GSI1 for inbox queries
	i.GSI1PK = fmt.Sprintf("INBOX#%s", i.ActorID)
	i.GSI1SK = i.Timestamp.Format(time.RFC3339Nano)
}