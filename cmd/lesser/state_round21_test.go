package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteReceipt_ErrorsOnNilReceipt(t *testing.T) {
	require.Error(t, writeReceipt(filepath.Join(t.TempDir(), "state.json"), nil))
}

func TestReadReceipt_ErrorPaths(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := readReceipt(filepath.Join(t.TempDir(), "missing.json"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "read receipt")
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
		_, err := readReceipt(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse receipt")
	})

	t.Run("missing required fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"app":"","base_domain":""}`), 0o600))
		_, err := readReceipt(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing required fields")
	})
}

