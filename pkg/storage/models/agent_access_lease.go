package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	agentAccessLeasePKPrefix      = "AGENT_ACCESS_LEASE#"
	agentAccessLeaseSKPrefix      = "LEASE#"
	agentAccessChallengePKPrefix  = "AGENT_ACCESS_CHALLENGE#"
	agentAccessChallengeSK        = "CHALLENGE"
	agentAccessLeaseStatusActive  = "active"
	agentAccessLeaseStatusRevoked = "revoked"
)

// AgentAccessLease represents a durable, wallet-backed local access lease for an agent.
type AgentAccessLease struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	ID                   string    `theorydb:"attr:id" json:"id"`
	Username             string    `theorydb:"attr:username" json:"username"`
	PrincipalUsername    string    `theorydb:"attr:principalUsername" json:"principal_username"`
	PrincipalWallet      string    `theorydb:"attr:principalWallet" json:"principal_wallet"`
	AgentWallet          string    `theorydb:"attr:agentWallet" json:"agent_wallet"`
	Scopes               []string  `theorydb:"attr:scopes" json:"scopes"`
	DeviceLabel          string    `theorydb:"attr:deviceLabel" json:"device_label"`
	Status               string    `theorydb:"attr:status" json:"status"`
	IdleTimeoutHours     int       `theorydb:"attr:idleTimeoutHours" json:"idle_timeout_hours"`
	IdleExpiresAt        time.Time `theorydb:"attr:idleExpiresAt" json:"idle_expires_at"`
	AbsoluteExpiresAt    time.Time `theorydb:"attr:absoluteExpiresAt" json:"absolute_expires_at"`
	LastUsedAt           time.Time `theorydb:"attr:lastUsedAt" json:"last_used_at"`
	LeaseVersion         int       `theorydb:"attr:leaseVersion" json:"lease_version"`
	SessionPublicKey     string    `theorydb:"attr:sessionPublicKey" json:"session_public_key,omitempty"`
	SessionKeyType       string    `theorydb:"attr:sessionKeyType" json:"session_key_type,omitempty"`
	SessionKeyCreatedAt  time.Time `theorydb:"attr:sessionKeyCreatedAt" json:"session_key_created_at,omitempty"`
	SessionKeyLastUsedAt time.Time `theorydb:"attr:sessionKeyLastUsedAt" json:"session_key_last_used_at,omitempty"`
	CreatedAt            time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt            time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	RevokedAt            time.Time `theorydb:"attr:revokedAt" json:"revoked_at,omitempty"`
	RevokedBy            string    `theorydb:"attr:revokedBy" json:"revoked_by,omitempty"`
	RevokedReason        string    `theorydb:"attr:revokedReason" json:"revoked_reason,omitempty"`
}

// TableName returns the DynamoDB table backing AgentAccessLease.
func (AgentAccessLease) TableName() string {
	return MainTableName
}

// BeforeCreate derives keys and timestamps before persistence.
func (l *AgentAccessLease) BeforeCreate() error {
	if l == nil {
		return fmt.Errorf("nil model")
	}

	now := time.Now().UTC()
	if l.CreatedAt.IsZero() {
		l.CreatedAt = now
	}
	if l.UpdatedAt.IsZero() {
		l.UpdatedAt = l.CreatedAt
	}
	if l.LastUsedAt.IsZero() {
		l.LastUsedAt = l.CreatedAt
	}
	if l.LeaseVersion <= 0 {
		l.LeaseVersion = 1
	}
	if strings.TrimSpace(l.Status) == "" {
		l.Status = agentAccessLeaseStatusActive
	}
	return l.UpdateKeys()
}

// BeforeUpdate refreshes timestamps before persistence.
func (l *AgentAccessLease) BeforeUpdate() error {
	if l == nil {
		return fmt.Errorf("nil model")
	}
	l.UpdatedAt = time.Now().UTC()
	return l.UpdateKeys()
}

// GetPK returns the partition key.
func (l *AgentAccessLease) GetPK() string {
	return l.PK
}

// GetSK returns the sort key.
func (l *AgentAccessLease) GetSK() string {
	return l.SK
}

