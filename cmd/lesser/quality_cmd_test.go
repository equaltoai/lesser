package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunFmtAndLint(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		captureCommandOutputFn = previousCapture
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "golangci-lint has version 2.5.0\n", nil
	}

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

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
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		captureCommandOutputFn = previousCapture
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runFmt(nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "golangci-lint has version 2.5.0\n", nil
	}
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

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
