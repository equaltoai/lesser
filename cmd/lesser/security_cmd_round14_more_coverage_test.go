package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityCommands_Round14_ErrorBranches(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runSecScan(nil), errSentinel)
	require.ErrorIs(t, runVulnCheck(nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	ensureToolAvailableFn = func(name string) error {
		if name == "gosec" || name == "govulncheck" {
			return errors.New("missing tool")
		}
		return nil
	}
	require.Error(t, runSecScan(nil))
	require.Error(t, runVulnCheck(nil))

	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runSecScan(nil), errSentinel)
	require.ErrorIs(t, runVulnCheck(nil), errSentinel)
}
