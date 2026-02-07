package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeEnvAndSetEnv(t *testing.T) {
	base := []string{"A=1", "B=2"}
	merged := mergeEnv(base, map[string]string{
		"B": "3",
		"C": "4",
	})
	require.ElementsMatch(t, []string{"A=1", "B=3", "C=4"}, merged)

	require.Equal(t, []string{"A=1", "B=2", "C=4"}, setEnv(base, "C", "4"))
	require.Equal(t, []string{"A=1", "B=9"}, setEnv(base, "B", "9"))
}

func TestRunCommand_SuccessAndFailure(t *testing.T) {
	require.NoError(t, runCommand(context.Background(), "bash", []string{"-lc", "exit 0"}, execOptions{}))

	err := runCommand(context.Background(), "bash", []string{"-lc", "exit 3"}, execOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bash -lc exit 3:")
}

func TestCacheDirHelpers(t *testing.T) {
	repoRoot := t.TempDir()

	t.Run("defaults", func(t *testing.T) {
		t.Setenv("GOCACHE", "")
		t.Setenv("XDG_CACHE_HOME", "")

		goCache, err := ensureGoCacheDir(repoRoot)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(repoRoot, "tmp", "go-cache", cacheDirVersionKey()), goCache)
		require.DirExists(t, goCache)

		xdgCache, err := ensureXDGCacheDir(repoRoot)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(repoRoot, "tmp", "xdg-cache", cacheDirVersionKey()), xdgCache)
		require.DirExists(t, xdgCache)
	})

	t.Run("overrides", func(t *testing.T) {
		goCacheOverride := filepath.Join(repoRoot, "go-cache-override")
		xdgCacheOverride := filepath.Join(repoRoot, "xdg-cache-override")
		t.Setenv("GOCACHE", goCacheOverride)
		t.Setenv("XDG_CACHE_HOME", xdgCacheOverride)

		otherRoot := t.TempDir()

		goCache, err := ensureGoCacheDir(otherRoot)
		require.NoError(t, err)
		require.Equal(t, goCacheOverride, goCache)
		require.DirExists(t, goCache)

		xdgCache, err := ensureXDGCacheDir(otherRoot)
		require.NoError(t, err)
		require.Equal(t, xdgCacheOverride, xdgCache)
		require.DirExists(t, xdgCache)
	})
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("LESSER_TEST_ENV_OR_DEFAULT", "")
	require.Equal(t, "fallback", envOrDefault("LESSER_TEST_ENV_OR_DEFAULT", "fallback"))

	t.Setenv("LESSER_TEST_ENV_OR_DEFAULT", "  value ")
	require.Equal(t, "value", envOrDefault("LESSER_TEST_ENV_OR_DEFAULT", "fallback"))
}

func TestCaptureCommandOutput_SuccessAndFailure(t *testing.T) {
	dir := t.TempDir()
	out, err := captureCommandOutput(context.Background(), dir, map[string]string{}, "bash", "-lc", "printf 'hello'")
	require.NoError(t, err)
	require.Equal(t, "hello", out)

	_, err = captureCommandOutput(context.Background(), dir, map[string]string{}, "bash", "-lc", "echo boom; exit 2")
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}
