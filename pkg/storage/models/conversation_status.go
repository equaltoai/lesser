package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// ConversationStatus represents a user's read status for a conversation.
// This tracks whether a user has unread messages in a conversation.
//
// Legacy note: DM rewrite M4 removes ConversationStatus as the canonical owner of DM unread state.
type ConversationStatus struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `theorydb:"pk,attr:PK" json:"PK"` // CONVERSATION_STATUS#conversationID
	SK string `theorydb:"sk,attr:SK" json:"SK"` // USER#username

	// Core fields from storage.ConversationStatus
	ConversationID string    `theorydb:"attr:conversationID" json:"conversation_id"`
	UserID         string    `theorydb:"attr:userID" json:"user_id"` // username
	Unread         bool      `theorydb:"attr:unread" json:"unread"`
	LastReadAt     time.Time `theorydb:"attr:lastReadAt" json:"last_read_at"`
}

// TableName returns the DynamoDB table name
func (ConversationStatus) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a status record
func (s *ConversationStatus) BeforeCreate() error {
	if err := common.ValidateRequiredParam("ConversationID", s.ConversationID); err != nil {
		return ErrConversationStatusIDRequired
	}
	if err := common.ValidateRequiredParam("UserID", s.UserID); err != nil {
		return ErrConversationStatusUserIDRequired
	}

	s.UserID = CanonicalConversationParticipantID(s.UserID)
	s.PK = fmt.Sprintf("CONVERSATION_STATUS#%s", s.ConversationID)
	s.SK = fmt.Sprintf(KeyPatternUser, s.UserID)

	if s.LastReadAt.IsZero() {
		s.LastReadAt = time.Now()
	}

	return nil
}

// UpdateKeys updates the composite keys based on conversation ID and user ID
func (s *ConversationStatus) UpdateKeys() error {
	if s.ConversationID != "" && s.UserID != "" {
		s.UserID = CanonicalConversationParticipantID(s.UserID)
		s.PK = fmt.Sprintf("CONVERSATION_STATUS#%s", s.ConversationID)
		s.SK = fmt.Sprintf(KeyPatternUser, s.UserID)
	}
	return nil
}

// GetPK returns the partition key
func (s *ConversationStatus) GetPK() string {
	return s.PK
}

// GetSK returns the sort key
func (s *ConversationStatus) GetSK() string {
	return s.SK
}

// ConversationMessage represents a message/status within a conversation.
// Note: Based on the legacy code and instructions, this appears to be handled
// differently - messages are stored as regular Status objects with conversation context
// The instructions mention PK=CONVERSATION#id, SK=STATUS#timestamp#statusID
// but the legacy code doesn't show this pattern being used.
// This model is included for completeness based on the instructions.
//
// Legacy note: DM rewrite M3/M8 removes ConversationMessage from live DM write logic.
type ConversationMessage struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys as specified in instructions
	PK string `theorydb:"pk,attr:PK" json:"PK"` // CONVERSATION#conversationID
	SK string `theorydb:"sk,attr:SK" json:"SK"` // STATUS#timestamp#statusID

	// Fields from instructions
	ConversationID string               `theorydb:"attr:conversationID" json:"conversation_id"`
	StatusID       string               `theorydb:"attr:statusID" json:"status_id"`
	SenderUsername string               `theorydb:"attr:senderUsername" json:"sender_username"`
	CreatedAt      time.Time            `theorydb:"attr:createdAt" json:"created_at"`
	ReadBy         map[string]time.Time `theorydb:"attr:readBy" json:"read_by,omitempty"` // username -> read timestamp

	// TTL for message retention (optional)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ConversationMessage) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a message record
func (m *ConversationMessage) BeforeCreate() error {
	if err := common.ValidateRequiredParam("ConversationID", m.ConversationID); err != nil {
		return ErrConversationStatusIDRequired
	}
	if err := common.ValidateRequiredParam("StatusID", m.StatusID); err != nil {
		return ErrConversationStatusStatusIDRequired
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

// UpdateKeys updates the composite keys based on conversation ID and status ID
func (m *ConversationMessage) UpdateKeys() error {
	if m.ConversationID != "" && m.StatusID != "" {
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}
		m.PK = fmt.Sprintf(KeyPatternConversation, m.ConversationID)
		m.SK = fmt.Sprintf("STATUS#%s#%s", m.CreatedAt.Format(time.RFC3339Nano), m.StatusID)
	}
	return nil
}

// GetPK returns the partition key
func (m *ConversationMessage) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *ConversationMessage) GetSK() string {
	return m.SK
}
