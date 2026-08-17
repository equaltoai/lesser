package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/spruceid/siwe-go"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
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
	repo        walletRepository
	logger      *zap.Logger
	auditLogger *AuditLogger
}

type walletRepository interface {
	StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error
	GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error)
	DeleteWalletChallenge(ctx context.Context, challengeID string) error
	MarkWalletChallengeUsed(ctx context.Context, challengeID string) error

	GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error)
	GetWalletCredential(ctx context.Context, address string) (*storage.WalletCredential, error)
	UpdateWalletLastUsed(ctx context.Context, username, address string) error
	GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error)
	StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error
	DeleteWalletCredential(ctx context.Context, username, address string) error
	DeleteWalletCredentialConditionedOnSurvivor(ctx context.Context, username, address, walletType string, survivingPasskeyID string, survivingWalletAddress string) error
}

// NewWalletService creates a new wallet service
func NewWalletService(repos StorageProvider) *WalletService {
	if repos == nil || repos.Account() == nil {
		return &WalletService{logger: common.Logger()}
	}
	return &WalletService{
		repo:        repos.Account(),
		logger:      common.Logger(),
		auditLogger: NewAuditLogger(repos, common.Logger(), DefaultAuditConfig()),
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

	instanceDomain := strings.TrimSpace(config.Get().Domain)
	if instanceDomain == "" {
		instanceDomain = "lesser.app"
	}

	message := buildAuthMessage(instanceDomain, address, chainID, nonce, username, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))

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
	if err := s.repo.StoreWalletChallenge(ctx, challenge); err != nil {
		s.logger.Error("failed to store wallet challenge", append([]zap.Field{zap.Error(err)}, challengeLogFields(challenge.ID)...)...)
		return nil, errors.Join(ErrChallengeStorage, err)
	}

	fields := append(challengeLogFields(challenge.ID),
		zap.String("address", address),
		zap.Int("chainId", chainID),
	)
	s.logger.Info("created wallet challenge", fields...)

	return challenge, nil
}

// VerifySignature verifies a wallet signature and returns user info
func (s *WalletService) VerifySignature(ctx context.Context, req *WalletVerifyRequest) (string, error) {
	// Get challenge from DynamoDB
	challenge, err := s.repo.GetWalletChallenge(ctx, req.ChallengeID)
	if err != nil {
		s.logger.Error("failed to retrieve wallet challenge", append([]zap.Field{zap.Error(err)}, challengeLogFields(req.ChallengeID)...)...)
		return "", errors.Join(ErrChallengeRetrieval, err)
	}

	// Check expiration
	if time.Now().After(challenge.ExpiresAt) {
		// Delete expired challenge
		_ = s.repo.DeleteWalletChallenge(ctx, req.ChallengeID)
		return "", ErrChallengeExpired
	}

	// Check if challenge is already spent (used twice)
	if challenge.Spent {
		fields := append(challengeLogFields(req.ChallengeID), zap.String("address", req.Address))
		s.logger.Warn("challenge already spent", fields...)
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
		fields := append(challengeLogFields(req.ChallengeID), zap.String("bound_username", challenge.Username))
		s.logger.Info("challenge has username binding", fields...)
	} else {
		s.logger.Warn("challenge created without username binding - this is a security risk", challengeLogFields(req.ChallengeID)...)
	}

	// Verify Ethereum signature
	if err := s.verifyEthereumSignature(req.Address, req.Message, req.Signature); err != nil {
		s.logger.Error("failed to verify Ethereum signature", zap.Error(err), zap.String("address", req.Address))
		return "", errors.Join(ErrSignatureVerification, err)
	}

	// Mark challenge as used (first verification)
	// The challenge can be used once more for wallet linking
	if err := s.repo.MarkWalletChallengeUsed(ctx, req.ChallengeID); err != nil {
		s.logger.Warn("failed to mark challenge as used", zap.Error(err))
		// Non-fatal - continue
	}

	// Check if wallet is linked to an account
	username := challenge.Username
	if err := common.ValidateRequiredParam("username", username); err != nil {
		// Try to find existing link
		wallet, err := s.repo.GetWalletCredential(ctx, req.Address)
		if err == nil && wallet != nil {
			username = wallet.Username
		}
	}

	// Update last used time if wallet exists
	if username != "" {
		if err := s.repo.UpdateWalletLastUsed(ctx, username, req.Address); err != nil {
			s.logger.Error("failed to update wallet last used", zap.Error(err))
		}
	}

	s.logger.Info("verified wallet signature",
		zap.String("address", req.Address),
		zap.String("username", username))

	return username, nil
}

