package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyCIHelpers_Round21_EnvBranches(t *testing.T) {
	t.Run("verify ci env overrides resource defaults", func(t *testing.T) {
		t.Setenv(lesserVerifyCIGOMEMLIMITEnv, "1024MiB")
		t.Setenv(lesserVerifyCIGOGCEnv, "25")
		t.Setenv(goMemoryLimitEnvVar, "")
		t.Setenv(goGCEnvVar, "")

		require.Equal(t, "1024MiB", resolveVerifyCIGOMEMLIMIT())
		require.Equal(t, "25", resolveVerifyCIGOGC())
	})

	t.Run("existing process env suppresses verify ci defaults", func(t *testing.T) {
		t.Setenv(lesserVerifyCIGOMEMLIMITEnv, "")
		t.Setenv(lesserVerifyCIGOGCEnv, "")
		t.Setenv(goMemoryLimitEnvVar, "2048MiB")
		t.Setenv(goGCEnvVar, "75")

		require.Equal(t, "", resolveVerifyCIGOMEMLIMIT())
		require.Equal(t, "", resolveVerifyCIGOGC())
	})

	t.Run("defaults and helper fallbacks apply when env is absent", func(t *testing.T) {
		t.Setenv(lesserVerifyCIGOMEMLIMITEnv, "")
		t.Setenv(lesserVerifyCIGOGCEnv, "")
		t.Setenv(goMemoryLimitEnvVar, "")
		t.Setenv(goGCEnvVar, "")
		t.Setenv("ROUND21_TRUTHY", "yes")
		t.Setenv("ROUND21_FALSEY", "off")

		require.Equal(t, defaultVerifyCIGOMEMLIMIT, resolveVerifyCIGOMEMLIMIT())
		require.Equal(t, defaultVerifyCIGOGC, resolveVerifyCIGOGC())
		require.True(t, envVarTruthy("ROUND21_TRUTHY"))
		require.False(t, envVarTruthy("ROUND21_FALSEY"))
		require.Equal(t, "override", effectiveOverrideValue(map[string]string{"key": "override"}, "key", "fallback"))
		require.Equal(t, "fallback", effectiveOverrideValue(map[string]string{}, "key", "fallback"))
		require.Equal(t, "unset", effectiveOverrideValue(map[string]string{}, "key", ""))
	})
}

func TestRunGoPackageSecurityTool_Round21_Fallbacks(t *testing.T) {
	previousCapture := captureCommandOutputFn
	previousRun := runCommandFn
	t.Cleanup(func() {
		captureCommandOutputFn = previousCapture
		runCommandFn = previousRun
	})

	repoRoot := t.TempDir()
	env := map[string]string{"GOCACHE": t.TempDir()}

	t.Run("disabled batching scans whole repo", func(t *testing.T) {
		var gotName string
		var gotArgs []string
		runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil
		}

		require.NoError(t, runGoPackageSecurityTool("govulncheck", nil, repoRoot, env, 0))
		require.Equal(t, "govulncheck", gotName)
		require.Equal(t, []string{"./..."}, gotArgs)
	})

	t.Run("empty package discovery falls back to whole repo", func(t *testing.T) {
		captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
			return "", nil
		}

		var gotName string
		var gotArgs []string
		runCommandFn = func(_ context.Context, name string, args []string, _ execOptions) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil
		}

		require.NoError(t, runGoPackageSecurityTool("gosec", []string{"-quiet"}, repoRoot, env, 2))
		require.Equal(t, "gosec", gotName)
		require.Equal(t, []string{"-quiet", "./..."}, gotArgs)
	})

	t.Run("package discovery errors are returned", func(t *testing.T) {
		captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
			return "", errSentinel
		}

		runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error {
			t.Fatal("runCommandFn should not be called when package discovery fails")
			return nil
		}

		require.ErrorIs(t, runGoPackageSecurityTool("govulncheck", nil, repoRoot, env, 2), errSentinel)
	})
}

func TestListGoPackageDirsForLint_Round21_NormalizesUniqueDirectories(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() {
		captureCommandOutputFn = previousCapture
	})

	repoRoot := t.TempDir()
	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return filepath.Join(repoRoot, "pkg", "common") + "\n" +
			repoRoot + "\n" +
			filepath.Join(repoRoot, "pkg", "common") + "\n", nil
	}

	dirs, err := listGoPackageDirsForLint(repoRoot, map[string]string{"GOCACHE": t.TempDir()})
	require.NoError(t, err)
	require.Equal(t, []string{".", "./pkg/common"}, dirs)
}

func TestRunLintInBatches_Round21_PropagatesDiscoveryErrors(t *testing.T) {
	previousCapture := captureCommandOutputFn
	previousRun := runCommandFn
	t.Cleanup(func() {
		captureCommandOutputFn = previousCapture
		runCommandFn = previousRun
	})

	captureCommandOutputFn = func(_ context.Context, _ string, _ map[string]string, _ string, _ ...string) (string, error) {
		return "", errSentinel
	}
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error {
		t.Fatal("runCommandFn should not be called when lint discovery fails")
		return nil
	}

	require.ErrorIs(t, runLintInBatches(t.TempDir(), []string{"run", "--config", ".golangci.yml"}, map[string]string{"GOCACHE": t.TempDir()}, 2), errSentinel)
}
