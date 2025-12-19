package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// CSRFToken represents a CSRF token stored in DynamoDB
type CSRFToken struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - using token as partition key for fast lookups
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "CSRF#{token}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "TOKEN"

	// GSI1 - User CSRF tokens lookup for rate limiting
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "USER_CSRF#{userID}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{created_at}#{token}"

	// Core CSRF token data
	Token     string `dynamorm:"attr:token" json:"token"`
	UserID    string `dynamorm:"attr:userID" json:"user_id"`
	CreatedAt int64  `dynamorm:"attr:createdAt" json:"created_at"` // Unix timestamp
	ExpiresAt int64  `dynamorm:"attr:expiresAt" json:"expires_at"` // Unix timestamp
	Used      bool   `dynamorm:"attr:used" json:"used"`

	// TTL for automatic cleanup - DynamoDB will automatically delete expired tokens
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl"` // Same as ExpiresAt for automatic cleanup
}

// TableName returns the DynamoDB table name for the CSRFToken model
func (CSRFToken) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (c *CSRFToken) BeforeCreate() error {
	now := time.Now()

	// Set timestamps if not already set
	if c.CreatedAt == 0 {
		c.CreatedAt = now.Unix()
	}

	// Set default expiry to 1 hour if not specified (matching legacy behavior)
	if c.ExpiresAt == 0 {
		c.ExpiresAt = now.Add(1 * time.Hour).Unix()
	}

	// Set TTL to same as expiry for automatic cleanup
	c.TTL = c.ExpiresAt

	// Set up primary key - CRITICAL: Use exact format from legacy
	c.PK = "CSRF#" + c.Token
	c.SK = SKToken

	// Set up GSI keys for user lookups (rate limiting)
	if err := c.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return c.Validate()
}

// BeforeUpdate sets up the model before update
func (c *CSRFToken) BeforeUpdate() error {
	// Update GSI keys in case user or other indexed fields changed
	if err := c.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Ensure TTL matches expiry
	c.TTL = c.ExpiresAt

	return c.Validate()
}

// GetPK returns the partition key for BaseModel interface
func (c *CSRFToken) GetPK() string {
	return c.PK
}

// GetSK returns the sort key for BaseModel interface
func (c *CSRFToken) GetSK() string {
	return c.SK
}

// UpdateKeys configures all GSI partition and sort keys
func (c *CSRFToken) UpdateKeys() error {
	// Validate required fields
	if err := common.ValidateRequiredParam("c.Token", strings.TrimSpace(c.Token)); err != nil {
		return ErrCSRFTokenRequired
	}

	// Set primary keys
	c.PK = "CSRF#" + c.Token
	c.SK = SKToken

	// GSI1 - User CSRF tokens lookup for rate limiting and cleanup
	if c.UserID != "" {
		c.GSI1PK = "USER_CSRF#" + c.UserID
		c.GSI1SK = fmt.Sprintf("%d#%s", c.CreatedAt, c.Token)
	} else {
		c.GSI1PK = ""
		c.GSI1SK = ""
	}
	return nil
}

// Validate performs validation on the CSRFToken
func (c *CSRFToken) Validate() error {
	if err := common.ValidateRequiredParam("strings.TrimSpace(c.Token)", strings.TrimSpace(c.Token)); err != nil {
		return ErrCSRFTokenRequired
	}
	if err := common.ValidateRequiredParam("strings.TrimSpace(c.UserID)", strings.TrimSpace(c.UserID)); err != nil {
		return ErrCSRFUserIDRequired
	}
	if c.ExpiresAt <= 0 {
		return ErrCSRFExpiresAtRequired
	}
	if c.CreatedAt <= 0 {
		return ErrCSRFCreatedAtRequired
	}
	if c.ExpiresAt <= c.CreatedAt {
		return ErrCSRFInvalidTimeRange
	}

	return nil
}

// IsExpired returns true if the token has expired
func (c *CSRFToken) IsExpired() bool {
	return time.Unix(c.ExpiresAt, 0).Before(time.Now())
}

// IsValid returns true if the token is valid (not used and not expired)
func (c *CSRFToken) IsValid() bool {
	if c.Used {
		return false
	}
	return !c.IsExpired()
}

// MarkAsUsed marks the token as used (single-use tokens)
func (c *CSRFToken) MarkAsUsed() {
	c.Used = true
}

// RemainingTime returns the time until the token expires
func (c *CSRFToken) RemainingTime() time.Duration {
	return time.Until(time.Unix(c.ExpiresAt, 0))
}
