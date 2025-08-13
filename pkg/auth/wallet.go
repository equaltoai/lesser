package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"

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

	// Generate nonce
	nonce, err := generateNonce()
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Create SIWE message
	now := time.Now()
	expiresAt := now.Add(5 * time.Minute)

	message := fmt.Sprintf(
		"Sign this message to authenticate with Lesser.\n\nURI: https://lesser.app\nVersion: 1\nChain ID: %d\nNonce: %s\nIssued At: %s\nExpiration Time: %s",
		chainID,
		nonce,
		now.Format(time.RFC3339),
		expiresAt.Format(time.RFC3339),
	)

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
		return nil, fmt.Errorf("failed to store challenge: %w", err)
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
		return "", fmt.Errorf("failed to get challenge: %w", err)
	}

	// Check expiration
	if time.Now().After(challenge.ExpiresAt) {
		// Delete expired challenge
		_ = s.repos.Account().DeleteWalletChallenge(ctx, req.ChallengeID)
		return "", errors.New("challenge expired")
	}

	// Verify the message matches
	if req.Message != challenge.Message {
		return "", errors.New("message mismatch")
	}

	// Normalize addresses
	req.Address = strings.ToLower(req.Address)
	challenge.Address = strings.ToLower(challenge.Address)

	// Verify address matches
	if req.Address != challenge.Address {
		return "", errors.New("address mismatch")
	}

	// Verify Ethereum signature
	if err := s.verifyEthereumSignature(req.Address, req.Message, req.Signature); err != nil {
		return "", fmt.Errorf("signature verification failed: %w", err)
	}

	// Delete used challenge
	if err := s.repos.Account().DeleteWalletChallenge(ctx, req.ChallengeID); err != nil {
		s.logger.Error("failed to delete challenge", zap.Error(err))
	}

	// Check if wallet is linked to an account
	username := challenge.Username
	if username == "" {
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

	// Check if wallet is already linked
	existing, err := s.repos.Account().GetWalletCredential(ctx, address)
	if err != nil {
		return fmt.Errorf("failed to check existing wallet: %w", err)
	}

	if existing != nil {
		if existing.Username != username {
			return errors.New("wallet already linked to another account")
		}
		return nil // Already linked to this user
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
		return fmt.Errorf("failed to store wallet: %w", err)
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
		return nil, fmt.Errorf("failed to get user wallets: %w", err)
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
		return fmt.Errorf("failed to delete wallet: %w", err)
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
		return fmt.Errorf("invalid signature format: %w", err)
	}

	// Ethereum signatures are 65 bytes (r: 32, s: 32, v: 1)
	if len(sig) != 65 {
		return fmt.Errorf("invalid signature length: %d", len(sig))
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
		return fmt.Errorf("failed to recover public key: %w", err)
	}

	// Get address from public key
	recoveredAddr := crypto.PubkeyToAddress(*pubKey)

	// Compare addresses (case-insensitive)
	if !strings.EqualFold(recoveredAddr.Hex(), address) {
		return fmt.Errorf("signature address mismatch: expected %s, got %s", address, recoveredAddr.Hex())
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
