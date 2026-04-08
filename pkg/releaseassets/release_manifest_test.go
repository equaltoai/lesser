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
		GoVersion:            "go1.26.2",
		CDKMajor:             2,
		ReceiptSchemaVersion: 7,
	})
	require.NoError(t, err)
	require.Equal(t, 1, manifest.Schema)
	require.Equal(t, "lesser", manifest.Name)
	require.Equal(t, "v1.2.3", manifest.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", manifest.GitSHA)
	require.Equal(t, "go1.26.2", manifest.GoVersion)
	require.Equal(t, 2, manifest.CDK.Major)
	require.Equal(t, 7, manifest.Artifacts.ReceiptSchemaVersion)
	require.Equal(t, ReleaseDeployArtifactsSchemaVersion, manifest.Artifacts.DeployArtifacts.SchemaVersion)
	require.Equal(t, LambdaBundleArchiveName, manifest.Artifacts.DeployArtifacts.LambdaBundle.Path)
	require.Equal(t, LambdaBundleManifestName, manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestPath)
	require.Equal(t, LambdaBundleManifestKind, manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestKind)
	require.Equal(t, LambdaBundleManifestSchemaVersion, manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestSchemaVersion)
	require.Equal(t, AuthUIBundleArchiveName, manifest.Artifacts.DeployArtifacts.AuthUIBundle.Path)
	require.Equal(t, "tar.gz", manifest.Artifacts.DeployArtifacts.AuthUIBundle.Format)
	require.Equal(t, DeployAssemblyArchiveName, manifest.Artifacts.DeployArtifacts.DeployAssembly.Path)
	require.Equal(t, DeployAssemblyManifestName, manifest.Artifacts.DeployArtifacts.DeployAssembly.ManifestPath)
	require.Equal(t, DeployAssemblyManifestKind, manifest.Artifacts.DeployArtifacts.DeployAssembly.ManifestKind)
	require.Equal(t, DeployAssemblyManifestSchemaVersion, manifest.Artifacts.DeployArtifacts.DeployAssembly.ManifestSchemaVersion)

	data, err := os.ReadFile(filepath.Join(outDir, ReleaseManifestName))
	require.NoError(t, err)
	require.Contains(t, string(data), `"deploy_artifacts": {`)
	require.Contains(t, string(data), `"lambda_bundle": {`)
	require.Contains(t, string(data), `"auth_ui_bundle": {`)
	require.Contains(t, string(data), `"deploy_assembly": {`)
}

func TestWriteReleaseManifest_RequiresMetadata(t *testing.T) {
	tests := []struct {
		name        string
		input       ReleaseManifestInput
		wantErrText string
	}{
		{
			name:        "version",
			input:       ReleaseManifestInput{},
			wantErrText: "release version is required",
		},
		{
			name: "git sha",
			input: ReleaseManifestInput{
				Version: "v1.2.3",
			},
			wantErrText: "release git SHA is required",
		},
		{
			name: "go version",
			input: ReleaseManifestInput{
				Version: "v1.2.3",
				GitSHA:  "0123456789abcdef0123456789abcdef01234567",
			},
			wantErrText: "go version is required",
		},
		{
			name: "cdk major",
			input: ReleaseManifestInput{
				Version:   "v1.2.3",
				GitSHA:    "0123456789abcdef0123456789abcdef01234567",
				GoVersion: "go1.26.2",
			},
			wantErrText: "cdk major version is required",
		},
		{
			name: "receipt schema version",
			input: ReleaseManifestInput{
				Version:   "v1.2.3",
				GitSHA:    "0123456789abcdef0123456789abcdef01234567",
				GoVersion: "go1.26.2",
				CDKMajor:  2,
			},
			wantErrText: "receipt schema version is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := WriteReleaseManifest(t.TempDir(), tt.input)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrText)
		})
	}
}

func TestWriteReleaseManifest_ErrorsWhenPathBlocked(t *testing.T) {
	outDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outDir, ReleaseManifestName), 0o755))

	_, err := WriteReleaseManifest(outDir, ReleaseManifestInput{
		Version:              "v1.2.3",
		GitSHA:               "0123456789abcdef0123456789abcdef01234567",
		GoVersion:            "go1.26.2",
		CDKMajor:             2,
		ReceiptSchemaVersion: 7,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "write release manifest")
}
