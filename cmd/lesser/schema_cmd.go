package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runSchema(argv []string) error {
	fs := flag.NewFlagSet("lesser schema", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var outPath string
	fs.StringVar(&outPath, "out", "", "write schema to this path (defaults to docs/contracts/graphql-schema.graphql)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}

	args := []string{"./scripts/generate_schema.sh"}
	if outPath != "" {
		args = append(args, "--out", outPath)
	}

	return runCommandFn(context.Background(), "bash", args, execOptions{
		Dir: repoRoot,
	})
}

func runExportSchema(argv []string) error {
	if len(argv) > 0 {
		switch argv[0] {
		case helpFlagShort, helpFlagLong, helpCommand:
			printUsage()
			return nil
		}
	}

	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}

	if err := runSchema(nil); err != nil {
		return err
	}

	repoFS, err := os.OpenRoot(repoRoot)
	if err != nil {
		return fmt.Errorf("open repository root: %w", err)
	}
	defer func() { _ = repoFS.Close() }()

	data, err := repoFS.ReadFile(filepath.Join("docs", "contracts", "graphql-schema.graphql"))
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if err := repoFS.WriteFile("schema.graphql", data, 0o644); err != nil { //nolint:gosec // G306: exported schema is a public contract artifact and must remain readable by repository tooling
		return fmt.Errorf("write schema.graphql: %w", err)
	}

	fmt.Println("✓ Schema exported to schema.graphql (source: docs/contracts/graphql-schema.graphql)")
	return nil
}
