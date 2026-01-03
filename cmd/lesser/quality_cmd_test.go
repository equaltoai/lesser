package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunFmtAndLint(t *testing.T) {
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

	var gotName string
	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		gotName = name
		return nil
	}

	require.NoError(t, runFmt(nil))
	require.Equal(t, "go", gotName)

	require.NoError(t, runLint([]string{"--fix"}))
	require.Equal(t, "golangci-lint", gotName)
}

func TestRunFmtAndLint_ErrorBranches(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runFmt(nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	ensureToolAvailableFn = func(name string) error {
		if name == "go" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runFmt(nil), errSentinel)

	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return nil }

	require.Error(t, runLint([]string{"--badflag"}))
	require.NoError(t, runLint(nil))

	ensureToolAvailableFn = func(name string) error {
		if name == "golangci-lint" {
			return errors.New("missing lint")
		}
		return nil
	}
	require.Error(t, runLint(nil))
}
