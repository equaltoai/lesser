package releaseassets

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteAuthUIBundle(t *testing.T) {
	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, "auth-ui", "dist", "assets")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "auth-ui", "dist", "index.html"), []byte("<html></html>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "app.js"), []byte("console.log('hi')"), 0o644))

	outDir := t.TempDir()
	require.NoError(t, WriteAuthUIBundle(repoRoot, outDir))

	entries := readTarGzEntries(t, filepath.Join(outDir, AuthUIBundleArchiveName))
	require.Equal(t, []bundleEntry{
		{Name: "assets/app.js", Content: "console.log('hi')"},
		{Name: "index.html", Content: "<html></html>"},
	}, entries)
}

func TestWriteAuthUIBundle_IsDeterministic(t *testing.T) {
	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, "auth-ui", "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html></html>"), 0o644))

	firstDir := t.TempDir()
	require.NoError(t, WriteAuthUIBundle(repoRoot, firstDir))
	firstBytes, err := os.ReadFile(filepath.Join(firstDir, AuthUIBundleArchiveName))
	require.NoError(t, err)

	secondDir := t.TempDir()
	require.NoError(t, WriteAuthUIBundle(repoRoot, secondDir))
	secondBytes, err := os.ReadFile(filepath.Join(secondDir, AuthUIBundleArchiveName))
	require.NoError(t, err)

	require.Equal(t, firstBytes, secondBytes)
}

func TestWriteAuthUIBundle_ErrorsWhenDistMissing(t *testing.T) {
	err := WriteAuthUIBundle(t.TempDir(), t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat auth-ui dist")
}

func TestWriteAuthUIBundle_ErrorsWhenDistIsFile(t *testing.T) {
	repoRoot := t.TempDir()
	authUIDir := filepath.Join(repoRoot, "auth-ui")
	require.NoError(t, os.MkdirAll(authUIDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(authUIDir, "dist"), []byte("blocked"), 0o644))

	err := WriteAuthUIBundle(repoRoot, t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not a directory")
}

func TestWriteAuthUIBundle_ErrorsWhenOutputDirBlocked(t *testing.T) {
	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, "auth-ui", "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html></html>"), 0o644))

	outDir := filepath.Join(t.TempDir(), "release")
	require.NoError(t, os.WriteFile(outDir, []byte("blocked"), 0o644))

	err := WriteAuthUIBundle(repoRoot, outDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create release dir")
}

func TestWriteAuthUIBundle_ErrorsWhenArchivePathIsDirectory(t *testing.T) {
	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, "auth-ui", "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html></html>"), 0o644))

	outDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outDir, AuthUIBundleArchiveName), 0o755))

	err := WriteAuthUIBundle(repoRoot, outDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "finalize auth-ui bundle")
}

func TestWriteAuthUIBundle_ErrorsWhenDistFileUnreadable(t *testing.T) {
	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, "auth-ui", "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html></html>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "secret.js"), []byte("nope"), 0o000))

	err := WriteAuthUIBundle(repoRoot, t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "read archive file")
}

func TestWriteAuthUIBundle_RejectsSymlinkedDistFile(t *testing.T) {
	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, "auth-ui", "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	target := filepath.Join(repoRoot, "outside.html")
	require.NoError(t, os.WriteFile(target, []byte("<html>outside</html>"), 0o644))
	if err := os.Symlink(target, filepath.Join(distDir, "index.html")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := WriteAuthUIBundle(repoRoot, t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
}

func TestWriteArchiveFile_ErrorsWhenSourceMissing(t *testing.T) {
	tw := tar.NewWriter(io.Discard)
	err := writeArchiveFile(tw, localFile{
		RelativePath: "index.html",
		FullPath:     filepath.Join(t.TempDir(), "missing.html"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read archive file")
	require.NoError(t, tw.Close())
}

func TestListFiles_ErrorsWhenRootMissing(t *testing.T) {
	_, err := listFiles(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestListFiles_RejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("target"), 0o644))
	if err := os.Symlink(target, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := listFiles(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
}

func TestZipDirectory(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "site")
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "index.html"), []byte("<html></html>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "nested", "app.js"), []byte("console.log('hi')"), 0o644))

	outPath := filepath.Join(t.TempDir(), "site.zip")
	require.NoError(t, zipDirectory(sourceDir, outPath))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"index.html":    "<html></html>",
		"nested/app.js": "console.log('hi')",
	}, readAuthUIZipEntries(t, data))
}

func TestZipDirectory_ErrorsWhenSourceMissing(t *testing.T) {
	err := zipDirectory(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "site.zip"))
	require.Error(t, err)
}

func TestZipDirectory_ErrorsWhenOutputPathBlocked(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "site")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "index.html"), []byte("<html></html>"), 0o644))

	outPath := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.MkdirAll(outPath, 0o755))

	err := zipDirectory(sourceDir, outPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create zip")
}

func TestZipDirectory_ErrorsWhenSourceUnreadable(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "site")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "index.html"), []byte("<html></html>"), 0o000))

	err := zipDirectory(sourceDir, filepath.Join(t.TempDir(), "site.zip"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "open zip source")
}

func TestZipDirectory_RejectsSymlinks(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "site")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	target := filepath.Join(sourceDir, "target.js")
	require.NoError(t, os.WriteFile(target, []byte("console.log('target')"), 0o644))
	if err := os.Symlink(target, filepath.Join(sourceDir, "link.js")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := zipDirectory(sourceDir, filepath.Join(t.TempDir(), "site.zip"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
}

func readTarGzEntries(t *testing.T, archivePath string) []bundleEntry {
	t.Helper()

	f, err := os.Open(archivePath) // #nosec G304 -- test reads temp fixture path
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

func readAuthUIZipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	entries := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		entries[file.Name] = string(content)
	}
	return entries
}
