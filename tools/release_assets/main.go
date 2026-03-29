package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

func main() {
	var (
		repoRoot string
		outDir   string
	)

	flag.StringVar(&repoRoot, "repo-root", ".", "repository root containing bin/ and infra/cdk/inventory/lambdas.go")
	flag.StringVar(&outDir, "out-dir", filepath.Join("dist", "release"), "directory where release assets are written")
	flag.Parse()

	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	files, err := releaseassets.WriteLambdaBundle(absRepoRoot, outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s with %d Lambda artifacts\n", filepath.Join(outDir, releaseassets.LambdaBundleArchiveName), len(files))
}
