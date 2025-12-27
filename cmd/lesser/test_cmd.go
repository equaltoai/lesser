package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runTest(argv []string) error {
	if len(argv) > 0 {
		switch argv[0] {
		case helpFlagShort, helpFlagLong, helpCommand:
			printUsage()
			return nil
		}
	}

	if len(argv) == 0 {
		return runTestAll(argv)
	}

	switch argv[0] {
	case valueAll:
		return runTestAll(argv[1:])
	case "unit":
		return runTestUnit(argv[1:])
	case "integration":
		return runTestIntegration(argv[1:])
	case "race":
		return runTestRace(argv[1:])
	case "coverage":
		return runTestCoverage(argv[1:])
	default:
		return fmt.Errorf("unknown test command %q", argv[0])
	}
}

type testArgs struct {
	Environment string
	Stage       string
}

func runTestAll(argv []string) error {
	fs := flag.NewFlagSet("lesser test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args testArgs
	fs.StringVar(&args.Environment, "environment", "test", "value for ENVIRONMENT (default: test)")
	fs.StringVar(&args.Stage, "stage", "test", "value for STAGE (default: test)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	return runGoTests(args, []string{"test", "-v", "./..."}, nil)
}

func runTestUnit(argv []string) error {
	fs := flag.NewFlagSet("lesser test unit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args testArgs
	fs.StringVar(&args.Environment, "environment", "test", "value for ENVIRONMENT (default: test)")
	fs.StringVar(&args.Stage, "stage", "test", "value for STAGE (default: test)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	return runGoTests(args, []string{"test", "-short", "-v", "./..."}, nil)
}

func runTestIntegration(argv []string) error {
	fs := flag.NewFlagSet("lesser test integration", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args testArgs
	fs.StringVar(&args.Environment, "environment", "integration", "value for ENVIRONMENT (default: integration)")
	fs.StringVar(&args.Stage, "stage", "integration", "value for STAGE (default: integration)")
	var timeout string
	fs.StringVar(&timeout, "timeout", "30m", "go test -timeout value (default: 30m)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	return runGoTests(args, []string{"test", "-tags=integration", "-v", "-timeout=" + timeout, "./pkg/testing/harness/..."}, map[string]string{
		"TEST_ENV": "integration",
	})
}

func runTestRace(argv []string) error {
	fs := flag.NewFlagSet("lesser test race", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args testArgs
	fs.StringVar(&args.Environment, "environment", "test", "value for ENVIRONMENT (default: test)")
	fs.StringVar(&args.Stage, "stage", "test", "value for STAGE (default: test)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	return runGoTests(args, []string{"test", "-race", "-v", "./..."}, nil)
}

func runTestCoverage(argv []string) error {
	fs := flag.NewFlagSet("lesser test coverage", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args testArgs
	fs.StringVar(&args.Environment, "environment", "test", "value for ENVIRONMENT (default: test)")
	fs.StringVar(&args.Stage, "stage", "test", "value for STAGE (default: test)")
	var scope string
	fs.StringVar(&scope, "scope", "all", "coverage scope: all|pkg (default: all)")
	var includeTesting bool
	fs.BoolVar(&includeTesting, "include-testing", false, "include pkg/testing/* in pkg scope (default: false)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	var (
		profileName string
		htmlName    string
		pkgPaths    []string
	)
	switch scope {
	case "all":
		profileName = "coverage.out"
		htmlName = "coverage.html"
		pkgPaths = []string{"./..."}
	case "pkg":
		profileName = "coverage_pkg.out"
		htmlName = "coverage_pkg.html"
		if includeTesting {
			pkgPaths = []string{"./pkg/..."}
			break
		}
		pkgPaths, err = listPackagesForPkgCoverage(repoRoot, goCache)
		if err != nil {
			return err
		}
		if len(pkgPaths) == 0 {
			return fmt.Errorf("no packages found for pkg coverage scope (after filtering)")
		}
	default:
		return fmt.Errorf("unknown coverage scope %q (want all|pkg)", scope)
	}

	if err := runGoTests(args, append([]string{"test", "-v", "-coverprofile=" + profileName}, pkgPaths...), nil); err != nil {
		return err
	}

	coveragePath := filepath.Join(repoRoot, profileName)
	return runCommand(context.Background(), "go", []string{"tool", "cover", "-html=" + coveragePath, "-o", htmlName}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runGoTests(args testArgs, goArgs []string, extraEnv map[string]string) error {
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

	env := map[string]string{
		"ENVIRONMENT": args.Environment,
		"STAGE":       args.Stage,
		"JWT_SECRET":  envOrDefault("JWT_SECRET", "dummy_value"),
		"GOCACHE":     goCache,
	}
	if _, ok := env["DYNAMODB_ENCRYPTION_KEY"]; !ok {
		env["DYNAMODB_ENCRYPTION_KEY"] = envOrDefault("DYNAMODB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	}
	for key, value := range extraEnv {
		env[key] = value
	}

	return runCommand(context.Background(), "go", goArgs, execOptions{
		Dir: repoRoot,
		Env: env,
	})
}

func listPackagesForPkgCoverage(repoRoot string, goCache string) ([]string, error) {
	modulePath, err := readModulePath(repoRoot)
	if err != nil {
		return nil, err
	}

	out, err := captureCommandOutput(context.Background(), repoRoot, map[string]string{
		"GOCACHE": goCache,
	}, "go", "list", "./pkg/...")
	if err != nil {
		return nil, err
	}

	testingPrefix := modulePath + "/pkg/testing"

	pkgs := make([]string, 0)
	for _, line := range strings.Split(out, "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}
		if pkg == testingPrefix || strings.HasPrefix(pkg, testingPrefix+"/") {
			continue
		}
		pkgs = append(pkgs, pkg)
	}

	sort.Strings(pkgs)
	return pkgs, nil
}

func readModulePath(repoRoot string) (string, error) {
	modPath := filepath.Join(repoRoot, "go.mod")
	content, err := os.ReadFile(modPath)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modulePath := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if modulePath == "" {
				break
			}
			return modulePath, nil
		}
	}

	return "", fmt.Errorf("unable to determine module path from go.mod")
}
