package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

type releaseLambdaInstallResult struct {
	Version string
	GitSHA  string
	Files   []string
}

const (
	lesserReleaseName = "lesser"
	tarGzFormat       = "tar.gz"
)

type releaseFileSet struct {
	checksumsPath        string
	releaseManifestPath  string
	bundleArchivePath    string
	bundleManifestPath   string
	authUIArchivePath    string
	assemblyArchivePath  string
	assemblyManifestPath string
}

func requiredReleaseFiles(releaseDir string) (releaseFileSet, error) {
	required := map[string]*string{
		releaseassets.ChecksumsFileName:          nil,
		releaseassets.ReleaseManifestName:        nil,
		releaseassets.LambdaBundleArchiveName:    nil,
		releaseassets.LambdaBundleManifestName:   nil,
		releaseassets.AuthUIBundleArchiveName:    nil,
		releaseassets.DeployAssemblyArchiveName:  nil,
		releaseassets.DeployAssemblyManifestName: nil,
	}

	files := releaseFileSet{
		checksumsPath:        filepath.Join(releaseDir, releaseassets.ChecksumsFileName),
		releaseManifestPath:  filepath.Join(releaseDir, releaseassets.ReleaseManifestName),
		bundleArchivePath:    filepath.Join(releaseDir, releaseassets.LambdaBundleArchiveName),
		bundleManifestPath:   filepath.Join(releaseDir, releaseassets.LambdaBundleManifestName),
		authUIArchivePath:    filepath.Join(releaseDir, releaseassets.AuthUIBundleArchiveName),
		assemblyArchivePath:  filepath.Join(releaseDir, releaseassets.DeployAssemblyArchiveName),
		assemblyManifestPath: filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName),
	}
	required[releaseassets.ChecksumsFileName] = &files.checksumsPath
	required[releaseassets.ReleaseManifestName] = &files.releaseManifestPath
	required[releaseassets.LambdaBundleArchiveName] = &files.bundleArchivePath
	required[releaseassets.LambdaBundleManifestName] = &files.bundleManifestPath
	required[releaseassets.AuthUIBundleArchiveName] = &files.authUIArchivePath
	required[releaseassets.DeployAssemblyArchiveName] = &files.assemblyArchivePath
	required[releaseassets.DeployAssemblyManifestName] = &files.assemblyManifestPath

	for name, fullPath := range required {
		info, err := os.Stat(*fullPath)
		if err != nil {
			return releaseFileSet{}, fmt.Errorf("required release file %s: %w", name, err)
		}
		if info.IsDir() {
			return releaseFileSet{}, fmt.Errorf("required release file %s is a directory", name)
		}
	}

	return files, nil
}

func readReleaseChecksums(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- checksum path is validated under the selected release directory
	if err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}

	checksums := map[string]string{}
	for lineNo, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("parse checksums line %d: expected '<sha256>  <file>'", lineNo+1)
		}
		checksums[fields[1]] = fields[0]
	}

	return checksums, nil
}

func verifyReleaseChecksums(files releaseFileSet, checksums map[string]string) error {
	for _, item := range []struct {
		name string
		path string
	}{
		{name: releaseassets.ReleaseManifestName, path: files.releaseManifestPath},
		{name: releaseassets.LambdaBundleArchiveName, path: files.bundleArchivePath},
		{name: releaseassets.LambdaBundleManifestName, path: files.bundleManifestPath},
		{name: releaseassets.AuthUIBundleArchiveName, path: files.authUIArchivePath},
		{name: releaseassets.DeployAssemblyArchiveName, path: files.assemblyArchivePath},
		{name: releaseassets.DeployAssemblyManifestName, path: files.assemblyManifestPath},
	} {
		expected, ok := checksums[item.name]
		if !ok {
			return fmt.Errorf("checksums.txt missing entry for %s", item.name)
		}
		if err := verifyFileChecksum(item.path, expected, item.name); err != nil {
			return err
		}
	}
	return nil
}

func readReleaseManifestFile(path string) (releaseassets.ReleaseManifest, error) {
	var manifest releaseassets.ReleaseManifest
	if err := readJSONFile(path, &manifest); err != nil {
		return releaseassets.ReleaseManifest{}, fmt.Errorf("read release manifest: %w", err)
	}
	return manifest, nil
}

