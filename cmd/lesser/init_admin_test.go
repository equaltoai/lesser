package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestRunInitAdmin_RejectsReservedWallet(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--username", "app",
		"--wallet-address", "0x80189edb676d51b2fb2257b2ad38e018b20ca46e",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved")
}

func TestRunInitAdmin_ReadsMessageFileAndVerifiesSignatureBeforeAWS(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	addr := crypto.PubkeyToAddress(key.PublicKey).Hex()
	message := "consent"

	msgHash := accounts.TextHash([]byte(message))
	sig, err := crypto.Sign(msgHash, key)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "message.txt")
	require.NoError(t, os.WriteFile(path, []byte(message), 0o600))

	previousLoadAWS := loadAWSConfigFromProfileFn
	t.Cleanup(func() { loadAWSConfigFromProfileFn = previousLoadAWS })

	errSentinel := errors.New("load aws called")
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{}, errSentinel }

	runErr := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--username", "app",
		"--wallet-address", addr,
		"--signature", hexutil.Encode(sig),
		"--message-file", path,
	})
	require.ErrorIs(t, runErr, errSentinel)
}

func TestRejectReservedWallet_ExtraListRejects(t *testing.T) {
	addr := "0x4444444444444444444444444444444444444444"
	require.Error(t, rejectReservedWallet(addr, addr))
}

func TestRejectReservedWallet_InvalidExtraListEntryErrors(t *testing.T) {
	addr := "0x4444444444444444444444444444444444444444"
	err := rejectReservedWallet(addr, "not-an-address")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --reserved-wallets entry")
}

func TestVerifyEthereumPersonalSign_SucceedsWithGeneratedSignature(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	message := "hello"

	msgHash := accounts.TextHash([]byte(message))
	sig, err := crypto.Sign(msgHash, key)
	require.NoError(t, err)

	require.NoError(t, verifyEthereumPersonalSign(address, message, hexutil.Encode(sig)))

	// Some wallets return V as 27/28. Ensure we accept that form too.
	sigWithV := make([]byte, len(sig))
	copy(sigWithV, sig)
	sigWithV[64] += 27
	require.NoError(t, verifyEthereumPersonalSign(address, message, hexutil.Encode(sigWithV)))

	otherKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	otherAddr := crypto.PubkeyToAddress(otherKey.PublicKey).Hex()
	require.Error(t, verifyEthereumPersonalSign(otherAddr, message, hexutil.Encode(sig)))
}

func TestParseInitAdminArgs_ProvisioningInputSuppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provision.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "schema": 1,
  "slug": "app",
  "stage": "dev",
  "admin_wallet_address": "0x4444444444444444444444444444444444444444",
  "admin_username": "alice"
}
`), 0o600))

	args, err := parseInitAdminArgs([]string{
		"--provisioning-input", path,
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.NoError(t, err)
	require.Equal(t, "app", args.App)
	require.Equal(t, "dev", args.Stage)
	require.Equal(t, "alice", args.Username)
	require.Equal(t, "0x4444444444444444444444444444444444444444", args.WalletAddr)
}
