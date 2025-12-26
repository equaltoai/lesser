package models

import "time"

// WebAuthnBeginLoginRequest represents POST /api/v1/auth/webauthn/login/begin.
type WebAuthnBeginLoginRequest struct {
	Username string `json:"username"`
}

// WebAuthnBeginResponse represents the publicKey + challenge response used for WebAuthn begin endpoints.
type WebAuthnBeginResponse struct {
	PublicKey map[string]any `json:"publicKey"`
	Challenge string         `json:"challenge"`
}

// WebAuthnFinishRegistrationRequest represents POST /api/v1/auth/webauthn/register/finish.
type WebAuthnFinishRegistrationRequest struct {
	Challenge      string         `json:"challenge"`
	Response       map[string]any `json:"response"`
	CredentialName string         `json:"credential_name"`
}

// WebAuthnFinishLoginRequest represents POST /api/v1/auth/webauthn/login/finish.
type WebAuthnFinishLoginRequest struct {
	Username   string         `json:"username"`
	Challenge  string         `json:"challenge"`
	Response   map[string]any `json:"response"`
	DeviceName string         `json:"device_name"`
}

// WebAuthnCredentialSummary represents a WebAuthn credential summary in API responses.
type WebAuthnCredentialSummary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// WebAuthnCredentialsResponse represents GET /api/v1/auth/webauthn/credentials.
type WebAuthnCredentialsResponse struct {
	Credentials []WebAuthnCredentialSummary `json:"credentials"`
}

// WebAuthnUpdateCredentialRequest represents PUT /api/v1/auth/webauthn/credentials/{credentialId}.
type WebAuthnUpdateCredentialRequest struct {
	Name string `json:"name"`
}
