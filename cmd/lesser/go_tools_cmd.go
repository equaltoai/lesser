package main

import (
	"context"
)

func runGqlgen(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := ensureToolAvailable("go"); err != nil {
		return err
	}
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	return runCommand(context.Background(), "go", []string{"run", "github.com/99designs/gqlgen@v0.17.78", "generate"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runTidy(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := ensureToolAvailable("go"); err != nil {
		return err
	}
	return runCommand(context.Background(), "go", []string{"mod", "tidy"}, execOptions{
		Dir: repoRoot,
	})
}
