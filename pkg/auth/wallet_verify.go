package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

// VerifySignatureOnly verifies a signature against a challenge without creating a session
// This is used for the second verification in the wallet link flow
func (as *AuthService) VerifySignatureOnly(ctx context.Context, challenge *storage.WalletChallenge, signature string) error {
	logger := common.WithContext(ctx)
	
	// Verify Ethereum signature
	sig, err := hexutil.Decode(signature)
	if err != nil {
		logger.Error("failed to decode signature", zap.Error(err))
		return fmt.Errorf("invalid signature format: %w", err)
	}

	// Ethereum signatures are 65 bytes (r: 32, s: 32, v: 1)
	if len(sig) != 65 {
		logger.Error("invalid signature length", zap.Int("got", len(sig)))
		return fmt.Errorf("invalid signature length: expected 65, got %d", len(sig))
	}

	// Transform V from Ethereum-specific to standard
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}

	// Hash the message with Ethereum prefix
	msgHash := accounts.TextHash([]byte(challenge.Message))

	// Recover public key from signature
	pubKey, err := crypto.SigToPub(msgHash, sig)
	if err != nil {
		logger.Error("failed to recover public key from signature", zap.Error(err))
		return fmt.Errorf("failed to recover public key: %w", err)
	}

	// Get address from public key
	recoveredAddr := crypto.PubkeyToAddress(*pubKey)

	// Normalize both addresses for comparison (remove 0x prefix, lowercase)
	recoveredHex := strings.ToLower(strings.TrimPrefix(recoveredAddr.Hex(), "0x"))
	expectedHex := strings.ToLower(strings.TrimPrefix(challenge.Address, "0x"))

	// Compare addresses
	if recoveredHex != expectedHex {
		logger.Error("signature address mismatch",
			zap.String("expected", expectedHex),
			zap.String("got", recoveredHex))
		return fmt.Errorf("signature address mismatch")
	}

	return nil
}

// GetWalletByAddress retrieves a wallet by address
func (as *AuthService) GetWalletByAddress(ctx context.Context, address string) (*storage.WalletCredential, error) {
	return as.repos.Account().GetWalletCredential(ctx, address)
}

