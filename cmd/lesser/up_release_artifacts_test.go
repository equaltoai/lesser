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
	t.Setenv("VAPID_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:vapid-live")

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

	homeDir, err := userHomeDirFn()
	require.NoError(t, err)
	lambdaAssetRoot := filepath.Join(homeDir, ".lesser", "app", "example.com", "deploy", "lambda-assets")

	apiBytes, err := os.ReadFile(filepath.Join(lambdaAssetRoot, "bin", "api.zip"))
	require.NoError(t, err)
	require.Equal(t, "api zip", string(apiBytes))

	inboxBytes, err := os.ReadFile(filepath.Join(lambdaAssetRoot, "bin", "inbox.zip"))
	require.NoError(t, err)
	require.Equal(t, "inbox zip", string(inboxBytes))
}

func TestRunUp_ReleaseDirErrorsDoNotFallbackToBuild(t *testing.T) {
	t.Setenv("VAPID_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:vapid-live")

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
	deployCloudFormationStackFn = func(context.Context, aws.Config, cloudFormationDeployRequest) (map[string]string, error) {
		t.Fatal("cloudformation deploy should not run when release assets fail validation")
		return nil, nil
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

func TestRunUp_ReleaseDirDeploysSharedAndStageStacksViaCDK(t *testing.T) {
	t.Setenv("VAPID_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:vapid-live")

	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})

	restore := stubRunUpReleaseArtifactDeps(t, targetRepo)
	defer restore()

	_, err := os.Stat(filepath.Join(targetRepo, "bin"))
	require.ErrorIs(t, err, os.ErrNotExist)

	buildLambdaZipsFn = func(string, bool) error {
		t.Fatal("buildLambdaZips should not run when --release-dir is provided")
		return nil
	}

	cdkBootstrapCalled := false
	cdkBootstrapFn = func(context.Context, string, string, string, string) error {
		cdkBootstrapCalled = true
		return nil
	}
	var requests []cdkDeployRequest
	cdkDeployWithOutputsFn = func(_ context.Context, repoRoot string, profile string, req cdkDeployRequest) (cdkDeployResult, error) {
		require.Equal(t, targetRepo, repoRoot)
		require.Equal(t, "profile", profile)
		requests = append(requests, req)
		return cdkDeployResult{
			StackName: req.StackName,
			Outputs: map[string]string{
				"AuthUIBucketName":       "auth-ui-bucket",
				"FrontendDistributionId": "DIST",
			},
		}, nil
	}
	deployCloudFormationStackFn = func(context.Context, aws.Config, cloudFormationDeployRequest) (map[string]string, error) {
		t.Fatal("release-dir deploy should not execute release assembly stacks through CloudFormation directly")
		return nil, nil
	}
	uploadReleaseAssemblyAssetsFn = func(context.Context, aws.Config, string, string, string, string, releaseDeployAssemblyInstallResult) (releaseAssemblyUploadResult, error) {
		t.Fatal("release-dir deploy should not upload release assembly stage templates")
		return releaseAssemblyUploadResult{}, nil
	}

	require.NoError(t, runUp([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--release-dir", releaseDir,
	}))

	require.True(t, cdkBootstrapCalled)
	require.Len(t, requests, 3)

	require.Equal(t, naming.SharedStackName("app"), requests[0].StackName)
	require.Equal(t, "app", requests[0].App)
	require.Equal(t, "example.com", requests[0].BaseDomain)
	require.Equal(t, "Z1", requests[0].HostedZoneID)
	require.NotEmpty(t, requests[0].LambdaAssetRoot)
	require.Equal(t, string(naming.StageShared), requests[0].StageFilter)
	require.False(t, requests[0].WithStaging)
	require.Equal(t, "v1.2.3", requests[0].Contexts["lesserVersion"])

	for _, req := range requests[1:] {
		require.Equal(t, "app", req.App)
		require.Equal(t, "example.com", req.BaseDomain)
		require.Equal(t, "Z1", req.HostedZoneID)
		require.Equal(t, requests[0].LambdaAssetRoot, req.LambdaAssetRoot)
		require.Equal(t, "v1.2.3", req.Contexts["lesserVersion"])
		require.NotEqual(t, string(naming.StageShared), req.StageFilter)
	}
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
	previousReleaseTools := ensureReleaseDeployToolsAvailableFn
	previousBuildZips := buildLambdaZipsFn
	previousInstallRelease := installReleaseDeployAssetsFn
	previousWriteBootstrap := writeBootstrapKeyMaterialFn
	previousBootstrap := ensureStageBootstrapStateFn
	previousCdkBootstrap := cdkBootstrapFn
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousCdkDeploy := cdkDeployWithOutputsFn
	previousCloudFormationDeploy := deployCloudFormationStackFn
	previousUploadAssembly := uploadReleaseAssemblyAssetsFn
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
	ensureReleaseDeployToolsAvailableFn = func() error { return nil }
	buildLambdaZipsFn = func(string, bool) error { return nil }
	installReleaseDeployAssetsFn = installReleaseDeployAssets
	writeBootstrapKeyMaterialFn = func(string, bootstrapWallet) error { return nil }
	ensureStageBootstrapStateFn = func(context.Context, bootstrapDBFactory, string, naming.Stage, string) (stageBootstrapState, error) {
		return stageBootstrapState{Locked: true, Address: "0xabc"}, nil
	}
	cdkBootstrapFn = func(context.Context, string, string, string, string) error { return nil }
	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }
	cdkDeployWithOutputsFn = func(_ context.Context, _ string, _ string, req cdkDeployRequest) (cdkDeployResult, error) {
		return cdkDeployResult{StackName: req.StackName, Outputs: map[string]string{}}, nil
	}
	deployCloudFormationStackFn = func(_ context.Context, _ aws.Config, req cloudFormationDeployRequest) (map[string]string, error) {
		if req.StackName == naming.SharedStackName("app") {
			return map[string]string{"ReleaseAssetBucketName": "app-shared-release-assets"}, nil
		}
		return map[string]string{
			"AuthUIBucketName":           "auth-ui-bucket",
			"FrontendDistributionId":     "DIST",
			"FrontendDistributionDomain": "dist.example.com",
		}, nil
	}
	uploadReleaseAssemblyAssetsFn = func(context.Context, aws.Config, string, string, string, string, releaseDeployAssemblyInstallResult) (releaseAssemblyUploadResult, error) {
		t.Fatal("release assembly execution should not run in release-dir mode")
		return releaseAssemblyUploadResult{}, nil
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
		ensureReleaseDeployToolsAvailableFn = previousReleaseTools
		buildLambdaZipsFn = previousBuildZips
		installReleaseDeployAssetsFn = previousInstallRelease
		writeBootstrapKeyMaterialFn = previousWriteBootstrap
		ensureStageBootstrapStateFn = previousBootstrap
		cdkBootstrapFn = previousCdkBootstrap
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		cdkDeployWithOutputsFn = previousCdkDeploy
		deployCloudFormationStackFn = previousCloudFormationDeploy
		uploadReleaseAssemblyAssetsFn = previousUploadAssembly
		buildAuthUIFn = previousBuildAuthUI
		replaceBucketWithDirPrefixFn = previousReplaceBucket
		invalidateFrontendFn = previousInvalidate
		writeReceiptFn = previousWriteReceipt
	}
}
