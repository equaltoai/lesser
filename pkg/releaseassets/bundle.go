// Package releaseassets builds the immutable release artifacts published for
// Lesser deployments.
package releaseassets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LambdaBundleArchiveName is the published tarball that contains the Lambda
// zip artifacts for a release.
const LambdaBundleArchiveName = "lesser-lambda-bundle.tar.gz"

// LambdaBundleManifestName is the published manifest that describes the Lambda
// bundle contents.
const LambdaBundleManifestName = "lesser-lambda-bundle.json"

// LambdaBundleManifestKind identifies Lesser Lambda bundle manifest documents.
const LambdaBundleManifestKind = "lesser.lambda_bundle_manifest"

// LambdaBundleManifestSchemaVersion is the current schema version for Lambda
// bundle manifests.
const LambdaBundleManifestSchemaVersion = 1

// LambdaInventoryKind identifies the lambda inventory source document kind.
const LambdaInventoryKind = "lesser.lambda_inventory"

var deterministicArchiveTime = time.Unix(0, 0).UTC()
var allowedExtraZipArtifacts = map[string]struct{}{
	"cloudfront-keygen": {},
}

// BundleFile describes one Lambda zip file included in the release bundle.
type BundleFile struct {
	Lambda     string
	SourcePath string
	Path       string
	SizeBytes  int64
}

// LambdaBundleManifest describes the published Lambda bundle asset and its
// contents.
type LambdaBundleManifest struct {
	Kind            string                      `json:"kind"`
	SchemaVersion   int                         `json:"schema_version"`
	Release         LambdaBundleRelease         `json:"release"`
	Bundle          LambdaBundleAsset           `json:"bundle"`
	InventorySource LambdaBundleInventorySource `json:"inventory_source"`
	Files           []LambdaBundleManifestFile  `json:"files"`
}

// LambdaBundleRelease identifies the release that produced a Lambda bundle.
type LambdaBundleRelease struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	GitSHA  string `json:"git_sha"`
}

// LambdaBundleAsset describes the published Lambda bundle archive.
type LambdaBundleAsset struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	SHA256 string `json:"sha256"`
}

