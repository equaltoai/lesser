package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunTest_DispatchAndCoverageScopes(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousCapture := captureCommandOutputFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		captureCommandOutputFn = previousCapture
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "cdk.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "coverage.out"), []byte("mode: set\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "coverage_overall.out"), []byte("mode: set\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "coverage_pkg.out"), []byte("mode: set\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "./pkg/...") {
			return strings.Join([]string{
				"github.com/equaltoai/lesser/pkg/foo",
				"github.com/equaltoai/lesser/pkg/testing/harness",
			}, "\n"), nil
		}
		return strings.Join([]string{
			"github.com/equaltoai/lesser/cmd/api",
			"github.com/equaltoai/lesser/pkg/foo",
			"github.com/equaltoai/lesser/tools/coverage_scoreboard",
		}, "\n"), nil
	}

	var gotGoArgs [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "go" {
			gotGoArgs = append(gotGoArgs, append([]string(nil), args...))
		}
		return nil
	}

	require.NoError(t, runTest([]string{helpCommand}))
	require.Error(t, runTest([]string{"nope"}))

	require.NoError(t, runTestAll(nil))
	require.NoError(t, runTestUnit(nil))
	require.NoError(t, runTestRace(nil))
	require.NoError(t, runTestIntegration(nil))

	require.NoError(t, runTestCoverage([]string{"--scope", "all"}))
	require.NoError(t, runTestCoverage([]string{"--scope", "overall"}))
	require.NoError(t, runTestCoverage([]string{"--scope", "pkg"}))
	require.Error(t, runTestCoverage([]string{"--scope", "wat"}))

	require.NotEmpty(t, gotGoArgs)
}

func TestReadModulePath_RejectsInvalidGoMod(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("not a module file\n"), 0o644))

	_, err := readModulePath(repoRoot)
	require.Error(t, err)
}

func TestRunTest_DispatchesSubcommands(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousCapture := captureCommandOutputFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		captureCommandOutputFn = previousCapture
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "coverage.out"), []byte("mode: set\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "coverage_pkg.out"), []byte("mode: set\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "./pkg/...") {
			return "github.com/equaltoai/lesser/pkg/foo\n", nil
		}
		return strings.Join([]string{
			"github.com/equaltoai/lesser/cmd/api",
			"github.com/equaltoai/lesser/pkg/foo",
			"github.com/equaltoai/lesser/tools/coverage_scoreboard",
		}, "\n"), nil
	}

	var goCalls [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "go" {
			goCalls = append(goCalls, append([]string(nil), args...))
		}
		return nil
	}

	require.NoError(t, runTest(nil))
	require.NoError(t, runTest([]string{helpFlagShort}))
	require.NoError(t, runTest([]string{helpFlagLong}))
	require.NoError(t, runTest([]string{"unit"}))
	require.NoError(t, runTest([]string{"integration"}))
	require.NoError(t, runTest([]string{"race"}))
	require.NoError(t, runTest([]string{"all"}))

	require.Error(t, runTestAll([]string{"--badflag"}))

	require.NoError(t, runTest([]string{"coverage", "--scope", "pkg", "--include-testing"}))
	require.NoError(t, runTest([]string{"coverage", "--scope", "all", "--include-tools"}))
	require.NotEmpty(t, goCalls)
}

