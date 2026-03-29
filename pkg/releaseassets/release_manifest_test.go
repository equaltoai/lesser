package releaseassets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteReleaseManifest(t *testing.T) {
	outDir := t.TempDir()

	manifest, err := WriteReleaseManifest(outDir, ReleaseManifestInput{
		Version:              "v1.2.3",
		GitSHA:               "0123456789abcdef0123456789abcdef01234567",
		GoVersion:            "go1.26.1",
		CDKMajor:             2,
		ReceiptSchemaVersion: 7,
	})
	require.NoError(t, err)
	require.Equal(t, 1, manifest.Schema)
	require.Equal(t, "lesser", manifest.Name)
	require.Equal(t, "v1.2.3", manifest.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", manifest.GitSHA)
	require.Equal(t, "go1.26.1", manifest.GoVersion)
	require.Equal(t, 2, manifest.CDK.Major)
	require.Equal(t, 7, manifest.Artifacts.ReceiptSchemaVersion)
	require.Equal(t, ReleaseDeployArtifactsSchemaVersion, manifest.Artifacts.DeployArtifacts.SchemaVersion)
	require.Equal(t, LambdaBundleArchiveName, manifest.Artifacts.DeployArtifacts.LambdaBundle.Path)
	require.Equal(t, LambdaBundleManifestName, manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestPath)
	require.Equal(t, LambdaBundleManifestKind, manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestKind)
	require.Equal(t, LambdaBundleManifestSchemaVersion, manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestSchemaVersion)

	data, err := os.ReadFile(filepath.Join(outDir, ReleaseManifestName))
	require.NoError(t, err)
	require.Contains(t, string(data), `"deploy_artifacts": {`)
	require.Contains(t, string(data), `"lambda_bundle": {`)
}

func TestWriteReleaseManifest_RequiresMetadata(t *testing.T) {
	_, err := WriteReleaseManifest(t.TempDir(), ReleaseManifestInput{})
	require.Error(t, err)
}
