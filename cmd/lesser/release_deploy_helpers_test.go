package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/releaseassets"
	"github.com/stretchr/testify/require"
)

func TestValidateDeployAssemblyDescriptor(t *testing.T) {
	releaseManifest := validReleaseManifest()
	descriptor := validDeployAssemblyDescriptor(releaseManifest)
	require.NoError(t, validateDeployAssemblyDescriptor(descriptor, releaseManifest))

	tests := []struct {
		name string
		mut  func(*releaseassets.DeployAssemblyDescriptor)
		want string
	}{
		{
			name: "kind",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Kind = "wrong"
			},
			want: "unexpected deploy assembly kind",
		},
		{
			name: "schema",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.SchemaVersion = 0
			},
			want: "unsupported deploy assembly schema",
		},
		{
			name: "release name",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Release.Name = "other"
			},
			want: "unexpected deploy assembly release name",
		},
		{
			name: "release version",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Release.Version = "v9.9.9"
			},
			want: "does not match release manifest",
		},
		{
			name: "git sha",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Release.GitSHA = "ffffffffffffffffffffffffffffffffffffffff"
			},
			want: "does not match release manifest",
		},
		{
			name: "assembly path",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Assembly.Path = "other.tar.gz"
			},
			want: "does not match release manifest",
		},
		{
			name: "assembly format",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Assembly.Format = "zip"
			},
			want: "unexpected deploy assembly archive format",
		},
		{
			name: "payload kind required",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Payload.Kind = "   "
			},
			want: "payload kind is required",
		},
		{
			name: "payload contract version",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Payload.ContractVersion = 0
			},
			want: "payload contract version must be positive",
		},
		{
			name: "entrypoint normalization",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Payload.Entrypoint = "templates/../manifest.json"
			},
			want: "payload entrypoint",
		},
		{
			name: "release manifest path",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Compatibility.ReleaseManifestPath = "other.json"
			},
			want: "unexpected deploy assembly release manifest path",
		},
		{
			name: "compatibility key",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Compatibility.DeployArtifactsKey = "wrong"
			},
			want: "unexpected deploy assembly compatibility key",
		},
		{
			name: "executor contract version",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Compatibility.ExecutorContractVersion = 0
			},
			want: "executor contract version must be positive",
		},
		{
			name: "required inputs",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.InstanceInputs.Required = nil
			},
			want: "required instance inputs must not be empty",
		},
		{
			name: "integrity requirements",
			mut: func(d *releaseassets.DeployAssemblyDescriptor) {
				d.Verification.IntegrityRequired = nil
			},
			want: "integrity requirements must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := validDeployAssemblyDescriptor(releaseManifest)
			tt.mut(&next)
			err := validateDeployAssemblyDescriptor(next, releaseManifest)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateReleaseAssemblyPayloadHeader(t *testing.T) {
	release := verifiedReleaseAssets{releaseManifest: validReleaseManifest()}
	payload := validReleaseAssemblyPayloadManifest()
	require.NoError(t, validateReleaseAssemblyPayloadHeader(payload, release))

	payload.Kind = "wrong"
	require.ErrorContains(t, validateReleaseAssemblyPayloadHeader(payload, release), "unexpected deploy assembly payload kind")

	payload = validReleaseAssemblyPayloadManifest()
	payload.SchemaVersion = 0
	require.ErrorContains(t, validateReleaseAssemblyPayloadHeader(payload, release), "unsupported deploy assembly payload schema")

	payload = validReleaseAssemblyPayloadManifest()
	payload.Release.Name = "other"
	require.ErrorContains(t, validateReleaseAssemblyPayloadHeader(payload, release), "unexpected deploy assembly payload release name")

	payload = validReleaseAssemblyPayloadManifest()
	payload.Release.Version = "v9.9.9"
	require.ErrorContains(t, validateReleaseAssemblyPayloadHeader(payload, release), "does not match release manifest")

	payload = validReleaseAssemblyPayloadManifest()
	payload.Release.GitSHA = "wrong"
	require.ErrorContains(t, validateReleaseAssemblyPayloadHeader(payload, release), "does not match release manifest")
}

func TestValidateReleaseAssemblyTemplatePathAndStageTemplate(t *testing.T) {
	path, err := validateReleaseAssemblyTemplatePath(releaseDeployAssemblyStackManifest{
		Name:         string(naming.StageDev),
		Stage:        string(naming.StageDev),
		TemplatePath: "templates/lesser-managed-dev.template.json",
		SHA256:       sha256HexString("template"),
	})
	require.NoError(t, err)
	require.Equal(t, "templates/lesser-managed-dev.template.json", path)

	_, err = validateReleaseAssemblyTemplatePath(releaseDeployAssemblyStackManifest{
		Name:         string(naming.StageDev),
		Stage:        string(naming.StageDev),
		TemplatePath: "templates/../bad.json",
		SHA256:       sha256HexString("template"),
	})
	require.ErrorContains(t, err, "deploy assembly stack template")

	_, err = validateReleaseAssemblyTemplatePath(releaseDeployAssemblyStackManifest{
		Name:         string(naming.StageDev),
		Stage:        string(naming.StageDev),
		TemplatePath: "templates/lesser-managed-dev.template.json",
		SHA256:       "bad",
	})
	require.ErrorContains(t, err, "invalid sha256")

	stage, err := validateReleaseAssemblyStageTemplate(releaseDeployAssemblyStackManifest{
		Name:         string(naming.StageLive),
		Stage:        string(naming.StageLive),
		TemplatePath: "templates/lesser-managed-live.template.json",
		SHA256:       sha256HexString("template"),
	})
	require.NoError(t, err)
	require.Equal(t, naming.StageLive, stage)

	_, err = validateReleaseAssemblyStageTemplate(releaseDeployAssemblyStackManifest{
		Name:         string(naming.StageLive),
		Stage:        string(naming.StageDev),
		TemplatePath: "templates/lesser-managed-live.template.json",
		SHA256:       sha256HexString("template"),
	})
	require.ErrorContains(t, err, "stage mismatch")

	_, err = validateReleaseAssemblyStageTemplate(releaseDeployAssemblyStackManifest{
		Name:         "preview",
		Stage:        "preview",
		TemplatePath: "templates/preview.template.json",
		SHA256:       sha256HexString("template"),
	})
	require.ErrorContains(t, err, "unsupported deploy assembly stack")
}

func TestValidateReleaseAssemblyAssetManifest(t *testing.T) {
	seenKeys := map[string]struct{}{}
	seenPaths := map[string]struct{}{}
	asset := releaseDeployAssemblyAssetManifest{
		ObjectKey:   "assets/plain.txt",
		ArchivePath: "assets/plain.txt",
		SHA256:      sha256HexString("plain"),
		SizeBytes:   5,
	}

	path, err := validateReleaseAssemblyAssetManifest(asset, seenKeys, seenPaths)
	require.NoError(t, err)
	require.Equal(t, "assets/plain.txt", path)

	_, err = validateReleaseAssemblyAssetManifest(asset, seenKeys, map[string]struct{}{})
	require.ErrorContains(t, err, "duplicate deploy assembly object key")

	_, err = validateReleaseAssemblyAssetManifest(asset, map[string]struct{}{}, seenPaths)
	require.ErrorContains(t, err, "duplicate deploy assembly asset path")

	asset = releaseDeployAssemblyAssetManifest{ObjectKey: "", ArchivePath: "assets/plain.txt", SHA256: sha256HexString("plain"), SizeBytes: 5}
	_, err = validateReleaseAssemblyAssetManifest(asset, map[string]struct{}{}, map[string]struct{}{})
	require.ErrorContains(t, err, "empty object key")

	asset = releaseDeployAssemblyAssetManifest{ObjectKey: "k", ArchivePath: "assets/../plain.txt", SHA256: sha256HexString("plain"), SizeBytes: 5}
	_, err = validateReleaseAssemblyAssetManifest(asset, map[string]struct{}{}, map[string]struct{}{})
	require.Error(t, err)

	asset = releaseDeployAssemblyAssetManifest{ObjectKey: "k", ArchivePath: "assets/plain.txt", SHA256: "bad", SizeBytes: 5}
	_, err = validateReleaseAssemblyAssetManifest(asset, map[string]struct{}{}, map[string]struct{}{})
	require.ErrorContains(t, err, "invalid sha256")

	asset = releaseDeployAssemblyAssetManifest{ObjectKey: "k", ArchivePath: "assets/plain.txt", SHA256: sha256HexString("plain"), SizeBytes: 0}
	_, err = validateReleaseAssemblyAssetManifest(asset, map[string]struct{}{}, map[string]struct{}{})
	require.ErrorContains(t, err, "invalid size")
}

func TestPopulateReleaseAssemblyTemplates_GuardsRequiredTemplates(t *testing.T) {
	writeTemplate := func(t *testing.T, root string, path string) {
		t.Helper()
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(`{"Parameters":{"AppSlug":{"Type":"String"}}}`), 0o644))
	}

	t.Run("missing shared template", func(t *testing.T) {
		stagingDir := t.TempDir()
		payload := validReleaseAssemblyPayloadManifest()
		stacks := payload.Stacks[1:]
		extractedFiles := map[string]struct{}{}
		for _, stack := range stacks {
			writeTemplate(t, stagingDir, stack.TemplatePath)
			extractedFiles[stack.TemplatePath] = struct{}{}
		}

		result := releaseDeployAssemblyInstallResult{StageTemplates: map[naming.Stage]string{}}
		err := populateReleaseAssemblyTemplates(&result, stacks, stagingDir, extractedFiles, map[string]struct{}{"manifest.json": {}})
		require.ErrorContains(t, err, "deploy assembly payload missing shared template")
	})

	t.Run("missing live stage template", func(t *testing.T) {
		stagingDir := t.TempDir()
		payload := validReleaseAssemblyPayloadManifest()
		stacks := payload.Stacks[:3]
		extractedFiles := map[string]struct{}{}
		for _, stack := range stacks {
			writeTemplate(t, stagingDir, stack.TemplatePath)
			extractedFiles[stack.TemplatePath] = struct{}{}
		}

		result := releaseDeployAssemblyInstallResult{StageTemplates: map[naming.Stage]string{}}
		err := populateReleaseAssemblyTemplates(&result, stacks, stagingDir, extractedFiles, map[string]struct{}{"manifest.json": {}})
		require.ErrorContains(t, err, "deploy assembly payload missing live stage template")
	})
}

