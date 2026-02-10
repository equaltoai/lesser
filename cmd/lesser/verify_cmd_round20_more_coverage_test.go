package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupVerifyCIRound20Harness(t *testing.T) string {
	t.Helper()

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

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, name string, args ...string) (string, error) {
		if name == "golangci-lint" && firstArgOrEmpty(args) == "version" {
			return "golangci-lint has version v2.8.0\n", nil
		}
		if name == "go" && len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			return strings.Join([]string{
				"github.com/equaltoai/lesser/cmd/lesser",
				"github.com/equaltoai/lesser/pkg/common",
			}, "\n"), nil
		}
		return "", nil
	}

	runCommandFn = func(context.Context, string, []string, execOptions) error { return nil }

	return repoRoot
}

func TestRunVerifyCI_Round20_PropagatesAuditFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "go" && len(args) >= 2 && args[0] == "run" && args[1] == "./tools/audit_gates" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesSecScanFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		if name == "gosec" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesVulnCheckFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		if name == "govulncheck" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesSupplyChainFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_supply_chain.sh" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesLambdaSetFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_lambda_set.sh" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesInventoryFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_inventory.sh" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}
