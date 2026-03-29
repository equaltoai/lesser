package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/equaltoai/lesser/pkg/releaseassets"
	"github.com/stretchr/testify/require"
)

func TestInstallReleaseLambdaAssets(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	result, err := installReleaseLambdaAssets(targetRepo, releaseDir)
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", result.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", result.GitSHA)
	require.Equal(t, []string{
		filepath.Join(targetRepo, "bin", "api.zip"),
		filepath.Join(targetRepo, "bin", "inbox.zip"),
	}, result.Files)

	apiBytes, err := os.ReadFile(filepath.Join(targetRepo, "bin", "api.zip"))
	require.NoError(t, err)
	require.Equal(t, "api zip", string(apiBytes))

	inboxBytes, err := os.ReadFile(filepath.Join(targetRepo, "bin", "inbox.zip"))
	require.NoError(t, err)
	require.Equal(t, "inbox zip", string(inboxBytes))
}

func TestInstallReleaseLambdaAssets_ErrorsWhenRequiredFileMissing(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	require.NoError(t, os.Remove(filepath.Join(releaseDir, releaseassets.ChecksumsFileName)))

	_, err := installReleaseLambdaAssets(targetRepo, releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "required release file checksums.txt")
}

func TestInstallReleaseLambdaAssets_ErrorsOnChecksumMismatch(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.LambdaBundleManifestName), []byte("{}\n"), 0o644))

	_, err := installReleaseLambdaAssets(targetRepo, releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lesser-lambda-bundle.json checksum mismatch")
}

func TestInstallReleaseLambdaAssets_ErrorsWhenInventoryDoesNotMatch(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "graphql"})

	_, err := installReleaseLambdaAssets(targetRepo, releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match canonical inventory")
}

func testReleaseDirFromRepo(t *testing.T, repoRoot string) string {
	t.Helper()

	releaseDir := t.TempDir()
	for _, assetName := range []string{
		"lesser-linux-amd64",
		"lesser-linux-arm64",
		"lesser-darwin-amd64",
		"lesser-darwin-arm64",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(releaseDir, assetName), []byte(assetName), 0o755))
	}
	files, err := releaseassets.WriteLambdaBundle(repoRoot, releaseDir)
	require.NoError(t, err)
	_, err = releaseassets.WriteLambdaBundleManifest(releaseDir, "v1.2.3", "0123456789abcdef0123456789abcdef01234567", files)
	require.NoError(t, err)
	_, err = releaseassets.WriteReleaseManifest(releaseDir, releaseassets.ReleaseManifestInput{
		Version:              "v1.2.3",
		GitSHA:               "0123456789abcdef0123456789abcdef01234567",
		GoVersion:            "go1.26.1",
		CDKMajor:             2,
		ReceiptSchemaVersion: 7,
	})
	require.NoError(t, err)
	require.NoError(t, releaseassets.WriteChecksums(releaseDir))

	return releaseDir
}

func testRepoWithCanonicalLambdaArtifacts(t *testing.T, files map[string]string) string {
	t.Helper()

	repoRoot := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})
	binDir := filepath.Join(repoRoot, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name+".zip"), []byte(content), 0o644))
	}

	return repoRoot
}

func testRepoWithCanonicalInventory(t *testing.T, lambdas []string) string {
	t.Helper()

	repoRoot := t.TempDir()
	inventoryPath := filepath.Join(repoRoot, "infra", "cdk", "inventory")
	require.NoError(t, os.MkdirAll(inventoryPath, 0o755))

	content := "package inventory\n\nvar LambdaInventory = []struct{ Name string }{\n"
	for _, lambdaName := range lambdas {
		content += "\t{Name: \"" + lambdaName + "\"},\n"
	}
	content += "}\n"

	require.NoError(t, os.WriteFile(filepath.Join(inventoryPath, "lambdas.go"), []byte(content), 0o644))
	return repoRoot
}
