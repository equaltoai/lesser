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

	if err := runGoPackageSecurityTool("gosec", []string{
		"-quiet",
		"-exclude-generated",
		"-exclude-dir=tmp",
		"-exclude-dir=infra",
	}, repoRoot, nil, resolveSecScanBatchSize()); err != nil {
		return err
	}

	infraCDKDir := filepath.Join(repoRoot, "infra", "cdk")
	if _, err := os.Stat(filepath.Join(infraCDKDir, "go.mod")); err == nil {
		return runGoPackageSecurityTool("gosec", []string{
			"-quiet",
			"-exclude-generated",
			"-exclude-dir=cdk.out",
		}, infraCDKDir, nil, resolveSecScanBatchSize())
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
	return runGoPackageSecurityTool("govulncheck", nil, repoRoot, env, resolveVulnCheckBatchSize())
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

func runGoPackageSecurityTool(name string, baseArgs []string, dir string, env map[string]string, batchSize int) error {
	if batchSize <= 0 {
		args := append(append([]string(nil), baseArgs...), "./...")
		return runCommandFn(context.Background(), name, args, execOptions{
			Dir: dir,
			Env: env,
		})
	}

	packages, err := listGoPackagesForSecurityTool(dir, env)
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

func listGoPackagesForSecurityTool(dir string, env map[string]string) ([]string, error) {
	out, err := captureCommandOutputFn(context.Background(), dir, env, "go", "list", "./...")
	if err != nil {
		return nil, err
	}

	pkgs := make([]string, 0)
	for _, line := range strings.Split(out, "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs, nil
}
