package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunVerifyAll_Round15_IncludesSmokeAndCDK(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	var calls []string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "cdk" {
			require.Equal(t, filepath.Join(repoRoot, "infra", "cdk"), opts.Dir)
		} else {
			require.Equal(t, repoRoot, opts.Dir)
		}
		return nil
	}

	err := runVerifyAll([]string{
		"--smoke",
		"--smoke-base-url", "https://example.com",
		"--smoke-token", "token",
		"--smoke-username", "bob",
		"--smoke-object-id", "https://example.com/objects/1",
		"--cdk",
		"--cdk-aws-profile", "profile",
		"--cdk-region", "us-east-1",
	})
	require.NoError(t, err)

	require.Condition(t, func() bool {
		for _, call := range calls {
			if strings.HasPrefix(call, "bash scripts/smoke_core.sh") {
				return true
			}
		}
		return false
	})
	require.Condition(t, func() bool {
		for _, call := range calls {
			if strings.HasPrefix(call, "bash scripts/smoke_federation.sh") {
				return true
			}
		}
		return false
	})
	require.Condition(t, func() bool {
		for _, call := range calls {
			if strings.HasPrefix(call, "cdk synth") {
				return true
			}
		}
		return false
	})
}

func TestRunVerifyGraphQLCoverage_Round15_ParseError(t *testing.T) {
	require.Error(t, runVerifyGraphQLCoverage([]string{"--unknown-flag"}))
}

func TestRunVerifyOpenAPI_Round15_ParseError(t *testing.T) {
	require.Error(t, runVerifyOpenAPI([]string{"--unknown-flag"}))
}

func TestRunCDKSynth_Round15_XDGCacheDirError(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "tmp"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp", "xdg-cache"), []byte("x"), 0o644))

	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	require.Error(t, runCDKSynth("profile", "us-east-1"))
}
