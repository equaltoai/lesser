package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/equaltoai/lesser/pkg/releaseassets"
	"github.com/stretchr/testify/require"
)

func TestVerifyPublishedReleaseAssets(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	require.NoError(t, verifyPublishedReleaseAssets(releaseDir))
}

func TestVerifyPublishedReleaseAssets_FailsWhenChecksumDoesNotMatchBinary(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, "lesser-linux-amd64"), []byte("tampered"), 0o755))

	err := verifyPublishedReleaseAssets(releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lesser-linux-amd64 checksum mismatch")
}

func TestRunVerifyArtifactDeploy_StagesReleaseAndInvokesCertifier(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	previousRepoRoot := findRepoRootFn
	previousCertifier := runArtifactDeployCertificationFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runArtifactDeployCertificationFn = previousCertifier
	})

	findRepoRootFn = func() (string, error) { return targetRepo, nil }

	var capturedAssetRoot string
	runArtifactDeployCertificationFn = func(_ string, assetRoot string) error {
		capturedAssetRoot = assetRoot
		apiBytes, err := os.ReadFile(filepath.Join(assetRoot, "bin", "api.zip"))
		require.NoError(t, err)
		require.Equal(t, "api zip", string(apiBytes))
		inboxBytes, err := os.ReadFile(filepath.Join(assetRoot, "bin", "inbox.zip"))
		require.NoError(t, err)
		require.Equal(t, "inbox zip", string(inboxBytes))
		return nil
	}

	require.NoError(t, runVerifyArtifactDeploy([]string{"--release-dir", releaseDir}))
	require.NotEmpty(t, capturedAssetRoot)
}

func TestRunVerifyArtifactDeploy_RequiresReleaseDir(t *testing.T) {
	err := runVerifyArtifactDeploy(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--release-dir")
}

func TestRunVerify_DispatchesArtifactDeploySubcommand(t *testing.T) {
	previous := runVerifyArtifactDeployFn
	t.Cleanup(func() { runVerifyArtifactDeployFn = previous })

	var got []string
	runVerifyArtifactDeployFn = func(argv []string) error {
		got = append([]string(nil), argv...)
		return nil
	}

	require.NoError(t, runVerify([]string{"artifact-deploy", "--release-dir", "dist/release"}))
	require.Equal(t, []string{"--release-dir", "dist/release"}, got)
}

func TestVerifyPublishedReleaseAssets_FailsWhenChecksumEntryMissing(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	checksumPath := filepath.Join(releaseDir, releaseassets.ChecksumsFileName)
	require.NoError(t, os.WriteFile(checksumPath, []byte(""), 0o644))

	err := verifyPublishedReleaseAssets(releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksums.txt missing entry for lesser-linux-amd64")
}

func TestParseVerifyArtifactDeployArgs_NormalizesReleaseDir(t *testing.T) {
	releaseDir := t.TempDir()

	args, err := parseVerifyArtifactDeployArgs([]string{"--release-dir", releaseDir})
	require.NoError(t, err)
	require.Equal(t, releaseDir, args.ReleaseDir)
}

func TestParseVerifyArtifactDeployArgs_RejectsUnknownFlag(t *testing.T) {
	_, err := parseVerifyArtifactDeployArgs([]string{"--unknown"})
	require.Error(t, err)
}

func TestParseVerifyArtifactDeployArgs_RejectsInvalidReleaseDir(t *testing.T) {
	_, err := parseVerifyArtifactDeployArgs([]string{"--release-dir", filepath.Join(t.TempDir(), "missing")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat --release-dir")
}

func TestRunArtifactDeployCertification_RunsGoCertifier(t *testing.T) {
	repoRoot := t.TempDir()

	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	ensureToolAvailableFn = func(string) error { return nil }

	var gotName string
	var gotArgs []string
	var gotOpts execOptions
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		gotOpts = opts
		return nil
	}

	require.NoError(t, runArtifactDeployCertification(repoRoot, "/tmp/lambda-assets"))
	require.Equal(t, "go", gotName)
	require.Equal(t, []string{"run", "./cmd/artifact_deploy_certify", "--asset-root", "/tmp/lambda-assets"}, gotArgs)
	require.Equal(t, filepath.Join(repoRoot, "infra", "cdk"), gotOpts.Dir)
	require.NotEmpty(t, gotOpts.Env["GOCACHE"])
	require.NotEmpty(t, gotOpts.Env["XDG_CACHE_HOME"])
}

func TestRunArtifactDeployCertification_PropagatesEnsureToolError(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() { ensureToolAvailableFn = previousEnsureTool })

	ensureToolAvailableFn = func(string) error { return errors.New("missing go") }
	err := runArtifactDeployCertification(t.TempDir(), "/tmp/lambda-assets")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing go")
}

func TestRunArtifactDeployCertification_PropagatesCommandError(t *testing.T) {
	repoRoot := t.TempDir()

	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	ensureToolAvailableFn = func(string) error { return nil }
	runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }

	err := runArtifactDeployCertification(repoRoot, "/tmp/lambda-assets")
	require.ErrorIs(t, err, errSentinel)
}

func TestRunArtifactDeployCertification_PropagatesGoCacheError(t *testing.T) {
	repoRoot := t.TempDir()
	blockingPath := filepath.Join(repoRoot, "tmp")
	require.NoError(t, os.WriteFile(blockingPath, []byte("block"), 0o644))
	t.Setenv("GOCACHE", "")
	t.Setenv("XDG_CACHE_HOME", "")

	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() { ensureToolAvailableFn = previousEnsureTool })

	ensureToolAvailableFn = func(string) error { return nil }

	err := runArtifactDeployCertification(repoRoot, "/tmp/lambda-assets")
	require.Error(t, err)
	require.Contains(t, err.Error(), "create go-cache dir")
}

