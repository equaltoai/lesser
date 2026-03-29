package releaseassets

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalLambdaNames(t *testing.T) {
	repoRoot := t.TempDir()
	inventoryPath := filepath.Join(repoRoot, "infra", "cdk", "inventory")
	require.NoError(t, os.MkdirAll(inventoryPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(inventoryPath, "lambdas.go"), []byte(`package inventory
var LambdaInventory = []struct{ Name string }{
	{Name: "api"},
	{Name: "inbox"},
	{Name: "api"},
}
`), 0o644))

	names, err := CanonicalLambdaNames(repoRoot)
	require.NoError(t, err)
	require.Equal(t, []string{"api", "inbox"}, names)
}

func TestWriteLambdaBundle(t *testing.T) {
	repoRoot := testRepoWithLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	outDir := t.TempDir()

	files, err := WriteLambdaBundle(repoRoot, outDir)
	require.NoError(t, err)
	require.Equal(t, []BundleFile{
		{Lambda: "api", SourcePath: filepath.Join(repoRoot, "bin", "api.zip"), Path: "bin/api.zip", SizeBytes: 7},
		{Lambda: "inbox", SourcePath: filepath.Join(repoRoot, "bin", "inbox.zip"), Path: "bin/inbox.zip", SizeBytes: 9},
	}, files)

	archiveEntries := readBundleEntries(t, filepath.Join(outDir, LambdaBundleArchiveName))
	require.Equal(t, []bundleEntry{
		{Name: "bin/api.zip", Content: "api zip"},
		{Name: "bin/inbox.zip", Content: "inbox zip"},
	}, archiveEntries)
}

func TestWriteLambdaBundle_IsDeterministic(t *testing.T) {
	repoRoot := testRepoWithLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})

	firstDir := t.TempDir()
	_, err := WriteLambdaBundle(repoRoot, firstDir)
	require.NoError(t, err)
	firstBytes, err := os.ReadFile(filepath.Join(firstDir, LambdaBundleArchiveName))
	require.NoError(t, err)

	secondDir := t.TempDir()
	_, err = WriteLambdaBundle(repoRoot, secondDir)
	require.NoError(t, err)
	secondBytes, err := os.ReadFile(filepath.Join(secondDir, LambdaBundleArchiveName))
	require.NoError(t, err)

	require.Equal(t, firstBytes, secondBytes)
}

func TestWriteLambdaBundleManifest(t *testing.T) {
	repoRoot := testRepoWithLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	outDir := t.TempDir()

	files, err := WriteLambdaBundle(repoRoot, outDir)
	require.NoError(t, err)

	manifest, err := WriteLambdaBundleManifest(outDir, "v1.2.3", "0123456789abcdef0123456789abcdef01234567", files)
	require.NoError(t, err)
	require.Equal(t, LambdaBundleManifestKind, manifest.Kind)
	require.Equal(t, LambdaBundleManifestSchemaVersion, manifest.SchemaVersion)
	require.Equal(t, "lesser", manifest.Release.Name)
	require.Equal(t, "v1.2.3", manifest.Release.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", manifest.Release.GitSHA)
	require.Equal(t, LambdaBundleArchiveName, manifest.Bundle.Path)
	require.Equal(t, "tar.gz", manifest.Bundle.Format)
	require.Len(t, manifest.Files, 2)
	require.Equal(t, []LambdaBundleManifestFile{
		{
			Path:      "bin/api.zip",
			Lambda:    "api",
			SHA256:    sha256Hex([]byte("api zip")),
			SizeBytes: 7,
		},
		{
			Path:      "bin/inbox.zip",
			Lambda:    "inbox",
			SHA256:    sha256Hex([]byte("inbox zip")),
			SizeBytes: 9,
		},
	}, manifest.Files)

	data, err := os.ReadFile(filepath.Join(outDir, LambdaBundleManifestName))
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind": "lesser.lambda_bundle_manifest"`)
}

func TestWriteLambdaBundleManifest_ErrorsWhenBundleMissing(t *testing.T) {
	repoRoot := testRepoWithLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})

	files, err := CollectBundleFiles(repoRoot)
	require.NoError(t, err)

	_, err = WriteLambdaBundleManifest(t.TempDir(), "v1.2.3", "0123456789abcdef0123456789abcdef01234567", files)
	require.Error(t, err)
	require.Contains(t, err.Error(), "hash lambda bundle")
}

func TestWriteLambdaBundle_RejectsUnexpectedExtraZip(t *testing.T) {
	repoRoot := testRepoWithLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "bin", "mystery.zip"), []byte("oops"), 0o644))

	_, err := WriteLambdaBundle(repoRoot, t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected zip artifact")
}

func TestWriteLambdaBundle_AllowsCloudfrontKeygenZip(t *testing.T) {
	repoRoot := testRepoWithLambdaArtifacts(t, map[string]string{
		"api":               "api zip",
		"inbox":             "inbox zip",
		"cloudfront-keygen": "helper zip",
	})

	_, err := WriteLambdaBundle(repoRoot, t.TempDir())
	require.NoError(t, err)
}

func TestWriteLambdaBundle_MissingArtifactErrors(t *testing.T) {
	repoRoot := testRepoWithLambdaArtifacts(t, map[string]string{
		"api": "api zip",
	})

	_, err := WriteLambdaBundle(repoRoot, t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "inbox.zip")
}

type bundleEntry struct {
	Name    string
	Content string
}

func readBundleEntries(t *testing.T, bundlePath string) []bundleEntry {
	t.Helper()

	f, err := os.Open(bundlePath) // #nosec G304 -- test reads temp fixture path
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var entries []bundleEntry
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		require.NoError(t, err)

		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		entries = append(entries, bundleEntry{
			Name:    header.Name,
			Content: string(content),
		})
	}
}

func testRepoWithLambdaArtifacts(t *testing.T, files map[string]string) string {
	t.Helper()

	repoRoot := t.TempDir()
	inventoryPath := filepath.Join(repoRoot, "infra", "cdk", "inventory")
	require.NoError(t, os.MkdirAll(inventoryPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(inventoryPath, "lambdas.go"), []byte(`package inventory
var LambdaInventory = []struct{ Name string }{
	{Name: "api"},
	{Name: "inbox"},
}
`), 0o644))

	binDir := filepath.Join(repoRoot, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name+".zip"), []byte(content), 0o644))
	}

	return repoRoot
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
