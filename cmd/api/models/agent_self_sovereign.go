package models

import "time"

// AgentKeyChallengeRequest is the request payload for challenge issuance endpoints.
type AgentKeyChallengeRequest struct {
	Username string `json:"username"`
}

// AgentKeyChallengeResponse is the response payload for agent key challenges.
type AgentKeyChallengeResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Message   string    `json:"message"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AgentSelfRegistrationRequest is the request payload for POST /api/v1/agents/register.
//
// Lesser is email-free. This endpoint does not accept email.
type AgentSelfRegistrationRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio,omitempty"`
	DeviceLabel string `json:"device_label,omitempty"`

	PublicKey string `json:"public_key"`
	KeyType   string `json:"key_type"`

	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`

	Scopes    []string `json:"scopes,omitempty"`
	AgentInfo any      `json:"agent_info,omitempty"`
}

// AgentSelfRegistrationResponse is the response payload for POST /api/v1/agents/register.
type AgentSelfRegistrationResponse struct {
	Account Account            `json:"account"`
	Token   OAuthTokenResponse `json:"token"`
}

// AgentSelfAuthTokenRequest is the request payload for POST /api/v1/agents/auth/token.
type AgentSelfAuthTokenRequest struct {
	Username    string `json:"username"`
	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`
	DeviceLabel string `json:"device_label,omitempty"`
}

// AgentRotateKeyRequest is the request payload for POST /api/v1/agents/{username}/rotate-key.
type AgentRotateKeyRequest struct {
	PublicKey string `json:"public_key"`
	KeyType   string `json:"key_type"`

	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`
}