func TestRunVerifyArtifactDeploy_PropagatesCertifierError(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	previousRepoRoot := findRepoRootFn
	previousCertifier := runArtifactDeployCertificationFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runArtifactDeployCertificationFn = previousCertifier
	})

	findRepoRootFn = func() (string, error) { return targetRepo, nil }
	runArtifactDeployCertificationFn = func(string, string) error { return errSentinel }

	err := runVerifyArtifactDeploy([]string{"--release-dir", releaseDir})
	require.ErrorIs(t, err, errSentinel)
}

func TestRunVerifyArtifactDeploy_PropagatesRepoRootError(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

	findRepoRootFn = func() (string, error) { return "", errSentinel }
	err := runVerifyArtifactDeploy([]string{"--release-dir", t.TempDir()})
	require.ErrorIs(t, err, errSentinel)
}

func TestStageReleaseAssetsForArtifactDeployVerification_CleanupRemovesWorkspace(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	assetRoot, cleanup, err := stageReleaseAssetsForArtifactDeployVerification(targetRepo, releaseDir)
	require.NoError(t, err)
	require.DirExists(t, filepath.Dir(assetRoot))

	cleanup()
	_, statErr := os.Stat(filepath.Dir(assetRoot))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestStageReleaseAssetsForArtifactDeployVerification_PropagatesInstallError(t *testing.T) {
	previousInstall := installReleaseLambdaAssetsFn
	t.Cleanup(func() { installReleaseLambdaAssetsFn = previousInstall })

	installReleaseLambdaAssetsFn = func(string, string, string) (releaseLambdaInstallResult, error) {
		return releaseLambdaInstallResult{}, errSentinel
	}

	assetRoot, cleanup, err := stageReleaseAssetsForArtifactDeployVerification(t.TempDir(), t.TempDir())
	require.ErrorIs(t, err, errSentinel)
	require.Empty(t, assetRoot)
	require.Nil(t, cleanup)
}
