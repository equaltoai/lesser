package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDeploySourceInputs(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "auth-ui"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "cdk.json"), []byte("{}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "auth-ui", "package.json"), []byte("{}\n"), 0o644))

	require.NoError(t, validateDeploySourceInputs(repoRoot))

	t.Run("missing requirement", func(t *testing.T) {
		missingRoot := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(missingRoot, "infra", "cdk"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(missingRoot, "infra", "cdk", "cdk.json"), []byte("{}\n"), 0o644))

		err := validateDeploySourceInputs(missingRoot)
		require.Error(t, err)
		require.Contains(t, err.Error(), "deploy requires repo-local auth-ui source")
		require.Contains(t, err.Error(), filepath.Join(missingRoot, "auth-ui", "package.json"))
	})

	t.Run("directory instead of file", func(t *testing.T) {
		directoryRoot := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(directoryRoot, "infra", "cdk", "cdk.json"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(directoryRoot, "auth-ui"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(directoryRoot, "auth-ui", "package.json"), []byte("{}\n"), 0o644))

		err := validateDeploySourceInputs(directoryRoot)
		require.Error(t, err)
		require.Contains(t, err.Error(), "deploy requires repo-local CDK application source file")
	})
}
