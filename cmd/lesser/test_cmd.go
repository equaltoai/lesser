package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
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
	var excludeGenerated bool
	fs.BoolVar(&excludeGenerated, "exclude-generated", true, "exclude generated files (\"Code generated... DO NOT EDIT\") from coverage profiles (default: true)")
	var includeTesting bool
	fs.BoolVar(&includeTesting, "include-testing", false, "include pkg/testing/* in pkg scope (default: false)")
	var includeTools bool
	fs.BoolVar(&includeTools, "include-tools", false, "include tools/* packages in all scope (default: false)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	repoRoot, err := findRepoRootFn()
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
		pkgPaths, err = listPackagesForAllCoverage(repoRoot, goCache, includeTools)
		if err != nil {
			return err
		}
		if len(pkgPaths) == 0 {
			return fmt.Errorf("no packages found for all coverage scope (after filtering)")
		}
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
	if excludeGenerated {
		if err := filterGeneratedFilesFromCoverProfile(repoRoot, coveragePath); err != nil {
			return err
		}
	}
	return runCommandFn(context.Background(), "go", []string{"tool", "cover", "-html=" + coveragePath, "-o", htmlName}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func filterGeneratedFilesFromCoverProfile(repoRoot string, coverProfilePath string) error {
	modulePath, err := readModulePath(repoRoot)
	if err != nil {
		return err
	}
	modulePrefix := modulePath + "/"

	// #nosec G304 -- reads a developer-controlled coverage profile path.
	in, err := os.Open(coverProfilePath)
	if err != nil {
		return fmt.Errorf("open coverprofile: %w", err)
	}
	defer func() {
		if err := in.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "warning: close coverprofile:", err)
		}
	}()

	tmp, err := os.CreateTemp(filepath.Dir(coverProfilePath), filepath.Base(coverProfilePath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp coverprofile: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	w := bufio.NewWriter(tmp)
	if err := filterCoverageData(in, w, repoRoot, modulePrefix); err != nil {
		_ = tmp.Close()
		return err
	}

	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("flush coverprofile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp coverprofile: %w", err)
	}

	if err := os.Rename(tmpName, coverProfilePath); err != nil {
		return fmt.Errorf("replace coverprofile: %w", err)
	}

	return nil
}

func filterCoverageData(r io.Reader, w *bufio.Writer, repoRoot, modulePrefix string) error {
	isGeneratedCache := make(map[string]bool)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			if _, err := w.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("write coverprofile: %w", err)
			}
			continue
		}

		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			if _, err := w.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("write coverprofile: %w", err)
			}
			continue
		}
		filePath := line[:colon]

		isGen, ok := isGeneratedCache[filePath]
		if !ok {
			isGen = isGeneratedFile(repoRoot, modulePrefix, filePath)
			isGeneratedCache[filePath] = isGen
		}

		if isGen {
			continue
		}

		if _, err := w.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("write coverprofile: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read coverprofile: %w", err)
	}
	return nil
}

func isGeneratedFile(repoRoot, modulePrefix, filePath string) bool {
	localPath := filePath
	if modulePrefix != "" && strings.HasPrefix(filePath, modulePrefix) {
		localPath = strings.TrimPrefix(filePath, modulePrefix)
	}
	localPath = filepath.Clean(localPath)
	if !filepath.IsAbs(localPath) {
		localPath = filepath.Join(repoRoot, localPath)
	}

	// #nosec G304 -- reads module-owned files from disk.
	f, err := os.Open(localPath)
	if err != nil {
		return false
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "warning: close source file:", err)
		}
	}()

	buf := make([]byte, 2048)
	n, readErr := f.Read(buf)
	if readErr != nil && readErr != io.EOF {
		return false
	}

	header := string(buf[:n])
	return strings.Contains(header, "Code generated") || strings.Contains(header, "DO NOT EDIT")
}

func runGoTests(args testArgs, goArgs []string, extraEnv map[string]string) error {
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

	return runCommandFn(context.Background(), "go", goArgs, execOptions{
		Dir: repoRoot,
		Env: env,
	})
}

func listPackagesForPkgCoverage(repoRoot string, goCache string) ([]string, error) {
	modulePath, err := readModulePath(repoRoot)
	if err != nil {
		return nil, err
	}

	out, err := captureCommandOutputFn(context.Background(), repoRoot, map[string]string{
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

func listPackagesForAllCoverage(repoRoot string, goCache string, includeTools bool) ([]string, error) {
	modulePath, err := readModulePath(repoRoot)
	if err != nil {
		return nil, err
	}

	out, err := captureCommandOutputFn(context.Background(), repoRoot, map[string]string{
		"GOCACHE": goCache,
	}, "go", "list", "./...")
	if err != nil {
		return nil, err
	}

	toolsPrefix := modulePath + "/tools"

	pkgs := make([]string, 0)
	for _, line := range strings.Split(out, "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}

		if !includeTools {
			if pkg == toolsPrefix || strings.HasPrefix(pkg, toolsPrefix+"/") {
				continue
			}
		}

		pkgs = append(pkgs, pkg)
	}

	sort.Strings(pkgs)
	return pkgs, nil
}

func readModulePath(repoRoot string) (string, error) {
	modPath := filepath.Join(repoRoot, "go.mod")
	content, err := os.ReadFile(filepath.Clean(modPath))
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
