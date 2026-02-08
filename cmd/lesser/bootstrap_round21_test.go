package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteBootstrapKeyMaterial_RequiresMnemonic(t *testing.T) {
	err := writeBootstrapKeyMaterial(filepath.Join(t.TempDir(), "bootstrap.json"), bootstrapWallet{
		Address:        "0xabc",
		Mnemonic:       "",
		DerivationPath: defaultBootstrapDerivationPath,
		ChainID:        1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mnemonic is empty")
}

func TestReadBootstrapKeyMaterial_ParseAndValidationErrors(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bootstrap.json")
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
		_, err := readBootstrapKeyMaterial(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse bootstrap key material")
	})

	t.Run("missing fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bootstrap.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"wallet":{}}`), 0o600))
		_, err := readBootstrapKeyMaterial(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing address or mnemonic")
	})
}

