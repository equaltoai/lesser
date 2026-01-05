// Command ai_training_verify validates docs/ai-training for completeness and drift.
package main

import (
	"bufio"
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

func main() {
	flag.Parse()

	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if err := run(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string) error {
	repoFS := os.DirFS(root)
	aiFS, err := fs.Sub(repoFS, filepath.ToSlash(filepath.Join("docs", "ai-training")))
	if err != nil {
		return fmt.Errorf("failed to open docs/ai-training: %w", err)
	}

	required := []string{
		"README.md",
		"PAY_THEORY_DOCUMENTATION_GUIDE.md",
		"api-patterns.yaml",
		"architecture-patterns.yaml",
		"cost-optimization-patterns.yaml",
		"deployment-patterns.yaml",
		"enhancement-patterns.yaml",
		"federation-patterns.yaml",
		"monitoring-patterns.yaml",
		"scaling-patterns.yaml",
		"security-patterns.yaml",
		"troubleshooting-patterns.yaml",
	}

	fmt.Println("=== AI Training Docs Verification ===")
	fmt.Println()

	status := 0

	fmt.Println("1) Required files")
	for _, name := range required {
		if _, err := fs.Stat(aiFS, name); err != nil {
			fmt.Printf("✗ Missing: %s\n", filepath.ToSlash(filepath.Join("docs/ai-training", name)))
			status = 1
		}
	}
	if status == 0 {
		fmt.Println("✓ Required files present")
	}
	fmt.Println()

	fmt.Println("2) README lists all training docs")
	readmeListed, err := readReadmeFileList(aiFS, "README.md")
	if err != nil {
		fmt.Println("✗ Failed to parse README file list:", err)
		status = 1
	} else {
		actual, err := listTrainingFiles(aiFS)
		if err != nil {
			fmt.Println("✗ Failed to list docs/ai-training contents:", err)
			status = 1
		} else {
			missing, extra := diffSets(readmeListed, actual)
			if len(missing) > 0 || len(extra) > 0 {
				fmt.Println("✗ README file list is out of sync with docs/ai-training contents")
				if len(missing) > 0 {
					fmt.Println("  Missing from README:")
					for _, item := range missing {
						fmt.Println("   -", item)
					}
				}
				if len(extra) > 0 {
					fmt.Println("  Listed but not present:")
					for _, item := range extra {
						fmt.Println("   -", item)
					}
				}
				fmt.Println("  -> Update docs/ai-training/README.md so its `Files` section matches the directory.")
				status = 1
			} else {
				fmt.Println("✓ README file list matches directory contents")
			}
		}
	}
	fmt.Println()

	fmt.Println("3) YAML parse + minimum schema")
	if err := verifyYAMLFiles(aiFS); err != nil {
		fmt.Println("✗", err)
		status = 1
	} else {
		fmt.Println("✓ All YAML files parse and include context.description")
	}
	fmt.Println()

	fmt.Println("4) Drift checks (Go + Lambdas)")
	if err := verifyGoVersion(repoFS, filepath.ToSlash(filepath.Join("docs", "ai-training", "deployment-patterns.yaml"))); err != nil {
		fmt.Println("✗", err)
		status = 1
	} else {
		fmt.Println("✓ Go version in deployment-patterns.yaml matches go.mod")
	}

	if err := verifyLambdaCount(repoFS, filepath.ToSlash(filepath.Join("docs", "ai-training", "architecture-patterns.yaml"))); err != nil {
		fmt.Println("✗", err)
		status = 1
	} else {
		fmt.Println("✓ Lambda count in architecture-patterns.yaml matches Makefile")
	}
	fmt.Println()

	if status == 0 {
		fmt.Println("✅ AI training docs check passed")
		return nil
	}
	fmt.Println("❌ AI training docs issues detected")
	return errors.New("ai-training verification failed")
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found; run from within the repository")
		}
		dir = parent
	}
}

func readReadmeFileList(fsys fs.FS, path string) (map[string]struct{}, error) {
	contents, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`^-\s+` + "`" + `([^` + "`" + `]+)` + "`" + `\s+-\s+.+$`)
	listed := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := re.FindStringSubmatch(line)
		if len(m) == 2 {
			listed[m[1]] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return listed, nil
}

func listTrainingFiles(fsys fs.FS) (map[string]struct{}, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	out := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "README.md" {
			continue
		}
		out[name] = struct{}{}
	}
	return out, nil
}

