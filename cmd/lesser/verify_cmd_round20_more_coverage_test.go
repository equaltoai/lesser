package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupVerifyCIRound20Harness(t *testing.T) string {
	t.Helper()

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

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".golangci.yml"), []byte("version: \"2\"\n"), 0o644))

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, name string, args ...string) (string, error) {
		if name == "golangci-lint" && firstArgOrEmpty(args) == "version" {
			return "golangci-lint has version v2.10.1\n", nil
		}
		if name == "go" && len(args) >= 2 && args[0] == "list" && args[1] == "./..." {
			return strings.Join([]string{
				"github.com/equaltoai/lesser/cmd/lesser",
				"github.com/equaltoai/lesser/pkg/common",
			}, "\n"), nil
		}
		return "", nil
	}

	runCommandFn = func(context.Context, string, []string, execOptions) error { return nil }

	return repoRoot
}

func TestRunVerifyCI_Round20_PropagatesAuditFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "go" && len(args) >= 2 && args[0] == "run" && args[1] == "./tools/audit_gates" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesSecScanFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		if name == "gosec" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesVulnCheckFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		if name == "govulncheck" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesSupplyChainFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_supply_chain.sh" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesLambdaSetFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_lambda_set.sh" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestRunVerifyCI_Round20_PropagatesInventoryFailure(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_inventory.sh" {
			return errSentinel
		}
		return nil
	}

	require.ErrorIs(t, runVerifyCI(nil), errSentinel)
}

func TestResolveVerifyCIJobs_Round20(t *testing.T) {
	t.Run("explicit verify ci override wins", func(t *testing.T) {
		t.Setenv(lesserVerifyCIJobsEnv, "3")
		t.Setenv(lesserToolJobsEnvVar, "8")
		require.Equal(t, 3, resolveVerifyCIJobs())
	})

	t.Run("explicit tool env disables automatic override", func(t *testing.T) {
		t.Setenv(lesserVerifyCIJobsEnv, "")
		t.Setenv(lesserToolJobsEnvVar, "8")
		require.Zero(t, resolveVerifyCIJobs())
	})

	t.Run("default cap stays at or below two", func(t *testing.T) {
		t.Setenv(lesserVerifyCIJobsEnv, "")
		t.Setenv(lesserToolJobsEnvVar, "")
		t.Setenv(goMaxProcsEnvVar, "")
		t.Setenv(goFlagsEnvVar, "")

		jobs := resolveVerifyCIJobs()
		require.GreaterOrEqual(t, jobs, 1)
		require.LessOrEqual(t, jobs, defaultVerifyCIJobs)
	})

	t.Run("goflags build parallelism disables automatic override", func(t *testing.T) {
		t.Setenv(lesserVerifyCIJobsEnv, "")
		t.Setenv(lesserToolJobsEnvVar, "")
		t.Setenv(goMaxProcsEnvVar, "")
		t.Setenv(goFlagsEnvVar, "-trimpath -p=8")

		require.Zero(t, resolveVerifyCIJobs())
	})
}

func TestRunVerifyCI_Round20_UsesVerifyCIJobsOverride(t *testing.T) {
	repoRoot := setupVerifyCIRound20Harness(t)

	t.Setenv(lesserVerifyCIJobsEnv, "2")
	t.Setenv(lesserToolJobsEnvVar, "8")
	t.Setenv(goMaxProcsEnvVar, "")
	t.Setenv(goFlagsEnvVar, "-trimpath")

	var lintCall string
	var coverageEnv map[string]string

	runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
		joinedArgs := strings.Join(args, " ")
		switch {
		case name == "golangci-lint":
			lintCall = name + " " + joinedArgs
		case name == "go" && firstArgOrEmpty(args) == "test" && strings.Contains(joinedArgs, "-coverprofile=coverage_overall.out"):
			coverageEnv = map[string]string{
				lesserToolJobsEnvVar: os.Getenv(lesserToolJobsEnvVar),
				goMaxProcsEnvVar:     os.Getenv(goMaxProcsEnvVar),
				goFlagsEnvVar:        os.Getenv(goFlagsEnvVar),
			}
			coveragePath := filepath.Join(repoRoot, "coverage_overall.out")
			coverageData := "mode: set\n" + "github.com/equaltoai/lesser/pkg/common/errors.go:1.1,1.2 1 1\n"
			return os.WriteFile(coveragePath, []byte(coverageData), 0o644)
		}
		return nil
	}

	require.NoError(t, runVerifyCI(nil))
	require.Contains(t, lintCall, "--concurrency 2")
	require.Equal(t, "2", coverageEnv[lesserToolJobsEnvVar])
	require.Equal(t, "2", coverageEnv[goMaxProcsEnvVar])
	require.Equal(t, "-trimpath -p=2", coverageEnv[goFlagsEnvVar])

	require.Equal(t, "8", os.Getenv(lesserToolJobsEnvVar))
	require.Equal(t, "", os.Getenv(goMaxProcsEnvVar))
	require.Equal(t, "-trimpath", os.Getenv(goFlagsEnvVar))
}

