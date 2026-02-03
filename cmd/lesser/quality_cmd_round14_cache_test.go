package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureGolangCILintCacheFresh_Round14_StampReuseAndWriteErrors(t *testing.T) {
	previousRunCommand := runCommandFn
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		captureCommandOutputFn = previousCapture
	})

	repoRoot := t.TempDir()
	xdgCache := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "golangci-lint has version 2.5.0\n", nil
	}

	cleanCalls := 0
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "golangci-lint" && len(args) >= 2 && args[0] == "cache" && args[1] == "clean" {
			cleanCalls++
		}
		return nil
	}

	require.NoError(t, ensureGolangCILintCacheFresh(repoRoot, xdgCache))
	require.Equal(t, 1, cleanCalls)

	cleanCalls = 0
	require.NoError(t, ensureGolangCILintCacheFresh(repoRoot, xdgCache))
	require.Equal(t, 0, cleanCalls)

	xdgCacheFile := filepath.Join(t.TempDir(), "xdg-cache")
	require.NoError(t, os.WriteFile(xdgCacheFile, []byte("x"), 0o644))
	require.Error(t, ensureGolangCILintCacheFresh(repoRoot, xdgCacheFile))
}

