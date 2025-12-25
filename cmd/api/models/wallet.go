package models

import "github.com/equaltoai/lesser/pkg/storage"

// WalletChallengeRequest represents POST /auth/wallet/challenge.
type WalletChallengeRequest struct {
	Address  string `json:"address"`
	ChainID  int    `json:"chainId,omitempty"`
	Username string `json:"username"`
}

// WalletVerifyResponse represents the success payload for POST /auth/wallet/verify.
type WalletVerifyResponse struct {
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

// WalletLinkRequest represents POST /auth/wallet/link.
type WalletLinkRequest struct {
	Address     string `json:"address"`
	ChainID     int    `json:"chainId,omitempty"`
	WalletType  string `json:"walletType,omitempty"`
	ChallengeID string `json:"challengeId,omitempty"`
	Signature   string `json:"signature,omitempty"`
	Message     string `json:"message,omitempty"`
	Username    string `json:"username,omitempty"`
}

// WalletLinkResponse represents the success payload for POST /auth/wallet/link.
type WalletLinkResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	Address     string `json:"address"`
	AccessToken string `json:"access_token,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
}

// WalletUnlinkResponse represents the success payload for DELETE /auth/wallet/unlink/{address}.
type WalletUnlinkResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Address string `json:"address"`
}

// WalletListResponse represents GET /auth/wallet/list.
type WalletListResponse struct {
	Wallets []*storage.WalletCredential `json:"wallets"`
	Count   int                         `json:"count"`
}
