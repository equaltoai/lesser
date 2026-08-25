package main

import (
	"context"
	"fmt"
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

	// pinnedGosecVersion is the exact gosec version the sec-scan triage record
	// (docs/security/security-gate-honesty-triage-1460.md) was produced under.
	// scripts/install_ci_tools.sh reads this constant for its install pin, and
	// assertPinnedGosecVersion fails the gate closed on any other resolved
	// version, so the enforcement environment always runs the toolchain the
	// excludes were justified against.
	pinnedGosecVersion = "v2.28.0"
	// gosecModulePath is the module the gosec binary embeds in its build info
	// (`go version -m`), used to locate the version in the assertion.
	gosecModulePath = "github.com/securego/gosec/v2"
)

var lookPathInEnvFn = lookPathInEnv

func runSecScan(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	if err := ensureToolAvailableFn("gosec"); err != nil {
		return err
	}
	if err := assertPinnedGosecVersion(repoRoot); err != nil {
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

// assertPinnedGosecVersion enforces the exact gosec toolchain the triage record
// and the sec-scan excludes were produced under. The gate must scan with the
// pinned version and nothing else: an older binary lacks the taint rules the
// excludes rely on (CI previously pinned v2.22.11 and silently enforced a
// subset of the triaged set), and a newer binary can emit untriaged rules.
// Resolution, read, and parse failures fail closed too, so an environment that
// cannot prove the pinned toolchain cannot run the scan.
func assertPinnedGosecVersion(repoRoot string) error {
	env := mergeEnvForDir(os.Environ(), nil, repoRoot)
	binary, err := lookPathInEnvFn("gosec", env)
	if err != nil {
		return fmt.Errorf("sec-scan gosec version assertion: resolve gosec: %w", err)
	}
	out, err := captureCommandOutputFn(context.Background(), repoRoot, nil, "go", "version", "-m", binary)
	if err != nil {
		return fmt.Errorf("sec-scan gosec version assertion: read build info of %s: %w", binary, err)
	}
	version, ok := gosecVersionFromBuildInfo(out)
	if !ok {
		return fmt.Errorf("sec-scan gosec version assertion: cannot determine gosec version of %s (expected %s): build info does not name module %s", binary, pinnedGosecVersion, gosecModulePath)
	}
	if version != pinnedGosecVersion {
		return fmt.Errorf("sec-scan gosec version assertion failed: resolved gosec %s is %s, but the gate requires exactly %s (install via scripts/install_ci_tools.sh)", binary, version, pinnedGosecVersion)
	}
	return nil
}

// gosecVersionFromBuildInfo extracts the module version of the gosec binary
// from `go version -m` output (the mod line for the gosec module, e.g.
// "v2.28.0"). The binary's -version flag is not usable here: binaries installed
// via `go install` carry no release ldflags and report "Version: dev".
func gosecVersionFromBuildInfo(out string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" && fields[1] == gosecModulePath {
			return fields[2], true
		}
	}
	return "", false
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
