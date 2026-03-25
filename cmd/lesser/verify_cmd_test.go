package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunVerify_Dispatch(t *testing.T) {
	t.Run("help returns nil", func(t *testing.T) {
		require.NoError(t, runVerify([]string{helpCommand}))
	})

	t.Run("unknown returns error", func(t *testing.T) {
		require.Error(t, runVerify([]string{"nope"}))
	})
}

func TestRunVerify_Subcommands_InvokeExpectedCommands(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	var calls []string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		if name == "go" && firstArgOrEmpty(args) == "run" {
			calls = append(calls, name+" "+strings.Join(args, " "))
		} else {
			calls = append(calls, name+" "+firstArgOrEmpty(args))
		}
		require.Equal(t, repoRoot, opts.Dir)
		return nil
	}

	require.NoError(t, runVerify([]string{"docs"}))
	require.NoError(t, runVerify([]string{"ai-training"}))
	require.NoError(t, runVerify([]string{valueSchema}))
	require.NoError(t, runVerify([]string{"audit"}))
	require.NoError(t, runVerify([]string{"supply-chain"}))
	require.NoError(t, runVerify([]string{"lambda-set"}))

	require.Contains(t, calls, "bash scripts/verify_docs.sh")
	require.Contains(t, calls, "bash scripts/verify_ai_training.sh")
	require.Contains(t, calls, "bash scripts/verify_schema.sh")
	require.Contains(t, calls, "go run ./tools/audit_gates --check")
	require.Contains(t, calls, "bash scripts/verify_supply_chain.sh")
	require.Contains(t, calls, "bash scripts/verify_lambda_set.sh")
}

func TestRunVerifyAll_WiresSubcommands(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	ensureToolAvailableFn = func(string) error { return nil }
	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }

	var calls []string
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		calls = append(calls, name+" "+firstArgOrEmpty(args))
		return nil
	}

	require.NoError(t, runVerifyAll(nil))
	require.NotEmpty(t, calls)
}

func TestRunVerifyCI_RunsLintSecurityAndVerifySuite(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
		captureCommandOutputFn = previousCapture
	})

	ensureToolAvailableFn = func(string) error { return nil }
	t.Setenv(lesserVerifyCIJobsEnv, "")
	t.Setenv("LESSER_JOBS", "8")
	t.Setenv(goMaxProcsEnvVar, "")
	t.Setenv(goFlagsEnvVar, "")
	t.Setenv(lesserSecScanBatchSizeEnv, "10")
	t.Setenv(lesserVulnCheckBatchSizeEnv, "10")

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, name string, args ...string) (string, error) {
		if name == "go" && len(args) >= 4 && args[0] == "list" && args[1] == "-f" {
			return strings.Join([]string{
				filepath.Join(repoRoot, "cmd", "lesser"),
				filepath.Join(repoRoot, "pkg", "common"),
				filepath.Join(repoRoot, "pkg", "testing", "harness"),
				filepath.Join(repoRoot, "tools", "coverage_scoreboard"),
			}, "\n"), nil
		}
		if name == "go" && len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			return strings.Join([]string{
				"github.com/equaltoai/lesser/cmd/lesser",
				"github.com/equaltoai/lesser/pkg/common",
				"github.com/equaltoai/lesser/pkg/testing/harness",
				"github.com/equaltoai/lesser/tools/coverage_scoreboard",
			}, "\n"), nil
		}
		return "", nil
	}

	var calls []string
	var coverageRuns int
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "golangci-lint" {
			calls = append(calls, name+" "+strings.Join(args, " "))
		} else if name == "go" && firstArgOrEmpty(args) == "run" {
			calls = append(calls, name+" "+strings.Join(args, " "))
		} else if name == "go" && firstArgOrEmpty(args) == "test" {
			calls = append(calls, name+" "+strings.Join(args, " "))
			coverageRuns++
			return writeCoverageProfileFromArgs(repoRoot, args)
		} else if name == "gosec" {
			calls = append(calls, name+" "+strings.Join(args, " "))
		} else if name == "govulncheck" {
			calls = append(calls, name+" "+strings.Join(args, " "))
		} else {
			calls = append(calls, name+" "+firstArgOrEmpty(args))
		}
		return nil
	}

	require.NoError(t, runVerify([]string{"ci"}))
	var sawBatchedLint bool
	for _, call := range calls {
		if !strings.HasPrefix(call, "golangci-lint run --config .golangci.yml --disable gosec --concurrency 1") {
			continue
		}
		if strings.Contains(call, "./cmd/lesser") && strings.Contains(call, "./pkg/common") {
			sawBatchedLint = true
			break
		}
	}
	require.True(t, sawBatchedLint)
	require.Contains(t, calls, "go run ./tools/audit_gates --check")
	var sawBatchedSecScan bool
	for _, call := range calls {
		if !strings.HasPrefix(call, "gosec ") {
			continue
		}
		if strings.Contains(call, "github.com/equaltoai/lesser/cmd/lesser") &&
			strings.Contains(call, "github.com/equaltoai/lesser/pkg/common") {
			sawBatchedSecScan = true
			break
		}
	}
	require.True(t, sawBatchedSecScan)
	var sawBatchedVulnCheck bool
	for _, call := range calls {
		if !strings.HasPrefix(call, "govulncheck ") {
			continue
		}
		if strings.Contains(call, "github.com/equaltoai/lesser/cmd/lesser") &&
			strings.Contains(call, "github.com/equaltoai/lesser/pkg/common") {
			sawBatchedVulnCheck = true
			break
		}
	}
	require.True(t, sawBatchedVulnCheck)
	require.Contains(t, calls, "bash scripts/verify_supply_chain.sh")
	require.Contains(t, calls, "bash scripts/verify_lambda_set.sh")
	require.Contains(t, calls, "bash scripts/verify_inventory.sh")
	require.Contains(t, calls, "bash scripts/verify_docs.sh")
	require.Contains(t, calls, "bash scripts/verify_ai_training.sh")
	require.Contains(t, calls, "bash scripts/verify_schema.sh")
	require.Contains(t, calls, "go run ./tools/graphql_coverage --check --strict")
	require.Contains(t, calls, "go run ./tools/openapi --check --strict")
	require.Contains(t, calls, "go run ./tools/coverage_scoreboard --mode package --top 10 --min 0 --sort-uncovered=true --exclude-generated=true --min-total 85 --profile coverage_overall.out")
	require.Contains(t, calls, "go run ./tools/coverage_scoreboard --mode package --top 10 --min 0 --sort-uncovered=true --exclude-generated=true --min-total 90 --profile coverage_overall.out --package github.com/equaltoai/lesser/pkg")
	require.Contains(t, calls, "go run ./tools/coverage_scoreboard --mode package --top 10 --min 0 --sort-uncovered=true --exclude-generated=true --min-total 90 --profile coverage_overall.out --package github.com/equaltoai/lesser/cmd")
	require.Equal(t, 1, coverageRuns)
}

