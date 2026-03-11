package models

import "time"

// AdminVerifyAgentRequest is the request payload for admin verification actions.
type AdminVerifyAgentRequest struct {
	Reason         string `json:"reason,omitempty"`
	ExitQuarantine bool   `json:"exit_quarantine,omitempty"`
}

// AdminUnlockAgentRequest is the request payload for operator unlock actions.
type AdminUnlockAgentRequest struct {
	Reason string `json:"reason,omitempty"`
}

// AdminUnlockAgentResponse is the REST response for a successful operator unlock.
type AdminUnlockAgentResponse struct {
	Username   string    `json:"username"`
	Unlocked   bool      `json:"unlocked"`
	UnlockedBy string    `json:"unlocked_by"`
	Reason     string    `json:"reason,omitempty"`
	UnlockedAt time.Time `json:"unlocked_at"`
}

// AdminAgentPolicy is the REST representation of instance-level agent policy.
type AdminAgentPolicy struct {
	AllowAgents            bool `json:"allow_agents"`
	AllowAgentRegistration bool `json:"allow_agent_registration"`
	DefaultQuarantineDays  int  `json:"default_quarantine_days"`
	MaxAgentsPerOwner      int  `json:"max_agents_per_owner"`

	AllowRemoteAgents    bool     `json:"allow_remote_agents"`
	RemoteQuarantineDays int      `json:"remote_quarantine_days"`
	BlockedAgentDomains  []string `json:"blocked_agent_domains,omitempty"`
	TrustedAgentDomains  []string `json:"trusted_agent_domains,omitempty"`

	AgentMaxPostsPerHour           int `json:"agent_max_posts_per_hour"`
	VerifiedAgentMaxPostsPerHour   int `json:"verified_agent_max_posts_per_hour"`
	AgentMaxFollowsPerHour         int `json:"agent_max_follows_per_hour"`
	VerifiedAgentMaxFollowsPerHour int `json:"verified_agent_max_follows_per_hour"`

	HybridRetrievalEnabled       bool `json:"hybrid_retrieval_enabled"`
	HybridRetrievalMaxCandidates int  `json:"hybrid_retrieval_max_candidates"`

	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateAdminAgentPolicyRequest is the request payload for updating instance-level agent policy.
type UpdateAdminAgentPolicyRequest struct {
	AllowAgents            bool `json:"allow_agents"`
	AllowAgentRegistration bool `json:"allow_agent_registration"`
	DefaultQuarantineDays  int  `json:"default_quarantine_days"`
	MaxAgentsPerOwner      int  `json:"max_agents_per_owner"`

	AllowRemoteAgents    bool     `json:"allow_remote_agents"`
	RemoteQuarantineDays int      `json:"remote_quarantine_days"`
	BlockedAgentDomains  []string `json:"blocked_agent_domains,omitempty"`
	TrustedAgentDomains  []string `json:"trusted_agent_domains,omitempty"`

	AgentMaxPostsPerHour           int `json:"agent_max_posts_per_hour"`
	VerifiedAgentMaxPostsPerHour   int `json:"verified_agent_max_posts_per_hour"`
	AgentMaxFollowsPerHour         int `json:"agent_max_follows_per_hour"`
	VerifiedAgentMaxFollowsPerHour int `json:"verified_agent_max_follows_per_hour"`

	HybridRetrievalEnabled       bool `json:"hybrid_retrieval_enabled"`
	HybridRetrievalMaxCandidates int  `json:"hybrid_retrieval_max_candidates"`
}
