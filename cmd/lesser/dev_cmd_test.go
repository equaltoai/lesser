package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevCommands(t *testing.T) {
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
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }

	require.NoError(t, runDev([]string{helpCommand}))
	require.Error(t, runDev([]string{"nope"}))

	require.NoError(t, runDevInit(nil))
	require.FileExists(t, filepath.Join(repoRoot, ".env"))

	require.NoError(t, runDevServer(nil))
	require.NoError(t, runDevDynamoDB(nil))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return "token\n", nil
	}
	require.NoError(t, runDevSeedAndValidate([]string{"--base-url", "https://example.com"}))
}

func TestDevSeedAndValidate_EmptyTokenIsError(t *testing.T) {
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
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return "\n", nil
	}

	err := runDevSeedAndValidate([]string{"--base-url", "https://example.com"})
	require.Error(t, err)
}

func TestDevInit_DoesNotClobberExistingEnv(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	envPath := filepath.Join(repoRoot, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("DOMAIN=localhost\n"), 0o600))

	require.NoError(t, runDevInit(nil))
	content, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.Equal(t, "DOMAIN=localhost\n", string(content))
}

func TestDevInit_StatErrorIsPropagated(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	tmp := t.TempDir()
	repoRoot := filepath.Join(tmp, "repo-root-is-a-file")
	require.NoError(t, os.WriteFile(repoRoot, []byte("x"), 0o644))
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	err := runDevInit(nil)
	require.Error(t, err)
}

func TestDevServer_MissingEnvIsError(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	require.Error(t, runDevServer(nil))
}

func TestDevSeedAndValidate_PropagatesCaptureError(t *testing.T) {
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
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return "", errors.New("boom")
	}

	require.Error(t, runDevSeedAndValidate([]string{"--base-url", "https://example.com"}))
}

func TestRunDev_DispatchesSubcommands(t *testing.T) {
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

	var calls []string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		calls = append(calls, name+" "+firstArgOrEmpty(args))
		return nil
	}
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return "token\n", nil
	}

	require.NoError(t, runDev([]string{"init"}))
	require.FileExists(t, filepath.Join(repoRoot, ".env"))

	require.NoError(t, runDev(nil))
	require.NoError(t, runDev([]string{"--verbose"}))
	require.NoError(t, runDev([]string{"dynamodb"}))
	require.NoError(t, runDev([]string{"seed-and-validate", "--base-url", "https://example.com", "--graphql-endpoint", "https://example.com/api/graphql"}))

	require.NotEmpty(t, calls)
}

func TestRunDevServer_ToolErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("DOMAIN=localhost\n"), 0o600))

	ensureToolAvailableFn = func(name string) error {
		if name == "go" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runDevServer(nil), errSentinel)

	ensureToolAvailableFn = func(name string) error {
		if name == "bash" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runDevServer(nil), errSentinel)
}

func TestRunDevDynamoDB_DockerMissingIsError(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(string) error { return errSentinel }
	require.ErrorIs(t, runDevDynamoDB(nil), errSentinel)
}

func TestDevSeedAndValidate_PropagatesStepError(t *testing.T) {
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

	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		if firstArgOrEmpty(args) == "scripts/clear_all_data.py" {
			return errSentinel
		}
		return nil
	}
	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "token\n", nil
	}

	require.ErrorIs(t, runDevSeedAndValidate([]string{"--base-url", "https://example.com"}), errSentinel)
}

func TestDevInit_RepoRootAndWriteErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runDevInit(nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	require.NoError(t, os.Chmod(repoRoot, 0o500))
	t.Cleanup(func() { _ = os.Chmod(repoRoot, 0o700) })
	require.Error(t, runDevInit(nil))
}

func TestDevSeedAndValidate_ToolMissingIsError(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(name string) error {
		if name == "python3" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runDevSeedAndValidate([]string{"--base-url", "https://example.com"}), errSentinel)
}

func TestDevServer_StatAndRunCommandErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	tmp := t.TempDir()
	repoRootFile := filepath.Join(tmp, "repo-root-is-file")
	require.NoError(t, os.WriteFile(repoRootFile, []byte("x"), 0o644))
	findRepoRootFn = func() (string, error) { return repoRootFile, nil }
	require.Error(t, runDevServer(nil))

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("DOMAIN=localhost\n"), 0o600))
	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runDevServer(nil), errSentinel)
}

func TestDevSeedAndValidate_ToolAndStepErrors(t *testing.T) {
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

	ensureToolAvailableFn = func(name string) error {
		if name == "aws" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runDevSeedAndValidate([]string{"--base-url", "https://example.com"}), errSentinel)

	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return nil }
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		if firstArgOrEmpty(args) == "scripts/seed_runner/main.py" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runDevSeedAndValidate([]string{"--base-url", "https://example.com"}), errSentinel)
}

func TestDevSeedAndValidate_ParseError(t *testing.T) {
	require.Error(t, runDevSeedAndValidate([]string{"--badflag"}))
}
