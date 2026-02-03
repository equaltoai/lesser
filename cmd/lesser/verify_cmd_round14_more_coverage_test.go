package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunVerifyCI_Round14_SkipsSecurityWhenDisabled(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousCapture := captureCommandOutputFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		captureCommandOutputFn = previousCapture
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			return strings.Join([]string{
				"github.com/equaltoai/lesser/cmd/api",
				"github.com/equaltoai/lesser/pkg/common",
			}, "\n"), nil
		}
		return "", nil
	}

	var names []string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		_ = args
		names = append(names, name)
		return nil
	}

	require.NoError(t, runVerifyCI([]string{"--security=false"}))
	for _, name := range names {
		require.NotEqual(t, "gosec", name)
		require.NotEqual(t, "govulncheck", name)
	}
}

func TestRunVerifyCI_Round14_PropagatesLintFailure(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousCapture := captureCommandOutputFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		captureCommandOutputFn = previousCapture
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) { return "", nil }

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "golangci-lint" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI([]string{"--security=false"}), errSentinel)
}

func TestRunVerifyAudit_Round14_FailsWhenGoMissing(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(name string) error {
		if name == "go" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyAudit(nil), errSentinel)
}

func TestRunVerifyInventory_Round14_GoCacheDirError(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))

	require.Error(t, runVerifyInventory(nil))
}
