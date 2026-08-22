package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/releaseassets"
)

const (
	releaseAuthUIWorkspaceName           = "auth-ui"
	releaseDeployAssemblyWorkspaceName   = "deploy-assembly"
	releaseAssemblyPayloadManifestKind   = "lesser.cloudformation_release_assembly_manifest"
	releaseAssemblyPayloadManifestSchema = 1
	releaseAssemblySharedTemplatePath    = "templates/lesser-shared.template.json"
)

var (
	installReleaseDeployAssetsFn = installReleaseDeployAssets
	sha256HexPattern             = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type releaseDeployAssetsInstallResult struct {
	Version         string
	GitSHA          string
	LambdaAssetRoot string
	LambdaFiles     []string
	AuthUIDir       string
	Assembly        releaseDeployAssemblyInstallResult
}

type releaseDeployAssemblyInstallResult struct {
	RootDir           string
	SharedTemplate    string
	StageTemplates    map[naming.Stage]string
	Assets            []releaseDeployAssemblyAsset
	PayloadEntrypoint string
}

type releaseDeployAssemblyAsset struct {
	ObjectKey string
	LocalPath string
	SHA256    string
	SizeBytes int64
}

type releaseDeployAssemblyPayloadManifest struct {
	Kind          string                               `json:"kind"`
	SchemaVersion int                                  `json:"schema_version"`
	Release       releaseassets.LambdaBundleRelease    `json:"release"`
	Stacks        []releaseDeployAssemblyStackManifest `json:"stacks"`
	Assets        []releaseDeployAssemblyAssetManifest `json:"assets"`
}

type releaseDeployAssemblyStackManifest struct {
	Name         string `json:"name"`
	Stage        string `json:"stage,omitempty"`
	TemplatePath string `json:"template_path"`
	SHA256       string `json:"sha256"`
}

type releaseDeployAssemblyAssetManifest struct {
	ObjectKey   string `json:"object_key"`
	ArchivePath string `json:"archive_path"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
}

func installReleaseDeployAssets(releaseDir string, workspaceRoot string) (releaseDeployAssetsInstallResult, error) {
	release, err := loadVerifiedReleaseAssets(releaseDir)
	if err != nil {
		return releaseDeployAssetsInstallResult{}, err
	}

	lambdaAssetRoot := deployLambdaAssetRootFromWorkspace(workspaceRoot)
	lambdaResult, err := installReleaseLambdaAssetsFromVerified(release, lambdaAssetRoot)
	if err != nil {
		return releaseDeployAssetsInstallResult{}, err
	}

	authUIDir := deployAuthUIRootFromWorkspace(workspaceRoot)
	if err := installReleaseAuthUIBundleFromVerified(release, authUIDir); err != nil {
		return releaseDeployAssetsInstallResult{}, err
	}

	assemblyRoot := deployAssemblyRootFromWorkspace(workspaceRoot)
	assembly, err := installReleaseDeployAssemblyFromVerified(release, assemblyRoot)
	if err != nil {
		return releaseDeployAssetsInstallResult{}, err
	}

	return releaseDeployAssetsInstallResult{
		Version:         release.releaseManifest.Version,
		GitSHA:          release.releaseManifest.GitSHA,
		LambdaAssetRoot: lambdaAssetRoot,
		LambdaFiles:     lambdaResult.Files,
		AuthUIDir:       authUIDir,
		Assembly:        assembly,
	}, nil
}

func installReleaseLambdaAssetsFromVerified(
	release verifiedReleaseAssets,
	assetRoot string,
) (releaseLambdaInstallResult, error) {
	if err := resetReleaseWorkspaceDir(assetRoot); err != nil {
		return releaseLambdaInstallResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(assetRoot, "bin"), 0o750); err != nil {
		return releaseLambdaInstallResult{}, fmt.Errorf("create lambda asset bin dir: %w", err)
	}

	stagingDir, err := ensureReleaseStagingDir(assetRoot)
	if err != nil {
		return releaseLambdaInstallResult{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	if err := extractBundleToStaging(release.files.bundleArchivePath, release.lambdaBundle, stagingDir); err != nil {
		return releaseLambdaInstallResult{}, err
	}

	installedFiles, err := installExtractedBundleFiles(assetRoot, stagingDir, release.lambdaBundle.Files)
	if err != nil {
		return releaseLambdaInstallResult{}, err
	}

	return releaseLambdaInstallResult{
		Version: release.releaseManifest.Version,
		GitSHA:  release.releaseManifest.GitSHA,
		Files:   installedFiles,
	}, nil
}

func installReleaseAuthUIBundleFromVerified(release verifiedReleaseAssets, authUIRoot string) error {
	parent := filepath.Dir(authUIRoot)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("create auth-ui workspace root: %w", err)
	}

	stagingDir, err := os.MkdirTemp(parent, "release-auth-ui.")
	if err != nil {
		return fmt.Errorf("create auth-ui staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	seen := map[string]struct{}{}
	if err := extractTarGzArchive(release.files.authUIArchivePath, func(header *tar.Header, tr *tar.Reader) error {
		normalizedPath, err := validatedArchiveEntryPath(header)
		if err != nil {
			return fmt.Errorf("auth-ui archive entry %q: %w", header.Name, err)
		}
		if _, exists := seen[normalizedPath]; exists {
			return fmt.Errorf("duplicate auth-ui archive entry %s", normalizedPath)
		}
		targetPath := filepath.Join(stagingDir, filepath.FromSlash(normalizedPath))
		if err := writeExtractedFile(targetPath, tr, header.Size, normalizedPath); err != nil {
			return err
		}
		seen[normalizedPath] = struct{}{}
		return nil
	}); err != nil {
		return err
	}

	if _, ok := seen["index.html"]; !ok {
		return fmt.Errorf("auth-ui archive missing index.html")
	}

	return finalizeReleaseWorkspaceDir(authUIRoot, stagingDir)
}

func installReleaseDeployAssemblyFromVerified(
	release verifiedReleaseAssets,
	assemblyRoot string,
) (releaseDeployAssemblyInstallResult, error) {
	parent := filepath.Dir(assemblyRoot)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return releaseDeployAssemblyInstallResult{}, fmt.Errorf("create deploy assembly workspace root: %w", err)
	}

	stagingDir, err := os.MkdirTemp(parent, "release-deploy-assembly.")
	if err != nil {
		return releaseDeployAssemblyInstallResult{}, fmt.Errorf("create deploy assembly staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	extractedFiles := map[string]struct{}{}
	if err := extractTarGzArchive(release.files.assemblyArchivePath, func(header *tar.Header, tr *tar.Reader) error {
		normalizedPath, err := validatedArchiveEntryPath(header)
		if err != nil {
			return fmt.Errorf("deploy assembly archive entry %q: %w", header.Name, err)
		}
		if _, exists := extractedFiles[normalizedPath]; exists {
			return fmt.Errorf("duplicate deploy assembly archive entry %s", normalizedPath)
		}
		targetPath := filepath.Join(stagingDir, filepath.FromSlash(normalizedPath))
		if err := writeExtractedFile(targetPath, tr, header.Size, normalizedPath); err != nil {
			return err
		}
		extractedFiles[normalizedPath] = struct{}{}
		return nil
	}); err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}

	entrypointPath := filepath.Join(stagingDir, filepath.FromSlash(release.deployAssembly.Payload.Entrypoint))
	payload, err := readReleaseAssemblyPayloadManifest(entrypointPath)
	if err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}
	result, err := validateReleaseAssemblyInstall(payload, release, stagingDir, extractedFiles)
	if err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}

	if err := finalizeReleaseWorkspaceDir(assemblyRoot, stagingDir); err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}

	result.RootDir = assemblyRoot
	result.SharedTemplate = filepath.Join(assemblyRoot, filepath.FromSlash(releaseAssemblySharedTemplatePath))
	for stage, path := range result.StageTemplates {
		result.StageTemplates[stage] = filepath.Join(assemblyRoot, filepath.FromSlash(path))
	}
	for idx := range result.Assets {
		result.Assets[idx].LocalPath = filepath.Join(assemblyRoot, filepath.FromSlash(result.Assets[idx].LocalPath))
	}
	return result, nil
}

func readReleaseAssemblyPayloadManifest(path string) (releaseDeployAssemblyPayloadManifest, error) {
	var payload releaseDeployAssemblyPayloadManifest
	if err := readJSONFile(path, &payload); err != nil {
		return releaseDeployAssemblyPayloadManifest{}, fmt.Errorf("read deploy assembly payload manifest: %w", err)
	}
	return payload, nil
}

func validateReleaseAssemblyInstall(
	payload releaseDeployAssemblyPayloadManifest,
	release verifiedReleaseAssets,
	stagingDir string,
	extractedFiles map[string]struct{},
) (releaseDeployAssemblyInstallResult, error) {
	if err := validateReleaseAssemblyPayloadHeader(payload, release); err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}

	result := releaseDeployAssemblyInstallResult{
		StageTemplates:    map[naming.Stage]string{},
		Assets:            make([]releaseDeployAssemblyAsset, 0, len(payload.Assets)),
		PayloadEntrypoint: release.deployAssembly.Payload.Entrypoint,
	}
	allowedFiles := map[string]struct{}{
		release.deployAssembly.Payload.Entrypoint: {},
	}

	if err := populateReleaseAssemblyTemplates(&result, payload.Stacks, stagingDir, extractedFiles, allowedFiles); err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}

	assets, err := validateReleaseAssemblyAssets(payload.Assets, stagingDir, allowedFiles)
	if err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}
	result.Assets = assets

	if err := ensureReleaseAssemblyArchiveMatchesManifest(extractedFiles, allowedFiles); err != nil {
		return releaseDeployAssemblyInstallResult{}, err
	}

	return result, nil
}

func validateReleaseAssemblyPayloadHeader(
	payload releaseDeployAssemblyPayloadManifest,
	release verifiedReleaseAssets,
) error {
	if payload.Kind != releaseAssemblyPayloadManifestKind {
		return fmt.Errorf("unexpected deploy assembly payload kind %q", payload.Kind)
	}
	if payload.SchemaVersion != releaseAssemblyPayloadManifestSchema {
		return fmt.Errorf("unsupported deploy assembly payload schema %d", payload.SchemaVersion)
	}
	if payload.Release.Name != lesserReleaseName {
		return fmt.Errorf("unexpected deploy assembly payload release name %q", payload.Release.Name)
	}
	if payload.Release.Version != release.releaseManifest.Version {
		return fmt.Errorf(
			"deploy assembly payload version %q does not match release manifest %q",
			payload.Release.Version,
			release.releaseManifest.Version,
		)
	}
	if payload.Release.GitSHA != release.releaseManifest.GitSHA {
		return fmt.Errorf(
			"deploy assembly payload git SHA %q does not match release manifest %q",
			payload.Release.GitSHA,
			release.releaseManifest.GitSHA,
		)
	}
	return nil
}

func populateReleaseAssemblyTemplates(
	result *releaseDeployAssemblyInstallResult,
	stacks []releaseDeployAssemblyStackManifest,
	stagingDir string,
	extractedFiles map[string]struct{},
	allowedFiles map[string]struct{},
) error {
	foundShared := false
	for _, stack := range stacks {
		templatePath, err := validateReleaseAssemblyTemplatePath(stack)
		if err != nil {
			return err
		}
		if _, ok := extractedFiles[templatePath]; !ok {
			return fmt.Errorf("deploy assembly stack template missing %s", templatePath)
		}
		if err := verifyFileChecksum(filepath.Join(stagingDir, filepath.FromSlash(templatePath)), stack.SHA256, templatePath); err != nil {
			return err
		}
		allowedFiles[templatePath] = struct{}{}

		if strings.TrimSpace(stack.Name) == string(naming.StageShared) {
			foundShared = true
			result.SharedTemplate = templatePath
			continue
		}

		stage, err := validateReleaseAssemblyStageTemplate(stack)
		if err != nil {
			return err
		}
		result.StageTemplates[stage] = templatePath
	}

	if !foundShared {
		return fmt.Errorf("deploy assembly payload missing shared template")
	}
	for _, stage := range []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive} {
		if _, ok := result.StageTemplates[stage]; !ok {
			return fmt.Errorf("deploy assembly payload missing %s stage template", stage)
		}
	}
	return nil
}

func validateReleaseAssemblyTemplatePath(stack releaseDeployAssemblyStackManifest) (string, error) {
	normalizedPath, err := normalizeReleaseAssetPath(stack.TemplatePath)
	if err != nil {
		return "", fmt.Errorf("deploy assembly stack template %q: %w", stack.TemplatePath, err)
	}
	if normalizedPath != stack.TemplatePath {
		return "", fmt.Errorf("deploy assembly stack template %q is not normalized", stack.TemplatePath)
	}
	if !sha256HexPattern.MatchString(stack.SHA256) {
		return "", fmt.Errorf("deploy assembly stack %s has invalid sha256 %q", stack.Name, stack.SHA256)
	}
	return normalizedPath, nil
}

func validateReleaseAssemblyStageTemplate(stack releaseDeployAssemblyStackManifest) (naming.Stage, error) {
	switch strings.TrimSpace(stack.Name) {
	case string(naming.StageDev), string(naming.StageStaging), string(naming.StageLive):
		stage := naming.Stage(stack.Name)
		if strings.TrimSpace(stack.Stage) != string(stage) {
			return "", fmt.Errorf(
				"deploy assembly stack %s stage mismatch: got %q want %q",
				stack.Name,
				stack.Stage,
				stage,
			)
		}
		return stage, nil
	default:
		return "", fmt.Errorf("unsupported deploy assembly stack %q", stack.Name)
	}
}

func validateReleaseAssemblyAssets(
	assets []releaseDeployAssemblyAssetManifest,
	stagingDir string,
	allowedFiles map[string]struct{},
) ([]releaseDeployAssemblyAsset, error) {
	seenObjectKeys := map[string]struct{}{}
	seenPaths := map[string]struct{}{}
	result := make([]releaseDeployAssemblyAsset, 0, len(assets))

	for _, asset := range assets {
		assetPath, err := validateReleaseAssemblyAssetManifest(asset, seenObjectKeys, seenPaths)
		if err != nil {
			return nil, err
		}
		if err := verifyReleaseAssemblyAssetFile(stagingDir, asset); err != nil {
			return nil, err
		}
		allowedFiles[assetPath] = struct{}{}
		result = append(result, releaseDeployAssemblyAsset{
			ObjectKey: asset.ObjectKey,
			LocalPath: assetPath,
			SHA256:    asset.SHA256,
			SizeBytes: asset.SizeBytes,
		})
	}

	return result, nil
}

func validateReleaseAssemblyAssetManifest(
	asset releaseDeployAssemblyAssetManifest,
	seenObjectKeys map[string]struct{},
	seenPaths map[string]struct{},
) (string, error) {
	normalizedPath, err := normalizeReleaseAssetPath(asset.ArchivePath)
	if err != nil {
		return "", fmt.Errorf("deploy assembly asset path %q: %w", asset.ArchivePath, err)
	}
	if normalizedPath != asset.ArchivePath {
		return "", fmt.Errorf("deploy assembly asset path %q is not normalized", asset.ArchivePath)
	}
	if strings.TrimSpace(asset.ObjectKey) == "" {
		return "", fmt.Errorf("deploy assembly asset %s has empty object key", asset.ArchivePath)
	}
	if !sha256HexPattern.MatchString(asset.SHA256) {
		return "", fmt.Errorf("deploy assembly asset %s has invalid sha256 %q", asset.ArchivePath, asset.SHA256)
	}
	if asset.SizeBytes <= 0 {
		return "", fmt.Errorf("deploy assembly asset %s has invalid size %d", asset.ArchivePath, asset.SizeBytes)
	}
	if _, exists := seenObjectKeys[asset.ObjectKey]; exists {
		return "", fmt.Errorf("duplicate deploy assembly object key %s", asset.ObjectKey)
	}
	if _, exists := seenPaths[asset.ArchivePath]; exists {
		return "", fmt.Errorf("duplicate deploy assembly asset path %s", asset.ArchivePath)
	}
	seenObjectKeys[asset.ObjectKey] = struct{}{}
	seenPaths[asset.ArchivePath] = struct{}{}
	return normalizedPath, nil
}

func verifyReleaseAssemblyAssetFile(stagingDir string, asset releaseDeployAssemblyAssetManifest) error {
	fullPath := filepath.Join(stagingDir, filepath.FromSlash(asset.ArchivePath))
	info, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("stat deploy assembly asset %s: %w", asset.ArchivePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("deploy assembly asset %s is a directory", asset.ArchivePath)
	}
	if info.Size() != asset.SizeBytes {
		return fmt.Errorf(
			"deploy assembly asset %s size mismatch: got %d want %d",
			asset.ArchivePath,
			info.Size(),
			asset.SizeBytes,
		)
	}
	return verifyFileChecksum(fullPath, asset.SHA256, asset.ArchivePath)
}

func ensureReleaseAssemblyArchiveMatchesManifest(
	extractedFiles map[string]struct{},
	allowedFiles map[string]struct{},
) error {
	extractedList := make([]string, 0, len(extractedFiles))
	for path := range extractedFiles {
		extractedList = append(extractedList, path)
	}
	allowedList := make([]string, 0, len(allowedFiles))
	for path := range allowedFiles {
		allowedList = append(allowedList, path)
	}
	slices.Sort(extractedList)
	slices.Sort(allowedList)
	if !slices.Equal(extractedList, allowedList) {
		return fmt.Errorf("deploy assembly archive contents do not match payload manifest")
	}
	return nil
}

func extractTarGzArchive(archivePath string, visit func(header *tar.Header, tr *tar.Reader) error) error {
	archiveFile, err := os.Open(archivePath) // #nosec G304 -- archive path is validated under the selected release directory
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer func() { _ = archiveFile.Close() }()

	gz, err := gzip.NewReader(archiveFile)
	if err != nil {
		return fmt.Errorf("open gzip stream for %s: %w", archivePath, err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive %s: %w", archivePath, err)
		}
		if err := visit(header, tr); err != nil {
			return err
		}
	}
}

func validatedArchiveEntryPath(header *tar.Header) (string, error) {
	if header == nil {
		return "", fmt.Errorf("archive entry is nil")
	}
	if header.Typeflag != tar.TypeReg {
		return "", fmt.Errorf("unexpected archive entry type")
	}
	if strings.Contains(header.Name, "..") {
		return "", fmt.Errorf("must not contain '..'")
	}
	if strings.Contains(header.Name, `\`) {
		return "", fmt.Errorf("must use forward slashes")
	}
	return normalizeReleaseAssetPath(header.Name)
}

func writeExtractedFile(targetPath string, reader io.Reader, expectedSize int64, label string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return fmt.Errorf("create extraction dir for %s: %w", label, err)
	}

	tmpPath := targetPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) // #nosec G304 -- target path comes from validated archive entries
	if err != nil {
		return fmt.Errorf("create extracted file %s: %w", label, err)
	}

	written, copyErr := io.Copy(f, reader)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("extract archive file %s: %w", label, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close extracted file %s: %w", label, closeErr)
	}
	if written != expectedSize {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("archive file %s size mismatch: got %d want %d", label, written, expectedSize)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize extracted file %s: %w", label, err)
	}
	return nil
}

func resetReleaseWorkspaceDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("reset release workspace %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("create release workspace %s: %w", path, err)
	}
	return nil
}

func finalizeReleaseWorkspaceDir(targetPath string, stagingDir string) error {
	//nolint:gosec // G703: targetPath is the fixed deployment workspace and is validated before this atomic replacement helper is called.
	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("reset release workspace %s: %w", targetPath, err)
	}
	//nolint:gosec // G703: stagingDir is created beside the validated target specifically for this atomic workspace replacement.
	if err := os.Rename(stagingDir, targetPath); err != nil {
		return fmt.Errorf("finalize release workspace %s: %w", targetPath, err)
	}
	return nil
}

func deployLambdaAssetRootFromWorkspace(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, "lambda-assets")
}

func deployAuthUIRootFromWorkspace(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, releaseAuthUIWorkspaceName)
}

func deployAssemblyRootFromWorkspace(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, releaseDeployAssemblyWorkspaceName)
}
