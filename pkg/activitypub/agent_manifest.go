package activitypub

// AgentManifest is a Lesser-specific ActivityPub extension describing an LLM agent account.
//
// It is intentionally optional and designed to degrade gracefully on remote servers that
// ignore unknown fields.
type AgentManifest struct {
	Type         string             `json:"type"`
	Version      string             `json:"version,omitempty"`
	Capabilities *AgentCapabilities `json:"capabilities,omitempty"`
	Purpose      string             `json:"purpose,omitempty"`
	OperatedBy   string             `json:"operatedBy,omitempty"`
	Source       string             `json:"source,omitempty"`
}

// AgentCapabilities describe what an agent claims to be able to do.
// These are informational and should not be treated as authorization.
type AgentCapabilities struct {
	CanPost   bool `json:"canPost"`
	CanReply  bool `json:"canReply"`
	CanBoost  bool `json:"canBoost"`
	CanFollow bool `json:"canFollow"`
	CanDM     bool `json:"canDM"`

	RestrictedDomains []string `json:"restrictedDomains,omitempty"`
	MaxPostsPerHour   int      `json:"maxPostsPerHour,omitempty"`
	RequiresApproval  bool     `json:"requiresApproval,omitempty"`
}
