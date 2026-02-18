package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadManagedProvisioningInput_ValidationAndDefaults(t *testing.T) {
	t.Run("empty path errors", func(t *testing.T) {
		_, err := readManagedProvisioningInput("   ")
		require.Error(t, err)
		require.Contains(t, err.Error(), "path is empty")
	})

	t.Run("missing file errors", func(t *testing.T) {
		_, err := readManagedProvisioningInput(filepath.Join(t.TempDir(), "missing.json"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "read managed provisioning input")
	})

	t.Run("empty file errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.json")
		require.NoError(t, os.WriteFile(path, []byte("   "), 0o600))
		_, err := readManagedProvisioningInput(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "input is empty")
	})

	t.Run("invalid json errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.json")
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
		_, err := readManagedProvisioningInput(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse managed provisioning input")
	})

	t.Run("unsupported schema errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"schema":3,"slug":"app","stage":"dev","admin_wallet_address":"0x1"}`), 0o600))
		_, err := readManagedProvisioningInput(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported")
	})

	t.Run("missing required fields errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"schema":1,"slug":"","stage":"dev","admin_wallet_address":""}`), 0o600))
		_, err := readManagedProvisioningInput(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing required fields")
	})

	t.Run("defaults schema and admin_username", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"slug":"app","stage":"dev","admin_wallet_address":"0x1111111111111111111111111111111111111111"}`), 0o600))
		in, err := readManagedProvisioningInput(path)
		require.NoError(t, err)
		require.Equal(t, 1, in.Schema)
		require.Equal(t, "app", in.AdminUsername)
	})

	t.Run("captures consent fields when provided", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.json")
		require.NoError(t, os.WriteFile(path, []byte(`{
  "schema": 1,
  "slug": "app",
  "stage": "dev",
  "admin_wallet_address": "0x1111111111111111111111111111111111111111",
  "admin_wallet_chain_id": 11155111,
  "consent_message": " consent ",
  "consent_signature": " 0xdeadbeef "
}`), 0o600))
		in, err := readManagedProvisioningInput(path)
		require.NoError(t, err)
		require.Equal(t, 11155111, in.AdminWalletChainID)
		require.Equal(t, "consent", in.ConsentMessage)
		require.Equal(t, "0xdeadbeef", in.ConsentSignature)
	})

	t.Run("captures integration config when provided", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.json")
		require.NoError(t, os.WriteFile(path, []byte(`{
  "schema": 1,
  "slug": "app",
  "stage": "dev",
  "admin_wallet_address": "0x1111111111111111111111111111111111111111",
  "lesser_host_url": " https://lab.lesser.host/ ",
  "lesser_host_attestations_url": " https://attest.lab.lesser.host/ ",
  "lesser_host_instance_key_arn": " arn:aws:secretsmanager:us-east-1:123456789012:secret:instanceKey ",
  "translation_enabled": false,
  "tip_enabled": true,
  "tip_chain_id": 10,
  "tip_contract_address": " 0xabc ",
  "ai_enabled": true
}`), 0o600))
		in, err := readManagedProvisioningInput(path)
		require.NoError(t, err)
		require.Equal(t, "https://lab.lesser.host", in.LesserHostURL)
		require.Equal(t, "https://attest.lab.lesser.host", in.LesserHostAttestationsURL)
		require.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:instanceKey", in.LesserHostInstanceKeyARN)
		require.NotNil(t, in.TranslationEnabled)
		require.False(t, *in.TranslationEnabled)
		require.NotNil(t, in.TipEnabled)
		require.True(t, *in.TipEnabled)
		require.NotNil(t, in.TipChainID)
		require.Equal(t, 10, *in.TipChainID)
		require.Equal(t, "0xabc", in.TipContractAddress)
		require.NotNil(t, in.AIEnabled)
		require.True(t, *in.AIEnabled)
	})
}
