package models

import (
	"fmt"
	"time"
)

// Object represents a generic ActivityPub object in DynamoDB
// This is used for storing various object types (Note, Article, etc.)
type Object struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - object by ID
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "object#{id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "object#{id}"

	// GSI1 - by actor
	GSI1PK string `dynamorm:"index:gsi1-index,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "actor#{actor_id}"
	GSI1SK string `dynamorm:"index:gsi1-index,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "object#{published}#{id}"

	// GSI2 - by type
	GSI2PK string `dynamorm:"index:gsi2-index,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "object#type#{type}"
	GSI2SK string `dynamorm:"index:gsi2-index,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{published}#{id}"

	// GSI6 - for replies (used when InReplyTo is set)
	GSI6PK string `dynamorm:"index:gsi6-index,pk,attr:gsi6PK" json:"gsi6_pk,omitempty"` // Format: "REPLIES#{parent_object_id}"
	GSI6SK string `dynamorm:"index:gsi6-index,sk,attr:gsi6SK" json:"gsi6_sk,omitempty"` // Format: "{timestamp}#{id}"

	// Object data - stored as JSON
	ID           string    `dynamorm:"attr:id" json:"id"`
	Type         string    `dynamorm:"attr:type" json:"type"`
	AttributedTo string    `dynamorm:"attr:attributedTo" json:"attributed_to,omitempty"`
	Content      string    `dynamorm:"attr:content" json:"content,omitempty"`
	Name         string    `dynamorm:"attr:name" json:"name,omitempty"`
	Summary      string    `dynamorm:"attr:summary" json:"summary,omitempty"`
	URL          string    `dynamorm:"attr:url" json:"url,omitempty"`
	Published    time.Time `dynamorm:"attr:published" json:"published"`
	Updated      time.Time `dynamorm:"attr:updated" json:"updated"`
	InReplyTo    *string   `dynamorm:"attr:inReplyTo" json:"in_reply_to,omitempty"`
	Sensitive    bool      `dynamorm:"attr:sensitive" json:"sensitive,omitempty"`

	// Addressing
	To  []string `dynamorm:"attr:to" json:"to,omitempty"`
	CC  []string `dynamorm:"attr:cc" json:"cc,omitempty"`
	BTo []string `dynamorm:"attr:bto" json:"bto,omitempty"`
	BCC []string `dynamorm:"attr:bcc" json:"bcc,omitempty"`

	// Additional fields stored as JSON
	AttachmentJSON string `dynamorm:"attr:attachmentJSON" json:"attachment_json,omitempty"` // JSON array of attachments
	TagJSON        string `dynamorm:"attr:tagJSON" json:"tag_json,omitempty"`               // JSON array of tags
	ContextJSON    string `dynamorm:"attr:contextJSON" json:"context_json,omitempty"`       // JSON for @context

	// Metadata
	ConversationID string    `dynamorm:"attr:conversationID" json:"conversation_id,omitempty"`
	Visibility     string    `dynamorm:"attr:visibility" json:"visibility,omitempty"`
	IsRemote       bool      `dynamorm:"attr:isRemote" json:"is_remote"`
	CreatedAt      time.Time `dynamorm:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing Object.
func (Object) TableName() string {
	return MainTableName
}

// NewObject creates a new object with proper key structure
func NewObject(id, objectType, actorID string) *Object {
	now := time.Now()
	obj := &Object{
		PK:           fmt.Sprintf("object#%s", id),
		SK:           fmt.Sprintf("object#%s", id),
		GSI1PK:       fmt.Sprintf("actor#%s", actorID),
		GSI1SK:       fmt.Sprintf("object#%s#%s", now.Format(time.RFC3339), id),
		GSI2PK:       fmt.Sprintf("object#type#%s", objectType),
		GSI2SK:       fmt.Sprintf("%s#%s", now.Format(time.RFC3339), id),
		ID:           id,
		Type:         objectType,
		AttributedTo: actorID,
		Published:    now,
		Updated:      now,
		CreatedAt:    now,
	}
	return obj
}

// UpdateGSIKeys updates the GSI keys based on current data
func (o *Object) UpdateGSIKeys() {
	if o.AttributedTo != "" {
		o.GSI1PK = fmt.Sprintf("actor#%s", o.AttributedTo)
		o.GSI1SK = fmt.Sprintf("object#%s#%s", o.Published.Format(time.RFC3339), o.ID)
	}
	if o.Type != "" {
		o.GSI2PK = fmt.Sprintf("object#type#%s", o.Type)
		o.GSI2SK = fmt.Sprintf("%s#%s", o.Published.Format(time.RFC3339), o.ID)
	}
	// Set GSI6 fields if this is a reply
	if o.InReplyTo != nil && *o.InReplyTo != "" {
		o.GSI6PK = fmt.Sprintf("REPLIES#%s", *o.InReplyTo)
		o.GSI6SK = fmt.Sprintf("%s#%s", o.Published.Format(time.RFC3339), o.ID)
	}
}

// UpdateKeys updates the GSI keys (required by BaseModel)
func (o *Object) UpdateKeys() error {
	// Validate required fields
	if o.ID == "" {
		return fmt.Errorf("ID is required")
	}

	// Set primary keys
	o.PK = fmt.Sprintf("object#%s", o.ID)
	o.SK = fmt.Sprintf("object#%s", o.ID)

	// Update GSI keys
	o.UpdateGSIKeys()
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (o *Object) GetPK() string {
	return o.PK
}

// GetSK returns the sort key (required by BaseModel)
func (o *Object) GetSK() string {
	return o.SK
}
