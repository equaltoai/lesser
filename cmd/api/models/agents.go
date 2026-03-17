package models

import "time"

// AgentCapabilities describes what an agent account is permitted to do.
// This mirrors `pkg/agents.Capabilities`, but is scoped to the public REST surface.
type AgentCapabilities struct {
	CanPost   bool `json:"can_post"`
	CanReply  bool `json:"can_reply"`
	CanBoost  bool `json:"can_boost"`
	CanFollow bool `json:"can_follow"`
	CanDM     bool `json:"can_dm"`

	RestrictedDomains []string `json:"restricted_domains,omitempty"`
	MaxPostsPerHour   int      `json:"max_posts_per_hour"`
	RequiresApproval  bool     `json:"requires_approval"`
}

// Agent is the REST representation of a local agent account.
type Agent struct {
	Username          string            `json:"username"`
	DisplayName       string            `json:"display_name"`
	Bio               string            `json:"bio,omitempty"`
	CreatedAt         *time.Time        `json:"created_at,omitempty"`
	Verified          bool              `json:"verified"`
	VerifiedAt        *time.Time        `json:"verified_at,omitempty"`
	AgentType         string            `json:"agent_type"`
	AgentVersion      string            `json:"agent_version"`
	AgentOwner        string            `json:"agent_owner,omitempty"`
	DelegatedScopes   []string          `json:"delegated_scopes,omitempty"`
	AgentCapabilities AgentCapabilities `json:"agent_capabilities"`
}

// AgentDelegationRequest is the request payload for POST /api/v1/agents/delegate.
//
// Lesser is email-free. This endpoint does not accept email.
type AgentDelegationRequest struct {
	AgentUsername string   `json:"agent_username"`
	DisplayName   string   `json:"display_name"`
	Bio           string   `json:"bio,omitempty"`
	Scopes        []string `json:"scopes"`
	ExpiresIn     int      `json:"expires_in,omitempty"`
	DeviceLabel   string   `json:"device_label,omitempty"`
	AgentInfo     any      `json:"agent_info,omitempty"`
}

// AgentDelegationResponse is the response payload for POST /api/v1/agents/delegate.
type AgentDelegationResponse struct {
	Account Account            `json:"account"`
	Token   OAuthTokenResponse `json:"token"`
}

// UpdateAgentRequest is the request payload for PATCH /api/v1/agents/{username}.
type UpdateAgentRequest struct {
	DisplayName       string             `json:"display_name,omitempty"`
	Bio               string             `json:"bio,omitempty"`
	AgentType         string             `json:"agent_type,omitempty"`
	AgentVersion      string             `json:"agent_version,omitempty"`
	AgentCapabilities *AgentCapabilities `json:"agent_capabilities,omitempty"`

	// ExitQuarantine allows an owner/admin to approve an agent to post publicly before the quarantine window ends.
	ExitQuarantine bool `json:"exit_quarantine,omitempty"`
}
