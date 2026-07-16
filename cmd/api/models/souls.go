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
	AuthorityModel         string           `json:"authority_model,omitempty"`
	AnchorState            string           `json:"anchor_state,omitempty"`
	OperationalBinding     string           `json:"operational_binding,omitempty"`
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
	PublishedVersion       int              `json:"published_version,omitempty"`
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

// BoundSoulResponse represents GET /api/v1/souls/bound/me.
type BoundSoulResponse struct {
	Agent        SoulAgentIdentity `json:"agent"`
	BindingState string            `json:"binding_state"`
	Binding      SoulAgentBinding  `json:"binding"`
}

// SoulBindingEvidence carries body/Ptah correlation evidence. Lesser treats it
// as a hint only; Host source truth remains authoritative.
type SoulBindingEvidence struct {
	Source          string `json:"source,omitempty"`
	HostRequestID   string `json:"host_request_id,omitempty"`
	DeclarationHash string `json:"declaration_hash,omitempty"`
	IssuedAt        string `json:"issued_at,omitempty"`
}

// SoulBindingRequest represents POST /api/v1/souls/bindings.
type SoulBindingRequest struct {
	ActorUsername      string              `json:"actor_username"`
	SoulAgentID        string              `json:"soul_agent_id"`
	BodyActorID        string              `json:"body_actor_id,omitempty"`
	HostRegistrationID string              `json:"host_registration_id,omitempty"`
	HostConversationID string              `json:"host_conversation_id,omitempty"`
	AuthorityModel     string              `json:"authority_model,omitempty"`
	AnchorState        string              `json:"anchor_state,omitempty"`
	OperationalBinding string              `json:"operational_binding,omitempty"`
	PrincipalAddress   string              `json:"principal_address,omitempty"`
	Evidence           SoulBindingEvidence `json:"evidence,omitempty"`
}

// SoulBindingAgent is the Host-refetched identity block used for binding responses.
type SoulBindingAgent struct {
	AgentID            string `json:"agent_id"`
	Domain             string `json:"domain"`
	LocalID            string `json:"local_id"`
	AuthorityModel     string `json:"authority_model"`
	AnchorState        string `json:"anchor_state"`
	OperationalBinding string `json:"operational_binding"`
	LifecycleStatus    string `json:"lifecycle_status"`
	PublishedVersion   int    `json:"published_version,omitempty"`
}

// SoulBindingIdempotency describes the POST replay scope and canonical payload hash.
type SoulBindingIdempotency struct {
	Key         string `json:"key"`
	Replayed    bool   `json:"replayed"`
	PayloadHash string `json:"payload_hash"`
}

// SoulBindingLinks contains binding follow-up links.
type SoulBindingLinks struct {
	Status string `json:"status"`
}

// SoulBindingResponse represents POST/GET /api/v1/souls/bindings*.
type SoulBindingResponse struct {
	Version      string                  `json:"version"`
	Status       string                  `json:"status"`
	BindingState string                  `json:"binding_state"`
	Agent        SoulBindingAgent        `json:"agent"`
	Binding      SoulAgentBinding        `json:"binding"`
	Idempotency  *SoulBindingIdempotency `json:"idempotency,omitempty"`
	Links        *SoulBindingLinks       `json:"links,omitempty"`
}

// SoulMintConversationSummary represents compact mint-conversation metadata for self-scoped private reads.
type SoulMintConversationSummary struct {
	AgentID        string         `json:"agent_id"`
	ConversationID string         `json:"conversation_id"`
	Model          string         `json:"model,omitempty"`
	Status         string         `json:"status"`
	Usage          map[string]any `json:"usage,omitempty"`
	ChargedCredits *int64         `json:"charged_credits,omitempty"`
	CreatedAt      string         `json:"created_at"`
	CompletedAt    string         `json:"completed_at,omitempty"`
}

// SoulMintConversationsResponse represents GET /api/v1/souls/bound/me/mint-conversations.
type SoulMintConversationsResponse struct {
	Version       string                        `json:"version"`
	Conversations []SoulMintConversationSummary `json:"conversations"`
	Count         int                           `json:"count"`
	Limit         int                           `json:"limit"`
}

// SoulMintConversation represents a bounded private mint-conversation record.
type SoulMintConversation struct {
	AgentID              string         `json:"agent_id"`
	ConversationID       string         `json:"conversation_id"`
	Model                string         `json:"model"`
	Messages             string         `json:"messages,omitempty"`
	ProducedDeclarations string         `json:"produced_declarations,omitempty"`
	Status               string         `json:"status"`
	Usage                map[string]any `json:"usage,omitempty"`
	ChargedCredits       *int64         `json:"charged_credits,omitempty"`
	CreatedAt            string         `json:"created_at"`
	CompletedAt          string         `json:"completed_at,omitempty"`
}

// SoulMintConversationResponse represents GET /api/v1/souls/bound/me/mint-conversations/{conversationId}.
type SoulMintConversationResponse struct {
	Version      string               `json:"version"`
	Conversation SoulMintConversation `json:"conversation"`
}

// SoulIncorporateRequest represents POST /api/v1/souls/{agentId}/incorporate.
type SoulIncorporateRequest struct {
	TargetAgentUsername string `json:"target_agent_username"`
}

// SoulIncorporateResponse represents POST /api/v1/souls/{agentId}/incorporate.
type SoulIncorporateResponse struct {
	Soul SoulInventoryItem `json:"soul"`
}
