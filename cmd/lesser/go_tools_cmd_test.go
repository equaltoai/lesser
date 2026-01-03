package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunGoToolsCommands(t *testing.T) {
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
	ensureToolAvailableFn = func(string) error { return nil }

	var gotName string
	var gotArgs []string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}

	require.NoError(t, runGqlgen(nil))
	require.Equal(t, "go", gotName)
	require.Contains(t, strings.Join(gotArgs, " "), "gqlgen@v0.17.78")

	require.NoError(t, runTidy(nil))
	require.Equal(t, "go", gotName)
	require.Equal(t, []string{"mod", "tidy"}, gotArgs)
}

func TestRunGoToolsCommands_ErrorBranches(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runGqlgen(nil), errSentinel)
	require.ErrorIs(t, runTidy(nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return errSentinel }
	require.ErrorIs(t, runGqlgen(nil), errSentinel)

	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))
	require.Error(t, runGqlgen(nil))

	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runTidy(nil), errSentinel)
}
