package releaseassets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ReleaseManifestName is the published top-level release manifest filename.
const ReleaseManifestName = "lesser-release.json"

// ReleaseDeployArtifactsSchemaVersion is the current schema version for the
// deploy artifact section in the top-level release manifest.
const ReleaseDeployArtifactsSchemaVersion = 1

// ReleaseManifest describes the full published release metadata.
type ReleaseManifest struct {
	Schema    int              `json:"schema"`
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	GitSHA    string           `json:"git_sha"`
	GoVersion string           `json:"go_version"`
	CDK       ReleaseCDK       `json:"cdk"`
	Artifacts ReleaseArtifacts `json:"artifacts"`
}

// ReleaseCDK records CDK compatibility metadata for the release.
type ReleaseCDK struct {
	Major int `json:"major"`
}

// ReleaseArtifacts groups the published release assets metadata.
type ReleaseArtifacts struct {
	ReceiptSchemaVersion int                    `json:"receipt_schema_version"`
	DeployArtifacts      ReleaseDeployArtifacts `json:"deploy_artifacts"`
}

// ReleaseDeployArtifacts describes artifacts consumed during deployment.
type ReleaseDeployArtifacts struct {
	SchemaVersion  int                      `json:"schema_version"`
	LambdaBundle   ReleaseLambdaBundleRef   `json:"lambda_bundle"`
	AuthUIBundle   ReleaseAuthUIBundleRef   `json:"auth_ui_bundle"`
	DeployAssembly ReleaseDeployAssemblyRef `json:"deploy_assembly"`
}

// ReleaseLambdaBundleRef points at the published Lambda bundle assets.
type ReleaseLambdaBundleRef struct {
	Path                  string `json:"path"`
	ManifestPath          string `json:"manifest_path"`
	ManifestKind          string `json:"manifest_kind"`
	ManifestSchemaVersion int    `json:"manifest_schema_version"`
}

// ReleaseAuthUIBundleRef points at the published auth UI bundle.
type ReleaseAuthUIBundleRef struct {
	Path   string `json:"path"`
	Format string `json:"format"`
}

// ReleaseDeployAssemblyRef points at the published deploy assembly assets.
type ReleaseDeployAssemblyRef struct {
	Path                  string `json:"path"`
	ManifestPath          string `json:"manifest_path"`
	ManifestKind          string `json:"manifest_kind"`
	ManifestSchemaVersion int    `json:"manifest_schema_version"`
}

// ReleaseManifestInput supplies the values required to write the release
// manifest.
type ReleaseManifestInput struct {
	Version              string
	GitSHA               string
	GoVersion            string
	CDKMajor             int
	ReceiptSchemaVersion int
}

// WriteReleaseManifest writes the top-level release manifest into outDir.
func WriteReleaseManifest(outDir string, input ReleaseManifestInput) (ReleaseManifest, error) {
	manifest := ReleaseManifest{
		Schema:    1,
		Name:      "lesser",
		Version:   input.Version,
		GitSHA:    input.GitSHA,
		GoVersion: input.GoVersion,
		CDK: ReleaseCDK{
			Major: input.CDKMajor,
		},
		Artifacts: ReleaseArtifacts{
			ReceiptSchemaVersion: input.ReceiptSchemaVersion,
			DeployArtifacts: ReleaseDeployArtifacts{
				SchemaVersion: ReleaseDeployArtifactsSchemaVersion,
				LambdaBundle: ReleaseLambdaBundleRef{
					Path:                  LambdaBundleArchiveName,
					ManifestPath:          LambdaBundleManifestName,
					ManifestKind:          LambdaBundleManifestKind,
					ManifestSchemaVersion: LambdaBundleManifestSchemaVersion,
				},
				AuthUIBundle: ReleaseAuthUIBundleRef{
					Path:   AuthUIBundleArchiveName,
					Format: "tar.gz",
				},
				DeployAssembly: ReleaseDeployAssemblyRef{
					Path:                  DeployAssemblyArchiveName,
					ManifestPath:          DeployAssemblyManifestName,
					ManifestKind:          DeployAssemblyManifestKind,
					ManifestSchemaVersion: DeployAssemblyManifestSchemaVersion,
				},
			},
		},
	}

	if manifest.Version == "" {
		return ReleaseManifest{}, fmt.Errorf("release version is required")
	}
	if manifest.GitSHA == "" {
		return ReleaseManifest{}, fmt.Errorf("release git SHA is required")
	}
	if manifest.GoVersion == "" {
		return ReleaseManifest{}, fmt.Errorf("go version is required")
	}
	if manifest.CDK.Major <= 0 {
		return ReleaseManifest{}, fmt.Errorf("cdk major version is required")
	}
	if manifest.Artifacts.ReceiptSchemaVersion <= 0 {
		return ReleaseManifest{}, fmt.Errorf("receipt schema version is required")
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ReleaseManifest{}, fmt.Errorf("marshal release manifest: %w", err)
	}
	data = append(data, '\n')

	//nolint:gosec // The manifest is an intentionally public GitHub release artifact with no secret material.
	if err := os.WriteFile(filepath.Join(outDir, ReleaseManifestName), data, 0o644); err != nil {
		return ReleaseManifest{}, fmt.Errorf("write release manifest: %w", err)
	}

	return manifest, nil
}
