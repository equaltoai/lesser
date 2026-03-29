package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/releaseassets"
	"github.com/stretchr/testify/require"
)

func TestRunUp_UsesVerifiedReleaseDirWithoutBuildingLambdas(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	restore := stubRunUpReleaseArtifactDeps(t, targetRepo)
	defer restore()

	buildCalled := false
	buildLambdaZipsFn = func(string, bool) error {
		buildCalled = true
		return nil
	}

	require.NoError(t, runUp([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--release-dir", releaseDir,
	}))
	require.False(t, buildCalled)

	apiBytes, err := os.ReadFile(filepath.Join(targetRepo, "bin", "api.zip"))
	require.NoError(t, err)
	require.Equal(t, "api zip", string(apiBytes))

	inboxBytes, err := os.ReadFile(filepath.Join(targetRepo, "bin", "inbox.zip"))
	require.NoError(t, err)
	require.Equal(t, "inbox zip", string(inboxBytes))
}

func TestRunUp_ReleaseDirErrorsDoNotFallbackToBuild(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	require.NoError(t, os.Remove(filepath.Join(releaseDir, releaseassets.LambdaBundleManifestName)))

	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})
	restore := stubRunUpReleaseArtifactDeps(t, targetRepo)
	defer restore()

	buildCalled := false
	buildLambdaZipsFn = func(string, bool) error {
		buildCalled = true
		return nil
	}
	cdkBootstrapFn = func(context.Context, string, string, string, string) error {
		t.Fatal("cdk bootstrap should not run when release assets fail validation")
		return nil
	}

	err := runUp([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--release-dir", releaseDir,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "required release file lesser-lambda-bundle.json")
	require.False(t, buildCalled)
}

func stubRunUpReleaseArtifactDeps(t *testing.T, repoRoot string) func() {
	t.Helper()

	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousLoadAWS := loadAWSConfigForCLIFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	previousTools := ensureToolsAvailableFn
	previousBuildZips := buildLambdaZipsFn
	previousInstallRelease := installReleaseLambdaAssetsFn
	previousWriteBootstrap := writeBootstrapKeyMaterialFn
	previousBootstrap := ensureStageBootstrapStateFn
	previousCdkBootstrap := cdkBootstrapFn
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousCdkDeploy := cdkDeployWithOutputsFn
	previousBuildAuthUI := buildAuthUIFn
	previousReplaceBucket := replaceBucketWithDirPrefixFn
	previousInvalidate := invalidateFrontendFn
	previousWriteReceipt := writeReceiptFn

	homeDir := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }
	userHomeDirFn = func() (string, error) { return homeDir, nil }
	loadAWSConfigForCLIFn = func(_ context.Context, profile string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, profile, nil
	}
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "0xabc", false, nil
	}
	ensureToolsAvailableFn = func() error { return nil }
	buildLambdaZipsFn = func(string, bool) error { return nil }
	installReleaseLambdaAssetsFn = installReleaseLambdaAssets
	writeBootstrapKeyMaterialFn = func(string, bootstrapWallet) error { return nil }
	ensureStageBootstrapStateFn = func(context.Context, bootstrapDBFactory, string, naming.Stage, string) (stageBootstrapState, error) {
		return stageBootstrapState{Locked: true, Address: "0xabc"}, nil
	}
	cdkBootstrapFn = func(context.Context, string, string, string, string) error { return nil }
	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }
	cdkDeployWithOutputsFn = func(_ context.Context, _ string, _ string, req cdkDeployRequest) (cdkDeployResult, error) {
		return cdkDeployResult{StackName: req.StackName, Outputs: map[string]string{}}, nil
	}
	buildAuthUIFn = func(string) (string, error) { return t.TempDir(), nil }
	replaceBucketWithDirPrefixFn = func(context.Context, s3BucketUploaderAPI, string, string, string) error { return nil }
	invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }
	writeReceiptFn = func(string, *upReceipt) error { return nil }

	return func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		loadAWSConfigForCLIFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
		ensureToolsAvailableFn = previousTools
		buildLambdaZipsFn = previousBuildZips
		installReleaseLambdaAssetsFn = previousInstallRelease
		writeBootstrapKeyMaterialFn = previousWriteBootstrap
		ensureStageBootstrapStateFn = previousBootstrap
		cdkBootstrapFn = previousCdkBootstrap
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		cdkDeployWithOutputsFn = previousCdkDeploy
		buildAuthUIFn = previousBuildAuthUI
		replaceBucketWithDirPrefixFn = previousReplaceBucket
		invalidateFrontendFn = previousInvalidate
		writeReceiptFn = previousWriteReceipt
	}
}
