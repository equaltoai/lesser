package models

import (
	"fmt"
	"strings"
	"time"
)

// InstanceSoulBodyBindingUsername stores the reverse username -> soul binding lookup.
//
// This record lives under PK="SOUL_BODY_BINDING_USERNAME#<username>" and SK="SOUL_BODY_BINDING".
type InstanceSoulBodyBindingUsername struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	Username  string    `theorydb:"attr:username" json:"username"`
	AgentID   string    `theorydb:"attr:agentId" json:"agent_id"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing InstanceSoulBodyBindingUsername.
func (InstanceSoulBodyBindingUsername) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (b *InstanceSoulBodyBindingUsername) UpdateKeys() error {
	if b == nil {
		return fmt.Errorf("soul body binding username index is nil")
	}

	b.Username = strings.TrimSpace(b.Username)
	b.AgentID = strings.ToLower(strings.TrimSpace(b.AgentID))

	if b.Username == "" {
		return fmt.Errorf("username is required")
	}
	if b.AgentID == "" {
		return fmt.Errorf("agent id is required")
	}

	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = time.Now().UTC()
	}

	b.PK = SoulBodyBindingUsernamePartitionKey(b.Username)
	b.SK = SKSoulBodyBindingUsername
	return nil
}

// GetPK returns the partition key.
func (b *InstanceSoulBodyBindingUsername) GetPK() string {
	return b.PK
}

// GetSK returns the sort key.
func (b *InstanceSoulBodyBindingUsername) GetSK() string {
	return b.SK
}

// NewInstanceSoulBodyBindingUsername creates a new reverse username binding record.
func NewInstanceSoulBodyBindingUsername(username string, agentID string) *InstanceSoulBodyBindingUsername {
	index := &InstanceSoulBodyBindingUsername{
		Username:  strings.TrimSpace(username),
		AgentID:   strings.ToLower(strings.TrimSpace(agentID)),
		UpdatedAt: time.Now().UTC(),
	}
	_ = index.UpdateKeys()
	return index
}
