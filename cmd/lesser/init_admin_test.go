package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	message := validInitAdminConsentMessage(t, "dev.example.com", "app")
	sig := signInitAdminMessage(t, key, message)

	path := filepath.Join(t.TempDir(), "message.txt")
	require.NoError(t, os.WriteFile(path, []byte(message), 0o600))

	previousLoadAWS := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWS })

	errSentinel := errors.New("load aws called")
	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{}, "", errSentinel
	}

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

func TestValidateInitAdminConsentMessage(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	valid := initAdminConsentMessage{
		Kind:      initAdminConsentKind,
		Instance:  "https://dev.example.com",
		Username:  "alice",
		Nonce:     "nonce-1234567890",
		ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339),
	}
	messageBytes, err := json.Marshal(valid)
	require.NoError(t, err)

	require.NoError(t, validateInitAdminConsentMessage(string(messageBytes), "dev.example.com", "alice", now))

	tests := []struct {
		name   string
		mutate func(*initAdminConsentMessage)
		want   string
	}{
		{
			name: "instance mismatch",
			mutate: func(msg *initAdminConsentMessage) {
				msg.Instance = "https://live.example.com"
			},
			want: "instance mismatch",
		},
		{
			name: "username mismatch",
			mutate: func(msg *initAdminConsentMessage) {
				msg.Username = "bob"
			},
			want: "username mismatch",
		},
		{
			name: "missing nonce",
			mutate: func(msg *initAdminConsentMessage) {
				msg.Nonce = ""
			},
			want: "nonce",
		},
		{
			name: "expired",
			mutate: func(msg *initAdminConsentMessage) {
				msg.ExpiresAt = now.Add(-time.Minute).Format(time.RFC3339)
			},
			want: "expired",
		},
		{
			name: "too far future",
			mutate: func(msg *initAdminConsentMessage) {
				msg.ExpiresAt = now.Add(2 * time.Hour).Format(time.RFC3339)
			},
			want: "too far",
		},
		{
			name: "wrong kind",
			mutate: func(msg *initAdminConsentMessage) {
				msg.Kind = "other"
			},
			want: "kind",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			tc.mutate(&candidate)
			data, err := json.Marshal(candidate)
			require.NoError(t, err)

			err = validateInitAdminConsentMessage(string(data), "dev.example.com", "alice", now)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestInitAdminConsentInstanceNormalization(t *testing.T) {
	got, err := normalizeInitAdminConsentInstance("dev.example.com.")
	require.NoError(t, err)
	require.Equal(t, "dev.example.com", got)

	_, err = normalizeInitAdminConsentInstance("http://dev.example.com")
	require.Error(t, err)

	_, err = normalizeInitAdminConsentInstance("https://dev.example.com/path")
	require.Error(t, err)

	_, err = normalizeInitAdminConsentInstance("https://user@dev.example.com")
	require.Error(t, err)
}

func TestValidateInitAdminConsentMessageRejectsMalformedJSON(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	err := validateInitAdminConsentMessage(`{"kind":"lesser.init_admin_consent.v1","unknown":true}`, "dev.example.com", "alice", now)
	require.Error(t, err)

	valid := initAdminConsentMessage{
		Kind:      initAdminConsentKind,
		Instance:  "https://dev.example.com",
		Username:  "alice",
		Nonce:     "nonce-1234567890",
		ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339),
	}
	data, err := json.Marshal(valid)
	require.NoError(t, err)

	err = validateInitAdminConsentMessage(string(data)+" {}", "dev.example.com", "alice", now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing")
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
  "admin_username": "alice",
  "admin_wallet_chain_id": 11155111,
  "consent_message": "consent",
  "consent_signature": "0xdeadbeef"
}
`), 0o600))

	args, err := parseInitAdminArgs([]string{
		"--provisioning-input", path,
		"--base-domain", "example.com",
	})
	require.NoError(t, err)
	require.Equal(t, "app", args.App)
	require.Equal(t, "dev", args.Stage)
	require.Equal(t, "alice", args.Username)
	require.Equal(t, "0x4444444444444444444444444444444444444444", args.WalletAddr)
	require.Equal(t, 11155111, args.ChainID)
	require.Equal(t, "consent", args.Message)
	require.Equal(t, "0xdeadbeef", args.Signature)
}

func validInitAdminConsentMessage(t *testing.T, instance string, username string) string {
	t.Helper()

	payload := initAdminConsentMessage{
		Kind:      initAdminConsentKind,
		Instance:  "https://" + instance,
		Username:  username,
		Nonce:     "test-nonce-1234567890",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

func signInitAdminMessage(t *testing.T, key *ecdsa.PrivateKey, message string) []byte {
	t.Helper()
	msgHash := accounts.TextHash([]byte(message))
	sig, err := crypto.Sign(msgHash, key)
	require.NoError(t, err)
	return sig
}
