package activitypub

import (
	"encoding/json"
	"net/url"
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
	ApprovedBy     string   `json:"approved_by,omitempty"`
	DelegatedByDID string   `json:"delegated_by_did,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`

	// ActedBy records the real local caller when a share-grant grantee acts on the
	// agent's account (agent-scoped action with mandatory caller attribution). It is
	// derived server-side from the authenticated claims, never from client input.
	ActedBy string `json:"acted_by,omitempty"`

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

func normalizeDelegatedByActorURI(value, actorIRI string) string {
	delegatedBy := strings.TrimSpace(value)
	if delegatedBy == "" {
		return ""
	}

	lowerValue := strings.ToLower(delegatedBy)
	if strings.HasPrefix(lowerValue, "http://") || strings.HasPrefix(lowerValue, "https://") {
		return delegatedBy
	}

	actorURL, err := url.Parse(strings.TrimSpace(actorIRI))
	if err != nil || actorURL.Scheme == "" || actorURL.Host == "" {
		return delegatedBy
	}

	actorURL.Path = "/users/" + strings.TrimPrefix(delegatedBy, "@")
	actorURL.RawPath = ""
	actorURL.RawQuery = ""
	actorURL.Fragment = ""

	return actorURL.String()
}

func normalizeAgentPostAttributionForActor(attr *AgentPostAttribution, actorIRI string) *AgentPostAttribution {
	if attr == nil {
		return nil
	}

	normalized := *attr
	normalized.DelegatedBy = normalizeDelegatedByActorURI(normalized.DelegatedBy, actorIRI)
	normalized.ApprovedBy = normalizeDelegatedByActorURI(normalized.ApprovedBy, actorIRI)
	normalized.ActedBy = normalizeDelegatedByActorURI(normalized.ActedBy, actorIRI)
	return &normalized
}