func validateReleaseManifest(manifest releaseassets.ReleaseManifest) error {
	if manifest.Schema != 1 {
		return fmt.Errorf("unsupported release manifest schema %d", manifest.Schema)
	}
	if manifest.Name != lesserReleaseName {
		return fmt.Errorf("unexpected release manifest name %q", manifest.Name)
	}
	if manifest.Artifacts.DeployArtifacts.SchemaVersion != releaseassets.ReleaseDeployArtifactsSchemaVersion {
		return fmt.Errorf(
			"unsupported deploy artifact schema %d",
			manifest.Artifacts.DeployArtifacts.SchemaVersion,
		)
	}

	bundle := manifest.Artifacts.DeployArtifacts.LambdaBundle
	if bundle.Path != releaseassets.LambdaBundleArchiveName {
		return fmt.Errorf("unexpected lambda bundle path %q", bundle.Path)
	}
	if bundle.ManifestPath != releaseassets.LambdaBundleManifestName {
		return fmt.Errorf("unexpected lambda bundle manifest path %q", bundle.ManifestPath)
	}
	if bundle.ManifestKind != releaseassets.LambdaBundleManifestKind {
		return fmt.Errorf("unexpected lambda bundle manifest kind %q", bundle.ManifestKind)
	}
	if bundle.ManifestSchemaVersion != releaseassets.LambdaBundleManifestSchemaVersion {
		return fmt.Errorf("unsupported lambda bundle manifest schema %d", bundle.ManifestSchemaVersion)
	}

	authUI := manifest.Artifacts.DeployArtifacts.AuthUIBundle
	if authUI.Path != releaseassets.AuthUIBundleArchiveName {
		return fmt.Errorf("unexpected auth-ui bundle path %q", authUI.Path)
	}
	if authUI.Format != tarGzFormat {
		return fmt.Errorf("unexpected auth-ui bundle format %q", authUI.Format)
	}

	assembly := manifest.Artifacts.DeployArtifacts.DeployAssembly
	if assembly.Path != releaseassets.DeployAssemblyArchiveName {
		return fmt.Errorf("unexpected deploy assembly path %q", assembly.Path)
	}
	if assembly.ManifestPath != releaseassets.DeployAssemblyManifestName {
		return fmt.Errorf("unexpected deploy assembly manifest path %q", assembly.ManifestPath)
	}
	if assembly.ManifestKind != releaseassets.DeployAssemblyManifestKind {
		return fmt.Errorf("unexpected deploy assembly manifest kind %q", assembly.ManifestKind)
	}
	if assembly.ManifestSchemaVersion != releaseassets.DeployAssemblyManifestSchemaVersion {
		return fmt.Errorf("unsupported deploy assembly manifest schema %d", assembly.ManifestSchemaVersion)
	}

	return nil
}

func readLambdaBundleManifestFile(path string) (releaseassets.LambdaBundleManifest, error) {
	var manifest releaseassets.LambdaBundleManifest
	if err := readJSONFile(path, &manifest); err != nil {
		return releaseassets.LambdaBundleManifest{}, fmt.Errorf("read lambda bundle manifest: %w", err)
	}
	return manifest, nil
}

func validateBundleManifest(bundle releaseassets.LambdaBundleManifest, releaseManifest releaseassets.ReleaseManifest) error {
	if bundle.Kind != releaseassets.LambdaBundleManifestKind {
		return fmt.Errorf("unexpected lambda bundle manifest kind %q", bundle.Kind)
	}
	if bundle.SchemaVersion != releaseassets.LambdaBundleManifestSchemaVersion {
		return fmt.Errorf("unsupported lambda bundle manifest schema %d", bundle.SchemaVersion)
	}
	if bundle.Release.Name != lesserReleaseName {
		return fmt.Errorf("unexpected lambda bundle release name %q", bundle.Release.Name)
	}
	if bundle.Release.Version != releaseManifest.Version {
		return fmt.Errorf("lambda bundle version %q does not match release manifest %q", bundle.Release.Version, releaseManifest.Version)
	}
	if bundle.Release.GitSHA != releaseManifest.GitSHA {
		return fmt.Errorf("lambda bundle git SHA %q does not match release manifest %q", bundle.Release.GitSHA, releaseManifest.GitSHA)
	}
	if bundle.Bundle.Path != releaseManifest.Artifacts.DeployArtifacts.LambdaBundle.Path {
		return fmt.Errorf("lambda bundle archive path %q does not match release manifest", bundle.Bundle.Path)
	}
	if bundle.Bundle.Format != tarGzFormat {
		return fmt.Errorf("unexpected lambda bundle format %q", bundle.Bundle.Format)
	}
	if bundle.InventorySource.Path != releaseassets.LambdaInventoryPath {
		return fmt.Errorf("unexpected lambda inventory path %q", bundle.InventorySource.Path)
	}
	if bundle.InventorySource.Kind != releaseassets.LambdaInventoryKind {
		return fmt.Errorf("unexpected lambda inventory kind %q", bundle.InventorySource.Kind)
	}
	if len(bundle.Files) == 0 {
		return fmt.Errorf("lambda bundle manifest has no files")
	}

	paths := make([]string, 0, len(bundle.Files))
	for _, file := range bundle.Files {
		normalizedPath, err := normalizeReleaseAssetPath(file.Path)
		if err != nil {
			return fmt.Errorf("lambda bundle manifest file path %q: %w", file.Path, err)
		}
		if normalizedPath != file.Path {
			return fmt.Errorf("lambda bundle manifest file path %q is not normalized", file.Path)
		}
		if !strings.HasPrefix(file.Path, "bin/") || filepath.Ext(file.Path) != ".zip" {
			return fmt.Errorf("lambda bundle manifest file path %q must be bin/<lambda>.zip", file.Path)
		}
		if file.Lambda != strings.TrimSuffix(path.Base(file.Path), ".zip") {
			return fmt.Errorf("lambda bundle manifest file %q does not match lambda %q", file.Path, file.Lambda)
		}
		paths = append(paths, file.Path)
	}

	if !slices.IsSorted(paths) {
		return fmt.Errorf("lambda bundle manifest files must be sorted")
	}
	for i := 1; i < len(paths); i++ {
		if paths[i] == paths[i-1] {
			return fmt.Errorf("duplicate lambda bundle manifest file %q", paths[i])
		}
	}

	return nil
}

