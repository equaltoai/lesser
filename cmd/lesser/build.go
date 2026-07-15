package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

var (
	loadLambdaNamesFromInventoryFn = loadLambdaNamesFromInventory
	latestGoSourceUpdateFn         = latestGoSourceUpdate
	buildLambdaBinaryFn            = buildLambdaBinary
	zipSingleFileFn                = zipSingleFile
	runExecCmdFn                   = func(cmd *exec.Cmd) error { return cmd.Run() }
)

func buildLambdaZips(repoRoot string, force bool) error {
	lambdaNames, err := loadLambdaNamesFromInventoryFn(repoRoot)
	if err != nil {
		return err
	}

	sourceUpdatedAt, err := latestGoSourceUpdateFn(repoRoot)
	if err != nil {
		return err
	}

	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	cacheDir, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	built := 0
	skipped := 0

	for _, lambdaName := range lambdaNames {
		zipPath := filepath.Join(binDir, fmt.Sprintf("%s.zip", lambdaName))
		if !force && fileExists(zipPath) {
			zipInfo, err := os.Stat(zipPath)
			if err != nil {
				return fmt.Errorf("stat %s: %w", zipPath, err)
			}
			if !sourceUpdatedAt.After(zipInfo.ModTime()) {
				skipped++
				continue
			}
		}

		fmt.Println("Building Lambda:", lambdaName)

		bootstrapPath := filepath.Join(binDir, "bootstrap")
		if err := buildLambdaBinaryFn(repoRoot, cacheDir, lambdaName, bootstrapPath); err != nil {
			return err
		}
		if err := zipSingleFileFn(zipPath, "bootstrap", bootstrapPath); err != nil {
			return err
		}
		_ = os.Remove(bootstrapPath)
		built++
	}

	if built == 0 {
		fmt.Printf("✓ Lambda artifacts already present (%d)\n", skipped)
		return nil
	}
	fmt.Printf("✓ Built %d Lambda artifact(s), skipped %d\n", built, skipped)
	return nil
}

func buildLambdaBinary(repoRoot string, cacheDir string, lambdaName string, outPath string) error {
	buildTags := []string{}
	if lambdaName == "sse" {
		buildTags = append(buildTags, "lambda.norpc")
	}

	args := []string{"build", "-ldflags=" + releaseBuildLdflags(), "-o", outPath}
	if len(buildTags) > 0 {
		args = append(args, "-tags", strings.Join(buildTags, ","))
	}
	args = append(args, "./"+filepath.Join("cmd", lambdaName))

	cmd := exec.Command("go", args...) // #nosec G204 -- tool invocation, args are not interpreted by a shell
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=arm64",
		"CGO_ENABLED=0",
		"GOCACHE="+cacheDir,
		"GOTOOLCHAIN="+defaultGoToolchainForDir(repoRoot),
	)

	if err := runExecCmdFn(cmd); err != nil {
		return fmt.Errorf("go build %s: %w", lambdaName, err)
	}
	return nil
}

func releaseBuildLdflags() string {
	parts := []string{"-s", "-w"}
	if v := strings.TrimSpace(os.Getenv("LESSER_BUILD_VERSION")); v != "" {
		parts = append(parts, "-X", "github.com/equaltoai/lesser/pkg/version.Version="+v)
	}
	return strings.Join(parts, " ")
}

func zipSingleFile(zipPath string, entryName string, filePath string) error {
	content, err := os.ReadFile(filePath) // #nosec G304 -- file path is derived from repo root
	if err != nil {
		return fmt.Errorf("read built binary: %w", err)
	}

	tmpPath := zipPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o750); err != nil {
		return err
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) // #nosec G304 -- file path is derived from repo root
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	w, err := zw.Create(entryName)
	if err != nil {
		_ = zw.Close()
		return fmt.Errorf("create zip entry: %w", err)
	}
	if _, err := io.Copy(w, bytes.NewReader(content)); err != nil {
		_ = zw.Close()
		return fmt.Errorf("write zip entry: %w", err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalize zip: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, zipPath); err != nil {
		return fmt.Errorf("finalize zip: %w", err)
	}
	return nil
}

func loadLambdaNamesFromInventory(repoRoot string) ([]string, error) {
	return releaseassets.CanonicalLambdaNames(repoRoot)
}

func latestGoSourceUpdate(repoRoot string) (time.Time, error) {
	latest := time.Time{}

	for _, path := range []string{
		filepath.Join(repoRoot, "go.mod"),
		filepath.Join(repoRoot, "go.sum"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}, fmt.Errorf("stat %s: %w", path, err)
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}

	for _, dir := range []string{
		filepath.Join(repoRoot, "cmd"),
		filepath.Join(repoRoot, "pkg"),
		filepath.Join(repoRoot, "graph"),
		filepath.Join(repoRoot, "graphql"),
	} {
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return time.Time{}, fmt.Errorf("stat %s: %w", dir, err)
		}

		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
			return nil
		}); err != nil {
			return time.Time{}, fmt.Errorf("scan %s: %w", dir, err)
		}
	}

	return latest, nil
}
