package models

// AgentPostAttribution captures transparency metadata for an agent-authored post.
//
// This is a Lesser extension and may be absent for human-authored content.
type AgentPostAttribution struct {
	TriggerType    string `json:"trigger_type,omitempty"`
	TriggerDetails string `json:"trigger_details,omitempty"`

	MemoryCitations []string `json:"memory_citations,omitempty"`

	DelegatedBy    string   `json:"delegated_by,omitempty"`
	ApprovedBy     string   `json:"approved_by,omitempty"`
	DelegatedByDID string   `json:"delegated_by_did,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`

	Constraints       []string `json:"constraints,omitempty"`
	SchemaVersion     string   `json:"schema_version,omitempty"`
	ModelID           string   `json:"model_id,omitempty"`
	IdentityState     string   `json:"identity_state,omitempty"`
	IdentityLabel     string   `json:"identity_label,omitempty"`
	ContinuityState   string   `json:"continuity_state,omitempty"`
	ContinuitySummary string   `json:"continuity_summary,omitempty"`
	SoulAgentID       string   `json:"soul_agent_id,omitempty"`
	ModerationLabel   string   `json:"moderation_label,omitempty"`
}

// AgentPostAttributionInput contains only client-provided provenance details. Delegation and
// approval identities are deliberately absent because Lesser derives them from signed credentials.
type AgentPostAttributionInput struct {
	TriggerType     string   `json:"trigger_type,omitempty"`
	TriggerDetails  string   `json:"trigger_details,omitempty"`
	MemoryCitations []string `json:"memory_citations,omitempty"`
}
