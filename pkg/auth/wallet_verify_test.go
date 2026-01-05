package auth

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthService_VerifySignatureOnly(t *testing.T) {
	ctx := context.Background()
	as := &AuthService{}

	challenge := &storage.WalletChallenge{
		Message: "hello",
		Address: "0xabc",
	}

	require.Error(t, as.VerifySignatureOnly(ctx, challenge, "not-hex"))
	require.Error(t, as.VerifySignatureOnly(ctx, challenge, "0x01"))

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	msgHash := accounts.TextHash([]byte(challenge.Message))
	sig, err := crypto.Sign(msgHash, privateKey)
	require.NoError(t, err)

	validChallenge := &storage.WalletChallenge{
		Message: "hello",
		Address: address,
	}

	require.NoError(t, as.VerifySignatureOnly(ctx, validChallenge, hexutil.Encode(sig)))

	// V normalization branch (27/28 -> 0/1).
	sigWithV := append([]byte(nil), sig...)
	sigWithV[64] += 27
	require.NoError(t, as.VerifySignatureOnly(ctx, validChallenge, hexutil.Encode(sigWithV)))

	// Mismatched address should fail.
	invalidAddrChallenge := &storage.WalletChallenge{Message: "hello", Address: "0x0000000000000000000000000000000000000000"}
	err = as.VerifySignatureOnly(ctx, invalidAddrChallenge, hexutil.Encode(sig))
	assert.Error(t, err)

	// Public key recovery error branch (valid length, invalid signature).
	invalidSig := make([]byte, 65)
	err = as.VerifySignatureOnly(ctx, validChallenge, hexutil.Encode(invalidSig))
	assert.Error(t, err)
}
