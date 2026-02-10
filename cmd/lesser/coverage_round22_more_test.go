package main

import (
	"context"
	crand "crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestWriteReceipt_ErrorPaths(t *testing.T) {
	t.Run("mkdir all fails when parent is a file", func(t *testing.T) {
		root := t.TempDir()
		parentFile := filepath.Join(root, "not-a-dir")
		require.NoError(t, os.WriteFile(parentFile, []byte("x"), 0o600))

		err := writeReceipt(filepath.Join(parentFile, "state.json"), &upReceipt{App: "app", BaseDomain: "example.com"})
		require.Error(t, err)
	})

	t.Run("write file fails when tmp path is a directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.Mkdir(path+".tmp", 0o700))

		err := writeReceipt(path, &upReceipt{App: "app", BaseDomain: "example.com"})
		require.Error(t, err)
	})
}

func TestWriteBootstrapKeyMaterial_ErrorPaths(t *testing.T) {
	wallet := bootstrapWallet{
		Address:        "0xabc",
		Mnemonic:       "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		DerivationPath: defaultBootstrapDerivationPath,
		ChainID:        1,
	}

	t.Run("create output dir fails when parent is a file", func(t *testing.T) {
		root := t.TempDir()
		parentFile := filepath.Join(root, "not-a-dir")
		require.NoError(t, os.WriteFile(parentFile, []byte("x"), 0o600))

		err := writeBootstrapKeyMaterial(filepath.Join(parentFile, "bootstrap.json"), wallet)
		require.Error(t, err)
		require.Contains(t, err.Error(), "create output dir")
	})

	t.Run("write output file fails when tmp path is a directory", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "bootstrap.json")
		require.NoError(t, os.Mkdir(outPath+".tmp", 0o700))

		err := writeBootstrapKeyMaterial(outPath, wallet)
		require.Error(t, err)
		require.Contains(t, err.Error(), "write output file")
	})

	t.Run("finalize output file fails when destination is a directory", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "bootstrap.json")
		require.NoError(t, os.Mkdir(outPath, 0o700))

		err := writeBootstrapKeyMaterial(outPath, wallet)
		require.Error(t, err)
		require.Contains(t, err.Error(), "finalize output file")
	})
}

func TestRandomBase64_ReadError(t *testing.T) {
	prev := crand.Reader
	crand.Reader = errReader{}
	t.Cleanup(func() { crand.Reader = prev })

	_, err := randomBase64(8)
	require.Error(t, err)
	require.Contains(t, err.Error(), "generate random secret")
}

func TestVerifyEthereumPersonalSign_DecodeSignatureError(t *testing.T) {
	err := verifyEthereumPersonalSign("0x4444444444444444444444444444444444444444", "hello", "not-hex")
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode signature")
}

func TestRunDown_RegionFallbackAndMissingRegionError(t *testing.T) {
	prevRepoRoot := findRepoRootFn
	prevLoadAWS := loadAWSConfigFromProfileFn
	prevEnsureTool := ensureToolAvailableFn
	prevDestroy := cdkDestroyStackFn
	t.Cleanup(func() {
		findRepoRootFn = prevRepoRoot
		loadAWSConfigFromProfileFn = prevLoadAWS
		ensureToolAvailableFn = prevEnsureTool
		cdkDestroyStackFn = prevDestroy
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(string) error { return nil }
	cdkDestroyStackFn = func(context.Context, string, string, cdkDestroyRequest) error { return nil }

	t.Run("falls back to aws config region when receipt is missing region", func(t *testing.T) {
		loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }

		receipt := newUpReceipt(
			"app",
			"example.com",
			"profile",
			"123456789012",
			"",
			[]naming.Stage{naming.StageDev},
			hostedZone{ID: "Z1", Name: "example.com"},
		)
		receipt.SharedStack = "app-shared"
		receipt.Stages["dev"].StackName = "app-dev"

		statePath := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, writeReceipt(statePath, receipt))

		require.NoError(t, runDown([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--state", statePath,
		}))
	})

	t.Run("errors when no region in receipt or aws config", func(t *testing.T) {
		loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: ""}, nil }

		receipt := newUpReceipt(
			"app",
			"example.com",
			"profile",
			"123456789012",
			"",
			[]naming.Stage{naming.StageDev},
			hostedZone{ID: "Z1", Name: "example.com"},
		)
		receipt.SharedStack = "app-shared"
		receipt.Stages["dev"].StackName = "app-dev"

		statePath := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, writeReceipt(statePath, receipt))

		err := runDown([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--state", statePath,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "receipt is missing AWS region")
	})
}

