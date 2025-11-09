package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/spruceid/siwe-go"
	"go.uber.org/zap"
)

// WalletChallenge represents a challenge for wallet authentication
type WalletChallenge struct {
	ID        string    `json:"id"`
	Username  string    `json:"username,omitempty"`
	Address   string    `json:"address"`
	ChainID   int       `json:"chainId"`
	Nonce     string    `json:"nonce"`
	Message   string    `json:"message"`
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// WalletCredential represents a linked wallet
type WalletCredential struct {
	Username string    `json:"username"`
	Address  string    `json:"address"`
	ChainID  int       `json:"chainId"`
	Type     string    `json:"type"` // ethereum, solana, etc.
	ENS      string    `json:"ens,omitempty"`
	LinkedAt time.Time `json:"linkedAt"`
	LastUsed time.Time `json:"lastUsed"`
}

// WalletVerifyRequest represents a wallet signature verification request
type WalletVerifyRequest struct {
	ChallengeID string `json:"challengeId"`
	Address     string `json:"address"`
	Signature   string `json:"signature"`
	Message     string `json:"message"`
}

// WalletService handles wallet authentication
type WalletService struct {
	repos  StorageProvider
	logger *zap.Logger
}

// NewWalletService creates a new wallet service
func NewWalletService(repos StorageProvider) *WalletService {
	return &WalletService{
		repos:  repos,
		logger: common.Logger(),
	}
}

// CreateChallenge creates a new authentication challenge for a wallet
func (s *WalletService) CreateChallenge(ctx context.Context, address string, chainID int, username string) (*storage.WalletChallenge, error) {
	// Normalize address
	address = strings.ToLower(address)

	// Validate username is provided (required for security - binds signature to username)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, errors.New("username is required for challenge creation - signature must bind to username")
	}

	// Generate nonce
	nonce, err := generateNonce()
	if err != nil {
		s.logger.Error("failed to generate nonce", zap.Error(err))
		return nil, errors.Join(ErrNonceGeneration, err)
	}

	// Create SIWE message with username binding
	now := time.Now()
	expiresAt := now.Add(5 * time.Minute)

	message := buildAuthMessage(chainID, nonce, username, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))

	challenge := &storage.WalletChallenge{
		ID:        uuid.New().String(),
		Username:  username,
		Address:   address,
		ChainID:   chainID,
		Nonce:     nonce,
		Message:   message,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}

	// Store challenge in DynamoDB
	if err := s.repos.Account().StoreWalletChallenge(ctx, challenge); err != nil {
		s.logger.Error("failed to store wallet challenge", zap.Error(err), zap.String("challengeId", challenge.ID))
		return nil, errors.Join(ErrChallengeStorage, err)
	}

	s.logger.Info("created wallet challenge",
		zap.String("challengeId", challenge.ID),
		zap.String("address", address),
		zap.Int("chainId", chainID))

	return challenge, nil
}

