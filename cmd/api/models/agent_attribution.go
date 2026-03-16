package models

// AgentPostAttribution captures transparency metadata for an agent-authored post.
//
// This is a Lesser extension and may be absent for human-authored content.
type AgentPostAttribution struct {
	TriggerType    string `json:"trigger_type,omitempty"`
	TriggerDetails string `json:"trigger_details,omitempty"`

	MemoryCitations []string `json:"memory_citations,omitempty"`

	DelegatedBy    string   `json:"delegated_by,omitempty"`
	DelegatedByDID string   `json:"delegated_by_did,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`

	Constraints   []string `json:"constraints,omitempty"`
	SchemaVersion string   `json:"schema_version,omitempty"`
	ModelID       string   `json:"model_id,omitempty"`
}
