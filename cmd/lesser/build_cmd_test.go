package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildCommands(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	previousRunCommand := runCommandFn
	previousBuildZips := buildLambdaZipsFn
	previousBuildCF := buildCloudfrontKeygenZipFn
	previousBuildAuthUI := buildAuthUIFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
		runCommandFn = previousRunCommand
		buildLambdaZipsFn = previousBuildZips
		buildCloudfrontKeygenZipFn = previousBuildCF
		buildAuthUIFn = previousBuildAuthUI
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }
	buildLambdaZipsFn = func(string, bool) error { return nil }
	buildCloudfrontKeygenZipFn = func(string, string) error { return nil }
	buildAuthUIFn = func(string) (string, error) { return "", nil }

	require.NoError(t, runBuild([]string{helpCommand}))
	require.NoError(t, runBuild(nil))
	require.NoError(t, runBuild([]string{valueAll}))
	require.NoError(t, runBuildAll(nil))
	require.NoError(t, runBuildAll([]string{valueAll}))
	require.NoError(t, runBuildLambdas([]string{"--rebuild"}))
}

func TestRunBuild_UnknownCommand(t *testing.T) {
	require.Error(t, runBuild([]string{"nope"}))
}

func TestRunBuild_DispatchesLambdaCommands(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	previousBuildZips := buildLambdaZipsFn
	previousBuild := buildLambdaBinaryFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
		buildLambdaZipsFn = previousBuildZips
		buildLambdaBinaryFn = previousBuild
		zipSingleFileFn = previousZip
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	buildLambdaZipsFn = func(string, bool) error { return nil }

	buildLambdaBinaryFn = func(_ string, _ string, _ string, outPath string) error {
		return os.WriteFile(outPath, []byte("bin"), 0o600)
	}
	zipSingleFileFn = func(zipPath string, _ string, _ string) error {
		return os.WriteFile(zipPath, []byte("zip"), 0o600)
	}

	require.NoError(t, runBuild([]string{"lambdas", "--rebuild"}))
	require.NoError(t, runBuild([]string{"lambda", "demo"}))
}

func TestRunBuildAll_PropagatesDependencyErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	previousBuildZips := buildLambdaZipsFn
	previousBuildCF := buildCloudfrontKeygenZipFn
	previousBuildAuthUI := buildAuthUIFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
		buildLambdaZipsFn = previousBuildZips
		buildCloudfrontKeygenZipFn = previousBuildCF
		buildAuthUIFn = previousBuildAuthUI
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	t.Run("missing tool", func(t *testing.T) {
		ensureToolAvailableFn = func(string) error { return errors.New("missing tool") }
		require.Error(t, runBuildAll(nil))
	})

	t.Run("lambda build failure", func(t *testing.T) {
		ensureToolAvailableFn = func(string) error { return nil }
		buildLambdaZipsFn = func(string, bool) error { return errors.New("zip fail") }
		require.Error(t, runBuildAll(nil))
	})

	t.Run("auth ui build failure", func(t *testing.T) {
		buildLambdaZipsFn = func(string, bool) error { return nil }
		buildCloudfrontKeygenZipFn = func(string, string) error { return nil }
		buildAuthUIFn = func(string) (string, error) { return "", errors.New("ui fail") }
		require.Error(t, runBuildAll(nil))
	})
}

func TestRunBuildLambdas_FailsWhenGoMissing(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	ensureToolAvailableFn = func(string) error { return errors.New("no go") }
	require.Error(t, runBuildLambdas(nil))
}

func TestRunBuildLambdas_ParseError(t *testing.T) {
	require.Error(t, runBuildLambdas([]string{"--badflag"}))
}

func TestRunBuildSingleLambda_UsageErrors(t *testing.T) {
	require.Error(t, runBuildSingleLambda(nil))
	require.Error(t, runBuildSingleLambda([]string{"a", "b"}))
	require.Error(t, runBuildSingleLambda([]string{"   "}))
}

func TestRunBuildSingleLambda_PropagatesBuildAndZipErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	previousBuild := buildLambdaBinaryFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
		buildLambdaBinaryFn = previousBuild
		zipSingleFileFn = previousZip
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	buildLambdaBinaryFn = func(string, string, string, string) error { return errors.New("build failed") }
	require.Error(t, runBuildSingleLambda([]string{"demo"}))

	buildLambdaBinaryFn = func(_ string, _ string, _ string, outPath string) error {
		return os.WriteFile(outPath, []byte("bin"), 0o600)
	}
	zipSingleFileFn = func(string, string, string) error { return errors.New("zip failed") }
	require.Error(t, runBuildSingleLambda([]string{"demo"}))
}

