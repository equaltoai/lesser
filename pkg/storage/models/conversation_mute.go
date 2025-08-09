package models

import (
	"fmt"
	"time"
)

// ConversationMute represents a muted conversation thread in DynamoDB
type ConversationMute struct {
	// Composite keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// TTL for auto-expiration
	TTL int64 `dynamorm:"ttl" json:"-"`

	// Business fields
	Username       string    `json:"username"`
	ConversationID string    `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
}

// TableName returns the DynamoDB table name
func (ConversationMute) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a mute
func (c *ConversationMute) BeforeCreate() error {
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	if c.ConversationID == "" {
		return fmt.Errorf("conversation ID is required")
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
func (c *ConversationMute) UpdateKeys() {
	if c.Username != "" && c.ConversationID != "" {
		c.PK = fmt.Sprintf(KeyPatternUser, c.Username)
		c.SK = fmt.Sprintf("CONVERSATION_MUTE#%s", c.ConversationID)
	}

	// Set TTL if expiration is specified
	if !c.ExpiresAt.IsZero() {
		c.TTL = c.ExpiresAt.Unix()
	}
}