func TestRunVerifyCI_Round20_SkipsSecurityWhenDisabled(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	t.Setenv(lesserToolJobsEnvVar, "8")

	var sawSecScan bool
	var sawVulnCheck bool

	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		switch name {
		case "gosec":
			sawSecScan = true
		case "govulncheck":
			sawVulnCheck = true
		}
		return nil
	}

	require.NoError(t, runVerifyCI([]string{"--security=false"}))
	require.False(t, sawSecScan)
	require.False(t, sawVulnCheck)
}

func TestRunVerifyCIContractsAndCoverage_Round20_PropagatesRemainingFailures(t *testing.T) {
	_ = setupVerifyCIRound20Harness(t)

	const (
		cmdPrefix = "github.com/equaltoai/lesser/cmd"
		pkgPrefix = "github.com/equaltoai/lesser/pkg"
	)

	tests := []struct {
		name       string
		shouldFail func(name string, args []string) bool
	}{
		{
			name: "docs",
			shouldFail: func(name string, args []string) bool {
				return name == "bash" && firstArgOrEmpty(args) == "scripts/verify_docs.sh"
			},
		},
		{
			name: "ai training",
			shouldFail: func(name string, args []string) bool {
				return name == "bash" && firstArgOrEmpty(args) == "scripts/verify_ai_training.sh"
			},
		},
		{
			name: "schema",
			shouldFail: func(name string, args []string) bool {
				return name == "bash" && firstArgOrEmpty(args) == "scripts/verify_schema.sh"
			},
		},
		{
			name: "graphql coverage",
			shouldFail: func(name string, args []string) bool {
				return name == "go" && len(args) >= 2 && args[0] == "run" && args[1] == "./tools/graphql_coverage"
			},
		},
		{
			name: "openapi",
			shouldFail: func(name string, args []string) bool {
				return name == "go" && len(args) >= 2 && args[0] == "run" && args[1] == "./tools/openapi"
			},
		},
		{
			name: "coverage run",
			shouldFail: func(name string, args []string) bool {
				return name == "go" && firstArgOrEmpty(args) == "test" && strings.Contains(strings.Join(args, " "), "-coverprofile=coverage_overall.out")
			},
		},
		{
			name: "overall scoreboard",
			shouldFail: func(name string, args []string) bool {
				joined := strings.Join(args, " ")
				return name == "go" &&
					len(args) >= 2 &&
					args[0] == "run" &&
					args[1] == "./tools/coverage_scoreboard" &&
					!strings.Contains(joined, "--package")
			},
		},
		{
			name: "pkg scoreboard",
			shouldFail: func(name string, args []string) bool {
				joined := strings.Join(args, " ")
				return name == "go" &&
					len(args) >= 2 &&
					args[0] == "run" &&
					args[1] == "./tools/coverage_scoreboard" &&
					strings.Contains(joined, "--package "+pkgPrefix)
			},
		},
		{
			name: "cmd scoreboard",
			shouldFail: func(name string, args []string) bool {
				joined := strings.Join(args, " ")
				return name == "go" &&
					len(args) >= 2 &&
					args[0] == "run" &&
					args[1] == "./tools/coverage_scoreboard" &&
					strings.Contains(joined, "--package "+cmdPrefix)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
				if tt.shouldFail(name, args) {
					return errSentinel
				}
				return nil
			}

			require.ErrorIs(t, runVerifyCIContractsAndCoverage(cmdPrefix, pkgPrefix), errSentinel)
		})
	}
}

func TestReadVerifyCIModulePath_Round20_PropagatesRepoRootError(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
	})

	findRepoRootFn = func() (string, error) { return "", errSentinel }

	_, err := readVerifyCIModulePath()
	require.ErrorIs(t, err, errSentinel)
}

func TestWithTemporaryEnv_Round20_RestoresAfterError(t *testing.T) {
	t.Setenv("ROUND20_EXISTING_ENV", "before")
	require.NoError(t, os.Unsetenv("ROUND20_NEW_ENV"))

	err := withTemporaryEnv(map[string]string{
		"ROUND20_EXISTING_ENV": "after",
		"ROUND20_NEW_ENV":      "during",
	}, func() error {
		require.Equal(t, "after", os.Getenv("ROUND20_EXISTING_ENV"))
		require.Equal(t, "during", os.Getenv("ROUND20_NEW_ENV"))
		return errSentinel
	})

	require.ErrorIs(t, err, errSentinel)
	require.Equal(t, "before", os.Getenv("ROUND20_EXISTING_ENV"))
	_, ok := os.LookupEnv("ROUND20_NEW_ENV")
	require.False(t, ok)
}

func TestGoFlagsWithBuildParallelism_Round20(t *testing.T) {
	t.Run("no jobs trims existing flags", func(t *testing.T) {
		require.Equal(t, "-trimpath -race", goFlagsWithBuildParallelism("  -trimpath   -race  ", 0))
	})

	t.Run("replaces inline p flag", func(t *testing.T) {
		require.Equal(t, "-trimpath -p=2", goFlagsWithBuildParallelism("-trimpath -p=9", 2))
	})

	t.Run("replaces split p flag", func(t *testing.T) {
		require.Equal(t, "-trimpath -p=4 -mod=readonly", goFlagsWithBuildParallelism("-trimpath -p 9 -mod=readonly", 4))
	})

	t.Run("appends p flag when missing", func(t *testing.T) {
		require.Equal(t, "-trimpath -p=3", goFlagsWithBuildParallelism("-trimpath", 3))
	})
}