func ensureReleaseStagingDir(assetRoot string) (string, error) {
	workspaceRoot := filepath.Dir(assetRoot)
	//nolint:gosec // G703: assetRoot is the validated release workspace path and only its direct parent is created.
	if err := os.MkdirAll(workspaceRoot, 0o750); err != nil {
		return "", fmt.Errorf("create release workspace root: %w", err)
	}

	stagingDir, err := os.MkdirTemp(workspaceRoot, "release-lambda-assets.")
	if err != nil {
		return "", fmt.Errorf("create release staging dir: %w", err)
	}
	return stagingDir, nil
}

func extractBundleToStaging(bundlePath string, manifest releaseassets.LambdaBundleManifest, stagingDir string) error {
	expectedFiles := make(map[string]releaseassets.LambdaBundleManifestFile, len(manifest.Files))
	for _, file := range manifest.Files {
		expectedFiles[file.Path] = file
	}
	seen := make(map[string]struct{}, len(expectedFiles))

	bundleFile, err := os.Open(bundlePath) // #nosec G304 -- bundle path is validated under the selected release directory
	if err != nil {
		return fmt.Errorf("open lambda bundle: %w", err)
	}
	defer func() { _ = bundleFile.Close() }()

	gz, err := gzip.NewReader(bundleFile)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read bundle archive: %w", err)
		}
		if err := extractBundleEntry(tr, header, stagingDir, expectedFiles, seen); err != nil {
			return err
		}
	}

	if len(seen) != len(expectedFiles) {
		missing := make([]string, 0, len(expectedFiles)-len(seen))
		for _, file := range manifest.Files {
			if _, ok := seen[file.Path]; !ok {
				missing = append(missing, file.Path)
			}
		}
		return fmt.Errorf("lambda bundle archive missing files: %s", strings.Join(missing, ", "))
	}

	return nil
}