// LinkWallet links a wallet to an existing user account.
// The returned boolean reports whether this call created a new link.
func (s *WalletService) LinkWallet(ctx context.Context, username, address string, chainID int, walletType string) (bool, error) {
	// Normalize address
	address = strings.ToLower(address)

	// Check if this wallet is already linked to THIS user (prevent duplicate entries)
	existingWallets, err := s.repo.GetUserWalletCredentials(ctx, username)
	if err != nil && !theorydb.IsNotFound(err) {
		s.logger.Error("failed to check user's existing wallets", zap.Error(err), zap.String("username", username))
		return false, errors.Join(ErrWalletCheck, err)
	}

	// Check if this specific user already has this wallet linked
	for _, wallet := range existingWallets {
		if strings.EqualFold(wallet.Address, address) {
			s.logger.Info("wallet already linked to this user",
				zap.String("username", username),
				zap.String("address", address))
			return false, nil // Already linked - idempotent operation
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
	if err := s.repo.StoreWalletCredential(ctx, wallet); err != nil {
		s.logger.Error("failed to store wallet credential", zap.Error(err), zap.String("username", username), zap.String("address", address))
		return false, errors.Join(ErrWalletStorage, err)
	}
	if s.auditLogger != nil {
		s.auditLogger.LogAuthEvent(ctx, username, "", "", AuditWalletConnected, map[string]interface{}{
			"authentication_method": "wallet",
			"credential_event":      "added",
			"wallet_type":           walletType,
		}, true, nil)
	}

	s.logger.Info("linked wallet to account",
		zap.String("username", username),
		zap.String("address", address),
		zap.String("type", walletType))

	return true, nil
}

// GetUserWallets returns all wallets linked to a user
func (s *WalletService) GetUserWallets(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	wallets, err := s.repo.GetUserWalletCredentials(ctx, username)
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

	plan, err := planAuthenticatorRemoval(ctx, s.repo, username, authenticatorRemovalWallet, "", address)
	if err != nil {
		return err
	}

	wallets, err := s.repo.GetUserWalletCredentials(ctx, username)
	if err != nil {
		s.logger.Error("failed to get user wallet credentials", zap.Error(err), zap.String("username", username))
		return errors.Join(ErrWalletRetrieval, err)
	}
	walletType := ""
	for _, wallet := range wallets {
		if wallet != nil && strings.EqualFold(wallet.Address, address) {
			walletType = wallet.Type
			break
		}
	}

	// Delete wallet credential
	if err := s.repo.DeleteWalletCredentialConditionedOnSurvivor(
		ctx,
		username,
		address,
		walletType,
		plan.survivingPasskeyID,
		plan.survivingWalletAddress,
	); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return ErrLastAuthMethodDelete
		}
		s.logger.Error("failed to delete wallet credential", zap.Error(err), zap.String("username", username), zap.String("address", address))
		return errors.Join(ErrWalletDeletion, err)
	}
	if s.auditLogger != nil {
		s.auditLogger.LogAuthEvent(ctx, username, "", "", AuditWalletDisconnected, map[string]interface{}{
			"authentication_method": "wallet",
			"credential_event":      "removed",
		}, true, nil)
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
		s.logger.Error("failed to decode signature",
			zap.Error(err),
			zap.Int("signature_len", len(signature)),
			zap.Bool("signature_has_0x_prefix", strings.HasPrefix(strings.ToLower(signature), "0x")))
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

func challengeLogFields(challengeID string) []zap.Field {
	return []zap.Field{
		zap.Bool("challenge_reference_present", strings.TrimSpace(challengeID) != ""),
	}
}

func generateNonce() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// buildAuthMessage creates the authentication message without fmt.Sprintf
func buildAuthMessage(domain, address string, chainID int, nonce, username, issuedAt, expiresAt string) string {
	var sb strings.Builder

	address = strings.ToLower(strings.TrimSpace(address))

	sb.WriteString(domain)
	sb.WriteString(" wants you to sign in with your Ethereum account:\n")
	sb.WriteString(address)
	sb.WriteString("\n\n")
	sb.WriteString("Sign this message to authenticate with Lesser as '")
	sb.WriteString(username)
	sb.WriteString("'\n\n")
	sb.WriteString("URI: https://")
	sb.WriteString(domain)
	sb.WriteString("\nVersion: 1\nChain ID: ")
	sb.WriteString(strconv.Itoa(chainID))
	sb.WriteString("\nNonce: ")
	sb.WriteString(nonce)
	sb.WriteString("\nIssued At: ")
	sb.WriteString(issuedAt)
	sb.WriteString("\nExpiration Time: ")
	sb.WriteString(expiresAt)

	return sb.String()
}
