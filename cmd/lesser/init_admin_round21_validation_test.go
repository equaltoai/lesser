package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	theorydb "github.com/theory-cloud/tabletheory/pkg/core"
	theorymocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

func TestRunInitAdmin_InvalidAppErrors(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "bad_app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--wallet-address", "0x4444444444444444444444444444444444444444",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid app name")
}

func TestRunInitAdmin_InvalidBaseDomainErrors(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "notadomain",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--wallet-address", "0x4444444444444444444444444444444444444444",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "base domain")
}

func TestRunInitAdmin_InvalidStageErrors(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "wat",
		"--wallet-address", "0x4444444444444444444444444444444444444444",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --stage")
}

func TestRunInitAdmin_ReservedBootstrapUsernameErrors(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--username", "bootstrap",
		"--wallet-address", "0x4444444444444444444444444444444444444444",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved")
}

func TestRunInitAdmin_InvalidUsernameErrors(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--username", "bob!",
		"--wallet-address", "0x4444444444444444444444444444444444444444",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "can only contain")
}

func TestRunInitAdmin_InvalidWalletAddressErrors(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--wallet-address", "not-an-address",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid wallet address")
}

func TestRunInitAdmin_MessageFileReadErrors(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--wallet-address", "0x4444444444444444444444444444444444444444",
		"--signature", "0xdeadbeef",
		"--message-file", "does-not-exist.txt",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read --message-file")
}

func TestRunInitAdmin_InvalidSignatureErrors(t *testing.T) {
	msg := validInitAdminConsentMessage(t, "dev.example.com", "app")
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--wallet-address", "0x4444444444444444444444444444444444444444",
		"--signature", "0xdeadbeef",
		"--message", msg,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature length")
}

func TestRunInitAdmin_SuccessPath_WithDependencyStubs(t *testing.T) {
	prevLoadAWS := loadAWSConfigForCLIFn
	prevNewDB := tabletheoryNewFn
	prevEnsureWalletNotLinked := ensureWalletNotLinkedElsewhereFn
	prevEnsureUser := ensureAdminUserFn
	prevEnsureActor := ensureActorFn
	prevEnsureCred := ensureWalletCredentialFn
	prevEnsureIndex := ensureWalletIndexFn
	prevEnsureActivated := ensureInstanceActivatedFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = prevLoadAWS
		tabletheoryNewFn = prevNewDB
		ensureWalletNotLinkedElsewhereFn = prevEnsureWalletNotLinked
		ensureAdminUserFn = prevEnsureUser
		ensureActorFn = prevEnsureActor
		ensureWalletCredentialFn = prevEnsureCred
		ensureWalletIndexFn = prevEnsureIndex
		ensureInstanceActivatedFn = prevEnsureActivated
	})

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{
			Region:      "us-east-1",
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "TOKEN")),
		}, "", nil
	}

	db := new(theorymocks.MockDB)
	db.On("Close").Return(nil)
	tabletheoryNewFn = func(session.Config) (theorydb.DB, error) { return db, nil }
	ensureWalletNotLinkedElsewhereFn = func(context.Context, theorydb.DB, string, string) error { return nil }
	ensureAdminUserFn = func(context.Context, theorydb.DB, string, time.Time) error { return nil }
	ensureActorFn = func(context.Context, theorydb.DB, *kms.Client, string, string, string, time.Time) error { return nil }
	ensureWalletCredentialFn = func(context.Context, theorydb.DB, string, string, int, time.Time) error { return nil }
	ensureWalletIndexFn = func(context.Context, theorydb.DB, string, string) error { return nil }
	ensureInstanceActivatedFn = func(context.Context, theorydb.DB, string, string, time.Time) error { return nil }

	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	addr := crypto.PubkeyToAddress(priv.PublicKey).Hex()
	msg := validInitAdminConsentMessage(t, "dev.example.com", "app")
	sig := signInitAdminMessage(t, priv, msg)
	// Exercise the V normalization logic (27/28 -> 0/1).
	sig[64] += 27

	msgPath := filepath.Join(t.TempDir(), "message.txt")
	require.NoError(t, os.WriteFile(msgPath, []byte(msg), 0o600))

	require.NoError(t, runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--wallet-address", addr,
		"--signature", hexutil.Encode(sig),
		"--message-file", msgPath,
	}))
}

func TestRunInitAdmin_LoadAWSConfigFailureSurfaces(t *testing.T) {
	prevLoadAWS := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = prevLoadAWS })

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{}, "", errors.New("aws boom")
	}

	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	addr := crypto.PubkeyToAddress(priv.PublicKey).Hex()
	msg := validInitAdminConsentMessage(t, "dev.example.com", "app")
	sig := signInitAdminMessage(t, priv, msg)

	err = runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--wallet-address", addr,
		"--signature", hexutil.Encode(sig),
		"--message", msg,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "aws boom")
}
