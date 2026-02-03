package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	buildLambdaZipsFn          = buildLambdaZips
	buildCloudfrontKeygenZipFn = buildCloudfrontKeygenZip
	buildAuthUIFn              = buildAuthUI
)

func runBuild(argv []string) error {
	if len(argv) > 0 {
		switch argv[0] {
		case helpFlagShort, helpFlagLong, helpCommand:
			printUsage()
			return nil
		}
	}

	if len(argv) == 0 || argv[0] == valueAll {
		return runBuildAll(argv)
	}

	switch argv[0] {
	case "lambdas":
		return runBuildLambdas(argv[1:])
	case "lambda":
		return runBuildSingleLambda(argv[1:])
	default:
		return fmt.Errorf("unknown build command %q", argv[0])
	}
}

func runBuildAll(argv []string) error {
	fs := flag.NewFlagSet("lesser build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var rebuildLambdas bool
	fs.BoolVar(&rebuildLambdas, "rebuild-lambdas", true, "force rebuild Lambda zip artifacts")

	if len(argv) > 0 && argv[0] == valueAll {
		argv = argv[1:]
	}
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
	if err := ensureToolAvailableFn("pnpm"); err != nil {
		return err
	}

	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	fmt.Println("Cleaning build artifacts...")
	if err := os.RemoveAll(filepath.Join(repoRoot, "bin")); err != nil {
		return fmt.Errorf("clean bin: %w", err)
	}
	_ = os.Remove(filepath.Join(repoRoot, "coverage.out"))
	_ = os.Remove(filepath.Join(repoRoot, "coverage.html"))
	fmt.Println("✓ Clean complete")

	if err := buildLambdaZipsFn(repoRoot, rebuildLambdas); err != nil {
		return err
	}

	if err := buildCloudfrontKeygenZipFn(repoRoot, goCache); err != nil {
		return err
	}

	if _, err := buildAuthUIFn(repoRoot); err != nil {
		return err
	}

	fmt.Println("\nBuilding Go binaries...")
	if err := runCommandFn(context.Background(), "go", []string{"build", "./..."}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	}); err != nil {
		return err
	}

	fmt.Println("✓ Build complete (Lambda zips, cloudfront-keygen.zip, auth-ui, and Go binaries refreshed)")
	return nil
}

func runBuildLambdas(argv []string) error {
	fs := flag.NewFlagSet("lesser build lambdas", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var rebuild bool
	fs.BoolVar(&rebuild, "rebuild", false, "force rebuild all lambda zip artifacts")

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

	return buildLambdaZipsFn(repoRoot, rebuild)
}

func runBuildSingleLambda(argv []string) error {
	fs := flag.NewFlagSet("lesser build lambda", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var rebuild bool
	fs.BoolVar(&rebuild, "rebuild", true, "force rebuild the target lambda zip (defaults to true)")

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: lesser build lambda <name>")
	}

	lambdaName := strings.TrimSpace(fs.Arg(0))
	if lambdaName == "" {
		return fmt.Errorf("lambda name is required")
	}

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

	bootstrapPath := filepath.Join(repoRoot, "bin", "bootstrap")
	zipPath := filepath.Join(repoRoot, "bin", fmt.Sprintf("%s.zip", lambdaName))

	if !rebuild && fileExists(zipPath) {
		fmt.Println("✓ Lambda artifact already present:", zipPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(zipPath), 0o750); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	if err := buildLambdaBinaryFn(repoRoot, goCache, lambdaName, bootstrapPath); err != nil {
		return err
	}
	if err := zipSingleFileFn(zipPath, "bootstrap", bootstrapPath); err != nil {
		return err
	}
	_ = os.Remove(bootstrapPath)

	fmt.Println("✓ Built", zipPath)
	return nil
}

func buildCloudfrontKeygenZip(repoRoot string, cacheDir string) error {
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(binDir, "cloudfront-keygen.")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	bootstrapPath := filepath.Join(tmpDir, "bootstrap")
	args := []string{"build", "-tags", "lambda.norpc", "-ldflags=-s -w", "-o", bootstrapPath, "./cmd/cloudfront-keygen"}

	if err := runCommandFn(context.Background(), "go", args, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOOS":        "linux",
			"GOARCH":      "arm64",
			"CGO_ENABLED": "0",
			"GOCACHE":     cacheDir,
		},
	}); err != nil {
		return err
	}

	zipPath := filepath.Join(binDir, "cloudfront-keygen.zip")
	if err := zipSingleFileFn(zipPath, "bootstrap", bootstrapPath); err != nil {
		return err
	}

	fmt.Println("✓ Built", strings.TrimPrefix(zipPath, repoRoot+string(filepath.Separator)))
	return nil
}
