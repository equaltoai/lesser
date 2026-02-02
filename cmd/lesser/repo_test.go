package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLooksLikeRepoRoot(t *testing.T) {
	tmp := t.TempDir()
	require.False(t, looksLikeRepoRoot(tmp))

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/x\n"), 0o644))
	require.False(t, looksLikeRepoRoot(tmp))

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "cdk"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "infra", "cdk", "cdk.json"), []byte("{}"), 0o644))
	require.True(t, looksLikeRepoRoot(tmp))
}

func TestFindRepoRoot_ReturnsErrorOutsideRepo(t *testing.T) {
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))

	_, err = findRepoRoot()
	require.Error(t, err)
}
