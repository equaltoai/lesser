package models

import (
	"fmt"
	"time"
)

// Like represents a Like activity in DynamoDB
type Like struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - by object and actor (to prevent duplicate likes)
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "object#{object_id}#likes"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "actor#{actor_id}"

	// GSI1 - by actor (to list user's likes)
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "actor#{actor_id}#likes"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{timestamp}#{object_id}"

	// Like data
	ID             string    `dynamorm:"attr:id" json:"id"`                           // Like activity ID
	Actor          string    `dynamorm:"attr:actor" json:"actor"`                     // Who liked (actor URL)
	Object         string    `dynamorm:"attr:object" json:"object"`                   // What was liked (object URL)
	StatusAuthorID string    `dynamorm:"attr:statusAuthorID" json:"status_author_id"` // Author of the status being liked
	Published      time.Time `dynamorm:"attr:published" json:"published"`             // When it was liked
	CreatedAt      time.Time `dynamorm:"attr:createdAt" json:"created_at"`            // When stored in DB
}

// TableName returns the DynamoDB table backing Like.
func (Like) TableName() string {
	return MainTableName
}

// NewLike creates a new like with proper key structure
func NewLike(actor, object, statusAuthorID string) *Like {
	now := time.Now()
	id := fmt.Sprintf("%s/activities/like-%d", actor, now.UnixNano())

	like := &Like{
		ID:             id,
		Actor:          actor,
		Object:         object,
		StatusAuthorID: statusAuthorID,
		Published:      now,
		CreatedAt:      now,
	}

	// Use UpdateKeys to set all key fields consistently
	// UpdateKeys() is safe to ignore error here as it only does string formatting
	_ = like.UpdateKeys()
	return like
}

// UpdateKeys updates the primary and GSI keys based on current field values
// This ensures consistency when Actor, Object, or timestamps change
func (l *Like) UpdateKeys() error {
	l.PK = fmt.Sprintf("object#%s#likes", l.Object)
	l.SK = fmt.Sprintf("actor#%s", l.Actor)
	l.GSI1PK = fmt.Sprintf("actor#%s#likes", l.Actor)
	l.GSI1SK = fmt.Sprintf("%s#%s", l.CreatedAt.Format(time.RFC3339), l.Object)
	return nil
}

// GetPK returns the primary key for BaseRepository interface
func (l *Like) GetPK() string {
	return l.PK
}

// GetSK returns the sort key for BaseRepository interface
func (l *Like) GetSK() string {
	return l.SK
}

// GetUserID returns the actor who liked (favoriter)
func (l *Like) GetUserID() string {
	return l.Actor
}

// GetStatusID returns the object that was liked (status)
func (l *Like) GetStatusID() string {
	return l.Object
}

// GetStatusAuthorID returns the author of the status being liked
func (l *Like) GetStatusAuthorID() string {
	return l.StatusAuthorID
}
