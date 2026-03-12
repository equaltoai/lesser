package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type execOptions struct {
	Dir string
	Env map[string]string
}

const goToolchainAuto = "auto"

var runCommandFn = runCommand

func runCommand(ctx context.Context, name string, args []string, opts execOptions) error {
	env := mergeEnvForDir(os.Environ(), opts.Env, opts.Dir)
	path := name
	if resolved, err := lookPathInEnv(name, env); err == nil && resolved != "" {
		path = resolved
	} else if err != nil && !strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}

	cmd := exec.CommandContext(ctx, path, args...) // #nosec G204 -- tool invocation, args are not interpreted by a shell
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func lookPathInEnv(name string, env []string) (string, error) {
	if strings.ContainsAny(name, "/\\") {
		return name, nil
	}

	pathEnv := getEnv(env, "PATH")
	if pathEnv == "" {
		return "", fmt.Errorf("executable file %q not found in PATH", name)
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		if isExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("executable file %q not found in PATH", name)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func mergeEnvForDir(base []string, overrides map[string]string, dir string) []string {
	env := append([]string(nil), base...)
	if _, ok := overrides["GOTOOLCHAIN"]; !ok && !envHasKey(env, "GOTOOLCHAIN") {
		env = append(env, "GOTOOLCHAIN="+defaultGoToolchainForDir(dir))
	}
	if _, ok := overrides["PATH"]; !ok && envHasKey(env, "PATH") {
		if goBin := strings.TrimSpace(goPathBin()); goBin != "" {
			current := getEnv(env, "PATH")
			sep := string(os.PathListSeparator)
			if current == "" {
				env = setEnv(env, "PATH", goBin)
			} else if !strings.HasPrefix(current, goBin+sep) && current != goBin {
				env = setEnv(env, "PATH", goBin+sep+current)
			}
		}
	}
	for key, value := range overrides {
		env = setEnv(env, key, value)
	}
	return env
}

func defaultGoToolchainForDir(dir string) string {
	start := dir
	if strings.TrimSpace(start) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return goToolchainAuto
		}
		start = wd
	}

	repoRoot, err := findRepoRootFrom(start)
	if err != nil {
		return goToolchainAuto
	}

	toolchain, err := readRequestedGoToolchain(repoRoot)
	if err != nil || strings.TrimSpace(toolchain) == "" {
		return goToolchainAuto
	}
	return toolchain
}

func goPathBin() string {
	gopath := strings.TrimSpace(os.Getenv("GOPATH"))
	if gopath == "" {
		cmd := exec.Command("go", "env", "GOPATH") //nolint:gosec // local tool invocation
		output, err := cmd.Output()
		if err == nil {
			gopath = strings.TrimSpace(string(output))
		}
	}
	if gopath == "" {
		return ""
	}

	parts := strings.Split(gopath, string(os.PathListSeparator))
	if len(parts) > 0 {
		gopath = parts[0]
	}
	if strings.TrimSpace(gopath) == "" {
		return ""
	}
	return filepath.Join(gopath, "bin")
}

func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func getEnv(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
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
	if override := strings.TrimSpace(os.Getenv("GOCACHE")); override != "" && override != "off" {
		if err := os.MkdirAll(override, 0o750); err != nil {
			return "", fmt.Errorf("create go-cache dir: %w", err)
		}
		return override, nil
	}

	path := filepath.Join(repoRoot, "tmp", "go-cache", cacheDirVersionKey())
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("create go-cache dir: %w", err)
	}
	return path, nil
}

func ensureXDGCacheDir(repoRoot string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); override != "" {
		if err := os.MkdirAll(override, 0o750); err != nil {
			return "", fmt.Errorf("create xdg-cache dir: %w", err)
		}
		return override, nil
	}

	path := filepath.Join(repoRoot, "tmp", "xdg-cache", cacheDirVersionKey())
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("create xdg-cache dir: %w", err)
	}
	return path, nil
}

func cacheDirVersionKey() string {
	// Prefer the effective Go tool version our invoked `go` commands will use.
	// Default to the repo's requested toolchain so cache directories stay
	// isolated across patch-level upgrades and avoid launcher/tool drift.
	cmd := exec.Command("go", "env", "GOVERSION") //nolint:gosec // tool invocation
	cmd.Env = append([]string(nil), os.Environ()...)
	if !envHasKey(cmd.Env, "GOTOOLCHAIN") {
		cmd.Env = append(cmd.Env, "GOTOOLCHAIN="+defaultGoToolchainForDir(""))
	}
	output, err := cmd.Output()
	if err == nil {
		if version := strings.TrimSpace(string(output)); version != "" {
			return sanitizeCacheKey(version)
		}
	}

	// Fall back to the Go runtime version used to build this binary.
	return sanitizeCacheKey(runtime.Version())
}

func sanitizeCacheKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	if value == "" {
		return "unknown"
	}
	return value
}

func envOrDefault(key string, value string) string {
	current := strings.TrimSpace(os.Getenv(key))
	if current != "" {
		return current
	}
	return value
}