func TestVerifyReleaseAssemblyAssetFile(t *testing.T) {
	stagingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stagingDir, "assets"), 0o755))
	fullPath := filepath.Join(stagingDir, "assets", "plain.txt")
	require.NoError(t, os.WriteFile(fullPath, []byte("plain"), 0o644))

	asset := releaseDeployAssemblyAssetManifest{
		ObjectKey:   "assets/plain.txt",
		ArchivePath: "assets/plain.txt",
		SHA256:      sha256HexString("plain"),
		SizeBytes:   5,
	}
	require.NoError(t, verifyReleaseAssemblyAssetFile(stagingDir, asset))

	mismatch := asset
	mismatch.SizeBytes = 6
	require.ErrorContains(t, verifyReleaseAssemblyAssetFile(stagingDir, mismatch), "size mismatch")

	badChecksum := asset
	badChecksum.SHA256 = sha256HexString("wrong")
	require.ErrorContains(t, verifyReleaseAssemblyAssetFile(stagingDir, badChecksum), "checksum mismatch")

	require.NoError(t, os.MkdirAll(filepath.Join(stagingDir, "assets", "dir"), 0o755))
	dirAsset := asset
	dirAsset.ArchivePath = "assets/dir"
	dirAsset.ObjectKey = "assets/dir"
	dirAsset.SHA256 = sha256HexString("")
	dirAsset.SizeBytes = 0
	require.ErrorContains(t, verifyReleaseAssemblyAssetFile(stagingDir, dirAsset), "is a directory")
}

