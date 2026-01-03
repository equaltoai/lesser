package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestParseStageSelection(t *testing.T) {
	stages, err := parseStageSelection("dev")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev}, stages)

	stages, err = parseStageSelection("staging")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageStaging}, stages)

	stages, err = parseStageSelection("live")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageLive}, stages)

	stages, err = parseStageSelection("")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageLive}, stages)

	stages, err = parseStageSelection("both")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageLive}, stages)

	stages, err = parseStageSelection("all")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive}, stages)

	_, err = parseStageSelection("nope")
	require.Error(t, err)
}

func TestParseClientDeployArgs_RequiresFlags(t *testing.T) {
	_, err := parseClientDeployArgs(nil)
	require.Error(t, err)

	_, err = parseClientDeployArgs([]string{"--app", "app"})
	require.Error(t, err)
}

func TestRunClientDeploy_HappyPathWithInjectedAWSDeps(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousReplace := replaceBucketWithDirFn
	previousInvalidate := invalidateClientPathsFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		loadAWSConfigFromProfileFn = previousLoadAWS
		replaceBucketWithDirFn = previousReplace
		invalidateClientPathsFn = previousInvalidate
	})

	ensureToolAvailableFn = func(string) error { return nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	var uploaded []string
	replaceBucketWithDirFn = func(_ context.Context, _ s3BucketUploaderAPI, bucket string, dir string) error {
		uploaded = append(uploaded, bucket+"@"+dir)
		return nil
	}
	var invalidated []string
	invalidateClientPathsFn = func(_ context.Context, _ *cloudfront.Client, distributionID string) error {
		invalidated = append(invalidated, distributionID)
		return nil
	}

	distDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html/>"), 0o644))

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		[]naming.Stage{naming.StageDev, naming.StageLive},
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages["dev"].StackOutputs = map[string]string{
		"ClientBucketName":       "bucket-dev",
		"FrontendDistributionId": "DIST-DEV",
	}
	receipt.Stages["live"].StackOutputs = map[string]string{
		"ClientBucketName":       "bucket-live",
		"FrontendDistributionId": "DIST-LIVE",
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	err := runClientDeploy([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--dist", distDir,
		"--stage", "both",
		"--state", statePath,
	})
	require.NoError(t, err)
	require.Len(t, uploaded, 2)
	require.Equal(t, []string{"DIST-DEV", "DIST-LIVE"}, invalidated)
}

func TestInvalidateClientPaths_PropagatesErrors(t *testing.T) {
	previous := createCloudfrontInvalidationFn
	t.Cleanup(func() { createCloudfrontInvalidationFn = previous })

	createCloudfrontInvalidationFn = func(context.Context, *cloudfront.Client, *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error) {
		return nil, errSentinel
	}

	require.ErrorIs(t, invalidateClientPaths(context.Background(), &cloudfront.Client{}, "DIST"), errSentinel)
}

func TestRunClientDeploy_FallsBackToDerivedClientBucket(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousReplace := replaceBucketWithDirFn
	previousInvalidate := invalidateClientPathsFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		loadAWSConfigFromProfileFn = previousLoadAWS
		replaceBucketWithDirFn = previousReplace
		invalidateClientPathsFn = previousInvalidate
	})

	ensureToolAvailableFn = func(string) error { return nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	var gotBucket string
	replaceBucketWithDirFn = func(_ context.Context, _ s3BucketUploaderAPI, bucket string, _ string) error {
		gotBucket = bucket
		return nil
	}
	invalidateClientPathsFn = func(context.Context, *cloudfront.Client, string) error { return nil }

	distDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html/>"), 0o644))

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		[]naming.Stage{naming.StageDev},
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages["dev"].StackOutputs = map[string]string{
		"FrontendDistributionId": "DIST-DEV",
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	require.NoError(t, runClientDeploy([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--dist", distDir,
		"--stage", "dev",
		"--state", statePath,
	}))

	require.Equal(t, naming.S3BucketName(receipt.App, naming.StageDev, "client", receipt.AccountID, receipt.Region), gotBucket)
}

func TestRunClientDeploy_ReceiptValidationErrors(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousReplace := replaceBucketWithDirFn
	previousInvalidate := invalidateClientPathsFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		loadAWSConfigFromProfileFn = previousLoadAWS
		replaceBucketWithDirFn = previousReplace
		invalidateClientPathsFn = previousInvalidate
	})

	ensureToolAvailableFn = func(string) error { return nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return nil }
	invalidateClientPathsFn = func(context.Context, *cloudfront.Client, string) error { return nil }

	distDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html/>"), 0o644))

	t.Run("missing stage receipt", func(t *testing.T) {
		receipt := newUpReceipt(
			"app",
			"example.com",
			"profile",
			"123456789012",
			"us-east-1",
			[]naming.Stage{naming.StageDev},
			hostedZone{ID: "Z1", Name: "example.com"},
		)
		receipt.Stages["dev"].StackOutputs = map[string]string{
			"FrontendDistributionId": "DIST-DEV",
		}
		statePath := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, writeReceipt(statePath, receipt))

		err := runClientDeploy([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--dist", distDir,
			"--stage", "live",
			"--state", statePath,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "receipt missing stage")
	})

	t.Run("missing FrontendDistributionId", func(t *testing.T) {
		receipt := newUpReceipt(
			"app",
			"example.com",
			"profile",
			"123456789012",
			"us-east-1",
			[]naming.Stage{naming.StageDev},
			hostedZone{ID: "Z1", Name: "example.com"},
		)
		receipt.Stages["dev"].StackOutputs = map[string]string{}
		statePath := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, writeReceipt(statePath, receipt))

		err := runClientDeploy([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--dist", distDir,
			"--stage", "dev",
			"--state", statePath,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing FrontendDistributionId")
	})
}

