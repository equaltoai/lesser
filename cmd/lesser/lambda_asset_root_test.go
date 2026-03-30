package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareLambdaAssetRoot_DeterministicAndClean(t *testing.T) {
	stateDir := t.TempDir()
	stalePath := filepath.Join(deployLambdaAssetRoot(stateDir), "bin", "stale.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(stalePath), 0o755))
	require.NoError(t, os.WriteFile(stalePath, []byte("stale"), 0o644))

	assetRoot, err := prepareLambdaAssetRoot(stateDir)
	require.NoError(t, err)
	require.Equal(t, deployLambdaAssetRoot(stateDir), assetRoot)
	require.NoFileExists(t, stalePath)
	require.DirExists(t, filepath.Join(assetRoot, "bin"))
}

func TestPrepareLambdaAssetRoot_ErrorsWhenWorkspaceBlocked(t *testing.T) {
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "deploy"), []byte("blocked"), 0o644))

	_, err := prepareLambdaAssetRoot(stateDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reset lambda asset root")
}

func TestStageLocalLambdaAssets(t *testing.T) {
	repoRoot := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assetRoot, "bin"), 0o755))

	files, err := stageLocalLambdaAssets(repoRoot, assetRoot)
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(assetRoot, "bin", "api.zip"),
		filepath.Join(assetRoot, "bin", "inbox.zip"),
	}, files)

	apiBytes, err := os.ReadFile(filepath.Join(assetRoot, "bin", "api.zip"))
	require.NoError(t, err)
	require.Equal(t, "api zip", string(apiBytes))
}

func TestStageLocalLambdaAssets_MissingZipReturnsError(t *testing.T) {
	repoRoot := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api": "api zip",
	})
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assetRoot, "bin"), 0o755))

	files, err := stageLocalLambdaAssets(repoRoot, assetRoot)
	require.Nil(t, files)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stage lambda asset inbox")
}
