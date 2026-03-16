package activitypub

import (
	"encoding/json"
	"strings"
)

const (
	// AgentAttributionSchemaVersion identifies the current Lesser agent attribution schema.
	AgentAttributionSchemaVersion       = "1.0"
	legacyAgentAttributionSchemaVersion = "0.9"
)

// AgentPostAttribution captures transparency metadata for an agent-authored post.
//
// This is a Lesser extension. It is stored on the underlying Note and exposed via the REST
// API as `agent_attribution`.
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

type agentPostAttributionAlias AgentPostAttribution

type legacyAgentPostAttribution struct {
	agentPostAttributionAlias
	ModelVersion string `json:"model_version,omitempty"`
}

// UnmarshalJSON preserves backward compatibility with stored records that still carry the
// legacy model_version field.
func (a *AgentPostAttribution) UnmarshalJSON(data []byte) error {
	var legacy legacyAgentPostAttribution
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	*a = AgentPostAttribution(legacy.agentPostAttributionAlias)

	if strings.TrimSpace(a.ModelID) == "" && strings.TrimSpace(legacy.ModelVersion) != "" {
		a.ModelID = strings.TrimSpace(legacy.ModelVersion)
	}
	if strings.TrimSpace(a.SchemaVersion) == "" && strings.TrimSpace(legacy.ModelVersion) != "" {
		a.SchemaVersion = legacyAgentAttributionSchemaVersion
	}

	return nil
}