func TestRunTest_MemorySafeDefaults(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousCapture := captureCommandOutputFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		captureCommandOutputFn = previousCapture
		ensureToolAvailableFn = previousEnsureTool
	})

	t.Setenv(lesserTestGOMEMLIMITEnv, "")
	t.Setenv(lesserTestParallelismEnv, "")
	t.Setenv(lesserTestRaceParallelismEnv, "")
	t.Setenv(goMemoryLimitEnvVar, "")

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return "github.com/equaltoai/lesser/pkg/foo\n", nil
	}

	type goCall struct {
		args []string
		env  map[string]string
	}
	var calls []goCall
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		if name != "go" || firstArgOrEmpty(args) != "test" {
			return nil
		}
		env := make(map[string]string, len(opts.Env))
		for key, value := range opts.Env {
			env[key] = value
		}
		calls = append(calls, goCall{args: append([]string(nil), args...), env: env})
		return writeCoverageProfileFromArgs(repoRoot, args)
	}

	require.NoError(t, runTestAll(nil))
	require.NoError(t, runTestUnit(nil))
	require.NoError(t, runTestRace(nil))
	require.NoError(t, runTestCoverage([]string{"--scope", "pkg", "--html=false"}))
	require.Len(t, calls, 4)

	require.Contains(t, calls[0].args, "-p=4")
	require.Equal(t, defaultTestGOMEMLIMIT, calls[0].env[goMemoryLimitEnvVar])

	require.Contains(t, calls[1].args, "-p=4")
	require.Contains(t, calls[1].args, "-short")
	require.Equal(t, defaultTestGOMEMLIMIT, calls[1].env[goMemoryLimitEnvVar])

	require.Contains(t, calls[2].args, "-p=2")
	require.Contains(t, calls[2].args, "-race")
	require.Equal(t, defaultTestGOMEMLIMIT, calls[2].env[goMemoryLimitEnvVar])

	require.Contains(t, calls[3].args, "-p=4")
	require.NotContains(t, calls[3].args, "-short")
	require.Equal(t, defaultTestGOMEMLIMIT, calls[3].env[goMemoryLimitEnvVar])
}

func TestRunTest_MemorySafeOverrides(t *testing.T) {
	t.Setenv(lesserTestParallelismEnv, "7")
	t.Setenv(lesserTestRaceParallelismEnv, "3")
	t.Setenv(lesserTestGOMEMLIMITEnv, "8GiB")
	t.Setenv(goMemoryLimitEnvVar, "5GiB")

	require.Equal(t, []string{"test", "-p=7", "-v", "./..."}, memorySafeGoTestArgs([]string{"test", "-v", "./..."}, false))
	require.Equal(t, []string{"test", "-p=3", "-race", "-v", "./..."}, memorySafeGoTestArgs([]string{"test", "-race", "-v", "./..."}, true))
	require.Equal(t, "8GiB", resolveTestGOMEMLIMIT())

	t.Setenv(lesserTestParallelismEnv, envValueOff)
	require.Equal(t, []string{"test", "-v", "./..."}, memorySafeGoTestArgs([]string{"test", "-v", "./..."}, false))
}

func TestRunTestCoverage_ErrorsWhenNoPackagesRemainAfterFiltering(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousCapture := captureCommandOutputFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		captureCommandOutputFn = previousCapture
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return "github.com/equaltoai/lesser/tools/coverage_scoreboard\n", nil
	}

	err := runTestCoverage([]string{"--scope", "all"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no packages found")
}

func TestRunGoTests_ErrorBranches(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runGoTests(testArgs{}, []string{"test"}, nil), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return errSentinel }
	require.ErrorIs(t, runGoTests(testArgs{}, []string{"test"}, nil), errSentinel)

	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))
	require.Error(t, runGoTests(testArgs{}, []string{"test"}, nil))

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runGoTests(testArgs{}, []string{"test"}, map[string]string{"X": "1"}), errSentinel)
}

func TestReadModulePath_MissingFileIsError(t *testing.T) {
	_, err := readModulePath(t.TempDir())
	require.Error(t, err)
}

func TestListPackagesForAllCoverage_PropagatesCaptureError(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })

	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))
	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "", errSentinel
	}

	_, err := listPackagesForAllCoverage(repoRoot, t.TempDir(), false)
	require.ErrorIs(t, err, errSentinel)
}

func TestRunTestSubcommands_ReportFlagParseErrors(t *testing.T) {
	require.Error(t, runTestUnit([]string{"--badflag"}))
	require.Error(t, runTestIntegration([]string{"--badflag"}))
	require.Error(t, runTestRace([]string{"--badflag"}))
	require.Error(t, runTestCoverage([]string{"--badflag"}))
}