func TestValidateReleaseAssemblyAssets_ErrorsWhenAssetFileMissing(t *testing.T) {
	_, err := validateReleaseAssemblyAssets([]releaseDeployAssemblyAssetManifest{
		{
			ObjectKey:   "assets/plain.txt",
			ArchivePath: "assets/plain.txt",
			SHA256:      sha256HexString("plain"),
			SizeBytes:   5,
		},
	}, t.TempDir(), map[string]struct{}{})
	require.ErrorContains(t, err, "stat deploy assembly asset assets/plain.txt")
}

func TestValidatedArchiveEntryPath(t *testing.T) {
	_, err := validatedArchiveEntryPath(nil)
	require.ErrorContains(t, err, "archive entry is nil")

	_, err = validatedArchiveEntryPath(&tar.Header{Name: "dir", Typeflag: tar.TypeDir})
	require.ErrorContains(t, err, "unexpected archive entry type")

	_, err = validatedArchiveEntryPath(&tar.Header{Name: "../bad", Typeflag: tar.TypeReg})
	require.ErrorContains(t, err, "must not contain '..'")

	_, err = validatedArchiveEntryPath(&tar.Header{Name: `bad\path`, Typeflag: tar.TypeReg})
	require.ErrorContains(t, err, "must use forward slashes")

	path, err := validatedArchiveEntryPath(&tar.Header{Name: "assets/plain.txt", Typeflag: tar.TypeReg})
	require.NoError(t, err)
	require.Equal(t, "assets/plain.txt", path)
}

