package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultSecScanBatchSize     = 5
	defaultVulnCheckBatchSize   = 3
	lesserSecScanBatchSizeEnv   = "LESSER_SEC_SCAN_BATCH_SIZE"
	lesserVulnCheckBatchSizeEnv = "LESSER_VULNCHECK_BATCH_SIZE"
)

func runSecScan(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	if err := ensureToolAvailableFn("gosec"); err != nil {
		return err
	}

	// gosec resolves package arguments to files only when they are directory
	// paths or package patterns; import paths resolve to zero files, which made
	// the batched scan vacuous. Pass directory paths (listDirs=true) so every
	// batch actually scans its packages. The -exclude rules below are the
	// triage-justified false-positive classes from
	// docs/security/security-gate-honesty-triage-1460.md; every excluded rule
	// traces to a per-finding classification in that document.
	if err := runGoPackageSecurityTool("gosec", []string{
		"-quiet",
		"-exclude-generated",
		"-exclude-dir=tmp",
		"-exclude-dir=infra",
		"-exclude=G703,G204,G304,G117,G702,G306,G302,G301,G101,G710,G704,G124,G115",
	}, repoRoot, nil, resolveSecScanBatchSize(), true); err != nil {
		return err
	}

	infraCDKDir := filepath.Join(repoRoot, "infra", "cdk")
	if _, err := os.Stat(filepath.Join(infraCDKDir, "go.mod")); err == nil {
		return runGoPackageSecurityTool("gosec", []string{
			"-quiet",
			"-exclude-generated",
			"-exclude-dir=cdk.out",
			"-exclude=G304,G301",
		}, infraCDKDir, nil, resolveSecScanBatchSize(), true)
	}

	return nil
}

func runVulnCheck(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	if err := ensureToolAvailableFn("govulncheck"); err != nil {
		return err
	}
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	env := map[string]string{
		"GOCACHE": goCache,
	}
	return runGoPackageSecurityTool("govulncheck", nil, repoRoot, env, resolveVulnCheckBatchSize(), false)
}

func resolveSecScanBatchSize() int {
	return resolveSecurityBatchSize(lesserSecScanBatchSizeEnv, defaultSecScanBatchSize)
}

func resolveVulnCheckBatchSize() int {
	return resolveSecurityBatchSize(lesserVulnCheckBatchSizeEnv, defaultVulnCheckBatchSize)
}

func resolveSecurityBatchSize(envVar string, defaultValue int) int {
	if raw := strings.TrimSpace(os.Getenv(envVar)); raw != "" {
		size, err := strconv.Atoi(raw)
		if err == nil {
			return size
		}
	}
	return defaultValue
}

func runGoPackageSecurityTool(name string, baseArgs []string, dir string, env map[string]string, batchSize int, listDirs bool) error {
	if batchSize <= 0 {
		args := append(append([]string(nil), baseArgs...), "./...")
		return runCommandFn(context.Background(), name, args, execOptions{
			Dir: dir,
			Env: env,
		})
	}

	packages, err := listGoPackagesForSecurityTool(dir, env, listDirs)
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		args := append(append([]string(nil), baseArgs...), "./...")
		return runCommandFn(context.Background(), name, args, execOptions{
			Dir: dir,
			Env: env,
		})
	}

	for i := 0; i < len(packages); i += batchSize {
		end := i + batchSize
		if end > len(packages) {
			end = len(packages)
		}
		args := append(append([]string(nil), baseArgs...), packages[i:end]...)
		if err := runCommandFn(context.Background(), name, args, execOptions{
			Dir: dir,
			Env: env,
		}); err != nil {
			return err
		}
	}

	return nil
}

// listGoPackagesForSecurityTool returns the batch targets for a security tool.
// govulncheck accepts import paths; gosec resolves import-path arguments to
// zero files, so it must receive directory paths (listDirs=true) to scan
// anything at all.
func listGoPackagesForSecurityTool(dir string, env map[string]string, listDirs bool) ([]string, error) {
	if listDirs {
		dirs, err := listGoPackageDirs(dir, env)
		if err != nil {
			return nil, err
		}
		// gosec's -exclude-dir flag only filters directories during package-
		// pattern expansion, not explicit directory arguments. Drop the same
		// paths the sec-scan flags exclude: tmp/ holds local scratch and
		// infra/ is a separate CDK module.
		filtered := dirs[:0]
		for _, d := range dirs {
			rel := strings.TrimPrefix(d, "."+string(filepath.Separator))
			first := strings.SplitN(rel, string(filepath.Separator), 2)[0]
			if first == "tmp" || first == "infra" {
				continue
			}
			filtered = append(filtered, d)
		}
		return filtered, nil
	}

	out, err := captureCommandOutputFn(context.Background(), dir, env, "go", "list", "./...")
	if err != nil {
		return nil, err
	}

	pkgs := make([]string, 0)
	pkgs = append(pkgs, goListLines(out)...)
	sort.Strings(pkgs)
	return pkgs, nil
}
