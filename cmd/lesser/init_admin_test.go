package main

import (
	"os"
	"path/filepath"
	"testing"

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
