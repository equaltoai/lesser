package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Conversation represents a direct message conversation between users
type Conversation struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // CONVERSATION#conversationID
	SK string `dynamorm:"sk" json:"SK"` // METADATA

	// GSI1 is used for participant records (additional records per participant)
	// Note: The main conversation record doesn't use GSI1, only participant records do
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK,omitempty"`

	// Core fields from legacy
	ID           string    `json:"id"`
	Participants []string  `json:"participants"` // Actor IDs/usernames
	LastStatusID string    `json:"last_status_id,omitempty"`
	Unread       bool      `json:"unread"` // Whether conversation has unread messages
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Message counting fields
	TotalMessageCount int64     `json:"total_message_count"`         // Total messages in conversation
	LastMessageTime   time.Time `json:"last_message_time,omitempty"` // Time of last message
}

// TableName returns the DynamoDB table name
func (Conversation) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a conversation
func (c *Conversation) BeforeCreate() error {
	if err := common.ValidateRequiredParam("c.ID", c.ID); err != nil {
		return ErrConversationIDRequired
	}

	c.PK = fmt.Sprintf(KeyPatternConversation, c.ID)
	c.SK = SKMetadata

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}

	// GSI keys are not set on the main record, only on participant records
	c.GSI1PK = ""
	c.GSI1SK = ""

	return nil
}

// UpdateKeys updates GSI keys - for conversation, this is mainly used for participant records
func (c *Conversation) UpdateKeys() error {
	// Validate required fields
	if err := common.ValidateRequiredParam("c.ID", c.ID); err != nil {
		return ErrConversationIDRequired
	}

	// Set primary keys
	c.PK = fmt.Sprintf(KeyPatternConversation, c.ID)
	c.SK = SKMetadata

	// For the main conversation record, GSI keys are empty
	// GSI keys are only used for participant records which are created separately
	c.GSI1PK = ""
	c.GSI1SK = ""

	return nil
}

// GetPK returns the partition key
func (c *Conversation) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *Conversation) GetSK() string {
	return c.SK
}

// ConversationParticipantRecord represents a participant's view of a conversation
// This is used for querying conversations by user
type ConversationParticipantRecord struct {
	// Primary keys for participant record
	PK string `dynamorm:"pk" json:"PK"` // USER_CONVERSATIONS#username
	SK string `dynamorm:"sk" json:"SK"` // timestamp#conversationID (for sorting by recent)

	// GSI1 for reverse lookup (find participants by conversation)
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK"` // CONVERSATION#conversationID
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK"` // PARTICIPANT#username

	// Embed the full conversation data
	*Conversation `json:",inline"`
}

// TableName returns the DynamoDB table name
func (ConversationParticipantRecord) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys for a participant record
func (p *ConversationParticipantRecord) BeforeCreate(participantID string) error {
	if p.Conversation == nil || p.ID == "" {
		return ErrConversationDataRequired
	}

	p.PK = fmt.Sprintf("USER_CONVERSATIONS#%s", participantID)
	p.SK = fmt.Sprintf("%s#%s", p.UpdatedAt.Format(time.RFC3339), p.ID)
	p.GSI1PK = fmt.Sprintf(KeyPatternConversation, p.ID)
	p.GSI1SK = fmt.Sprintf("PARTICIPANT#%s", participantID)

	return nil
}

// UpdateKeys updates the GSI keys for the participant record
func (p *ConversationParticipantRecord) UpdateKeys() error {
	// Keys are set in BeforeCreate
	return nil
}

// GetPK returns the partition key
func (p *ConversationParticipantRecord) GetPK() string {
	return p.PK
}

// GetSK returns the sort key
func (p *ConversationParticipantRecord) GetSK() string {
	return p.SK
}

// ConversationParticipantKey is used for looking up conversations by exact participants
// Legacy uses GSI1PK = CONVERSATION_PARTICIPANTS#sorted_participants_list
type ConversationParticipantKey struct {
	PK     string `dynamorm:"pk" json:"PK"`
	SK     string `dynamorm:"sk" json:"SK"`
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK,omitempty"`

	ConversationID string `json:"conversation_id"`
}

// TableName returns the DynamoDB table name
func (ConversationParticipantKey) TableName() string {
	return MainTableName
}

// UpdateKeys updates the composite keys based on conversation ID
func (k *ConversationParticipantKey) UpdateKeys() error {
	// Keys are set when creating the lookup key
	return nil
}

// GetPK returns the partition key
func (k *ConversationParticipantKey) GetPK() string {
	return k.PK
}

// GetSK returns the sort key
func (k *ConversationParticipantKey) GetSK() string {
	return k.SK
}