func TestWriteExtractedFile(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "assets", "plain.txt")
	require.NoError(t, writeExtractedFile(targetPath, bytes.NewReader([]byte("plain")), 5, "plain.txt"))

	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, "plain", string(data))

	err = writeExtractedFile(filepath.Join(t.TempDir(), "assets", "short.txt"), bytes.NewReader([]byte("tiny")), 5, "short.txt")
	require.ErrorContains(t, err, "size mismatch")
}

func TestResetAndFinalizeReleaseWorkspaceDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "workspace")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "old.txt"), []byte("old"), 0o644))

	require.NoError(t, resetReleaseWorkspaceDir(target))
	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	require.Empty(t, entries)

	staging := filepath.Join(root, "staging")
	require.NoError(t, os.MkdirAll(staging, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "new.txt"), []byte("new"), 0o644))
	require.NoError(t, finalizeReleaseWorkspaceDir(target, staging))

	data, err := os.ReadFile(filepath.Join(target, "new.txt"))
	require.NoError(t, err)
	require.Equal(t, "new", string(data))
}

func TestReleaseStageParameterHelpers(t *testing.T) {
	require.Equal(t, "https://example.com", normalizeReleaseStageParameter("lesserHostUrl", "https://example.com/"))
	require.Equal(t, "value", normalizeReleaseStageParameter("other", "value"))
	require.Equal(t, "", optionalBoolString(nil))

	valueTrue := true
	require.Equal(t, "true", optionalBoolString(&valueTrue))
	require.Equal(t, "", optionalIntString(nil))

	valueTen := 10
	require.Equal(t, "10", optionalIntString(&valueTen))
}

func validDeployAssemblyDescriptor(releaseManifest releaseassets.ReleaseManifest) releaseassets.DeployAssemblyDescriptor {
	return releaseassets.DeployAssemblyDescriptor{
		Kind:          releaseassets.DeployAssemblyManifestKind,
		SchemaVersion: releaseassets.DeployAssemblyManifestSchemaVersion,
		Release: releaseassets.LambdaBundleRelease{
			Name:    releaseManifest.Name,
			Version: releaseManifest.Version,
			GitSHA:  releaseManifest.GitSHA,
		},
		Assembly: releaseassets.DeployAssemblyAsset{
			Path:   releaseassets.DeployAssemblyArchiveName,
			Format: tarGzFormat,
			SHA256: sha256HexString("assembly"),
		},
		Payload: releaseassets.DeployAssemblyPayload{
			Kind:            "lesser.cloudformation_release_assembly",
			ContractVersion: 1,
			Entrypoint:      "manifest.json",
		},
		Compatibility: releaseassets.DeployAssemblyCompatibility{
			ReleaseManifestPath:     releaseassets.ReleaseManifestName,
			DeployArtifactsKey:      "deploy_assembly",
			ExecutorContractVersion: 1,
		},
		InstanceInputs: releaseassets.DeployAssemblyInstanceInputs{
			Required: []string{"app_identity", "aws_target", "base_domain", "hosted_zone", "stage_plan"},
			Optional: []string{"feature_config", "managed_service_urls", "provisioning_input", "bootstrap_io"},
		},
		Verification: releaseassets.DeployAssemblyVerification{
			IntegrityRequired: []string{"assembly.sha256", "checksums.txt"},
			PreflightRequired: []string{"instance_input_validation", "release_manifest_compatibility"},
		},
	}
}

func validReleaseAssemblyPayloadManifest() releaseDeployAssemblyPayloadManifest {
	return releaseDeployAssemblyPayloadManifest{
		Kind:          releaseAssemblyPayloadManifestKind,
		SchemaVersion: releaseAssemblyPayloadManifestSchema,
		Release: releaseassets.LambdaBundleRelease{
			Name:    lesserReleaseName,
			Version: "v1.2.3",
			GitSHA:  "0123456789abcdef0123456789abcdef01234567",
		},
		Stacks: []releaseDeployAssemblyStackManifest{
			{Name: string(naming.StageShared), TemplatePath: releaseAssemblySharedTemplatePath, SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
			{Name: string(naming.StageDev), Stage: string(naming.StageDev), TemplatePath: "templates/lesser-managed-dev.template.json", SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
			{Name: string(naming.StageStaging), Stage: string(naming.StageStaging), TemplatePath: "templates/lesser-managed-staging.template.json", SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
			{Name: string(naming.StageLive), Stage: string(naming.StageLive), TemplatePath: "templates/lesser-managed-live.template.json", SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
		},
	}
}
