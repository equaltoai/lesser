package models

import (
	"fmt"
	"strings"
	"time"
)

// InstanceSoulBodyBinding stores an explicit soul -> local body binding.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="SOUL_BODY_BINDING#<agentId>".
type InstanceSoulBodyBinding struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	AgentID          string    `theorydb:"attr:agentId" json:"agent_id"`
	Username         string    `theorydb:"attr:username" json:"username"`
	PrincipalAddress string    `theorydb:"attr:principalAddress,omitempty" json:"principal_address,omitempty"`
	BoundAt          time.Time `theorydb:"attr:boundAt" json:"bound_at"`
	UpdatedAt        time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing InstanceSoulBodyBinding.
func (InstanceSoulBodyBinding) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (b *InstanceSoulBodyBinding) UpdateKeys() error {
	if b == nil {
		return fmt.Errorf("soul body binding is nil")
	}

	b.AgentID = strings.ToLower(strings.TrimSpace(b.AgentID))
	b.Username = strings.TrimSpace(b.Username)
	b.PrincipalAddress = strings.ToLower(strings.TrimSpace(b.PrincipalAddress))

	if b.AgentID == "" {
		return fmt.Errorf("agent id is required")
	}
	if b.Username == "" {
		return fmt.Errorf("username is required")
	}

	if b.BoundAt.IsZero() {
		b.BoundAt = time.Now().UTC()
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = b.BoundAt
	}

	b.PK = instanceConfigPK
	b.SK = SoulBodyBindingSortKey(b.AgentID)
	return nil
}

// GetPK returns the partition key.
func (b *InstanceSoulBodyBinding) GetPK() string {
	return b.PK
}

// GetSK returns the sort key.
func (b *InstanceSoulBodyBinding) GetSK() string {
	return b.SK
}

// NewInstanceSoulBodyBinding creates a new soul/body binding record.
func NewInstanceSoulBodyBinding(agentID string, username string, principalAddress string) *InstanceSoulBodyBinding {
	binding := &InstanceSoulBodyBinding{
		AgentID:          strings.ToLower(strings.TrimSpace(agentID)),
		Username:         strings.TrimSpace(username),
		PrincipalAddress: strings.ToLower(strings.TrimSpace(principalAddress)),
		BoundAt:          time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	_ = binding.UpdateKeys()
	return binding
}
