package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gosecBuildInfo renders the `go version -m` build-info block for a fake gosec
// binary, mirroring the mod line the assertion parses.
func gosecBuildInfo(version string) string {
	return fmt.Sprintf("/fake/bin/gosec: go1.26.6\n\tpath\tgithub.com/securego/gosec/v2/cmd/gosec\n\tmod\tgithub.com/securego/gosec/v2\t%s\th1:fake\n", version)
}

// stubLookPathInEnv makes binary resolution deterministic so the gosec version
// assertion does not depend on a real gosec being installed on the test host.
func stubLookPathInEnv(t *testing.T) {
	prev := lookPathInEnvFn
	t.Cleanup(func() { lookPathInEnvFn = prev })
	lookPathInEnvFn = func(name string, _ []string) (string, error) {
		return "/fake/bin/" + name, nil
	}
}

// gosecVersionCaptureBranch handles the assertion's `go version -m <binary>`
// capture call in a test capture stub, returning build info for the given
// version; it reports false when the call is not the assertion's.
func gosecVersionCaptureBranch(version string, args []string) (string, bool) {
	if len(args) >= 2 && args[0] == "version" && args[1] == "-m" {
		return gosecBuildInfo(version), true
	}
	return "", false
}

func TestSecurityCommands(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		captureCommandOutputFn = previousCapture
	})
	stubLookPathInEnv(t)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	t.Setenv(lesserSecScanBatchSizeEnv, "10")
	t.Setenv(lesserVulnCheckBatchSizeEnv, "10")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.26\n"), 0o644))
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if out, ok := gosecVersionCaptureBranch(pinnedGosecVersion, args); ok {
			return out, nil
		}
		if len(args) >= 2 && args[0] == "list" {
			switch args[1] {
			case "./...":
				return strings.Join([]string{
					"github.com/equaltoai/lesser/cmd/lesser",
					"github.com/equaltoai/lesser/pkg/common",
				}, "\n"), nil
			case "-f":
				return filepath.Join(repoRoot, "cmd", "lesser") + "\n" +
					filepath.Join(repoRoot, "pkg", "common") + "\n", nil
			}
		}
		return "", nil
	}

	var called int
	var gosecCalls [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		called++
		if name == "gosec" {
			gosecCalls = append(gosecCalls, append([]string(nil), args...))
		}
		return nil
	}

	require.NoError(t, runSecScan(nil))
	require.Equal(t, 1, called)
	require.Len(t, gosecCalls, 1)
	require.Equal(t, []string{
		"-quiet",
		"-exclude-generated",
		"-exclude-dir=tmp",
		"-exclude-dir=infra",
		"-exclude=G703,G204,G304,G117,G702,G306,G302,G301,G101,G710,G704,G124,G115",
		"./cmd/lesser",
		"./pkg/common",
	}, gosecCalls[0], "gosec must receive directory paths, not import paths")

	require.NoError(t, runVulnCheck(nil))
	require.Equal(t, 2, called)
}

func TestRunSecScan_RunsCDKModuleScanWhenPresent(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		captureCommandOutputFn = previousCapture
	})
	stubLookPathInEnv(t)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	t.Setenv(lesserSecScanBatchSizeEnv, "10")

	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "go.mod"), []byte("module example.com/cdk\n"), 0o644))
	captureCommandOutputFn = func(_ context.Context, dir string, _ map[string]string, _ string, args ...string) (string, error) {
		if out, ok := gosecVersionCaptureBranch(pinnedGosecVersion, args); ok {
			return out, nil
		}
		if len(args) >= 2 && args[0] == "list" && args[1] == "-f" {
			switch dir {
			case repoRoot:
				return filepath.Join(repoRoot, "cmd", "lesser") + "\n", nil
			case filepath.Join(repoRoot, "infra", "cdk"):
				return filepath.Join(repoRoot, "infra", "cdk", "app") + "\n", nil
			}
		}
		return "", nil
	}

	var called int
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error {
		called++
		return nil
	}

	require.NoError(t, runSecScan(nil))
	require.Equal(t, 2, called)
}

func TestAssertPinnedGosecVersion_PassesOnPinnedVersion(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })
	stubLookPathInEnv(t)

	repoRoot := t.TempDir()
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if out, ok := gosecVersionCaptureBranch(pinnedGosecVersion, args); ok {
			return out, nil
		}
		return "", nil
	}

	require.NoError(t, assertPinnedGosecVersion(repoRoot))
}

func TestAssertPinnedGosecVersion_FailsClosedOnVersionMismatch(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })
	stubLookPathInEnv(t)

	repoRoot := t.TempDir()
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if out, ok := gosecVersionCaptureBranch("v2.22.11", args); ok {
			return out, nil
		}
		return "", nil
	}

	err := assertPinnedGosecVersion(repoRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "v2.22.11", "mismatch error must name the resolved version")
	require.Contains(t, err.Error(), pinnedGosecVersion, "mismatch error must name the pinned version")
}

func TestAssertPinnedGosecVersion_FailsClosedOnUnreadableBuildInfo(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })
	stubLookPathInEnv(t)

	repoRoot := t.TempDir()
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "version" && args[1] == "-m" {
			return "", nil // empty build info: cannot prove the pinned version
		}
		return "", nil
	}

	err := assertPinnedGosecVersion(repoRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), pinnedGosecVersion)
}

func TestAssertPinnedGosecVersion_FailsClosedOnMissingModuleLine(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })
	stubLookPathInEnv(t)

	repoRoot := t.TempDir()
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "version" && args[1] == "-m" {
			// A gosec binary built outside the pinned module (e.g. a checkout
			// build with no embedded mod version) must not pass the gate.
			return "/fake/bin/gosec: go1.26.6\n\tpath\tgithub.com/securego/gosec/v2/cmd/gosec\n", nil
		}
		return "", nil
	}

	err := assertPinnedGosecVersion(repoRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), gosecModulePath)
}

func TestAssertPinnedGosecVersion_FailsClosedWhenBinaryUnresolvable(t *testing.T) {
	prev := lookPathInEnvFn
	t.Cleanup(func() { lookPathInEnvFn = prev })
	lookPathInEnvFn = func(_ string, _ []string) (string, error) {
		return "", fmt.Errorf("executable file %q not found in PATH", "gosec")
	}

	err := assertPinnedGosecVersion(t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve gosec")
}

func TestRunSecScan_FailsClosedOnGosecVersionMismatch(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
		captureCommandOutputFn = previousCapture
	})
	stubLookPathInEnv(t)

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(string) error { return nil }
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if out, ok := gosecVersionCaptureBranch("v2.22.11", args); ok {
			return out, nil
		}
		return "", nil
	}

	err := runSecScan(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "v2.22.11")
	require.Contains(t, err.Error(), pinnedGosecVersion)
}
