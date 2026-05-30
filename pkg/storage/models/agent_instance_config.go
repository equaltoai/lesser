package models

import "time"

// AgentInstanceConfig stores instance-level agent policy.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="AGENT_CONFIG".
type AgentInstanceConfig struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	AllowAgents            bool `theorydb:"attr:allowAgents" json:"allow_agents"`
	AllowAgentRegistration bool `theorydb:"attr:allowAgentRegistration" json:"allow_agent_registration"`
	DefaultQuarantineDays  int  `theorydb:"attr:defaultQuarantineDays" json:"default_quarantine_days"`
	MaxAgentsPerOwner      int  `theorydb:"attr:maxAgentsPerOwner" json:"max_agents_per_owner"`

	// Remote agent policy (federated bot accounts)
	AllowRemoteAgents    bool     `theorydb:"attr:allowRemoteAgents" json:"allow_remote_agents"`
	RemoteQuarantineDays int      `theorydb:"attr:remoteQuarantineDays" json:"remote_quarantine_days"`
	BlockedAgentDomains  []string `theorydb:"attr:blockedAgentDomains" json:"blocked_agent_domains,omitempty"`
	TrustedAgentDomains  []string `theorydb:"attr:trustedAgentDomains" json:"trusted_agent_domains,omitempty"`

	// Verified/trust tier limits
	AgentMaxPostsPerHour           int `theorydb:"attr:agentMaxPostsPerHour" json:"agent_max_posts_per_hour"`
	VerifiedAgentMaxPostsPerHour   int `theorydb:"attr:verifiedAgentMaxPostsPerHour" json:"verified_agent_max_posts_per_hour"`
	AgentMaxFollowsPerHour         int `theorydb:"attr:agentMaxFollowsPerHour" json:"agent_max_follows_per_hour"`
	VerifiedAgentMaxFollowsPerHour int `theorydb:"attr:verifiedAgentMaxFollowsPerHour" json:"verified_agent_max_follows_per_hour"`

	// Optional hybrid retrieval (timeline + semantic fallback). Disabled by default.
	HybridRetrievalEnabled       bool `theorydb:"attr:hybridRetrievalEnabled" json:"hybrid_retrieval_enabled"`
	HybridRetrievalMaxCandidates int  `theorydb:"attr:hybridRetrievalMaxCandidates" json:"hybrid_retrieval_max_candidates"`

	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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

// NewAgentInstanceConfig returns secure-by-default agent policy.
// Operators must explicitly enable agents and agent registration via env or admin policy.
func NewAgentInstanceConfig() *AgentInstanceConfig {
	return &AgentInstanceConfig{
		PK:                             instanceConfigPK,
		SK:                             "AGENT_CONFIG",
		AllowAgents:                    false,
		AllowAgentRegistration:         false,
		DefaultQuarantineDays:          7,
		MaxAgentsPerOwner:              3,
		AllowRemoteAgents:              true,
		RemoteQuarantineDays:           7,
		AgentMaxPostsPerHour:           50,
		VerifiedAgentMaxPostsPerHour:   200,
		AgentMaxFollowsPerHour:         20,
		VerifiedAgentMaxFollowsPerHour: 100,
		HybridRetrievalEnabled:         false,
		HybridRetrievalMaxCandidates:   200,
		UpdatedAt:                      time.Now(),
	}
}