func extractBundleEntry(
	tr *tar.Reader,
	header *tar.Header,
	stagingDir string,
	expectedFiles map[string]releaseassets.LambdaBundleManifestFile,
	seen map[string]struct{},
) error {
	if header == nil {
		return fmt.Errorf("bundle archive entry is nil")
	}
	if header.Typeflag != tar.TypeReg {
		return fmt.Errorf("unexpected bundle entry type for %s", header.Name)
	}
	if strings.Contains(header.Name, "..") {
		return fmt.Errorf("bundle archive entry %q must not contain '..'", header.Name)
	}
	if strings.Contains(header.Name, `\`) {
		return fmt.Errorf("bundle archive entry %q must use forward slashes", header.Name)
	}

	normalizedPath, err := normalizeReleaseAssetPath(header.Name)
	if err != nil {
		return fmt.Errorf("bundle archive entry %q: %w", header.Name, err)
	}

	manifestFile, ok := expectedFiles[normalizedPath]
	if !ok {
		return fmt.Errorf("unexpected bundle archive entry %s", normalizedPath)
	}
	if _, exists := seen[normalizedPath]; exists {
		return fmt.Errorf("duplicate bundle archive entry %s", normalizedPath)
	}
	if header.Size != manifestFile.SizeBytes {
		return fmt.Errorf("bundle archive entry %s size mismatch: got %d want %d", normalizedPath, header.Size, manifestFile.SizeBytes)
	}

	targetPath := filepath.Join(stagingDir, filepath.FromSlash(normalizedPath))
	if err := writeVerifiedExtractedFile(targetPath, tr, manifestFile); err != nil {
		return err
	}

	seen[normalizedPath] = struct{}{}
	return nil
}

func writeVerifiedExtractedFile(targetPath string, reader io.Reader, manifestFile releaseassets.LambdaBundleManifestFile) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return fmt.Errorf("create staging dir for %s: %w", manifestFile.Path, err)
	}

	tmpPath := targetPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) // #nosec G304 -- staging path is derived from validated manifest entries
	if err != nil {
		return fmt.Errorf("create staging file %s: %w", manifestFile.Path, err)
	}

	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(f, hasher), reader)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("extract bundle file %s: %w", manifestFile.Path, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close staging file %s: %w", manifestFile.Path, closeErr)
	}

	if written != manifestFile.SizeBytes {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("bundle file %s size mismatch after extraction: got %d want %d", manifestFile.Path, written, manifestFile.SizeBytes)
	}

	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA != manifestFile.SHA256 {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("bundle file %s checksum mismatch", manifestFile.Path)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize staging file %s: %w", manifestFile.Path, err)
	}

	return nil
}

func installExtractedBundleFiles(assetRoot string, stagingDir string, files []releaseassets.LambdaBundleManifestFile) ([]string, error) {
	binDir := filepath.Join(assetRoot, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		return nil, fmt.Errorf("create bin dir: %w", err)
	}

	installed := make([]string, 0, len(files))
	for _, file := range files {
		sourcePath := filepath.Join(stagingDir, filepath.FromSlash(file.Path))
		targetPath := filepath.Join(assetRoot, filepath.FromSlash(file.Path))
		if err := copyFile(targetPath, sourcePath); err != nil {
			return nil, err
		}
		installed = append(installed, targetPath)
	}

	return installed, nil
}

func copyFile(targetPath string, sourcePath string) error {
	//nolint:gosec // G703: sourcePath was containment-checked against the checksum-verified release staging directory by the caller.
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open extracted file %s: %w", sourcePath, err)
	}
	defer func() { _ = sourceFile.Close() }()

	tmpPath := targetPath + ".tmp"
	//nolint:gosec // G703: targetPath is containment-checked under repoRoot/bin and tmpPath is its fixed 0600 sibling.
	targetFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create target file %s: %w", targetPath, err)
	}

	_, copyErr := io.Copy(targetFile, sourceFile)
	closeErr := targetFile.Close()
	if copyErr != nil {
		//nolint:gosec // G703: tmpPath is the fixed sibling of the containment-checked repoRoot/bin target.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("copy extracted file to %s: %w", targetPath, copyErr)
	}
	if closeErr != nil {
		//nolint:gosec // G703: tmpPath is the fixed sibling of the containment-checked repoRoot/bin target.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close target file %s: %w", targetPath, closeErr)
	}

	//nolint:gosec // G703: both paths are fixed siblings beneath the containment-checked repoRoot/bin destination.
	if err := os.Rename(tmpPath, targetPath); err != nil {
		//nolint:gosec // G703: tmpPath is the fixed sibling of the containment-checked repoRoot/bin target.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize target file %s: %w", targetPath, err)
	}
	return nil
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path) // #nosec G304 -- JSON path is validated under the selected release directory
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	return nil
}

func verifyFileChecksum(path string, expectedSHA string, label string) error {
	actualSHA, err := sha256File(path)
	if err != nil {
		return fmt.Errorf("hash %s: %w", label, err)
	}
	if actualSHA != expectedSHA {
		return fmt.Errorf("%s checksum mismatch", label)
	}
	return nil
}

func sha256File(path string) (string, error) {
	//nolint:gosec // G703: callers pass only containment-checked paths under checksum-verified release or staging directories.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hashToString(hasher), nil
}

func hashToString(hasher hash.Hash) string {
	return hex.EncodeToString(hasher.Sum(nil))
}

func normalizeReleaseAssetPath(input string) (string, error) {
	normalized := path.Clean(strings.TrimSpace(input))
	switch {
	case normalized == "", normalized == ".":
		return "", fmt.Errorf("path is empty")
	case path.IsAbs(normalized):
		return "", fmt.Errorf("path must be relative")
	case strings.HasPrefix(normalized, "../"):
		return "", fmt.Errorf("path must stay within release asset root")
	default:
		return normalized, nil
	}
}
