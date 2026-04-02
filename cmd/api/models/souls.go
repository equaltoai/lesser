package models

import "time"

// SoulAgentAvatarStyle represents one available renderer-backed avatar style.
type SoulAgentAvatarStyle struct {
	StyleID         int    `json:"style_id"`
	StyleName       string `json:"style_name,omitempty"`
	RendererAddress string `json:"renderer_address,omitempty"`
	Image           string `json:"image,omitempty"`
	Selected        bool   `json:"selected,omitempty"`
}

// SoulAgentAvatar represents the current avatar plus all configured style variants.
type SoulAgentAvatar struct {
	TokenURI               string                 `json:"token_uri,omitempty"`
	Image                  string                 `json:"image,omitempty"`
	CurrentStyleID         *int                   `json:"current_style_id,omitempty"`
	CurrentStyleName       string                 `json:"current_style_name,omitempty"`
	CurrentRendererAddress string                 `json:"current_renderer_address,omitempty"`
	Styles                 []SoulAgentAvatarStyle `json:"styles,omitempty"`
}

// SoulAgentIdentity represents a soul identity returned by Lesser's souls inventory API.
type SoulAgentIdentity struct {
	AgentID                string           `json:"agent_id"`
	Domain                 string           `json:"domain"`
	LocalID                string           `json:"local_id"`
	ENSName                *string          `json:"ens_name"`
	Wallet                 string           `json:"wallet"`
	TokenID                string           `json:"token_id,omitempty"`
	MetaURI                string           `json:"meta_uri,omitempty"`
	Avatar                 *SoulAgentAvatar `json:"avatar,omitempty"`
	PrincipalAddress       string           `json:"principal_address,omitempty"`
	PrincipalSignature     string           `json:"principal_signature,omitempty"`
	PrincipalDeclaration   string           `json:"principal_declaration,omitempty"`
	PrincipalDeclaredAt    string           `json:"principal_declared_at,omitempty"`
	Status                 string           `json:"status"`
	LifecycleStatus        string           `json:"lifecycle_status,omitempty"`
	LifecycleReason        string           `json:"lifecycle_reason,omitempty"`
	SuccessorAgentID       string           `json:"successor_agent_id,omitempty"`
	PredecessorAgentID     string           `json:"predecessor_agent_id,omitempty"`
	SelfDescriptionVersion *int             `json:"self_description_version,omitempty"`
	Capabilities           []string         `json:"capabilities,omitempty"`
	MintTxHash             string           `json:"mint_tx_hash,omitempty"`
	MintedAt               *time.Time       `json:"minted_at,omitempty"`
	UpdatedAt              *time.Time       `json:"updated_at,omitempty"`
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
