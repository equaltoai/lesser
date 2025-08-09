package models

import (
	"fmt"
	"time"
)

// ConversationStatus represents a user's read status for a conversation
// This tracks whether a user has unread messages in a conversation
type ConversationStatus struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // CONVERSATION_STATUS#conversationID
	SK string `dynamorm:"sk" json:"SK"` // USER#username

	// Core fields from storage.ConversationStatus
	ConversationID string    `json:"conversation_id"`
	UserID         string    `json:"user_id"` // username
	Unread         bool      `json:"unread"`
	LastReadAt     time.Time `json:"last_read_at"`
}

// TableName returns the DynamoDB table name
func (ConversationStatus) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a status record
func (s *ConversationStatus) BeforeCreate() error {
	if s.ConversationID == "" {
		return fmt.Errorf("conversation ID is required")
	}
	if s.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	s.PK = fmt.Sprintf("CONVERSATION_STATUS#%s", s.ConversationID)
	s.SK = fmt.Sprintf(KeyPatternUser, s.UserID)

	if s.LastReadAt.IsZero() {
		s.LastReadAt = time.Now()
	}

	return nil
}

// ConversationMessage represents a message/status within a conversation
// Note: Based on the legacy code and instructions, this appears to be handled
// differently - messages are stored as regular Status objects with conversation context
// The instructions mention PK=CONVERSATION#id, SK=STATUS#timestamp#statusID
// but the legacy code doesn't show this pattern being used.
// This model is included for completeness based on the instructions.
type ConversationMessage struct {
	// Primary keys as specified in instructions
	PK string `dynamorm:"pk" json:"PK"` // CONVERSATION#conversationID
	SK string `dynamorm:"sk" json:"SK"` // STATUS#timestamp#statusID

	// Fields from instructions
	ConversationID string               `json:"conversation_id"`
	StatusID       string               `json:"status_id"`
	SenderUsername string               `json:"sender_username"`
	CreatedAt      time.Time            `json:"created_at"`
	ReadBy         map[string]time.Time `json:"read_by,omitempty"` // username -> read timestamp

	// TTL for message retention (optional)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table name
func (ConversationMessage) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a message record
func (m *ConversationMessage) BeforeCreate() error {
	if m.ConversationID == "" {
		return fmt.Errorf("conversation ID is required")
	}
	if m.StatusID == "" {
		return fmt.Errorf("status ID is required")
	}

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	m.PK = fmt.Sprintf(KeyPatternConversation, m.ConversationID)
	m.SK = fmt.Sprintf("STATUS#%s#%s", m.CreatedAt.Format(time.RFC3339Nano), m.StatusID)

	if m.ReadBy == nil {
		m.ReadBy = make(map[string]time.Time)
	}

	return nil
}
