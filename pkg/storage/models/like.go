package models

import (
	"fmt"
	"time"
)

// Like represents a Like activity in DynamoDB
type Like struct {
	// Primary key - by object and actor (to prevent duplicate likes)
	PK string `dynamorm:"pk" json:"pk"` // Format: "object#{object_id}#likes"
	SK string `dynamorm:"sk" json:"sk"` // Format: "actor#{actor_id}"

	// GSI1 - by actor (to list user's likes)
	GSI1PK string `dynamorm:"index:gsi1-index,pk" json:"gsi1_pk"` // Format: "actor#{actor_id}#likes"
	GSI1SK string `dynamorm:"index:gsi1-index,sk" json:"gsi1_sk"` // Format: "{timestamp}#{object_id}"

	// Like data
	ID        string    `json:"id"`         // Like activity ID
	Actor     string    `json:"actor"`      // Who liked (actor URL)
	Object    string    `json:"object"`     // What was liked (object URL)
	Published time.Time `json:"published"`  // When it was liked
	CreatedAt time.Time `json:"created_at"` // When stored in DB
}

// NewLike creates a new like with proper key structure
func NewLike(actor, object string) *Like {
	now := time.Now()
	id := fmt.Sprintf("%s/activities/like-%d", actor, now.UnixNano())

	like := &Like{
		PK:        fmt.Sprintf("object#%s#likes", object),
		SK:        fmt.Sprintf("actor#%s", actor),
		GSI1PK:    fmt.Sprintf("actor#%s#likes", actor),
		GSI1SK:    fmt.Sprintf("%s#%s", now.Format(time.RFC3339), object),
		ID:        id,
		Actor:     actor,
		Object:    object,
		Published: now,
		CreatedAt: now,
	}

	return like
}