// LambdaBundleInventorySource identifies the canonical inventory that defined
// the expected Lambda set.
type LambdaBundleInventorySource struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// LambdaBundleManifestFile records metadata for one Lambda zip in the bundle.
type LambdaBundleManifestFile struct {
	Path      string `json:"path"`
	Lambda    string `json:"lambda"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// CollectBundleFiles resolves the canonical Lambda zip files that must be
// included in the published release bundle.
func CollectBundleFiles(repoRoot string) ([]BundleFile, error) {
	lambdaNames, err := CanonicalLambdaNames(repoRoot)
	if err != nil {
		return nil, err
	}

	if err := verifyLocalLambdaZipSet(repoRoot, lambdaNames); err != nil {
		return nil, err
	}

	files := make([]BundleFile, 0, len(lambdaNames))
	for _, lambdaName := range lambdaNames {
		sourcePath := filepath.Join(repoRoot, "bin", lambdaName+".zip")
		info, err := os.Stat(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("stat lambda artifact %s: %w", sourcePath, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("lambda artifact %s is a directory", sourcePath)
		}

		files = append(files, BundleFile{
			Lambda:     lambdaName,
			SourcePath: sourcePath,
			Path:       filepath.ToSlash(filepath.Join("bin", lambdaName+".zip")),
			SizeBytes:  info.Size(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files, nil
}

// WriteLambdaBundle writes the deterministic Lambda bundle archive into outDir
// and returns the files that were included.
func WriteLambdaBundle(repoRoot string, outDir string) ([]BundleFile, error) {
	files, err := CollectBundleFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create release dir: %w", err)
	}

	bundlePath := filepath.Join(outDir, LambdaBundleArchiveName)
	tmpPath := bundlePath + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) // #nosec G304 -- file path is derived from caller-provided output dir
	if err != nil {
		return nil, fmt.Errorf("create lambda bundle: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}
	gz.ModTime = deterministicArchiveTime
	gz.OS = 255

	tw := tar.NewWriter(gz)
	for _, file := range files {
		if err := writeBundleFile(tw, file); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = f.Close()
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = f.Close()
		return nil, fmt.Errorf("finalize tar stream: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("finalize gzip stream: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, bundlePath); err != nil {
		return nil, fmt.Errorf("finalize lambda bundle: %w", err)
	}

	return files, nil
}

// WriteLambdaBundleManifest writes the published metadata document that
// describes the Lambda bundle archive and each bundled Lambda zip.
func WriteLambdaBundleManifest(outDir string, version string, gitSHA string, files []BundleFile) (LambdaBundleManifest, error) {
	if version == "" {
		return LambdaBundleManifest{}, fmt.Errorf("release version is required")
	}
	if gitSHA == "" {
		return LambdaBundleManifest{}, fmt.Errorf("release git SHA is required")
	}

	bundlePath := filepath.Join(outDir, LambdaBundleArchiveName)
	bundleSHA, err := fileSHA256(bundlePath)
	if err != nil {
		return LambdaBundleManifest{}, fmt.Errorf("hash lambda bundle: %w", err)
	}

	manifestFiles := make([]LambdaBundleManifestFile, 0, len(files))
	for _, file := range files {
		fileSHA, err := fileSHA256(file.SourcePath)
		if err != nil {
			return LambdaBundleManifest{}, fmt.Errorf("hash lambda artifact %s: %w", file.SourcePath, err)
		}
		manifestFiles = append(manifestFiles, LambdaBundleManifestFile{
			Path:      file.Path,
			Lambda:    file.Lambda,
			SHA256:    fileSHA,
			SizeBytes: file.SizeBytes,
		})
	}

	manifest := LambdaBundleManifest{
		Kind:          LambdaBundleManifestKind,
		SchemaVersion: LambdaBundleManifestSchemaVersion,
		Release: LambdaBundleRelease{
			Name:    "lesser",
			Version: version,
			GitSHA:  gitSHA,
		},
		Bundle: LambdaBundleAsset{
			Path:   LambdaBundleArchiveName,
			Format: "tar.gz",
			SHA256: bundleSHA,
		},
		InventorySource: LambdaBundleInventorySource{
			Path: LambdaInventoryPath,
			Kind: LambdaInventoryKind,
		},
		Files: manifestFiles,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return LambdaBundleManifest{}, fmt.Errorf("marshal lambda bundle manifest: %w", err)
	}
	data = append(data, '\n')

	manifestPath := filepath.Join(outDir, LambdaBundleManifestName)
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return LambdaBundleManifest{}, fmt.Errorf("write lambda bundle manifest: %w", err)
	}

	return manifest, nil
}

func writeBundleFile(tw *tar.Writer, file BundleFile) error {
	content, err := os.ReadFile(file.SourcePath) // #nosec G304 -- bundle file path is derived from the canonical repo layout
	if err != nil {
		return fmt.Errorf("read lambda artifact %s: %w", file.SourcePath, err)
	}

	header := &tar.Header{
		Name:     file.Path,
		Mode:     0o644,
		Size:     int64(len(content)),
		ModTime:  deterministicArchiveTime,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header %s: %w", file.Path, err)
	}
	if _, err := io.Copy(tw, bytesReader(content)); err != nil {
		return fmt.Errorf("write tar content %s: %w", file.Path, err)
	}
	return nil
}

func bytesReader(content []byte) io.Reader {
	return bytes.NewReader(content)
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- caller passes a validated file path under repo or release output roots
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func verifyLocalLambdaZipSet(repoRoot string, expected []string) error {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, name := range expected {
		expectedSet[name] = struct{}{}
	}

	present := map[string]struct{}{}
	matches, err := filepath.Glob(filepath.Join(repoRoot, "bin", "*.zip"))
	if err != nil {
		return fmt.Errorf("scan lambda zip artifacts: %w", err)
	}

	for _, match := range matches {
		name := strings.TrimSuffix(filepath.Base(match), ".zip")
		if _, ok := expectedSet[name]; ok {
			present[name] = struct{}{}
			continue
		}
		if _, ok := allowedExtraZipArtifacts[name]; ok {
			continue
		}
		return fmt.Errorf("unexpected zip artifact outside canonical Lambda inventory: %s", match)
	}

	for _, name := range expected {
		if _, ok := present[name]; ok {
			continue
		}
		return fmt.Errorf("missing canonical Lambda artifact %s", filepath.Join(repoRoot, "bin", name+".zip"))
	}

	return nil
}
