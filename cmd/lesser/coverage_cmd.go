package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func runCoverage(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("missing coverage command (try `lesser coverage scoreboard --help`)")
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "scoreboard":
		return runCoverageScoreboard(argv[1:])
	default:
		return fmt.Errorf("unknown coverage command %q", argv[0])
	}
}

type coverageScoreboardArgs struct {
	Profile          string
	Mode             string
	Package          string
	Top              int
	Min              int
	MinTotal         float64
	Zero             bool
	SortUnc          bool
	ExcludeGenerated bool
}

func runCoverageScoreboard(argv []string) error {
	fs := flag.NewFlagSet("lesser coverage scoreboard", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args coverageScoreboardArgs
	fs.StringVar(&args.Profile, "profile", "", "coverage profile path (coverprofile) (default: auto)")
	fs.StringVar(&args.Mode, "mode", "package", "summary mode: package|file")
	fs.StringVar(&args.Package, "package", "", "package prefix filter (optional)")
	fs.IntVar(&args.Top, "top", 30, "number of entries to print (after filtering)")
	fs.IntVar(&args.Min, "min", 0, "minimum statements per entry to include")
	fs.Float64Var(&args.MinTotal, "min-total", 0, "fail if total coverage is below this percentage (optional)")
	fs.BoolVar(&args.Zero, "zero-only", false, "only show 0% coverage entries (after filtering)")
	fs.BoolVar(&args.SortUnc, "sort-uncovered", true, "sort by uncovered statements (desc) instead of total statements")
	fs.BoolVar(&args.ExcludeGenerated, "exclude-generated", true, "exclude generated files (\"Code generated... DO NOT EDIT\") from metrics")
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
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	toolArgs := []string{
		"run", "./tools/coverage_scoreboard",
		"--mode", args.Mode,
		"--top", fmt.Sprintf("%d", args.Top),
		"--min", fmt.Sprintf("%d", args.Min),
		"--sort-uncovered=" + boolToFlag(args.SortUnc),
		"--exclude-generated=" + boolToFlag(args.ExcludeGenerated),
	}
	if args.MinTotal > 0 {
		toolArgs = append(toolArgs, "--min-total", fmt.Sprintf("%g", args.MinTotal))
	}
	if args.Profile != "" {
		toolArgs = append(toolArgs, "--profile", args.Profile)
	}
	if args.Zero {
		toolArgs = append(toolArgs, "--zero-only")
	}
	if args.Package != "" {
		toolArgs = append(toolArgs, "--package", args.Package)
	}

	return runCommandFn(context.Background(), "go", toolArgs, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func boolToFlag(value bool) string {
	if value {
		return flagTrue
	}
	return flagFalse
}