func TestRunVerifyGraphQLCoverage_StrictFlag(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	ensureToolAvailableFn = func(string) error { return nil }
	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }

	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}

	require.NoError(t, runVerifyGraphQLCoverage([]string{"--strict"}))
	require.Contains(t, gotArgs, "--strict")
}

func TestRunCDKSynth_RequiresAWSProfile(t *testing.T) {
	require.Error(t, runCDKSynth("", "us-east-1"))
}

func TestRunCDKSynth_RunsCdkWithCaches(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	var got execOptions
	var gotName string
	var gotArgs []string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		got = opts
		return nil
	}

	require.NoError(t, runCDKSynth("profile", "us-east-1"))
	require.Equal(t, "cdk", gotName)
	require.Contains(t, gotArgs, "synth")
	require.Equal(t, filepath.Join(repoRoot, "infra", "cdk"), got.Dir)
	require.Equal(t, "profile", got.Env["AWS_PROFILE"])
	require.Equal(t, "us-east-1", got.Env["AWS_REGION"])
	require.NotEmpty(t, got.Env["GOCACHE"])
	require.NotEmpty(t, got.Env["XDG_CACHE_HOME"])
}

func TestRunVerifyOpenAPI_StrictPassesFlag(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, opts execOptions) error {
		gotArgs = append([]string(nil), args...)
		require.Equal(t, repoRoot, opts.Dir)
		require.NotEmpty(t, opts.Env["GOCACHE"])
		return nil
	}

	require.NoError(t, runVerifyOpenAPI([]string{"--strict"}))
	require.Contains(t, gotArgs, "--strict")
}

func TestRunVerifyUnit_UsesGoTestHarness(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		require.Equal(t, "go", name)
		require.Contains(t, args, "test")
		require.Equal(t, "test", opts.Env["ENVIRONMENT"])
		require.Equal(t, "test", opts.Env["STAGE"])
		require.NotEmpty(t, opts.Env["JWT_SECRET"])
		return nil
	}

	require.NoError(t, runVerifyUnit(nil))
}

