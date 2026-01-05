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

	src := filepath.Join(repoRoot, "docs", "contracts", "graphql-schema.graphql")
	dst := filepath.Join(repoRoot, "schema.graphql")

	data, err := os.ReadFile(src) // #nosec G304 -- file path is derived from repo root
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	// #nosec G306 -- schema contract output is not sensitive
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write schema.graphql: %w", err)
	}

	fmt.Println("✓ Schema exported to schema.graphql (source: docs/contracts/graphql-schema.graphql)")
	return nil
}
