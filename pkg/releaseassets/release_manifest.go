package releaseassets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ReleaseManifestName = "lesser-release.json"
const ReleaseDeployArtifactsSchemaVersion = 1

type ReleaseManifest struct {
	Schema    int              `json:"schema"`
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	GitSHA    string           `json:"git_sha"`
	GoVersion string           `json:"go_version"`
	CDK       ReleaseCDK       `json:"cdk"`
	Artifacts ReleaseArtifacts `json:"artifacts"`
}

type ReleaseCDK struct {
	Major int `json:"major"`
}

type ReleaseArtifacts struct {
	ReceiptSchemaVersion int                    `json:"receipt_schema_version"`
	DeployArtifacts      ReleaseDeployArtifacts `json:"deploy_artifacts"`
}

type ReleaseDeployArtifacts struct {
	SchemaVersion int                    `json:"schema_version"`
	LambdaBundle  ReleaseLambdaBundleRef `json:"lambda_bundle"`
}

type ReleaseLambdaBundleRef struct {
	Path                  string `json:"path"`
	ManifestPath          string `json:"manifest_path"`
	ManifestKind          string `json:"manifest_kind"`
	ManifestSchemaVersion int    `json:"manifest_schema_version"`
}

type ReleaseManifestInput struct {
	Version              string
	GitSHA               string
	GoVersion            string
	CDKMajor             int
	ReceiptSchemaVersion int
}

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

	if err := os.WriteFile(filepath.Join(outDir, ReleaseManifestName), data, 0o644); err != nil {
		return ReleaseManifest{}, fmt.Errorf("write release manifest: %w", err)
	}

	return manifest, nil
}