func TestRunVerifySmoke_RunsCoreAndFederation(t *testing.T) {
	previousRunCommand := runCommandFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	var scripts []string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		require.Equal(t, "bash", name)
		require.Equal(t, repoRoot, opts.Dir)
		scripts = append(scripts, firstArgOrEmpty(args))
		require.Equal(t, "http://example.com", opts.Env["SMOKE_BASE_URL"])
		return nil
	}

	require.NoError(t, runVerifySmoke([]string{
		"--base-url", "http://example.com",
		"--username", "u",
		"--object-id", "id",
		"--token", "tok",
	}))
	require.Equal(t, []string{"scripts/smoke_core.sh", "scripts/smoke_federation.sh"}, scripts)
}

func TestRunVerifyAll_WithSmokeAndCDK(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	var calledCdk bool
	var calledSmoke bool
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "cdk" {
			calledCdk = true
		}
		if name == "bash" && (firstArgOrEmpty(args) == "scripts/smoke_core.sh" || firstArgOrEmpty(args) == "scripts/smoke_federation.sh") {
			calledSmoke = true
		}
		return nil
	}

	err := runVerifyAll([]string{
		"--smoke",
		"--cdk",
		"--smoke-base-url", "http://example.com",
		"--smoke-token", "tok",
		"--smoke-username", "u",
		"--smoke-object-id", "id",
		"--cdk-aws-profile", "profile",
		"--cdk-region", "us-east-1",
	})
	require.NoError(t, err)
	require.True(t, calledSmoke)
	require.True(t, calledCdk)
}

func TestRunVerify_FlagFirstArgRunsVerifyAll(t *testing.T) {
	previousRunCommand := runCommandFn
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(string) error { return nil }

	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }
	require.NoError(t, runVerify([]string{
		"--smoke",
		"--smoke-base-url", "http://example.com",
		"--smoke-username", "u",
		"--smoke-object-id", "id",
	}))
}

func firstArgOrEmpty(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func TestRunVerifyGraphQLCoverage_FailsWhenGoMissing(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(string) error { return errors.New("no go") }

	err := runVerifyGraphQLCoverage(nil)
	require.Error(t, err)
}

func TestRunVerifyDocs_AITraining_Schema_FailsWhenRepoRootMissing(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	findRepoRootFn = func() (string, error) { return "", errors.New("no root") }
	require.Error(t, runVerifyDocs(nil))
	require.Error(t, runVerifyAITraining(nil))
	require.Error(t, runVerifySchema(nil))
}

func TestRunVerifyCDK_ParsesFlagsAndRunsSynth(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	var sawCdk bool
	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "cdk" && len(args) > 0 && args[0] == "synth" {
			sawCdk = true
		}
		return nil
	}

	require.NoError(t, runVerifyCDK([]string{"--aws-profile", "profile"}))
	require.True(t, sawCdk)
}

func TestRunVerify_DispatchesAdditionalSubcommands(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }

	require.NoError(t, runVerify(nil))
	require.NoError(t, runVerify([]string{"graphql-coverage"}))
	require.NoError(t, runVerify([]string{"openapi"}))
	require.NoError(t, runVerify([]string{"inventory"}))
	require.NoError(t, runVerify([]string{"lambda-set"}))
	require.NoError(t, runVerify([]string{"unit"}))
	require.NoError(t, runVerify([]string{"smoke", "--base-url", "http://example.com", "--username", "u", "--object-id", "id"}))
	require.NoError(t, runVerify([]string{"cdk", "--aws-profile", "profile"}))
}

func TestRunVerifyAll_PropagatesErrors(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_inventory.sh" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runVerifyAll(nil), errSentinel)
}

func TestRunVerifyAll_SmokeAndCDK_ErrorBranches(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/smoke_core.sh" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runVerifyAll([]string{"--smoke", "--smoke-base-url", "http://example.com"}), errSentinel)

	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		if name == "cdk" {
			return errSentinel
		}
		return nil
	}
	require.ErrorIs(t, runVerifyAll([]string{"--cdk", "--cdk-aws-profile", "profile"}), errSentinel)
}

func TestRunVerifyGraphQLCoverage_GoCacheDirError(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	ensureToolAvailableFn = func(string) error { return nil }
	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))

	require.Error(t, runVerifyGraphQLCoverage(nil))
}

func TestRunCDKSynth_ErrorBranches(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
		findRepoRootFn = previousRepoRoot
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	require.ErrorIs(t, runCDKSynth("profile", "us-east-1"), errSentinel)

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return errSentinel }
	require.ErrorIs(t, runCDKSynth("profile", "us-east-1"), errSentinel)

	ensureToolAvailableFn = func(string) error { return nil }
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "tmp"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp", "xdg-cache"), []byte("x"), 0o644))
	require.Error(t, runCDKSynth("profile", "us-east-1"))

	repoRoot2 := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot2, nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runCDKSynth("profile", "us-east-1"), errSentinel)
}
