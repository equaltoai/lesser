package main

import (
	"context"
	"os"
	"path/filepath"
)

func runSecScan(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := ensureToolAvailable("gosec"); err != nil {
		return err
	}

	if err := runCommand(context.Background(), "gosec", []string{
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
		return runCommand(context.Background(), "gosec", []string{
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
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := ensureToolAvailable("govulncheck"); err != nil {
		return err
	}
	return runCommand(context.Background(), "govulncheck", []string{"./..."}, execOptions{
		Dir: repoRoot,
	})
}
