package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteLambdaAssetMetadata_CreatesDirAndPersistsJSON(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "deploy", "lambda-assets")
	preparedAt := time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC)

	err := writeLambdaAssetMetadata(assetRoot, lambdaAssetMetadata{
		Schema:         1,
		Mode:           "release",
		Files:          []string{"bin/api.zip", "bin/inbox.zip"},
		ReleaseVersion: "v1.2.3",
		ReleaseGitSHA:  "abc123",
		PreparedAt:     preparedAt,
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(assetRoot, lambdaAssetMetadataFileName))
	require.NoError(t, err)
	require.Equal(t, byte('\n'), data[len(data)-1])

	var got lambdaAssetMetadata
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, lambdaAssetMetadata{
		Schema:         1,
		Mode:           "release",
		Files:          []string{"bin/api.zip", "bin/inbox.zip"},
		ReleaseVersion: "v1.2.3",
		ReleaseGitSHA:  "abc123",
		PreparedAt:     preparedAt,
	}, got)
}

func TestWriteLambdaAssetMetadata_ErrorsWhenAssetRootIsFile(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	require.NoError(t, os.WriteFile(assetRoot, []byte("blocked"), 0o644))

	err := writeLambdaAssetMetadata(assetRoot, lambdaAssetMetadata{Schema: 1, Mode: "source"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create lambda asset metadata dir")
}

func TestWriteLambdaAssetMetadata_ErrorsWhenMetadataPathIsDirectory(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assetRoot, lambdaAssetMetadataFileName), 0o755))

	err := writeLambdaAssetMetadata(assetRoot, lambdaAssetMetadata{Schema: 1, Mode: "source"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "write lambda asset metadata")
}

func TestRelativeLambdaAssetFiles_SortsRelativePaths(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	files, err := relativeLambdaAssetFiles(assetRoot, []string{
		filepath.Join(assetRoot, "bin", "inbox.zip"),
		filepath.Join(assetRoot, "bin", "api.zip"),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"bin/api.zip", "bin/inbox.zip"}, files)
}

func TestRelativeLambdaAssetFiles_RejectsEscapingPaths(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	_, err := relativeLambdaAssetFiles(assetRoot, []string{
		filepath.Join(assetRoot, "bin", "api.zip"),
		filepath.Join(t.TempDir(), "outside.zip"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes asset root")
}

func TestRelativeLambdaAssetFiles_RejectsAssetRootPath(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	_, err := relativeLambdaAssetFiles(assetRoot, []string{assetRoot})
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes asset root")
}

func TestUpEnv_RecordLocalLambdaAssets_WritesSourceMetadata(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assetRoot, "bin"), 0o755))
	env := &upEnv{lambdaAssetRoot: assetRoot}

	err := env.recordLocalLambdaAssets([]string{
		filepath.Join(assetRoot, "bin", "inbox.zip"),
		filepath.Join(assetRoot, "bin", "api.zip"),
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(assetRoot, lambdaAssetMetadataFileName))
	require.NoError(t, err)

	var got lambdaAssetMetadata
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, "source", got.Mode)
	require.Equal(t, []string{"bin/api.zip", "bin/inbox.zip"}, got.Files)
	require.Equal(t, 1, got.Schema)
	require.False(t, got.PreparedAt.IsZero())
}

func TestUpEnv_RecordLocalLambdaAssets_RejectsEscapingFiles(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	env := &upEnv{lambdaAssetRoot: assetRoot}

	err := env.recordLocalLambdaAssets([]string{filepath.Join(t.TempDir(), "outside.zip")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes asset root")
}
