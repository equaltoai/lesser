package main

import (
	"context"
	"flag"
	"os"
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

	args := []string{"run", "--config", ".golangci.yml"}
	if fix {
		args = append(args, "--fix")
	}

	return runCommandFn(context.Background(), "golangci-lint", args, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE":        goCache,
			"XDG_CACHE_HOME": xdgCache,
		},
	})
}
