package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// ConversationMute represents a muted conversation thread in DynamoDB
type ConversationMute struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Composite keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// TTL for auto-expiration
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	// Business fields
	Username       string    `theorydb:"attr:username" json:"username"`
	ConversationID string    `theorydb:"attr:conversationID" json:"conversation_id"`
	CreatedAt      time.Time `theorydb:"attr:createdAt" json:"created_at"`
	ExpiresAt      time.Time `theorydb:"attr:expiresAt" json:"expires_at,omitempty"`
}

// TableName returns the DynamoDB table name
func (ConversationMute) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a mute
func (c *ConversationMute) BeforeCreate() error {
	if err := common.ValidateRequiredParam("c.Username", c.Username); err != nil {
		return ErrConversationMuteUsernameRequired
	}
	if err := common.ValidateRequiredParam("c.ConversationID", c.ConversationID); err != nil {
		return ErrConversationMuteConversationIDRequired
	}

	c.PK = fmt.Sprintf(KeyPatternUser, c.Username)
	c.SK = fmt.Sprintf("CONVERSATION_MUTE#%s", c.ConversationID)

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	// Set TTL if expiration is specified
	if !c.ExpiresAt.IsZero() {
		c.TTL = c.ExpiresAt.Unix()
	}

	return nil
}

// UpdateKeys updates the composite keys based on the username and conversation ID
func (c *ConversationMute) UpdateKeys() error {
	if c.Username != "" && c.ConversationID != "" {
		c.PK = fmt.Sprintf(KeyPatternUser, c.Username)
		c.SK = fmt.Sprintf("CONVERSATION_MUTE#%s", c.ConversationID)
	}

	// Set TTL if expiration is specified
	if !c.ExpiresAt.IsZero() {
		c.TTL = c.ExpiresAt.Unix()
	}
	return nil
}

// GetPK returns the partition key
func (c *ConversationMute) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *ConversationMute) GetSK() string {
	return c.SK
}
