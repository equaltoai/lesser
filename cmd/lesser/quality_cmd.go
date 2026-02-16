package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runFmt(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	if err := ensureToolAvailableFn("go"); err != nil {
		return err
	}
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}
	return runCommandFn(context.Background(), "go", []string{"fmt", "./..."}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runLint(argv []string) error {
	fs := flag.NewFlagSet("lesser lint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var fix bool
	fs.BoolVar(&fix, "fix", false, "auto-fix issues where possible")
	var disableGosec bool
	fs.BoolVar(&disableGosec, "disable-gosec", false, "disable gosec linter (useful when running standalone gosec separately)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	if err := ensureToolAvailableFn("go"); err != nil {
		return err
	}
	if err := ensureToolAvailableFn("golangci-lint"); err != nil {
		return err
	}

	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}
	xdgCache, err := ensureXDGCacheDir(repoRoot)
	if err != nil {
		return err
	}

	if err := ensureGolangCILintCacheFresh(repoRoot, xdgCache); err != nil {
		return err
	}

	args := []string{"run", "--config", ".golangci.yml"}
	if disableGosec {
		args = append(args, "--disable", "gosec")
	}
	if fix {
		args = append(args, "--fix")
	}

	if jobs := resolveToolJobs(); jobs > 0 {
		args = append(args, "--concurrency", fmt.Sprintf("%d", jobs))
	}

	return runCommandFn(context.Background(), "golangci-lint", args, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE":        goCache,
			"XDG_CACHE_HOME": xdgCache,
		},
	})
}

func ensureGolangCILintCacheFresh(repoRoot, xdgCache string) error {
	cfgPath := filepath.Join(repoRoot, ".golangci.yml")
	cfgBytes, err := os.ReadFile(filepath.Clean(cfgPath))
	if err != nil {
		return fmt.Errorf("read golangci-lint config: %w", err)
	}
	cfgHash := sha256.Sum256(cfgBytes)

	versionOut, err := captureCommandOutputFn(context.Background(), repoRoot, map[string]string{
		"XDG_CACHE_HOME": xdgCache,
	}, "golangci-lint", "version")
	if err != nil {
		return err
	}
	versionLine := strings.TrimSpace(strings.SplitN(versionOut, "\n", 2)[0])

	stampPath := filepath.Join(xdgCache, "golangci-lint.cache.stamp")
	stamp := fmt.Sprintf("%s\nconfig-sha256:%x\n", versionLine, cfgHash[:])
	if existing, err := os.ReadFile(filepath.Clean(stampPath)); err == nil && string(existing) == stamp {
		return nil
	}

	if err := runCommandFn(context.Background(), "golangci-lint", []string{"cache", "clean"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"XDG_CACHE_HOME": xdgCache,
		},
	}); err != nil {
		return err
	}

	if err := os.WriteFile(stampPath, []byte(stamp), 0o600); err != nil {
		return fmt.Errorf("write golangci-lint cache stamp: %w", err)
	}
	return nil
}
