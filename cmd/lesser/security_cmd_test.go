package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityCommands(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	var called int
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error {
		called++
		return nil
	}

	require.NoError(t, runSecScan(nil))
	require.Equal(t, 1, called)

	require.NoError(t, runVulnCheck(nil))
	require.Equal(t, 2, called)
}

func TestRunSecScan_RunsCDKModuleScanWhenPresent(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "go.mod"), []byte("module example.com/cdk\n"), 0o644))

	var called int
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error {
		called++
		return nil
	}

	require.NoError(t, runSecScan(nil))
	require.Equal(t, 2, called)
}
