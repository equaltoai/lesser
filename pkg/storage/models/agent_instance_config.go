package models

import "time"

// AgentInstanceConfig stores instance-level agent policy.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="AGENT_CONFIG".
type AgentInstanceConfig struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	AllowAgents            bool      `theorydb:"attr:allowAgents" json:"allow_agents"`
	AllowAgentRegistration bool      `theorydb:"attr:allowAgentRegistration" json:"allow_agent_registration"`
	DefaultQuarantineDays  int       `theorydb:"attr:defaultQuarantineDays" json:"default_quarantine_days"`
	MaxAgentsPerOwner      int       `theorydb:"attr:maxAgentsPerOwner" json:"max_agents_per_owner"`
	UpdatedAt              time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing AgentInstanceConfig.
func (AgentInstanceConfig) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (c *AgentInstanceConfig) UpdateKeys() error {
	c.PK = instanceConfigPK
	c.SK = "AGENT_CONFIG"
	return nil
}

// GetPK returns the partition key.
func (c *AgentInstanceConfig) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *AgentInstanceConfig) GetSK() string {
	return c.SK
}

// NewAgentInstanceConfig returns a config with conservative defaults.
func NewAgentInstanceConfig() *AgentInstanceConfig {
	return &AgentInstanceConfig{
		PK:                     instanceConfigPK,
		SK:                     "AGENT_CONFIG",
		AllowAgents:            false,
		AllowAgentRegistration: false,
		DefaultQuarantineDays:  7,
		MaxAgentsPerOwner:      3,
		UpdatedAt:              time.Now(),
	}
}
