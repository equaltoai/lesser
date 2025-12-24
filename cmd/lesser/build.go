package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func buildLambdaZips(repoRoot string, force bool) error {
	lambdaNames, err := loadLambdaNamesFromInventory(repoRoot)
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
			skipped++
			continue
		}

		fmt.Println("Building Lambda:", lambdaName)

		bootstrapPath := filepath.Join(binDir, "bootstrap")
		if err := buildLambdaBinary(repoRoot, cacheDir, lambdaName, bootstrapPath); err != nil {
			return err
		}
		if err := zipSingleFile(zipPath, "bootstrap", bootstrapPath); err != nil {
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
	args = append(args, filepath.Join("./cmd", lambdaName))

	cmd := exec.Command("go", args...) //nolint:gosec // tool invocation
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=arm64",
		"CGO_ENABLED=0",
		"GOCACHE="+cacheDir,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build %s: %w", lambdaName, err)
	}
	return nil
}

func zipSingleFile(zipPath string, entryName string, filePath string) error {
	content, err := os.ReadFile(filePath) //nolint:gosec // file path is derived from repo root
	if err != nil {
		return fmt.Errorf("read built binary: %w", err)
	}

	tmpPath := zipPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o750); err != nil {
		return err
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) //nolint:gosec // file path is derived from repo root
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
	data, err := os.ReadFile(path) //nolint:gosec // file path is derived from repo root
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
