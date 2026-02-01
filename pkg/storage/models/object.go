package models

import (
	"fmt"
	"time"
)

// Object represents a generic ActivityPub object in DynamoDB
// This is used for storing various object types (Note, Article, etc.)
type Object struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - object by ID
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "object#{id}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "object#{id}"

	// GSI1 - by actor
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "actor#{actor_id}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "object#{published}#{id}"

	// GSI2 - by type
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "object#type#{type}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{published}#{id}"

	// GSI6 - for replies (used when InReplyTo is set)
	GSI6PK string `theorydb:"index:gsi6,pk,attr:gsi6PK" json:"gsi6_pk,omitempty"` // Format: "REPLIES#{parent_object_id}"
	GSI6SK string `theorydb:"index:gsi6,sk,attr:gsi6SK" json:"gsi6_sk,omitempty"` // Format: "{timestamp}#{id}"

	// Object data - stored as JSON
	ID           string    `theorydb:"attr:id" json:"id"`
	Type         string    `theorydb:"attr:type" json:"type"`
	AttributedTo string    `theorydb:"attr:attributedTo" json:"attributed_to,omitempty"`
	Content      string    `theorydb:"attr:content" json:"content,omitempty"`
	Name         string    `theorydb:"attr:name" json:"name,omitempty"`
	Summary      string    `theorydb:"attr:summary" json:"summary,omitempty"`
	URL          string    `theorydb:"attr:url" json:"url,omitempty"`
	Published    time.Time `theorydb:"attr:published" json:"published"`
	Updated      time.Time `theorydb:"attr:updated" json:"updated"`
	InReplyTo    *string   `theorydb:"attr:inReplyTo" json:"in_reply_to,omitempty"`
	Sensitive    bool      `theorydb:"attr:sensitive" json:"sensitive,omitempty"`

	// Addressing
	To  []string `theorydb:"attr:to" json:"to,omitempty"`
	CC  []string `theorydb:"attr:cc" json:"cc,omitempty"`
	BTo []string `theorydb:"attr:bto" json:"bto,omitempty"`
	BCC []string `theorydb:"attr:bcc" json:"bcc,omitempty"`

	// Additional fields stored as JSON
	AttachmentJSON string `theorydb:"attr:attachmentJSON" json:"attachment_json,omitempty"` // JSON array of attachments
	TagJSON        string `theorydb:"attr:tagJSON" json:"tag_json,omitempty"`               // JSON array of tags
	ContextJSON    string `theorydb:"attr:contextJSON" json:"context_json,omitempty"`       // JSON for @context

	// Metadata
	ConversationID string    `theorydb:"attr:conversationID" json:"conversation_id,omitempty"`
	Visibility     string    `theorydb:"attr:visibility" json:"visibility,omitempty"`
	IsRemote       bool      `theorydb:"attr:isRemote" json:"is_remote"`
	CreatedAt      time.Time `theorydb:"attr:createdAt" json:"created_at"`
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
