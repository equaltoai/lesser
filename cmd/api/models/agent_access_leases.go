package models

import "time"

// AgentAccessLeaseChallengeRequest requests a wallet-signing challenge for lease enrollment.
type AgentAccessLeaseChallengeRequest struct {
	PrincipalWallet  string   `json:"principal_wallet"`
	AgentWallet      string   `json:"agent_wallet"`
	SessionPublicKey string   `json:"session_public_key,omitempty"`
	Scopes           []string `json:"scopes"`
	DeviceLabel      string   `json:"device_label"`
	LeaseID          string   `json:"lease_id,omitempty"`

	IdleTimeoutHours int `json:"idle_timeout_hours,omitempty"`
	AbsoluteTTLHours int `json:"absolute_ttl_hours,omitempty"`
}

// AgentAccessLeaseChallengeResponse returns a one-time signing challenge.
type AgentAccessLeaseChallengeResponse struct {
	ID               string    `json:"id"`
	LeaseID          string    `json:"lease_id"`
	Username         string    `json:"username"`
	Action           string    `json:"action"`
	WalletAddress    string    `json:"wallet_address"`
	PrincipalWallet  string    `json:"principal_wallet"`
	AgentWallet      string    `json:"agent_wallet"`
	SessionPublicKey string    `json:"session_public_key,omitempty"`
	SessionKeyType   string    `json:"session_key_type,omitempty"`
	Scopes           []string  `json:"scopes"`
	DeviceLabel      string    `json:"device_label"`
	IdleTimeoutHours int       `json:"idle_timeout_hours"`
	AbsoluteTTLHours int       `json:"absolute_ttl_hours"`
	Message          string    `json:"message"`
	TypedData        any       `json:"typed_data,omitempty"`
	IssuedAt         time.Time `json:"issued_at"`
	ExpiresAt        time.Time `json:"expires_at"`
}

// CreateAgentAccessLeaseRequest finalizes a lease after both wallet signatures are collected.
type CreateAgentAccessLeaseRequest struct {
	PrincipalChallengeID string `json:"principal_challenge_id"`
	PrincipalSignature   string `json:"principal_signature"`
	AgentChallengeID     string `json:"agent_challenge_id"`
	AgentSignature       string `json:"agent_signature"`
}

// RevokeAgentAccessLeaseRequest revokes a lease.
type RevokeAgentAccessLeaseRequest struct {
	Reason string `json:"reason,omitempty"`
}

// AgentAccessLease is the REST representation of a wallet-backed access lease.
type AgentAccessLease struct {
	ID                   string     `json:"id"`
	Username             string     `json:"username"`
	PrincipalUsername    string     `json:"principal_username"`
	PrincipalWallet      string     `json:"principal_wallet"`
	AgentWallet          string     `json:"agent_wallet"`
	Scopes               []string   `json:"scopes"`
	DeviceLabel          string     `json:"device_label"`
	Status               string     `json:"status"`
	IdleTimeoutHours     int        `json:"idle_timeout_hours"`
	IdleExpiresAt        time.Time  `json:"idle_expires_at"`
	AbsoluteExpiresAt    time.Time  `json:"absolute_expires_at"`
	LastUsedAt           time.Time  `json:"last_used_at"`
	LeaseVersion         int        `json:"lease_version"`
	SessionPublicKey     string     `json:"session_public_key,omitempty"`
	SessionKeyType       string     `json:"session_key_type,omitempty"`
	SessionKeyCreatedAt  *time.Time `json:"session_key_created_at,omitempty"`
	SessionKeyLastUsedAt *time.Time `json:"session_key_last_used_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	RevokedAt            *time.Time `json:"revoked_at,omitempty"`
	RevokedBy            string     `json:"revoked_by,omitempty"`
	RevokedReason        string     `json:"revoked_reason,omitempty"`
}

// AgentAccessLeaseListResponse wraps a lease list.
type AgentAccessLeaseListResponse struct {
	Leases []AgentAccessLease `json:"leases"`
}

// RenewAgentAccessLeaseTokenRequest exchanges a renewal proof for a short-lived access token.
type RenewAgentAccessLeaseTokenRequest struct {
	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`
}

// AgentAccessLeaseSessionKeyChallengeRequest requests authorization of a session key for a lease.
type AgentAccessLeaseSessionKeyChallengeRequest struct {
	SessionPublicKey string `json:"session_public_key"`
}

// AuthorizeAgentAccessLeaseSessionKeyRequest finalizes a session key authorization.
type AuthorizeAgentAccessLeaseSessionKeyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`
}

// AgentAccessLeaseTokenResponse returns a short-lived access token for a lease.
type AgentAccessLeaseTokenResponse struct {
	LeaseID string             `json:"lease_id"`
	Token   OAuthTokenResponse `json:"token"`
}