// VerifySignature verifies a wallet signature and returns user info
func (s *WalletService) VerifySignature(ctx context.Context, req *WalletVerifyRequest) (string, error) {
	// Get challenge from DynamoDB
	challenge, err := s.repos.Account().GetWalletChallenge(ctx, req.ChallengeID)
	if err != nil {
		s.logger.Error("failed to retrieve wallet challenge", zap.Error(err), zap.String("challengeId", req.ChallengeID))
		return "", errors.Join(ErrChallengeRetrieval, err)
	}

	// Check expiration
	if time.Now().After(challenge.ExpiresAt) {
		// Delete expired challenge
		_ = s.repos.Account().DeleteWalletChallenge(ctx, req.ChallengeID)
		return "", ErrChallengeExpired
	}

	// Check if challenge is already spent (used twice)
	if challenge.Spent {
		s.logger.Warn("challenge already spent",
			zap.String("challengeId", req.ChallengeID),
			zap.String("address", req.Address))
		return "", ErrChallengeExpired
	}

	// Verify the message matches (includes cryptographic binding to username)
	if req.Message != challenge.Message {
		return "", ErrMessageMismatch
	}

	// Normalize addresses
	req.Address = strings.ToLower(req.Address)
	challenge.Address = strings.ToLower(challenge.Address)

	// Verify address matches
	if req.Address != challenge.Address {
		return "", ErrAddressMismatch
	}

	// CRITICAL: Verify the username in the challenge matches what was signed
	// This prevents replay attacks where someone uses your signature for a different username
	// The username was embedded in the message and signed by the wallet
	if challenge.Username != "" {
		s.logger.Info("challenge has username binding",
			zap.String("challengeId", req.ChallengeID),
			zap.String("bound_username", challenge.Username))
	} else {
		s.logger.Warn("challenge created without username binding - this is a security risk",
			zap.String("challengeId", req.ChallengeID))
	}

	// Verify Ethereum signature
	if err := s.verifyEthereumSignature(req.Address, req.Message, req.Signature); err != nil {
		s.logger.Error("failed to verify Ethereum signature", zap.Error(err), zap.String("address", req.Address))
		return "", errors.Join(ErrSignatureVerification, err)
	}

	// Mark challenge as used (first verification)
	// The challenge can be used once more for wallet linking
	if err := s.repos.Account().MarkWalletChallengeUsed(ctx, req.ChallengeID); err != nil {
		s.logger.Warn("failed to mark challenge as used", zap.Error(err))
		// Non-fatal - continue
	}

	// Check if wallet is linked to an account
	username := challenge.Username
	if err := common.ValidateRequiredParam("username", username); err != nil {
		// Try to find existing link
		wallet, err := s.repos.Account().GetWalletCredential(ctx, req.Address)
		if err == nil && wallet != nil {
			username = wallet.Username
		}
	}

	// Update last used time if wallet exists
	if username != "" {
		if err := s.repos.Account().UpdateWalletLastUsed(ctx, username, req.Address); err != nil {
			s.logger.Error("failed to update wallet last used", zap.Error(err))
		}
	}

	s.logger.Info("verified wallet signature",
		zap.String("address", req.Address),
		zap.String("username", username))

	return username, nil
}

// LinkWallet links a wallet to an existing user account
func (s *WalletService) LinkWallet(ctx context.Context, username, address string, chainID int, walletType string) error {
	// Normalize address
	address = strings.ToLower(address)

	// Check if this wallet is already linked to THIS user (prevent duplicate entries)
	existingWallets, err := s.repos.Account().GetUserWalletCredentials(ctx, username)
	if err != nil && !dynamorm.IsNotFound(err) {
		s.logger.Error("failed to check user's existing wallets", zap.Error(err), zap.String("username", username))
		return errors.Join(ErrWalletCheck, err)
	}

	// Check if this specific user already has this wallet linked
	for _, wallet := range existingWallets {
		if strings.EqualFold(wallet.Address, address) {
			s.logger.Info("wallet already linked to this user",
				zap.String("username", username),
				zap.String("address", address))
			return nil // Already linked - idempotent operation
		}
	}

	// Create wallet credential
	wallet := &storage.WalletCredential{
		Username: username,
		Address:  address,
		ChainID:  chainID,
		Type:     walletType,
		LinkedAt: time.Now(),
		LastUsed: time.Now(),
	}

	// Store wallet credential
	if err := s.repos.Account().StoreWalletCredential(ctx, wallet); err != nil {
		s.logger.Error("failed to store wallet credential", zap.Error(err), zap.String("username", username), zap.String("address", address))
		return errors.Join(ErrWalletStorage, err)
	}

	s.logger.Info("linked wallet to account",
		zap.String("username", username),
		zap.String("address", address),
		zap.String("type", walletType))

	return nil
}

// GetUserWallets returns all wallets linked to a user
func (s *WalletService) GetUserWallets(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	wallets, err := s.repos.Account().GetUserWalletCredentials(ctx, username)
	if err != nil {
		s.logger.Error("failed to get user wallet credentials", zap.Error(err), zap.String("username", username))
		return nil, errors.Join(ErrWalletRetrieval, err)
	}

	if wallets == nil {
		return []*storage.WalletCredential{}, nil
	}

	return wallets, nil
}

