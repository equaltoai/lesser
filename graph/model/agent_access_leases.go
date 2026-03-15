package model

// AgentAccessLease mirrors the GraphQL wallet-backed access lease type.
type AgentAccessLease struct {
	ID                   string   `json:"id"`
	Username             string   `json:"username"`
	PrincipalUsername    string   `json:"principalUsername"`
	PrincipalWallet      string   `json:"principalWallet"`
	AgentWallet          string   `json:"agentWallet"`
	Scopes               []string `json:"scopes"`
	DeviceLabel          string   `json:"deviceLabel"`
	Status               string   `json:"status"`
	IdleTimeoutHours     int      `json:"idleTimeoutHours"`
	TokenTTLHours        int      `json:"tokenTTLHours"`
	IdleExpiresAt        Time     `json:"idleExpiresAt"`
	AbsoluteExpiresAt    Time     `json:"absoluteExpiresAt"`
	LastUsedAt           Time     `json:"lastUsedAt"`
	LeaseVersion         int      `json:"leaseVersion"`
	SessionPublicKey     *string  `json:"sessionPublicKey,omitempty"`
	SessionKeyType       *string  `json:"sessionKeyType,omitempty"`
	SessionKeyCreatedAt  *Time    `json:"sessionKeyCreatedAt,omitempty"`
	SessionKeyLastUsedAt *Time    `json:"sessionKeyLastUsedAt,omitempty"`
	CreatedAt            Time     `json:"createdAt"`
	UpdatedAt            Time     `json:"updatedAt"`
	RevokedAt            *Time    `json:"revokedAt,omitempty"`
	RevokedBy            *string  `json:"revokedBy,omitempty"`
	RevokedReason        *string  `json:"revokedReason,omitempty"`
}

// AgentAccessLeaseChallenge mirrors the GraphQL wallet-backed access lease challenge type.
type AgentAccessLeaseChallenge struct {
	ID               string   `json:"id"`
	LeaseID          string   `json:"leaseID"`
	Username         string   `json:"username"`
	Action           string   `json:"action"`
	WalletAddress    string   `json:"walletAddress"`
	PrincipalWallet  string   `json:"principalWallet"`
	AgentWallet      string   `json:"agentWallet"`
	SessionPublicKey *string  `json:"sessionPublicKey,omitempty"`
	SessionKeyType   *string  `json:"sessionKeyType,omitempty"`
	Scopes           []string `json:"scopes"`
	DeviceLabel      string   `json:"deviceLabel"`
	IdleTimeoutHours int      `json:"idleTimeoutHours"`
	AbsoluteTTLHours int      `json:"absoluteTTLHours"`
	TokenTTLHours    int      `json:"tokenTTLHours"`
	Message          string   `json:"message"`
	TypedDataJSON    *string  `json:"typedDataJson,omitempty"`
	IssuedAt         Time     `json:"issuedAt"`
	ExpiresAt        Time     `json:"expiresAt"`
}

// AgentAccessLeaseTokenPayload mirrors the GraphQL renewal token payload.
type AgentAccessLeaseTokenPayload struct {
	LeaseID     string `json:"leaseID"`
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	Scope       string `json:"scope"`
	CreatedAt   Time   `json:"createdAt"`
	ExpiresIn   int    `json:"expiresIn"`
}

// AgentAccessLeaseChallengeInput is the GraphQL input for principal/agent lease challenges.
type AgentAccessLeaseChallengeInput struct {
	PrincipalWallet  string   `json:"principalWallet"`
	AgentWallet      string   `json:"agentWallet"`
	SessionPublicKey *string  `json:"sessionPublicKey,omitempty"`
	Scopes           []string `json:"scopes"`
	DeviceLabel      *string  `json:"deviceLabel,omitempty"`
	LeaseID          *string  `json:"leaseID,omitempty"`
	IdleTimeoutHours *int     `json:"idleTimeoutHours,omitempty"`
	AbsoluteTTLHours *int     `json:"absoluteTTLHours,omitempty"`
	TokenTTLHours    *int     `json:"tokenTTLHours,omitempty"`
}

// CreateAgentAccessLeaseInput is the GraphQL input for finalizing a lease.
type CreateAgentAccessLeaseInput struct {
	PrincipalChallengeID string `json:"principalChallengeID"`
	PrincipalSignature   string `json:"principalSignature"`
	AgentChallengeID     string `json:"agentChallengeID"`
	AgentSignature       string `json:"agentSignature"`
}

// RevokeAgentAccessLeaseInput is the GraphQL input for revoking a lease.
type RevokeAgentAccessLeaseInput struct {
	Reason *string `json:"reason,omitempty"`
}

// AgentAccessLeaseSessionKeyChallengeInput is the GraphQL input for requesting a session-key auth challenge.
type AgentAccessLeaseSessionKeyChallengeInput struct {
	SessionPublicKey string `json:"sessionPublicKey"`
}

// AuthorizeAgentAccessLeaseSessionKeyInput is the GraphQL input for authorizing a session key.
type AuthorizeAgentAccessLeaseSessionKeyInput struct {
	ChallengeID string `json:"challengeID"`
	Signature   string `json:"signature"`
}

// ExchangeAgentAccessLeaseTokenInput is the GraphQL input for exchanging a renewal proof.
type ExchangeAgentAccessLeaseTokenInput struct {
	ChallengeID string `json:"challengeID"`
	Signature   string `json:"signature"`
}
