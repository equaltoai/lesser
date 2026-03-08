package models

import "time"

// SoulAgentIdentity represents a soul identity returned by Lesser's souls inventory API.
type SoulAgentIdentity struct {
	AgentID                string     `json:"agent_id"`
	Domain                 string     `json:"domain"`
	LocalID                string     `json:"local_id"`
	ENSName                *string    `json:"ens_name"`
	Wallet                 string     `json:"wallet"`
	PrincipalAddress       string     `json:"principal_address,omitempty"`
	Status                 string     `json:"status"`
	LifecycleStatus        string     `json:"lifecycle_status,omitempty"`
	SelfDescriptionVersion *int       `json:"self_description_version,omitempty"`
	Capabilities           []string   `json:"capabilities,omitempty"`
	MintTxHash             string     `json:"mint_tx_hash,omitempty"`
	MintedAt               *time.Time `json:"minted_at,omitempty"`
	UpdatedAt              *time.Time `json:"updated_at,omitempty"`
}

// SoulAgentBinding represents a local soul -> agent incorporation binding.
type SoulAgentBinding struct {
	AgentUsername    string    `json:"agent_username"`
	PrincipalAddress string    `json:"principal_address,omitempty"`
	BoundAt          time.Time `json:"bound_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// SoulInventoryItem represents one soul plus its local binding state.
type SoulInventoryItem struct {
	Agent                     SoulAgentIdentity `json:"agent"`
	BindingState              string            `json:"binding_state"`
	AvailableForIncorporation bool              `json:"available_for_incorporation"`
	Binding                   *SoulAgentBinding `json:"binding,omitempty"`
}

// SoulsMineResponse represents GET /api/v1/souls/mine.
type SoulsMineResponse struct {
	Souls []SoulInventoryItem `json:"souls"`
	Count int                 `json:"count"`
}

// SoulIncorporateRequest represents POST /api/v1/souls/{agentId}/incorporate.
type SoulIncorporateRequest struct {
	TargetAgentUsername string `json:"target_agent_username"`
}

// SoulIncorporateResponse represents POST /api/v1/souls/{agentId}/incorporate.
type SoulIncorporateResponse struct {
	Soul SoulInventoryItem `json:"soul"`
}
