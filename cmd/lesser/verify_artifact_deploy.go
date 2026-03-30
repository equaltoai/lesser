package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

const artifactDeployCertifierPackage = "./cmd/artifact_deploy_certify"

var (
	runVerifyArtifactDeployFn        = runVerifyArtifactDeploy
	runArtifactDeployCertificationFn = runArtifactDeployCertification
)

type verifyArtifactDeployArgs struct {
	ReleaseDir string
}

func runVerifyArtifactDeploy(argv []string) error {
	args, err := parseVerifyArtifactDeployArgs(argv)
	if err != nil {
		return err
	}

	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}

	if err := verifyPublishedReleaseAssets(args.ReleaseDir); err != nil {
		return err
	}

	assetRoot, cleanup, err := stageReleaseAssetsForArtifactDeployVerification(repoRoot, args.ReleaseDir)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := runArtifactDeployCertificationFn(repoRoot, assetRoot); err != nil {
		return err
	}

	fmt.Printf("✓ artifact-driven deploy certification passed for %s\n", args.ReleaseDir)
	return nil
}

func parseVerifyArtifactDeployArgs(argv []string) (verifyArtifactDeployArgs, error) {
	fs := flag.NewFlagSet("lesser verify artifact-deploy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args verifyArtifactDeployArgs
	fs.StringVar(&args.ReleaseDir, "release-dir", "", "directory containing published release assets")
	if err := fs.Parse(argv); err != nil {
		return verifyArtifactDeployArgs{}, err
	}

	releaseDir, err := normalizeReleaseDir(args.ReleaseDir)
	if err != nil {
		return verifyArtifactDeployArgs{}, err
	}
	if releaseDir == "" {
		return verifyArtifactDeployArgs{}, fmt.Errorf("artifact deploy verification requires --release-dir")
	}
	args.ReleaseDir = releaseDir

	return args, nil
}

func verifyPublishedReleaseAssets(releaseDir string) error {
	files, err := requiredReleaseFiles(releaseDir)
	if err != nil {
		return err
	}

	checksums, err := readReleaseChecksums(files.checksumsPath)
	if err != nil {
		return err
	}

	for _, assetName := range releaseassets.PublishedReleaseAssetNames() {
		expectedSHA, ok := checksums[assetName]
		if !ok {
			return fmt.Errorf("checksums.txt missing entry for %s", assetName)
		}
		if err := verifyFileChecksum(filepath.Join(releaseDir, assetName), expectedSHA, assetName); err != nil {
			return err
		}
	}

	return nil
}

func stageReleaseAssetsForArtifactDeployVerification(repoRoot string, releaseDir string) (string, func(), error) {
	workRoot, err := os.MkdirTemp("", "lesser-artifact-deploy.")
	if err != nil {
		return "", nil, fmt.Errorf("create artifact deploy verification workspace: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(workRoot)
	}

	assetRoot := filepath.Join(workRoot, "lambda-assets")
	result, err := installReleaseLambdaAssetsFn(repoRoot, releaseDir, assetRoot)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	fmt.Printf("✓ Staged %d Lambda artifact(s) from release %s into %s\n", len(result.Files), result.Version, assetRoot)
	return assetRoot, cleanup, nil
}

func runArtifactDeployCertification(repoRoot string, assetRoot string) error {
	if err := ensureToolAvailableFn("go"); err != nil {
		return err
	}

	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}
	xdgCache, err := ensureXDGCacheDir(repoRoot)
	if err != nil {
		return err
	}

	return runCommandFn(context.Background(), "go", []string{
		"run",
		artifactDeployCertifierPackage,
		"--asset-root", assetRoot,
	}, execOptions{
		Dir: filepath.Join(repoRoot, "infra", "cdk"),
		Env: map[string]string{
			"GOCACHE":        goCache,
			"XDG_CACHE_HOME": xdgCache,
		},
	})
}
