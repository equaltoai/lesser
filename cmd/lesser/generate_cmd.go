package main

import (
	"context"
	"fmt"
	"path/filepath"
)

func runGenerate(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("missing generate command (expected: openapi|graphql-coverage|inventory|schema)")
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "openapi":
		return runGenerateOpenAPI(argv[1:])
	case "graphql-coverage":
		return runGenerateGraphQLCoverage(argv[1:])
	case "inventory":
		return runGenerateInventory(argv[1:])
	case valueSchema:
		return runSchema(argv[1:])
	default:
		return fmt.Errorf("unknown generate command %q", argv[0])
	}
}

func runGenerateOpenAPI(_ []string) error {
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
	return runCommandFn(context.Background(), "go", []string{"run", "./tools/openapi", "--write"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runGenerateGraphQLCoverage(_ []string) error {
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
	return runCommandFn(context.Background(), "go", []string{"run", "./tools/graphql_coverage", "--write"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runGenerateInventory(_ []string) error {
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

	return runCommandFn(context.Background(), "go", []string{"run", "./cmd/generate-inventory"}, execOptions{
		Dir: filepath.Join(repoRoot, "infra", "cdk"),
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}