func TestRunClientDeploy_DistMissingIndexIsError(t *testing.T) {
	distDir := t.TempDir()

	err := runClientDeploy([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--dist", distDir,
		"--stage", "dev",
		"--state", filepath.Join(t.TempDir(), "missing-state.json"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "client dist is missing")
}

func TestRunClientDeploy_PropagatesUploadAndInvalidationErrors(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousReplace := replaceBucketWithDirFn
	previousInvalidate := invalidateClientPathsFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		loadAWSConfigFromProfileFn = previousLoadAWS
		replaceBucketWithDirFn = previousReplace
		invalidateClientPathsFn = previousInvalidate
	})

	ensureToolAvailableFn = func(string) error { return nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	distDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html/>"), 0o644))

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		[]naming.Stage{naming.StageDev},
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages["dev"].StackOutputs = map[string]string{
		"ClientBucketName":       "bucket-dev",
		"FrontendDistributionId": "DIST-DEV",
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	t.Run("upload error", func(t *testing.T) {
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return errSentinel }
		invalidateClientPathsFn = func(context.Context, *cloudfront.Client, string) error { return nil }

		err := runClientDeploy([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--dist", distDir,
			"--stage", "dev",
			"--state", statePath,
		})
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "upload client UI")
	})

	t.Run("invalidation error", func(t *testing.T) {
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return nil }
		invalidateClientPathsFn = func(context.Context, *cloudfront.Client, string) error { return errSentinel }

		err := runClientDeploy([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--dist", distDir,
			"--stage", "dev",
			"--state", statePath,
		})
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "cloudfront invalidation")
	})
}

func TestRunClientDeploy_PropagatesEarlyErrors(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	t.Cleanup(func() {
		ensureToolAvailableFn = previousEnsureTool
		loadAWSConfigFromProfileFn = previousLoadAWS
	})

	distDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html/>"), 0o644))

	baseArgs := []string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--dist", distDir,
	}

	t.Run("aws cli missing", func(t *testing.T) {
		ensureToolAvailableFn = func(string) error { return errSentinel }
		err := runClientDeploy(append(append([]string(nil), baseArgs...), "--state", filepath.Join(t.TempDir(), "state.json")))
		require.ErrorIs(t, err, errSentinel)
	})

	t.Run("load aws config error", func(t *testing.T) {
		ensureToolAvailableFn = func(string) error { return nil }
		loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{}, errSentinel }
		err := runClientDeploy(append(append([]string(nil), baseArgs...), "--state", filepath.Join(t.TempDir(), "state.json")))
		require.ErrorIs(t, err, errSentinel)
	})

	t.Run("missing receipt file", func(t *testing.T) {
		ensureToolAvailableFn = func(string) error { return nil }
		loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
		err := runClientDeploy(append(append([]string(nil), baseArgs...), "--state", filepath.Join(t.TempDir(), "state.json")))
		require.Error(t, err)
		require.Contains(t, err.Error(), "deployment receipt not found")
	})

	t.Run("invalid receipt file", func(t *testing.T) {
		ensureToolAvailableFn = func(string) error { return nil }
		loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }

		statePath := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, os.WriteFile(statePath, []byte("not-json"), 0o644))

		err := runClientDeploy(append(append([]string(nil), baseArgs...), "--state", statePath))
		require.Error(t, err)
	})
}
