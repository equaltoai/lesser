package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	// SoulENSChainSepolia publishes the ENS channel for Sepolia CCIP-read resolution.
	SoulENSChainSepolia = "sepolia"
	// SoulENSChainMainnet publishes the ENS channel for Ethereum mainnet CCIP-read resolution.
	SoulENSChainMainnet = "mainnet"
)

// InstanceSoulENSChannel stores instance-owned ENS channel publication config for a soul agent.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="SOUL_ENS_CHANNEL#<agentId>".
type InstanceSoulENSChannel struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	AgentID         string    `theorydb:"attr:agentId" json:"agent_id"`
	Name            string    `theorydb:"attr:name" json:"name"`
	ResolverAddress string    `theorydb:"attr:resolverAddress,omitempty" json:"resolver_address,omitempty"`
	Chain           string    `theorydb:"attr:chain" json:"chain"`
	UpdatedAt       time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing InstanceSoulENSChannel.
func (InstanceSoulENSChannel) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (c *InstanceSoulENSChannel) UpdateKeys() error {
	if c == nil {
		return fmt.Errorf("soul ens channel config is nil")
	}

	c.AgentID = strings.ToLower(strings.TrimSpace(c.AgentID))
	if c.AgentID == "" {
		return fmt.Errorf("agent id is required")
	}

	c.Name = strings.ToLower(strings.TrimSpace(c.Name))
	c.ResolverAddress = strings.TrimSpace(c.ResolverAddress)
	c.Chain = strings.ToLower(strings.TrimSpace(c.Chain))
	c.PK = instanceConfigPK
	c.SK = SoulENSChannelSortKey(c.AgentID)
	return nil
}

// GetPK returns the partition key.
func (c *InstanceSoulENSChannel) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *InstanceSoulENSChannel) GetSK() string {
	return c.SK
}

// NewInstanceSoulENSChannel creates a new soul ENS channel config.
func NewInstanceSoulENSChannel(agentID string) *InstanceSoulENSChannel {
	cfg := &InstanceSoulENSChannel{
		AgentID:   strings.ToLower(strings.TrimSpace(agentID)),
		Chain:     SoulENSChainSepolia,
		UpdatedAt: time.Now().UTC(),
	}
	_ = cfg.UpdateKeys()
	return cfg
}
