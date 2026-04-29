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
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("release asset root %s is a symlink", root)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("release asset root %s is not a directory", root)
	}

	var files []localFile

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("release asset %s is a symlink", path)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat release asset %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("release asset %s is not a regular file", path)
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
		src, info, err := openReleaseAssetFile(file.FullPath)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("open zip source %s: %w", file.FullPath, err)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = src.Close()
			_ = zw.Close()
			return fmt.Errorf("create zip header %s: %w", file.RelativePath, err)
		}
		header.Name = filepath.ToSlash(file.RelativePath)
		header.Method = zip.Deflate
		header.Modified = deterministicArchiveTime

		writer, err := zw.CreateHeader(header)
		if err != nil {
			_ = src.Close()
			_ = zw.Close()
			return fmt.Errorf("create zip entry %s: %w", file.RelativePath, err)
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

func readReleaseAssetFile(path string) ([]byte, error) {
	if _, err := releaseAssetFileInfo(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path) // #nosec G304 -- callers pass repo-local release asset paths
}

func openReleaseAssetFile(path string) (*os.File, os.FileInfo, error) {
	info, err := releaseAssetFileInfo(path)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(path) // #nosec G304 -- callers pass repo-local release asset paths
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
}

func releaseAssetFileInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("release asset %s is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("release asset %s is not a regular file", path)
	}
	return info, nil
}
