package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	t.Setenv(lesserLintBatchSizeEnv, "10")
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, name string, args ...string) (string, error) {
		if name == "golangci-lint" {
			return "golangci-lint has version 2.5.0\n", nil
		}
		if name == "go" && len(args) >= 4 && args[0] == "list" && args[1] == "-f" {
			return filepath.Join(repoRoot, "cmd", "lesser") + "\n", nil
		}
		return "", nil
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
	t.Setenv(lesserLintBatchSizeEnv, "10")
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, name string, args ...string) (string, error) {
		if name == "golangci-lint" {
			return "golangci-lint has version 2.5.0\n", nil
		}
		if name == "go" && len(args) >= 4 && args[0] == "list" && args[1] == "-f" {
			return filepath.Join(repoRoot, "cmd", "lesser") + "\n", nil
		}
		return "", nil
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

func TestRunLint_BatchesDirectories(t *testing.T) {
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
	t.Setenv(lesserLintBatchSizeEnv, "2")
	t.Setenv(lesserToolJobsEnvVar, "4")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, name string, args ...string) (string, error) {
		if name == "golangci-lint" {
			return "golangci-lint has version 2.5.0\n", nil
		}
		if name == "go" && len(args) >= 4 && args[0] == "list" && args[1] == "-f" {
			return strings.Join([]string{
				filepath.Join(repoRoot, "cmd", "lesser"),
				filepath.Join(repoRoot, "pkg", "common"),
				filepath.Join(repoRoot, "pkg", "services"),
			}, "\n"), nil
		}
		return "", nil
	}

	var calls [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "golangci-lint" && len(args) > 0 && args[0] == "run" {
			calls = append(calls, append([]string(nil), args...))
		}
		return nil
	}

	require.NoError(t, runLint([]string{"--fix"}))
	require.Len(t, calls, 2)
	require.Equal(t, []string{
		"run",
		"--config",
		".golangci.yml",
		"--timeout",
		defaultGolangCILintTimeout,
		"--fix",
		"--concurrency",
		"4",
		"./cmd/lesser",
		"./pkg/common",
	}, calls[0])
	require.Equal(t, []string{
		"run",
		"--config",
		".golangci.yml",
		"--timeout",
		defaultGolangCILintTimeout,
		"--fix",
		"--concurrency",
		"4",
		"./pkg/services",
	}, calls[1])
}

func TestRunLintInBatches_FallsBackToSingleInvocationWhenNoDirectoriesFound(t *testing.T) {
	previousCapture := captureCommandOutputFn
	previousRun := runCommandFn
	t.Cleanup(func() {
		captureCommandOutputFn = previousCapture
		runCommandFn = previousRun
	})

	repoRoot := t.TempDir()
	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "", nil
	}

	var gotName string
	var gotArgs []string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}

	require.NoError(t, runLintInBatches(
		repoRoot,
		[]string{"run", "--config", ".golangci.yml", "--timeout", defaultGolangCILintTimeout},
		map[string]string{"GOCACHE": t.TempDir()},
		2,
	))
	require.Equal(t, "golangci-lint", gotName)
	require.Equal(t, []string{"run", "--config", ".golangci.yml", "--timeout", defaultGolangCILintTimeout}, gotArgs)
}
