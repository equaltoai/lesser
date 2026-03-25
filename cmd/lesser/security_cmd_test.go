package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityCommands(t *testing.T) {
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
	t.Setenv(lesserSecScanBatchSizeEnv, "10")
	t.Setenv(lesserVulnCheckBatchSizeEnv, "10")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.26\n"), 0o644))
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			return strings.Join([]string{
				"github.com/equaltoai/lesser/cmd/lesser",
				"github.com/equaltoai/lesser/pkg/common",
			}, "\n"), nil
		}
		return "", nil
	}

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
	t.Setenv(lesserSecScanBatchSizeEnv, "10")

	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "go.mod"), []byte("module example.com/cdk\n"), 0o644))
	captureCommandOutputFn = func(_ context.Context, dir string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			switch dir {
			case repoRoot:
				return "github.com/equaltoai/lesser/cmd/lesser\n", nil
			case filepath.Join(repoRoot, "infra", "cdk"):
				return "example.com/cdk/app\n", nil
			}
		}
		return "", nil
	}

	var called int
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error {
		called++
		return nil
	}

	require.NoError(t, runSecScan(nil))
	require.Equal(t, 2, called)
}
