package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunSchemaAndExportSchema(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }

	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "docs", "contracts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "docs", "contracts", "graphql-schema.graphql"), []byte("schema {}"), 0o644))

	require.NoError(t, runSchema([]string{"--out", filepath.Join(repoRoot, "docs", "contracts", "graphql-schema.graphql")}))
	require.NoError(t, runExportSchema(nil))
	require.FileExists(t, filepath.Join(repoRoot, "schema.graphql"))
}

func TestRunSchemaAndExportSchema_ErrorBranches(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return nil }

	require.NoError(t, runExportSchema([]string{helpCommand}))
	require.Error(t, runSchema([]string{"--unknown"}))

	require.Error(t, runExportSchema(nil))

	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "docs", "contracts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "docs", "contracts", "graphql-schema.graphql"), []byte("schema {}"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "schema.graphql"), 0o755))
	require.Error(t, runExportSchema(nil))
}
