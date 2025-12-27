// coverage_scoreboard prints a package- or file-level coverage summary from a Go coverprofile.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type packageCoverage struct {
	Package         string
	TotalStatements int
	Covered         int
}

func (p packageCoverage) Uncovered() int {
	return p.TotalStatements - p.Covered
}

func (p packageCoverage) Percent() float64 {
	if p.TotalStatements == 0 {
		return 0
	}
	return float64(p.Covered) / float64(p.TotalStatements) * 100
}

type fileCoverage struct {
	File           string
	Package        string
	TotalStatements int
	Covered         int
}

func (f fileCoverage) Uncovered() int {
	return f.TotalStatements - f.Covered
}

func (f fileCoverage) Percent() float64 {
	if f.TotalStatements == 0 {
		return 0
	}
	return float64(f.Covered) / float64(f.TotalStatements) * 100
}

type scoreboardConfig struct {
	ProfilePath      string
	Prefix           string
	Top              int
	MinStatements    int
	ZeroOnly         bool
	SortByUncovered  bool
	Mode             string
	PackageFilter    string
	StripModulePath  string
}

func parseFlags() scoreboardConfig {
	var cfg scoreboardConfig
	flag.StringVar(&cfg.ProfilePath, "profile", "coverage_pkg.out", "path to coverage profile (coverprofile)")
	flag.StringVar(&cfg.Prefix, "prefix", "github.com/equaltoai/lesser/pkg/", "only include files with this import-path prefix")
	flag.IntVar(&cfg.Top, "top", 30, "number of packages to print (after filtering)")
	flag.IntVar(&cfg.MinStatements, "min", 0, "minimum statements per package to include")
	flag.BoolVar(&cfg.ZeroOnly, "zero-only", false, "only show 0% coverage packages (after filtering)")
	flag.BoolVar(&cfg.SortByUncovered, "sort-uncovered", true, "sort by uncovered statements (desc) instead of total statements")
	flag.StringVar(&cfg.Mode, "mode", "package", "summary mode: package|file")
	flag.StringVar(&cfg.PackageFilter, "package", "", "only include entries whose package has this prefix (optional)")
	flag.StringVar(&cfg.StripModulePath, "strip", "github.com/equaltoai/lesser/", "strip this import-path prefix when printing file paths (file mode only)")
	flag.Parse()
	return cfg
}

func main() {
	cfg := parseFlags()

	switch cfg.Mode {
	case "package":
		byPkg, err := readPackageCoverage(cfg.ProfilePath, cfg.Prefix, cfg.PackageFilter)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

		pkgs, totalCovered, totalStatements := summarizePackages(byPkg, cfg.MinStatements, cfg.ZeroOnly)
		sortPackages(pkgs, cfg.SortByUncovered)

		if cfg.Top > 0 && len(pkgs) > cfg.Top {
			pkgs = pkgs[:cfg.Top]
		}

		totalPct := 0.0
		if totalStatements > 0 {
			totalPct = float64(totalCovered) / float64(totalStatements) * 100
		}

		fmt.Printf("profile: %s\n", cfg.ProfilePath)
		if cfg.Prefix != "" {
			fmt.Printf("prefix:  %s\n", cfg.Prefix)
		}
		if cfg.PackageFilter != "" {
			fmt.Printf("package: %s\n", cfg.PackageFilter)
		}
		fmt.Printf("total:   %.1f%% (%d/%d statements)\n\n", totalPct, totalCovered, totalStatements)
		fmt.Printf("%6s  %14s  %s\n", "%cov", "covered/total", "package")
		for _, p := range pkgs {
			fmt.Printf("%6.1f  %6d/%-6d  %s\n", p.Percent(), p.Covered, p.TotalStatements, p.Package)
		}
	case "file":
		byFile, err := readFileCoverage(cfg.ProfilePath, cfg.Prefix, cfg.PackageFilter)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

		files, totalCovered, totalStatements := summarizeFiles(byFile, cfg.MinStatements, cfg.ZeroOnly)
		sortFiles(files, cfg.SortByUncovered)

		if cfg.Top > 0 && len(files) > cfg.Top {
			files = files[:cfg.Top]
		}

		totalPct := 0.0
		if totalStatements > 0 {
			totalPct = float64(totalCovered) / float64(totalStatements) * 100
		}

		fmt.Printf("profile: %s\n", cfg.ProfilePath)
		if cfg.Prefix != "" {
			fmt.Printf("prefix:  %s\n", cfg.Prefix)
		}
		if cfg.PackageFilter != "" {
			fmt.Printf("package: %s\n", cfg.PackageFilter)
		}
		fmt.Printf("total:   %.1f%% (%d/%d statements)\n\n", totalPct, totalCovered, totalStatements)
		fmt.Printf("%6s  %14s  %s\n", "%cov", "covered/total", "file")
		for _, f := range files {
			path := f.File
			if cfg.StripModulePath != "" {
				path = strings.TrimPrefix(path, cfg.StripModulePath)
			}
			fmt.Printf("%6.1f  %6d/%-6d  %s\n", f.Percent(), f.Covered, f.TotalStatements, path)
		}
	default:
		fmt.Fprintln(os.Stderr, "error: unknown -mode (want package|file):", cfg.Mode)
		os.Exit(2)
	}
}