func TestRunVerifySupportCommands_Round20_ErrorPaths(t *testing.T) {
	t.Run("openapi propagates repo root error", func(t *testing.T) {
		previousRepoRoot := findRepoRootFn
		t.Cleanup(func() {
			findRepoRootFn = previousRepoRoot
		})

		findRepoRootFn = func() (string, error) { return "", errSentinel }
		require.ErrorIs(t, runVerifyOpenAPI(nil), errSentinel)
	})

	t.Run("openapi propagates missing go tool", func(t *testing.T) {
		_ = setupVerifyCIRound20Harness(t)

		ensureToolAvailableFn = func(name string) error {
			if name == "go" {
				return errSentinel
			}
			return nil
		}

		require.ErrorIs(t, runVerifyOpenAPI(nil), errSentinel)
	})

	t.Run("openapi propagates go cache error", func(t *testing.T) {
		repoRoot := setupVerifyCIRound20Harness(t)
		t.Setenv("GOCACHE", "")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))

		require.Error(t, runVerifyOpenAPI(nil))
	})

	t.Run("openapi propagates command failure", func(t *testing.T) {
		_ = setupVerifyCIRound20Harness(t)

		runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
			if name == "go" && len(args) >= 2 && args[0] == "run" && args[1] == "./tools/openapi" {
				return errSentinel
			}
			return nil
		}

		require.ErrorIs(t, runVerifyOpenAPI(nil), errSentinel)
	})

	t.Run("inventory propagates repo root error", func(t *testing.T) {
		previousRepoRoot := findRepoRootFn
		t.Cleanup(func() {
			findRepoRootFn = previousRepoRoot
		})

		findRepoRootFn = func() (string, error) { return "", errSentinel }
		require.ErrorIs(t, runVerifyInventory(nil), errSentinel)
	})

	t.Run("inventory propagates command failure", func(t *testing.T) {
		_ = setupVerifyCIRound20Harness(t)

		runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
			if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_inventory.sh" {
				return errSentinel
			}
			return nil
		}

		require.ErrorIs(t, runVerifyInventory(nil), errSentinel)
	})

	t.Run("lambda set propagates repo root error", func(t *testing.T) {
		previousRepoRoot := findRepoRootFn
		t.Cleanup(func() {
			findRepoRootFn = previousRepoRoot
		})

		findRepoRootFn = func() (string, error) { return "", errSentinel }
		require.ErrorIs(t, runVerifyLambdaSet(nil), errSentinel)
	})

	t.Run("lambda set propagates command failure", func(t *testing.T) {
		_ = setupVerifyCIRound20Harness(t)

		runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
			if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_lambda_set.sh" {
				return errSentinel
			}
			return nil
		}

		require.ErrorIs(t, runVerifyLambdaSet(nil), errSentinel)
	})
}

func TestRunVerifySupportCommands_Round20_SupplyChainAndAuditErrors(t *testing.T) {
	t.Run("supply chain propagates repo root error", func(t *testing.T) {
		previousRepoRoot := findRepoRootFn
		t.Cleanup(func() {
			findRepoRootFn = previousRepoRoot
		})

		findRepoRootFn = func() (string, error) { return "", errSentinel }
		require.ErrorIs(t, runVerifySupplyChain(nil), errSentinel)
	})

	t.Run("supply chain propagates command failure", func(t *testing.T) {
		_ = setupVerifyCIRound20Harness(t)

		runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
			if name == "bash" && firstArgOrEmpty(args) == "scripts/verify_supply_chain.sh" {
				return errSentinel
			}
			return nil
		}

		require.ErrorIs(t, runVerifySupplyChain(nil), errSentinel)
	})

	t.Run("audit propagates repo root error", func(t *testing.T) {
		previousRepoRoot := findRepoRootFn
		t.Cleanup(func() {
			findRepoRootFn = previousRepoRoot
		})

		findRepoRootFn = func() (string, error) { return "", errSentinel }
		require.ErrorIs(t, runVerifyAudit(nil), errSentinel)
	})

	t.Run("audit propagates command failure", func(t *testing.T) {
		_ = setupVerifyCIRound20Harness(t)

		runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
			if name == "go" && len(args) >= 2 && args[0] == "run" && args[1] == "./tools/audit_gates" {
				return errSentinel
			}
			return nil
		}

		require.ErrorIs(t, runVerifyAudit(nil), errSentinel)
	})

	t.Run("audit propagates go cache error", func(t *testing.T) {
		repoRoot := setupVerifyCIRound20Harness(t)
		t.Setenv("GOCACHE", "")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))

		require.Error(t, runVerifyAudit(nil))
	})
}
