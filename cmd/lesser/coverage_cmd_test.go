package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoolToFlag(t *testing.T) {
	require.Equal(t, "true", boolToFlag(true))
	require.Equal(t, "false", boolToFlag(false))
}

func TestRunCoverageScoreboard_BuildsArgs(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	ensureToolAvailableFn = func(string) error { return nil }
	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }

	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}

	require.NoError(t, runCoverageScoreboard([]string{
		"--profile", "coverage.out",
		"--mode", "file",
		"--package", "github.com/equaltoai/lesser/pkg/",
		"--top", "5",
		"--min", "10",
		"--zero-only",
		"--sort-uncovered=false",
	}))

	require.Contains(t, gotArgs, "./tools/coverage_scoreboard")
	require.Contains(t, gotArgs, "--profile")
	require.Contains(t, gotArgs, "coverage.out")
}

func TestRunCoverage_Dispatch(t *testing.T) {
	require.Error(t, runCoverage(nil))
	require.NoError(t, runCoverage([]string{helpCommand}))
	require.Error(t, runCoverage([]string{"nope"}))
}

func TestRunCoverage_DispatchesScoreboardAndParsesFlags(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	ensureToolAvailableFn = func(string) error { return nil }
	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return nil }

	require.NoError(t, runCoverage([]string{"scoreboard"}))
	require.Error(t, runCoverageScoreboard([]string{"--badflag"}))
}