// UpdateKeys derives the storage keys and TTL.
func (l *AgentAccessLease) UpdateKeys() error {
	if l == nil {
		return fmt.Errorf("nil model")
	}

	l.ID = strings.TrimSpace(l.ID)
	l.Username = strings.TrimSpace(l.Username)
	if l.ID == "" || l.Username == "" {
		return fmt.Errorf("missing required fields")
	}

	l.PK = agentAccessLeasePKPrefix + l.Username
	l.SK = agentAccessLeaseSKPrefix + l.ID
	if !l.AbsoluteExpiresAt.IsZero() {
		l.TTL = l.AbsoluteExpiresAt.UTC().Unix()
	}
	return nil
}

// AgentAccessLeaseChallenge represents a one-time wallet-signing challenge for lease enrollment or renewal.
type AgentAccessLeaseChallenge struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	ID                string    `theorydb:"attr:id" json:"id"`
	LeaseID           string    `theorydb:"attr:leaseID" json:"lease_id"`
	Username          string    `theorydb:"attr:username" json:"username"`
	Action            string    `theorydb:"attr:action" json:"action"`
	Address           string    `theorydb:"attr:address" json:"address"`
	PrincipalUsername string    `theorydb:"attr:principalUsername" json:"principal_username"`
	PrincipalWallet   string    `theorydb:"attr:principalWallet" json:"principal_wallet"`
	AgentWallet       string    `theorydb:"attr:agentWallet" json:"agent_wallet"`
	SessionPublicKey  string    `theorydb:"attr:sessionPublicKey" json:"session_public_key,omitempty"`
	SessionKeyType    string    `theorydb:"attr:sessionKeyType" json:"session_key_type,omitempty"`
	Scopes            []string  `theorydb:"attr:scopes" json:"scopes"`
	DeviceLabel       string    `theorydb:"attr:deviceLabel" json:"device_label"`
	IdleTimeoutHours  int       `theorydb:"attr:idleTimeoutHours" json:"idle_timeout_hours"`
	AbsoluteTTLHours  int       `theorydb:"attr:absoluteTTLHours" json:"absolute_ttl_hours"`
	Nonce             string    `theorydb:"attr:nonce" json:"nonce"`
	Message           string    `theorydb:"attr:message" json:"message"`
	IssuedAt          time.Time `theorydb:"attr:issuedAt" json:"issued_at"`
	ExpiresAt         time.Time `theorydb:"attr:expiresAt" json:"expires_at"`
	Used              bool      `theorydb:"attr:used" json:"used"`
}

// TableName returns the DynamoDB table backing AgentAccessLeaseChallenge.
func (AgentAccessLeaseChallenge) TableName() string {
	return MainTableName
}

// BeforeCreate derives keys and timestamps before persistence.
func (c *AgentAccessLeaseChallenge) BeforeCreate() error {
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
func (c *AgentAccessLeaseChallenge) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *AgentAccessLeaseChallenge) GetSK() string {
	return c.SK
}

// UpdateKeys derives the storage keys and TTL.
func (c *AgentAccessLeaseChallenge) UpdateKeys() error {
	if c == nil {
		return fmt.Errorf("nil model")
	}

	c.ID = strings.TrimSpace(c.ID)
	c.LeaseID = strings.TrimSpace(c.LeaseID)
	c.Username = strings.TrimSpace(c.Username)
	c.Action = strings.TrimSpace(c.Action)
	c.Address = strings.TrimSpace(strings.ToLower(c.Address))
	c.PrincipalUsername = strings.TrimSpace(c.PrincipalUsername)
	c.PrincipalWallet = strings.TrimSpace(strings.ToLower(c.PrincipalWallet))
	c.AgentWallet = strings.TrimSpace(strings.ToLower(c.AgentWallet))
	c.SessionPublicKey = strings.TrimSpace(c.SessionPublicKey)
	c.SessionKeyType = strings.TrimSpace(c.SessionKeyType)
	c.DeviceLabel = strings.TrimSpace(c.DeviceLabel)

	if c.ID == "" || c.LeaseID == "" || c.Username == "" || c.Action == "" || strings.TrimSpace(c.Message) == "" || c.ExpiresAt.IsZero() {
		return fmt.Errorf("missing required fields")
	}
	if c.Address == "" && c.SessionPublicKey == "" {
		return fmt.Errorf("missing required fields")
	}

	c.PK = agentAccessChallengePKPrefix + c.ID
	c.SK = agentAccessChallengeSK
	c.TTL = c.ExpiresAt.UTC().Unix()
	return nil
}