// UnlinkWallet removes a wallet link from a user account
func (s *WalletService) UnlinkWallet(ctx context.Context, username, address string) error {
	// Normalize address
	address = strings.ToLower(address)

	// Delete wallet credential
	if err := s.repos.Account().DeleteWalletCredential(ctx, username, address); err != nil {
		s.logger.Error("failed to delete wallet credential", zap.Error(err), zap.String("username", username), zap.String("address", address))
		return errors.Join(ErrWalletDeletion, err)
	}

	s.logger.Info("unlinked wallet from account",
		zap.String("username", username),
		zap.String("address", address))

	return nil
}

// Private helper methods

func (s *WalletService) verifyEthereumSignature(address, message, signature string) error {
	// Decode signature
	sig, err := hexutil.Decode(signature)
	if err != nil {
		s.logger.Error("failed to decode signature", zap.Error(err), zap.String("signature", signature))
		return errors.Join(ErrInvalidSignatureFormat, err)
	}

	// Ethereum signatures are 65 bytes (r: 32, s: 32, v: 1)
	if len(sig) != 65 {
		s.logger.Error("invalid signature length", zap.Int("got", len(sig)), zap.Int("expected", 65))
		return ErrInvalidSignatureLength
	}

	// Transform V from Ethereum-specific to standard
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}

	// Hash the message with Ethereum prefix
	msgHash := accounts.TextHash([]byte(message))

	// Recover public key from signature
	pubKey, err := crypto.SigToPub(msgHash, sig)
	if err != nil {
		s.logger.Error("failed to recover public key from signature", zap.Error(err))
		return errors.Join(ErrPublicKeyRecovery, err)
	}

	// Get address from public key
	recoveredAddr := crypto.PubkeyToAddress(*pubKey)

	// Normalize both addresses for comparison (remove 0x prefix, lowercase)
	recoveredHex := strings.ToLower(strings.TrimPrefix(recoveredAddr.Hex(), "0x"))
	expectedHex := strings.ToLower(strings.TrimPrefix(address, "0x"))

	// Compare addresses
	if recoveredHex != expectedHex {
		s.logger.Error("signature address mismatch",
			zap.String("expected", expectedHex),
			zap.String("got", recoveredHex))
		return ErrSignatureAddressMismatch
	}

	// Additional validation using SIWE library if message follows SIWE format
	if strings.Contains(message, "Sign this message to authenticate with Lesser") {
		_, err := siwe.ParseMessage(message)
		if err != nil {
			s.logger.Warn("message is not valid SIWE format", zap.Error(err))
			// Don't fail here, as we already verified the signature
		}
	}

	return nil
}

func generateNonce() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// buildAuthMessage creates the authentication message without fmt.Sprintf
func buildAuthMessage(chainID int, nonce, username, issuedAt, expiresAt string) string {
	var sb strings.Builder
	sb.WriteString("Sign this message to authenticate with Lesser as '")
	sb.WriteString(username)
	sb.WriteString("'\n\n")
	sb.WriteString("URI: https://lesser.app\n")
	sb.WriteString("Version: 1\n")
	sb.WriteString("Chain ID: ")

	// Convert chainID to string manually
	if chainID == 0 {
		sb.WriteString("0")
	} else {
		// Simple integer to string conversion for positive numbers
		digits := make([]byte, 0, 10)
		n := chainID
		for n > 0 {
			digits = append([]byte{byte('0' + n%10)}, digits...)
			n /= 10
		}
		sb.Write(digits)
	}

	sb.WriteString("\nUsername: ")
	sb.WriteString(username)
	sb.WriteString("\nNonce: ")
	sb.WriteString(nonce)
	sb.WriteString("\nIssued At: ")
	sb.WriteString(issuedAt)
	sb.WriteString("\nExpiration Time: ")
	sb.WriteString(expiresAt)

	return sb.String()
}
