package releaseassets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const LambdaBundleArchiveName = "lesser-lambda-bundle.tar.gz"

var deterministicArchiveTime = time.Unix(0, 0).UTC()

type BundleFile struct {
	Lambda     string
	SourcePath string
	Path       string
	SizeBytes  int64
}

func CollectBundleFiles(repoRoot string) ([]BundleFile, error) {
	lambdaNames, err := CanonicalLambdaNames(repoRoot)
	if err != nil {
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
	gz.Header.ModTime = deterministicArchiveTime
	gz.Header.OS = 255

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