func TestRunBuildSingleLambda_FailsWhenGoCacheDirCannotBeCreated(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	tmp := t.TempDir()
	repoRootFile := filepath.Join(tmp, "repo")
	require.NoError(t, os.WriteFile(repoRootFile, []byte("x"), 0o600))

	findRepoRootFn = func() (string, error) { return repoRootFile, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	require.Error(t, runBuildSingleLambda([]string{"demo"}))
}

func TestBuildCloudfrontKeygenZip_PropagatesErrors(t *testing.T) {
	previousRunCommand := runCommandFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		zipSingleFileFn = previousZip
	})

	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "tmp", "go-cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	runCommandFn = func(context.Context, string, []string, execOptions) error {
		return errors.New("go build failed")
	}
	require.Error(t, buildCloudfrontKeygenZip(repoRoot, cacheDir))

	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		bootstrapPath := ""
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-o" {
				bootstrapPath = args[i+1]
				break
			}
		}
		return os.WriteFile(bootstrapPath, []byte("bin"), 0o600)
	}
	zipSingleFileFn = func(string, string, string) error { return errors.New("zip failed") }
	require.Error(t, buildCloudfrontKeygenZip(repoRoot, cacheDir))
}

func TestRunBuildSingleLambda_SkipsExistingWhenRebuildFalse(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	zipPath := filepath.Join(repoRoot, "bin", "demo.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(zipPath), 0o755))
	require.NoError(t, os.WriteFile(zipPath, []byte("zip"), 0o600))

	require.NoError(t, runBuildSingleLambda([]string{"--rebuild=false", "demo"}))
}

func TestRunBuildSingleLambda_BuildsWhenRebuildTrue(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	previousBuild := buildLambdaBinaryFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
		buildLambdaBinaryFn = previousBuild
		zipSingleFileFn = previousZip
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	buildLambdaBinaryFn = func(_ string, _ string, _ string, outPath string) error {
		return os.WriteFile(outPath, []byte("bin"), 0o600)
	}
	var zipped bool
	zipSingleFileFn = func(zipPath string, _ string, _ string) error {
		zipped = true
		return os.WriteFile(zipPath, []byte("zip"), 0o600)
	}

	require.NoError(t, runBuildSingleLambda([]string{"demo"}))
	require.True(t, zipped)
}

func TestBuildCloudfrontKeygenZip_UsesInjectedCommandAndZipper(t *testing.T) {
	previousRunCommand := runCommandFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		zipSingleFileFn = previousZip
	})

	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "tmp", "go-cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		bootstrapPath := ""
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-o" {
				bootstrapPath = args[i+1]
				break
			}
		}
		if bootstrapPath == "" {
			return nil
		}
		return os.WriteFile(bootstrapPath, []byte("bin"), 0o600)
	}
	zipSingleFileFn = func(zipPath string, _ string, _ string) error {
		return os.WriteFile(zipPath, []byte("zip"), 0o600)
	}

	require.NoError(t, buildCloudfrontKeygenZip(repoRoot, cacheDir))
	require.FileExists(t, filepath.Join(repoRoot, "bin", "cloudfront-keygen.zip"))
}

func TestRunBuildAll_PropagatesGoCacheAndGoBuildErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousEnsureTool := ensureToolAvailableFn
	previousRunCommand := runCommandFn
	previousBuildZips := buildLambdaZipsFn
	previousBuildCF := buildCloudfrontKeygenZipFn
	previousBuildAuthUI := buildAuthUIFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		ensureToolAvailableFn = previousEnsureTool
		runCommandFn = previousRunCommand
		buildLambdaZipsFn = previousBuildZips
		buildCloudfrontKeygenZipFn = previousBuildCF
		buildAuthUIFn = previousBuildAuthUI
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	buildLambdaZipsFn = func(string, bool) error { return nil }
	buildCloudfrontKeygenZipFn = func(string, string) error { return nil }
	buildAuthUIFn = func(string) (string, error) { return "", nil }

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tmp"), []byte("x"), 0o644))
	require.Error(t, runBuildAll(nil))

	_ = os.Remove(filepath.Join(repoRoot, "tmp"))
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }
	require.ErrorIs(t, runBuildAll(nil), errSentinel)
}
