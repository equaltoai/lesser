package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunGenerate_DispatchAndTooling(t *testing.T) {
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

	var gotCmd string
	var gotArgs []string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		gotCmd = name
		gotArgs = append([]string(nil), args...)
		return nil
	}

	require.Error(t, runGenerate(nil))
	require.NoError(t, runGenerate([]string{helpCommand}))
	require.Error(t, runGenerate([]string{"nope"}))

	require.NoError(t, runGenerate([]string{"openapi"}))
	require.Equal(t, "go", gotCmd)
	require.Contains(t, gotArgs, "./tools/openapi")

	require.NoError(t, runGenerate([]string{"graphql-coverage"}))
	require.Contains(t, gotArgs, "./tools/graphql_coverage")

	require.NoError(t, runGenerate([]string{"inventory"}))
	require.Contains(t, gotArgs, "./cmd/generate-inventory")
}

func TestRunGenerate_SchemaDispatch(t *testing.T) {
	previousRunCommand := runCommandFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	var gotName string
	var gotArgs []string
	var gotDir string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		gotDir = opts.Dir
		return nil
	}

	require.NoError(t, runGenerate([]string{valueSchema, "--out", "schema.graphql"}))
	require.Equal(t, "bash", gotName)
	require.Contains(t, gotArgs, "./scripts/generate_schema.sh")
	require.Equal(t, repoRoot, gotDir)
}

func TestRunGenerateOpenAPI_ErrorsWhenGoCacheCannotBeCreated(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	ensureToolAvailableFn = func(string) error { return nil }

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))

	err := runGenerateOpenAPI(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create go-cache dir")
}

func TestRunGenerateGraphQLCoverage_ErrorsWhenGoMissing(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	wantErr := errors.New("no go")
	ensureToolAvailableFn = func(string) error { return wantErr }

	require.ErrorIs(t, runGenerateGraphQLCoverage(nil), wantErr)
}

func TestGenerateSubcommands_PropagateRepoRootAndCommandErrors(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runGenerateOpenAPI(nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runGenerateInventory(nil), errSentinel)
}

func TestGenerateSubcommands_MoreErrorBranches(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runGenerateGraphQLCoverage(nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return errSentinel }
	require.ErrorIs(t, runGenerateInventory(nil), errSentinel)
}
