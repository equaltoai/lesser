package releaseassets

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AuthUIBundleArchiveName is the published auth UI archive for immutable
// release-driven deploys.
const AuthUIBundleArchiveName = "lesser-auth-ui.tar.gz"

// WriteAuthUIBundle writes a deterministic auth UI archive from auth-ui/dist.
func WriteAuthUIBundle(repoRoot string, outDir string) error {
	distDir := filepath.Join(repoRoot, "auth-ui", "dist")
	info, err := os.Stat(distDir)
	if err != nil {
		return fmt.Errorf("stat auth-ui dist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("auth-ui dist %s is not a directory", distDir)
	}

	files, err := listFiles(distDir)
	if err != nil {
		return fmt.Errorf("list auth-ui dist: %w", err)
	}

	//nolint:gosec // Release output is a publication directory whose artifacts must be readable by packaging/upload workers.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create release dir: %w", err)
	}

	archivePath := filepath.Join(outDir, AuthUIBundleArchiveName)
	tmpPath := archivePath + ".tmp"

	//nolint:gosec // This temporary contains only the public release archive and must retain the archive's published 0644 mode.
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create auth-ui bundle: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("create auth-ui gzip writer: %w", err)
	}
	gz.ModTime = deterministicArchiveTime
	gz.OS = 255

	tw := tar.NewWriter(gz)
	for _, file := range files {
		if err := writeArchiveFile(tw, file); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = f.Close()
			return err
		}
	}

	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = f.Close()
		return fmt.Errorf("finalize auth-ui tar stream: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		return fmt.Errorf("finalize auth-ui gzip stream: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return fmt.Errorf("finalize auth-ui bundle: %w", err)
	}

	return nil
}

func writeArchiveFile(tw *tar.Writer, file localFile) error {
	data, err := readReleaseAssetFile(file.FullPath)
	if err != nil {
		return fmt.Errorf("read archive file %s: %w", file.FullPath, err)
	}

	header := &tar.Header{
		Name:     file.RelativePath,
		Mode:     0o644,
		Size:     int64(len(data)),
		ModTime:  deterministicArchiveTime,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write archive header %s: %w", file.RelativePath, err)
	}
	if _, err := io.Copy(tw, bytesReader(data)); err != nil {
		return fmt.Errorf("write archive content %s: %w", file.RelativePath, err)
	}
	return nil
}