func readPackageCoverage(profilePath string, prefix string, packageFilter string) (map[string]*packageCoverage, error) {
	// #nosec G304 -- local developer tool reads a developer-supplied local path
	f, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("open profile: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "warning: close profile:", err)
		}
	}()

	byPkg := make(map[string]*packageCoverage)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		filepath := line[:colon]
		if prefix != "" && !strings.HasPrefix(filepath, prefix) {
			continue
		}

		lastSlash := strings.LastIndexByte(filepath, '/')
		if lastSlash < 0 {
			continue
		}
		pkg := filepath[:lastSlash]
		if packageFilter != "" && !strings.HasPrefix(pkg, packageFilter) {
			continue
		}

		numStatements, count, ok := parseProfileCounts(line[colon+1:])
		if !ok {
			continue
		}

		entry := byPkg[pkg]
		if entry == nil {
			entry = &packageCoverage{Package: pkg}
			byPkg[pkg] = entry
		}

		entry.TotalStatements += numStatements
		if count > 0 {
			entry.Covered += numStatements
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	return byPkg, nil
}

func readFileCoverage(profilePath string, prefix string, packageFilter string) (map[string]*fileCoverage, error) {
	// #nosec G304 -- local developer tool reads a developer-supplied local path
	f, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("open profile: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "warning: close profile:", err)
		}
	}()

	byFile := make(map[string]*fileCoverage)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		filepath := line[:colon]
		if prefix != "" && !strings.HasPrefix(filepath, prefix) {
			continue
		}

		pkg := ""
		lastSlash := strings.LastIndexByte(filepath, '/')
		if lastSlash >= 0 {
			pkg = filepath[:lastSlash]
		}
		if packageFilter != "" && pkg != "" && !strings.HasPrefix(pkg, packageFilter) {
			continue
		}

		numStatements, count, ok := parseProfileCounts(line[colon+1:])
		if !ok {
			continue
		}

		entry := byFile[filepath]
		if entry == nil {
			entry = &fileCoverage{File: filepath, Package: pkg}
			byFile[filepath] = entry
		}

		entry.TotalStatements += numStatements
		if count > 0 {
			entry.Covered += numStatements
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	return byFile, nil
}

func parseProfileCounts(rest string) (numStatements int, count int, ok bool) {
	parts := strings.Fields(rest)
	if len(parts) < 3 {
		return 0, 0, false
	}

	ns, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, 0, false
	}
	c, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, 0, false
	}

	return ns, c, true
}

func summarizePackages(byPkg map[string]*packageCoverage, minStatements int, zeroOnly bool) (pkgs []packageCoverage, totalCovered int, totalStatements int) {
	pkgs = make([]packageCoverage, 0, len(byPkg))
	for _, v := range byPkg {
		if v.TotalStatements < minStatements {
			continue
		}
		if zeroOnly && v.Percent() != 0 {
			continue
		}
		pkgs = append(pkgs, *v)
		totalStatements += v.TotalStatements
		totalCovered += v.Covered
	}
	return pkgs, totalCovered, totalStatements
}

func summarizeFiles(byFile map[string]*fileCoverage, minStatements int, zeroOnly bool) (files []fileCoverage, totalCovered int, totalStatements int) {
	files = make([]fileCoverage, 0, len(byFile))
	for _, v := range byFile {
		if v.TotalStatements < minStatements {
			continue
		}
		if zeroOnly && v.Percent() != 0 {
			continue
		}
		files = append(files, *v)
		totalStatements += v.TotalStatements
		totalCovered += v.Covered
	}
	return files, totalCovered, totalStatements
}

func sortPackages(pkgs []packageCoverage, sortByUncovered bool) {
	sort.Slice(pkgs, func(i, j int) bool {
		if sortByUncovered {
			if pkgs[i].Uncovered() == pkgs[j].Uncovered() {
				return pkgs[i].TotalStatements > pkgs[j].TotalStatements
			}
			return pkgs[i].Uncovered() > pkgs[j].Uncovered()
		}
		if pkgs[i].TotalStatements == pkgs[j].TotalStatements {
			return pkgs[i].Percent() < pkgs[j].Percent()
		}
		return pkgs[i].TotalStatements > pkgs[j].TotalStatements
	})
}

func sortFiles(files []fileCoverage, sortByUncovered bool) {
	sort.Slice(files, func(i, j int) bool {
		if sortByUncovered {
			if files[i].Uncovered() == files[j].Uncovered() {
				return files[i].TotalStatements > files[j].TotalStatements
			}
			return files[i].Uncovered() > files[j].Uncovered()
		}
		if files[i].TotalStatements == files[j].TotalStatements {
			return files[i].Percent() < files[j].Percent()
		}
		return files[i].TotalStatements > files[j].TotalStatements
	})
}
