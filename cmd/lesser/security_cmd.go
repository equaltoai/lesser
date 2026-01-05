package main

import (
	"context"
	"os"
	"path/filepath"
)

func runSecScan(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	if err := ensureToolAvailableFn("gosec"); err != nil {
		return err
	}

	if err := runCommandFn(context.Background(), "gosec", []string{
		"-quiet",
		"-exclude-generated",
		"-exclude-dir=tmp",
		"-exclude-dir=infra",
		"./...",
	}, execOptions{
		Dir: repoRoot,
	}); err != nil {
		return err
	}

	infraCDKDir := filepath.Join(repoRoot, "infra", "cdk")
	if _, err := os.Stat(filepath.Join(infraCDKDir, "go.mod")); err == nil {
		return runCommandFn(context.Background(), "gosec", []string{
			"-quiet",
			"-exclude-generated",
			"-exclude-dir=cdk.out",
			"./...",
		}, execOptions{
			Dir: infraCDKDir,
		})
	}

	return nil
}

func runVulnCheck(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	if err := ensureToolAvailableFn("govulncheck"); err != nil {
		return err
	}
	return runCommandFn(context.Background(), "govulncheck", []string{"./..."}, execOptions{
		Dir: repoRoot,
	})
}
