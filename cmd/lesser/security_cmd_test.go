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
		if len(args) >= 2 && args[0] == "list" {
			switch args[1] {
			case "./...":
				return strings.Join([]string{
					"github.com/equaltoai/lesser/cmd/lesser",
					"github.com/equaltoai/lesser/pkg/common",
				}, "\n"), nil
			case "-f":
				return filepath.Join(repoRoot, "cmd", "lesser") + "\n" +
					filepath.Join(repoRoot, "pkg", "common") + "\n", nil
			}
		}
		return "", nil
	}

	var called int
	var gosecCalls [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		called++
		if name == "gosec" {
			gosecCalls = append(gosecCalls, append([]string(nil), args...))
		}
		return nil
	}

	require.NoError(t, runSecScan(nil))
	require.Equal(t, 1, called)
	require.Len(t, gosecCalls, 1)
	require.Equal(t, []string{
		"-quiet",
		"-exclude-generated",
		"-exclude-dir=tmp",
		"-exclude-dir=infra",
		"-exclude=G703,G204,G304,G117,G702,G306,G302,G301,G101,G710,G704,G124,G115",
		"./cmd/lesser",
		"./pkg/common",
	}, gosecCalls[0], "gosec must receive directory paths, not import paths")

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
		if len(args) >= 2 && args[0] == "list" && args[1] == "-f" {
			switch dir {
			case repoRoot:
				return filepath.Join(repoRoot, "cmd", "lesser") + "\n", nil
			case filepath.Join(repoRoot, "infra", "cdk"):
				return filepath.Join(repoRoot, "infra", "cdk", "app") + "\n", nil
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
