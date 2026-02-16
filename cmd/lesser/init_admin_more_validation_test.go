package main

import (
	"os"
	"path/filepath"
	"testing"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRunInitAdmin_AdditionalValidationBranches(t *testing.T) {
	t.Run("invalid stage rejected", func(t *testing.T) {
		err := runInitAdmin([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--stage", "nope",
			"--wallet-address", "0x4444444444444444444444444444444444444444",
			"--signature", "0xdeadbeef",
			"--message", "consent",
			"--chain-id", "1",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid --stage")
	})

	t.Run("reserved username rejected", func(t *testing.T) {
		err := runInitAdmin([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--stage", "dev",
			"--username", storagemodels.DefaultBootstrapUsername,
			"--wallet-address", "0x4444444444444444444444444444444444444444",
			"--signature", "0xdeadbeef",
			"--message", "consent",
			"--chain-id", "1",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "reserved")
	})

	t.Run("invalid username rejected", func(t *testing.T) {
		err := runInitAdmin([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--stage", "dev",
			"--username", "bad name",
			"--wallet-address", "0x4444444444444444444444444444444444444444",
			"--signature", "0xdeadbeef",
			"--message", "consent",
			"--chain-id", "1",
		})
		require.Error(t, err)
	})

	t.Run("invalid wallet address rejected", func(t *testing.T) {
		err := runInitAdmin([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--stage", "dev",
			"--wallet-address", "not-an-address",
			"--signature", "0xdeadbeef",
			"--message", "consent",
			"--chain-id", "1",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid wallet address")
	})

	t.Run("message-file read error surfaced", func(t *testing.T) {
		err := runInitAdmin([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--stage", "dev",
			"--wallet-address", "0x4444444444444444444444444444444444444444",
			"--signature", "0xdeadbeef",
			"--message-file", filepath.Join(t.TempDir(), "missing"),
			"--chain-id", "1",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "read --message-file")
	})

	t.Run("empty message-file rejected", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.txt")
		require.NoError(t, os.WriteFile(path, []byte(""), 0o600))

		err := runInitAdmin([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--stage", "dev",
			"--wallet-address", "0x4444444444444444444444444444444444444444",
			"--signature", "0xdeadbeef",
			"--message-file", path,
			"--chain-id", "1",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "message is required")
	})
}
