// Package main provides static audit gates used by `lesser verify audit`.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type baseline struct {
	Version int `yaml:"version"`

	DisabledGoFiles []string `yaml:"disabledGoFiles"`

	GoReadAllRespBody    map[string]int `yaml:"goReadAllRespBody"`
	GoInsecureSkipVerify map[string]int `yaml:"goInsecureSkipVerify"`

	InfraCdkCspUnsafeInline map[string]int `yaml:"infraCdkCspUnsafeInline"`
	InfraCdkCspUnsafeEval   map[string]int `yaml:"infraCdkCspUnsafeEval"`
}

type options struct {
	Check        bool
	BaselinePath string
}

func main() {
	var opts options
	flag.BoolVar(&opts.Check, "check", false, "run audit gates in check mode (exit non-zero on failures)")
	flag.StringVar(&opts.BaselinePath, "baseline", "tools/audit_gates/baseline.yml", "baseline file path (repo-relative)")
	flag.Parse()

	if !opts.Check {
		fmt.Fprintln(os.Stderr, "error: --check is required")
		os.Exit(2)
	}

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(opts options) error {
	b, err := loadBaseline(opts.BaselinePath)
	if err != nil {
		return err
	}

	var problems []string

	if err := checkStaticcheckGoVersion("go.mod", ".golangci.yml"); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkDisabledGoFiles(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGoReadAllRespBody(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGoInsecureSkipVerify(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkInfraCdkCspUnsafe(b); err != nil {
		problems = append(problems, err.Error())
	}

	if len(problems) > 0 {
		return errors.New("audit gates failed:\n- " + strings.Join(problems, "\n- "))
	}

	fmt.Println("✓ audit gates passed")
	return nil
}

func loadBaseline(path string) (baseline, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- baseline is a repo-local file
	if err != nil {
		return baseline{}, fmt.Errorf("failed to read baseline file %q: %w", path, err)
	}

	var b baseline
	if err := yaml.Unmarshal(content, &b); err != nil {
		return baseline{}, fmt.Errorf("failed to parse baseline file %q: %w", path, err)
	}

	if b.Version == 0 {
		return baseline{}, fmt.Errorf("baseline file %q missing version", path)
	}

	return b, nil
}

func checkStaticcheckGoVersion(goModPath string, golangciPath string) error {
	goVersion, err := readGoModVersion(goModPath)
	if err != nil {
		return err
	}

	staticcheckGo, err := readGolangciStaticcheckGoVersion(golangciPath)
	if err != nil {
		return err
	}

	if goVersion != staticcheckGo {
		return fmt.Errorf("staticcheck go version mismatch: go.mod=%q .golangci.yml=%q", goVersion, staticcheckGo)
	}

	return nil
}

func readGoModVersion(path string) (string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- repo-local file path
	if err != nil {
		return "", fmt.Errorf("failed to read %q: %w", path, err)
	}

	re := regexp.MustCompile(`(?m)^go\s+([0-9]+\.[0-9]+)\s*$`)
	match := re.FindSubmatch(content)
	if len(match) != 2 {
		return "", fmt.Errorf("failed to parse go version from %q", path)
	}

	return string(match[1]), nil
}

type golangciConfig struct {
	LintersSettings struct {
		Staticcheck struct {
			Go string `yaml:"go"`
		} `yaml:"staticcheck"`
	} `yaml:"linters-settings"`
}

func readGolangciStaticcheckGoVersion(path string) (string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- repo-local file path
	if err != nil {
		return "", fmt.Errorf("failed to read %q: %w", path, err)
	}

	var cfg golangciConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse %q: %w", path, err)
	}

	version := strings.TrimSpace(cfg.LintersSettings.Staticcheck.Go)
	if version == "" {
		return "", fmt.Errorf("missing linters-settings.staticcheck.go in %q", path)
	}

	return version, nil
}

func checkDisabledGoFiles(b baseline) error {
	want := make(map[string]struct{}, len(b.DisabledGoFiles))
	for _, path := range b.DisabledGoFiles {
		want[normalizePath(path)] = struct{}{}
	}

	actual, err := findFilesWithSuffix(".", ".go.disabled", defaultSkips())
	if err != nil {
		return err
	}

	var unexpected []string
	for _, path := range actual {
		if _, ok := want[normalizePath(path)]; !ok {
			unexpected = append(unexpected, path)
		}
	}

	var missing []string
	actualSet := make(map[string]struct{}, len(actual))
	for _, path := range actual {
		actualSet[normalizePath(path)] = struct{}{}
	}
	for path := range want {
		if _, ok := actualSet[path]; !ok {
			missing = append(missing, path)
		}
	}

	sort.Strings(unexpected)
	sort.Strings(missing)

	if len(unexpected) == 0 && len(missing) == 0 {
		return nil
	}

	var lines []string
	if len(unexpected) > 0 {
		lines = append(lines, fmt.Sprintf("unexpected *.go.disabled files: %s", strings.Join(unexpected, ", ")))
	}
	if len(missing) > 0 {
		lines = append(lines, fmt.Sprintf("baseline lists missing *.go.disabled files: %s", strings.Join(missing, ", ")))
	}
	lines = append(lines, "update tools/audit_gates/baseline.yml when intentionally changing these")
	return errors.New(strings.Join(lines, "; "))
}

func checkGoReadAllRespBody(b baseline) error {
	const needle = "io.ReadAll(resp.Body)"

	actual, err := countSubstringOccurrences([]string{"cmd", "pkg", "graph"}, needle, scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	return compareCounts("io.ReadAll(resp.Body) occurrences", actual, b.GoReadAllRespBody)
}

func checkGoInsecureSkipVerify(b baseline) error {
	const needle = "InsecureSkipVerify: true"

	actual, err := countSubstringOccurrences([]string{"cmd", "pkg", "graph"}, needle, scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	return compareCounts("InsecureSkipVerify: true occurrences", actual, b.GoInsecureSkipVerify)
}

func checkInfraCdkCspUnsafe(b baseline) error {
	inlineActual, err := countSubstringOccurrences([]string{"infra/cdk"}, "'unsafe-inline'", scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	evalActual, err := countSubstringOccurrences([]string{"infra/cdk"}, "'unsafe-eval'", scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	if err := compareCounts("infra/cdk CSP 'unsafe-inline' occurrences", inlineActual, b.InfraCdkCspUnsafeInline); err != nil {
		return err
	}
	if err := compareCounts("infra/cdk CSP 'unsafe-eval' occurrences", evalActual, b.InfraCdkCspUnsafeEval); err != nil {
		return err
	}

	return nil
}

type scanOptions struct {
	IncludeTests bool
	Skips        map[string]struct{}
}

func countSubstringOccurrences(roots []string, needle string, opts scanOptions) (map[string]int, error) {
	counts := make(map[string]int)
	if needle == "" {
		return counts, fmt.Errorf("internal error: empty needle")
	}

	for _, root := range roots {
		if err := walkGoFiles(root, opts, func(path string) error {
			n, err := countNeedleInFile(path, needle)
			if err != nil {
				return err
			}
			if n == 0 {
				return nil
			}
			counts[normalizePath(path)] = n
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return counts, nil
}

func walkGoFiles(root string, opts scanOptions, fn func(path string) error) error {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if shouldSkipDir(path, opts.Skips) {
				return fs.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}
		if !opts.IncludeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		return fn(path)
	})
}

func countNeedleInFile(path string, needle string) (int, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- local source file path (repo scan)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(data), needle), nil
}

func compareCounts(label string, actual map[string]int, expected map[string]int) error {
	expectedNormalized := make(map[string]int, len(expected))
	for path, count := range expected {
		expectedNormalized[normalizePath(path)] = count
	}

	var problems []string

	for path, count := range actual {
		want, ok := expectedNormalized[normalizePath(path)]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: unexpected %q (%d)", label, path, count))
			continue
		}
		if count != want {
			problems = append(problems, fmt.Sprintf("%s: %q expected %d got %d", label, path, want, count))
		}
	}

	for path, want := range expectedNormalized {
		if _, ok := actual[path]; !ok {
			problems = append(problems, fmt.Sprintf("%s: baseline expects %q (%d) but found 0", label, path, want))
		}
		if _, err := os.Stat(filepath.FromSlash(path)); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				problems = append(problems, fmt.Sprintf("%s: baseline references missing file %q", label, path))
				continue
			}
			problems = append(problems, fmt.Sprintf("%s: failed to stat %q: %v", label, path, err))
		}
	}

	sort.Strings(problems)

	if len(problems) == 0 {
		return nil
	}

	problems = append(problems, "update tools/audit_gates/baseline.yml when intentionally changing these")
	return errors.New(strings.Join(problems, "; "))
}

func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func defaultSkips() map[string]struct{} {
	return map[string]struct{}{
		".git":         {},
		"bin":          {},
		"build":        {},
		"cdk.out":      {},
		"dist":         {},
		"node_modules": {},
		"report":       {},
		"tmp":          {},
		"vendor":       {},
	}
}

func shouldSkipDir(path string, skips map[string]struct{}) bool {
	base := filepath.Base(path)
	if base == "" || base == "." {
		return false
	}
	if _, ok := skips[base]; ok {
		return true
	}
	return false
}

func findFilesWithSuffix(root string, suffix string, skips map[string]struct{}) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(path, skips) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, suffix) {
			matches = append(matches, normalizePath(strings.TrimPrefix(path, "./")))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}
