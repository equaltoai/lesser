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
	"regexp"
	"sort"
	"strings"
	"time"
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

	cacheDir := filepath.Join(repoRoot, "tmp", "go-cache")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return fmt.Errorf("create go-cache dir: %w", err)
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

	args := []string{"build", "-ldflags=-s -w", "-o", outPath}
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
	)

	if err := runExecCmdFn(cmd); err != nil {
		return fmt.Errorf("go build %s: %w", lambdaName, err)
	}
	return nil
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
	path := filepath.Join(repoRoot, "infra", "cdk", "inventory", "lambdas.go")
	data, err := os.ReadFile(path) // #nosec G304 -- file path is derived from repo root
	if err != nil {
		return nil, fmt.Errorf("read lambda inventory: %w", err)
	}

	re := regexp.MustCompile(`\bName:\s*"([^"]+)"`)
	matches := re.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no Lambda names found in %s", path)
	}

	seen := map[string]struct{}{}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		name := string(match[1])
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	sort.Strings(names)
	return names, nil
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
