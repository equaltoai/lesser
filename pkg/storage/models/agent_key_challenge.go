package models

import (
	"fmt"
	"strings"
	"time"
)

// AgentKeyChallenge represents a server-issued challenge for self-sovereign agent authentication.
//
// It is used for registration, token minting, and key rotation flows. Challenges are short-lived
// and are marked used after successful verification to prevent replay.
type AgentKeyChallenge struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	ID        string    `theorydb:"attr:id" json:"id"`
	Username  string    `theorydb:"attr:username" json:"username"`
	Action    string    `theorydb:"attr:action" json:"action"`
	Nonce     string    `theorydb:"attr:nonce" json:"nonce"`
	Message   string    `theorydb:"attr:message" json:"message"`
	IssuedAt  time.Time `theorydb:"attr:issuedAt" json:"issued_at"`
	ExpiresAt time.Time `theorydb:"attr:expiresAt" json:"expires_at"`
	Used      bool      `theorydb:"attr:used" json:"used"`
}

// TableName returns the DynamoDB table backing AgentKeyChallenge.
func (AgentKeyChallenge) TableName() string {
	return MainTableName
}

// BeforeCreate derives keys and TTL before persistence.
func (c *AgentKeyChallenge) BeforeCreate() error {
	if c == nil {
		return fmt.Errorf("nil model")
	}

	now := time.Now().UTC()
	if c.IssuedAt.IsZero() {
		c.IssuedAt = now
	}

	return c.UpdateKeys()
}

// GetPK returns the partition key.
func (c *AgentKeyChallenge) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *AgentKeyChallenge) GetSK() string {
	return c.SK
}

// UpdateKeys derives keys and TTL.
func (c *AgentKeyChallenge) UpdateKeys() error {
	if c == nil {
		return fmt.Errorf("nil model")
	}

	c.ID = strings.TrimSpace(c.ID)
	c.Username = strings.TrimSpace(c.Username)
	c.Action = strings.TrimSpace(c.Action)

	if c.ID == "" || c.Username == "" || c.Action == "" || strings.TrimSpace(c.Message) == "" || c.ExpiresAt.IsZero() {
		return fmt.Errorf("missing required fields")
	}

	c.PK = fmt.Sprintf("AGENT_KEY_CHALLENGE#%s", c.ID)
	c.SK = "CHALLENGE"
	c.TTL = c.ExpiresAt.UTC().Unix()
	return nil
}
