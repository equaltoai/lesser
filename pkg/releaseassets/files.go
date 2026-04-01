package releaseassets

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type localFile struct {
	RelativePath string
	FullPath     string
}

func listFiles(root string) ([]localFile, error) {
	var files []localFile

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "/")

		files = append(files, localFile{
			RelativePath: rel,
			FullPath:     path,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files, nil
}

func zipDirectory(sourceDir string, outPath string) error {
	files, err := listFiles(sourceDir)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) // #nosec G304 -- caller controls output path
	if err != nil {
		return fmt.Errorf("create zip %s: %w", outPath, err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for _, file := range files {
		info, err := os.Stat(file.FullPath)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("stat zip source %s: %w", file.FullPath, err)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("create zip header %s: %w", file.RelativePath, err)
		}
		header.Name = filepath.ToSlash(file.RelativePath)
		header.Method = zip.Deflate
		header.Modified = deterministicArchiveTime

		writer, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("create zip entry %s: %w", file.RelativePath, err)
		}

		src, err := os.Open(file.FullPath) // #nosec G304 -- file path comes from the synthesized asset directory
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("open zip source %s: %w", file.FullPath, err)
		}
		if _, err := io.Copy(writer, src); err != nil {
			_ = src.Close()
			_ = zw.Close()
			return fmt.Errorf("write zip entry %s: %w", file.RelativePath, err)
		}
		_ = src.Close()
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalize zip %s: %w", outPath, err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}
