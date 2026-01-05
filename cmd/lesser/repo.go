package main

import (
	"errors"
	"os"
	"path/filepath"
)

var findRepoRootFn = findRepoRoot

func findRepoRoot() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := start
	for {
		if looksLikeRepoRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("unable to locate repo root (expected to find go.mod and infra/cdk/cdk.json)")
}

func looksLikeRepoRoot(dir string) bool {
	if !fileExists(filepath.Join(dir, "go.mod")) {
		return false
	}
	if !fileExists(filepath.Join(dir, "infra", "cdk", "cdk.json")) {
		return false
	}
	return true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
