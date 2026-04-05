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

// AgentMCPAccess describes the client-neutral actor-scoped MCP access surface
// for an agent page.
type AgentMCPAccess struct {
	MCPURL                 string   `json:"mcp_url"`
	ProtectedResourceURL   string   `json:"protected_resource_url"`
	AuthorizationServerURL string   `json:"authorization_server_url"`
	RegistrationURL        string   `json:"registration_url"`
	Scopes                 []string `json:"scopes"`
	Guidance               []string `json:"guidance"`
}

// AgentIdentitySemantics captures the public drone/graduating/souled semantics for an agent.
type AgentIdentitySemantics struct {
	IdentityState             string `json:"identity_state"`
	IdentityLabel             string `json:"identity_label"`
	LifecycleState            string `json:"lifecycle_state"`
	SoulBindingState          string `json:"soul_binding_state"`
	SoulAgentID               string `json:"soul_agent_id,omitempty"`
	ContinuityState           string `json:"continuity_state"`
	ContinuitySummary         string `json:"continuity_summary"`
	BodyIdentityPreserved     bool   `json:"body_identity_preserved"`
	TimelinePresencePreserved bool   `json:"timeline_presence_preserved"`
	MemoryReferencesPreserved bool   `json:"memory_references_preserved"`
	AttributionLabel          string `json:"attribution_label"`
	ModerationLabel           string `json:"moderation_label"`
}

// Agent is the REST representation of a local agent account.
type Agent struct {
	Username             string                 `json:"username"`
	DisplayName          string                 `json:"display_name"`
	Bio                  string                 `json:"bio,omitempty"`
	CreatedAt            *time.Time             `json:"created_at,omitempty"`
	Verified             bool                   `json:"verified"`
	VerifiedAt           *time.Time             `json:"verified_at,omitempty"`
	QuarantineStatus     string                 `json:"quarantine_status,omitempty"`
	QuarantineStart      *time.Time             `json:"quarantine_start,omitempty"`
	QuarantineEnd        *time.Time             `json:"quarantine_end,omitempty"`
	QuarantineApprovedBy string                 `json:"quarantine_approved_by,omitempty"`
	QuarantineApprovedAt *time.Time             `json:"quarantine_approved_at,omitempty"`
	QuarantineActive     bool                   `json:"quarantine_active"`
	AgentType            string                 `json:"agent_type"`
	AgentVersion         string                 `json:"agent_version"`
	AgentOwner           string                 `json:"agent_owner,omitempty"`
	DelegatedScopes      []string               `json:"delegated_scopes,omitempty"`
	AgentCapabilities    AgentCapabilities      `json:"agent_capabilities"`
	MCPAccess            AgentMCPAccess         `json:"mcp_access"`
	IdentitySemantics    AgentIdentitySemantics `json:"identity_semantics"`
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
