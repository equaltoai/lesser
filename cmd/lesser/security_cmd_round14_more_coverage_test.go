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

func TestSecurityCommands_Round14_ErrorBranches(t *testing.T) {
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
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.26\n"), 0o644))
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			return "github.com/equaltoai/lesser/cmd/lesser\n", nil
		}
		return "", nil
	}
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runSecScan(nil), errSentinel)
	require.ErrorIs(t, runVulnCheck(nil), errSentinel)
}

func TestRunVulnCheck_Round14_BatchesLocally(t *testing.T) {
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
	t.Setenv("CI", "")
	t.Setenv(lesserVulnCheckBatchSizeEnv, "2")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.26\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			return strings.Join([]string{
				"github.com/equaltoai/lesser/cmd/lesser",
				"github.com/equaltoai/lesser/pkg/common",
				"github.com/equaltoai/lesser/pkg/services",
			}, "\n"), nil
		}
		return "", nil
	}

	var calls [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "govulncheck" {
			calls = append(calls, append([]string(nil), args...))
		}
		return nil
	}

	require.NoError(t, runVulnCheck(nil))
	require.Len(t, calls, 2)
	require.Equal(t, []string{"github.com/equaltoai/lesser/cmd/lesser", "github.com/equaltoai/lesser/pkg/common"}, calls[0])
	require.Equal(t, []string{"github.com/equaltoai/lesser/pkg/services"}, calls[1])
}

func TestRunSecScan_Round14_BatchesByPackage(t *testing.T) {
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
	t.Setenv(lesserSecScanBatchSizeEnv, "2")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.26\n"), 0o644))
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "-f" {
			return filepath.Join(repoRoot, "cmd", "lesser") + "\n" +
				filepath.Join(repoRoot, "pkg", "common") + "\n" +
				filepath.Join(repoRoot, "pkg", "services") + "\n", nil
		}
		return "", nil
	}

	var calls [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "gosec" {
			calls = append(calls, append([]string(nil), args...))
		}
		return nil
	}

	require.NoError(t, runSecScan(nil))
	require.Len(t, calls, 2)
	require.Equal(t, []string{
		"-quiet",
		"-exclude-generated",
		"-exclude-dir=tmp",
		"-exclude-dir=infra",
		"-exclude=G703,G204,G304,G117,G702,G306,G302,G301,G101,G710,G704,G124,G115",
		"./cmd/lesser",
		"./pkg/common",
	}, calls[0])
	require.Equal(t, []string{
		"-quiet",
		"-exclude-generated",
		"-exclude-dir=tmp",
		"-exclude-dir=infra",
		"-exclude=G703,G204,G304,G117,G702,G306,G302,G301,G101,G710,G704,G124,G115",
		"./pkg/services",
	}, calls[1])
}
