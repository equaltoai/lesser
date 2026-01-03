package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type execOptions struct {
	Dir string
	Env map[string]string
}

var runCommandFn = runCommand

func runCommand(ctx context.Context, name string, args []string, opts execOptions) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // tool invocation
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = mergeEnv(os.Environ(), opts.Env)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func mergeEnv(base []string, overrides map[string]string) []string {
	env := append([]string(nil), base...)
	for key, value := range overrides {
		env = setEnv(env, key, value)
	}
	return env
}

func setEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func ensureGoCacheDir(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, "tmp", "go-cache")
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("create go-cache dir: %w", err)
	}
	return path, nil
}

func ensureXDGCacheDir(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, "tmp", "xdg-cache")
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("create xdg-cache dir: %w", err)
	}
	return path, nil
}

func envOrDefault(key string, value string) string {
	current := strings.TrimSpace(os.Getenv(key))
	if current != "" {
		return current
	}
	return value
}
