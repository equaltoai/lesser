package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func resolveReceiptPath(app, baseDomain, explicitPath string) (string, error) {
	if path := strings.TrimSpace(explicitPath); path != "" {
		if !fileExists(path) {
			return "", fmt.Errorf("deployment receipt not found at %s", path)
		}
		return path, nil
	}

	home, err := userHomeDirFn()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	statePath := filepath.Join(home, ".lesser", app, baseDomain, "state.json")
	if !fileExists(statePath) {
		return "", fmt.Errorf("deployment receipt not found at %s (run `lesser up` first or pass --state)", statePath)
	}
	return statePath, nil
}
