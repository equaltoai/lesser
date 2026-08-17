package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
)

// SetupStageURLs describes the URLs relevant to a stage (single-domain path routing).
type SetupStageURLs struct {
	Client      string `json:"client"`
	API         string `json:"api"`
	Auth        string `json:"auth"`
	Setup       string `json:"setup"`
	SetupStatus string `json:"setup_status"`
	WS          string `json:"ws"`
	Media       string `json:"media"`
	AuthSetup   string `json:"auth_setup"`
}

// SetupBootstrapActor describes the bootstrap actor identity.
type SetupBootstrapActor struct {
	Username string `json:"username"`
	Acct     string `json:"acct"`
	Actor    string `json:"actor"`
}

// SetupBootstrapState describes bootstrap activation state.
type SetupBootstrapState struct {
	Username           string `json:"username"`
	WalletAddressSet   bool   `json:"wallet_address_set"`
	WalletAddress      string `json:"wallet_address,omitempty"`
	PrimaryAdminSet    bool   `json:"primary_admin_set"`
	PrimaryAdmin       string `json:"primary_admin,omitempty"`
	SetupSessionScheme string `json:"setup_session_scheme"`
}

// SetupStatusResponse represents GET /setup/status.
type SetupStatusResponse struct {
	InstanceState   string              `json:"instance_state"`
	Locked          bool                `json:"locked"`
	FinalizeAllowed bool                `json:"finalize_allowed"`
	BootstrapActor  SetupBootstrapActor `json:"bootstrap_actor"`
	URLs            SetupStageURLs      `json:"urls"`
	Bootstrap       SetupBootstrapState `json:"bootstrap"`
	ActivatedAt     *time.Time          `json:"activated_at,omitempty"`
}

// SetupBootstrapChallengeRequest represents POST /setup/bootstrap/challenge.
type SetupBootstrapChallengeRequest struct {
	Address string `json:"address"`
	ChainID int    `json:"chainId,omitempty"`
}

// SetupBootstrapChallengeResponse represents POST /setup/bootstrap/challenge.
//
// Includes backwards-compatible fields used by earlier clients.
type SetupBootstrapChallengeResponse struct {
	ChallengeID string    `json:"challenge_id"`
	Challenge   string    `json:"challenge"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`

	ID              string    `json:"id"`
	Username        string    `json:"username"`
	Address         string    `json:"address"`
	ChainID         int       `json:"chainId"`
	Nonce           string    `json:"nonce"`
	Message         string    `json:"message"`
	IssuedAtLegacy  time.Time `json:"issuedAt"`
	ExpiresAtLegacy time.Time `json:"expiresAt"`
}

// SetupBootstrapVerifyRequest represents POST /setup/bootstrap/verify.
//
// Supports both snake_case and camelCase variants for compatibility.
type SetupBootstrapVerifyRequest struct {
	ChallengeID      string `json:"challengeId,omitempty"`
	ChallengeIDSnake string `json:"challenge_id,omitempty"`
	Address          string `json:"address"`
	Signature        string `json:"signature"`
	Message          string `json:"message,omitempty"`
	Challenge        string `json:"challenge,omitempty"`
}

// SetupBootstrapVerifyResponse represents POST /setup/bootstrap/verify.
type SetupBootstrapVerifyResponse struct {
	TokenType  string    `json:"token_type"`
	Token      string    `json:"token"`
	SetupToken string    `json:"setup_token"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// SetupCreateAdminRequest represents POST /setup/admin.
type SetupCreateAdminRequest struct {
	Username                 string                   `json:"username"`
	DisplayName              string                   `json:"displayName,omitempty"`
	Wallet                   auth.WalletVerifyRequest `json:"wallet,omitempty"`
	PasskeyRegistrationProof string                   `json:"passkey_registration_proof,omitempty"`
}

// SetupCreateAdminResponse represents POST /setup/admin.
type SetupCreateAdminResponse struct {
	Username string `json:"username"`
	Actor    string `json:"actor"`
}

// SetupFinalizeResponse represents POST /setup/finalize.
type SetupFinalizeResponse struct {
	InstanceState string         `json:"instance_state"`
	Locked        bool           `json:"locked"`
	ActivatedAt   *time.Time     `json:"activated_at,omitempty"`
	URLs          SetupStageURLs `json:"urls"`
}