func TestRunTestCoverage_PropagatesRepoRootAndCacheErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runTestCoverage([]string{"--scope", "all"}), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	goCacheFile := filepath.Join(repoRoot, "go-cache-file")
	require.NoError(t, os.WriteFile(goCacheFile, []byte("x"), 0o644))
	t.Setenv("GOCACHE", goCacheFile)
	require.Error(t, runTestCoverage([]string{"--scope", "all"}))
}

func TestRunGoCoverageTests_BatchesAndMergesProfiles(t *testing.T) {
	previousRunCommand := runCommandFn
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))

	t.Setenv(coverageBatchSizeEnvVar, "2")

	var goTestCalls [][]string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name != "go" || len(args) == 0 || args[0] != "test" {
			return nil
		}
		goTestCalls = append(goTestCalls, append([]string(nil), args...))

		profileArg := ""
		for _, arg := range args {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profileArg = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		require.NotEmpty(t, profileArg)

		lastPkg := args[len(args)-1]
		content := "mode: set\n" + lastPkg + ".go:1.1,1.2 1 1\n"
		return os.WriteFile(profileArg, []byte(content), 0o644)
	}

	err := runGoCoverageTests(testArgs{}, repoRoot, filepath.Join(repoRoot, "coverage.out"), []string{"test"}, []string{
		"github.com/equaltoai/lesser/pkg/a",
		"github.com/equaltoai/lesser/pkg/b",
		"github.com/equaltoai/lesser/pkg/c",
	})
	require.NoError(t, err)
	require.Len(t, goTestCalls, 2)

	merged, err := os.ReadFile(filepath.Join(repoRoot, "coverage.out"))
	require.NoError(t, err)
	require.Equal(t, "mode: set\ngithub.com/equaltoai/lesser/pkg/b.go:1.1,1.2 1 1\ngithub.com/equaltoai/lesser/pkg/c.go:1.1,1.2 1 1\n", string(merged))
}

func TestRunGoCoverageTests_UsesSingleBatchProfileWhenWithinLimit(t *testing.T) {
	previousRunCommand := runCommandFn
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))

	t.Setenv(coverageBatchSizeEnvVar, "10")

	var profileArg string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name != "go" || len(args) == 0 || args[0] != "test" {
			return nil
		}
		for _, arg := range args {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profileArg = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		require.Equal(t, filepath.Join(repoRoot, "coverage.out"), profileArg)
		return os.WriteFile(profileArg, []byte("mode: set\npkg.go:1.1,1.2 1 1\n"), 0o644)
	}

	err := runGoCoverageTests(testArgs{}, repoRoot, filepath.Join(repoRoot, "coverage.out"), []string{"test"}, []string{
		"github.com/equaltoai/lesser/pkg/a",
		"github.com/equaltoai/lesser/pkg/b",
	})
	require.NoError(t, err)
}

func TestMergeCoverageProfiles_RejectsMismatchedModes(t *testing.T) {
	repoRoot := t.TempDir()
	first := filepath.Join(repoRoot, "first.out")
	second := filepath.Join(repoRoot, "second.out")

	require.NoError(t, os.WriteFile(first, []byte("mode: set\none.go:1.1,1.2 1 1\n"), 0o644))
	require.NoError(t, os.WriteFile(second, []byte("mode: count\ntwo.go:1.1,1.2 1 1\n"), 0o644))

	err := mergeCoverageProfiles(filepath.Join(repoRoot, "merged.out"), []string{first, second})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mismatched coverage mode")
}

func TestMergeCoverageProfiles_WritesDefaultModeWhenNoProfilesProvided(t *testing.T) {
	repoRoot := t.TempDir()
	mergedPath := filepath.Join(repoRoot, "merged.out")

	require.NoError(t, mergeCoverageProfiles(mergedPath, nil))

	merged, err := os.ReadFile(mergedPath)
	require.NoError(t, err)
	require.Equal(t, "mode: set\n", string(merged))
}
