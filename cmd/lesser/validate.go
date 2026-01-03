package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var (
	userHomeDirFn = os.UserHomeDir
	mkdirAllFn    = os.MkdirAll
)

func normalizeBaseDomain(input string) (string, error) {
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(input)), ".")
	if domain == "" {
		return "", fmt.Errorf("base domain is required")
	}
	if strings.Contains(domain, "/") || strings.Contains(domain, ":") {
		return "", fmt.Errorf("invalid base domain %q", input)
	}
	if len(domain) > 253 {
		return "", fmt.Errorf("base domain is too long")
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("base domain must be a fully-qualified domain name (got %q)", input)
	}
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("invalid base domain %q", input)
		}
		if len(part) > 63 {
			return "", fmt.Errorf("invalid base domain %q (label too long)", input)
		}
		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return "", fmt.Errorf("invalid base domain %q (labels cannot start/end with '-')", input)
		}
		for _, r := range part {
			if r == '-' {
				continue
			}
			if unicode.IsLower(r) || unicode.IsDigit(r) {
				continue
			}
			return "", fmt.Errorf("invalid base domain %q (only lowercase letters, digits, and '-' are allowed)", input)
		}
	}

	return domain, nil
}

func ensureLocalStateDir(app, baseDomain string) (string, error) {
	home, err := userHomeDirFn()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	dir := filepath.Join(home, ".lesser", app, baseDomain)
	if err := mkdirAllFn(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir: %w", err)
	}
	return dir, nil
}