func diffSets(listed map[string]struct{}, actual map[string]struct{}) (missingFromReadme []string, extraInReadme []string) {
	for item := range actual {
		if _, ok := listed[item]; !ok {
			missingFromReadme = append(missingFromReadme, item)
		}
	}
	for item := range listed {
		if _, ok := actual[item]; !ok {
			extraInReadme = append(extraInReadme, item)
		}
	}
	sort.Strings(missingFromReadme)
	sort.Strings(extraInReadme)
	return missingFromReadme, extraInReadme
}

func verifyYAMLFiles(fsys fs.FS) error {
	matches, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return err
	}
	sort.Strings(matches)

	for _, path := range matches {
		contents, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		var doc struct {
			Context struct {
				Description string `yaml:"description"`
			} `yaml:"context"`
		}
		if err := yaml.Unmarshal(contents, &doc); err != nil {
			return fmt.Errorf("YAML parse error: %s: %w", filepath.ToSlash(filepath.Join("docs", "ai-training", path)), err)
		}
		if strings.TrimSpace(doc.Context.Description) == "" {
			return fmt.Errorf("missing context.description: %s", filepath.ToSlash(filepath.Join("docs", "ai-training", path)))
		}
	}
	return nil
}

func verifyGoVersion(repoFS fs.FS, deploymentYAMLPath string) error {
	goVersion, err := readGoVersion(repoFS, "go.mod")
	if err != nil {
		return err
	}

	contents, err := fs.ReadFile(repoFS, deploymentYAMLPath)
	if err != nil {
		return err
	}

	var doc struct {
		Context struct {
			Prerequisites []map[string]any `yaml:"prerequisites"`
		} `yaml:"context"`
	}
	if err := yaml.Unmarshal(contents, &doc); err != nil {
		return fmt.Errorf("failed to parse deployment patterns: %w", err)
	}

	declared := ""
	for _, item := range doc.Context.Prerequisites {
		if value, ok := item["go_version"]; ok {
			declared = strings.TrimSpace(fmt.Sprintf("%v", value))
			break
		}
	}
	if declared == "" {
		return errors.New("deployment-patterns.yaml missing context.prerequisites.go_version")
	}
	if !strings.Contains(declared, goVersion) {
		return fmt.Errorf("deployment-patterns.yaml go_version (%s) does not match go.mod (%s)", declared, goVersion)
	}
	return nil
}

func readGoVersion(repoFS fs.FS, goModPath string) (string, error) {
	contents, err := fs.ReadFile(repoFS, goModPath)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?m)^go\s+([0-9]+\.[0-9]+)\s*$`)
	m := re.FindStringSubmatch(string(contents))
	if len(m) != 2 {
		return "", errors.New("unable to determine Go version from go.mod")
	}
	return strings.TrimSpace(m[1]), nil
}

func verifyLambdaCount(repoFS fs.FS, architectureYAMLPath string) error {
	lambdaCount, err := countMakefileLambdas(repoFS, "Makefile")
	if err != nil {
		return err
	}

	contents, err := fs.ReadFile(repoFS, architectureYAMLPath)
	if err != nil {
		return err
	}

	var doc struct {
		SystemComponents struct {
			LambdaFunctions struct {
				TotalCount *int `yaml:"total_count"`
			} `yaml:"lambda_functions"`
		} `yaml:"system_components"`
	}
	if err := yaml.Unmarshal(contents, &doc); err != nil {
		return fmt.Errorf("failed to parse architecture patterns: %w", err)
	}

	if doc.SystemComponents.LambdaFunctions.TotalCount == nil {
		return errors.New("architecture-patterns.yaml missing system_components.lambda_functions.total_count")
	}
	if *doc.SystemComponents.LambdaFunctions.TotalCount != lambdaCount {
		return fmt.Errorf("architecture-patterns.yaml total_count=%d does not match Makefile LAMBDAS (%d)", *doc.SystemComponents.LambdaFunctions.TotalCount, lambdaCount)
	}
	return nil
}

func countMakefileLambdas(repoFS fs.FS, makefilePath string) (int, error) {
	contents, err := fs.ReadFile(repoFS, makefilePath)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(contents), "\n")

	inBlock := false
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "LAMBDAS :=") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}

		if strings.TrimSpace(line) == "" {
			break
		}

		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimSuffix(trimmed, "\\")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		count++
	}
	if !inBlock {
		return 0, errors.New("unable to find LAMBDAS block in Makefile")
	}
	return count, nil
}
