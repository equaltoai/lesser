package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// TimelineEntry represents an entry in a user's timeline stored in DynamoDB
type TimelineEntry struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // TIMELINE#{type}#{id}
	SK string `dynamorm:"sk,attr:SK" json:"-"` // {timestamp}#{entryID}

	// GSI for public timeline queries
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"-"` // TIMELINE#PUBLIC#{local/federated}
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"-"` // {timestamp}#{entryID}

	// Entry data
	TimelineType string    `dynamorm:"attr:timelineType" json:"timeline_type"` // HOME, PUBLIC, LIST, DIRECT
	TimelineID   string    `dynamorm:"attr:timelineID" json:"timeline_id"`     // Username for HOME, LOCAL/FEDERATED for PUBLIC, list ID for LIST
	EntryID      string    `dynamorm:"attr:entryID" json:"entry_id"`           // Unique ID for this entry (usually timestamp + post ID)
	PostID       string    `dynamorm:"attr:postID" json:"post_id"`             // The actual post/object ID
	ActorID      string    `dynamorm:"attr:actorID" json:"actor_id"`           // Who created the post
	ActorHandle  string    `dynamorm:"attr:actorHandle" json:"actor_handle"`   // Actor's handle for quick display
	Content      string    `dynamorm:"attr:content" json:"content"`            // First 500 chars for preview
	ContentType  string    `dynamorm:"attr:contentType" json:"content_type"`   // Note, Article, etc.
	HasMedia     bool      `dynamorm:"attr:hasMedia" json:"has_media"`         // Quick flag for media
	IsReply      bool      `dynamorm:"attr:isReply" json:"is_reply"`           // Is this a reply?
	InReplyTo    string    `dynamorm:"attr:inReplyTo" json:"in_reply_to"`      // ID of post being replied to
	IsBoost      bool      `dynamorm:"attr:isBoost" json:"is_boost"`           // Is this a boost/announce?
	BoostedBy    string    `dynamorm:"attr:boostedBy" json:"boosted_by"`       // Who boosted it (if applicable)
	Visibility   string    `dynamorm:"attr:visibility" json:"visibility"`      // public, unlisted, private, direct
	Language     string    `dynamorm:"attr:language" json:"language"`          // Language code
	Sensitive    bool      `dynamorm:"attr:sensitive" json:"sensitive"`        // Content warning flag
	SpoilerText  string    `dynamorm:"attr:spoilerText" json:"spoiler_text"`   // Content warning text
	CreatedAt    time.Time `dynamorm:"attr:createdAt" json:"created_at"`       // When the post was created
	TimelineAt   time.Time `dynamorm:"attr:timelineAt" json:"timeline_at"`     // When it was added to timeline (for sorting)
	ExpiresAt    time.Time `dynamorm:"attr:expiresAt" json:"expires_at"`       // TTL for auto-deletion

	// TTL for DynamoDB auto-deletion
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the timeline entry data
func (e *TimelineEntry) UpdateKeys() {
	// Primary key
	e.PK = fmt.Sprintf("TIMELINE#%s#%s", e.TimelineType, e.TimelineID)
	e.SK = fmt.Sprintf("%d#%s", e.TimelineAt.Unix(), e.EntryID)

	// GSI for public timeline
	if e.TimelineType == "PUBLIC" {
		e.GSI1PK = fmt.Sprintf("TIMELINE#PUBLIC#%s", e.TimelineID) // LOCAL or FEDERATED
		e.GSI1SK = e.SK
	} else {
		e.GSI1PK = ""
		e.GSI1SK = ""
	}

	// Set TTL if ExpiresAt is set
	if !e.ExpiresAt.IsZero() {
		e.TTL = e.ExpiresAt.Unix()
	}
}

// SetEntryID generates a unique entry ID if not already set
func (e *TimelineEntry) SetEntryID() {
	if err := common.ValidateRequiredParam("e.EntryID", e.EntryID); err != nil {
		e.EntryID = fmt.Sprintf("%d#%s", e.TimelineAt.UnixNano(), e.PostID)
	}
}

// IsExpired checks if the timeline entry has expired
func (e *TimelineEntry) IsExpired() bool {
	return !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt)
}

// TableName returns the DynamoDB table backing TimelineEntry.
func (TimelineEntry) TableName() string {
	return MainTableName
}
