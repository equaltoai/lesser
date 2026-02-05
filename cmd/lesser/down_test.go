package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestParseDownArgs(t *testing.T) {
	_, err := parseDownArgs(nil)
	require.Error(t, err)

	args, err := parseDownArgs([]string{"--app", "app", "--base-domain", "example.com", "--aws-profile", "profile"})
	require.NoError(t, err)
	require.Equal(t, "app", args.App)
	require.Equal(t, "example.com", args.BaseDomain)
	require.Equal(t, "profile", args.AWSProfile)
}

func TestRunDown_HappyPath(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousEnsureTool := ensureToolAvailableFn
	previousDestroy := cdkDestroyStackFn
	previousDeleteBucketObjects := deleteBucketObjectsFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		loadAWSConfigFromProfileFn = previousLoadAWS
		ensureToolAvailableFn = previousEnsureTool
		cdkDestroyStackFn = previousDestroy
		deleteBucketObjectsFn = previousDeleteBucketObjects
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		[]naming.Stage{naming.StageDev, naming.StageLive},
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.SharedStack = "app-shared"
	receipt.Stages["dev"].StackName = "app-dev"
	receipt.Stages["live"].StackName = "app-live"

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	var destroyed []string
	cdkDestroyStackFn = func(_ context.Context, _ string, _ string, req cdkDestroyRequest) error {
		destroyed = append(destroyed, req.StackName)
		return nil
	}
	deleteBucketObjectsFn = func(context.Context, s3BucketAPI, string) error { return nil }

	require.NoError(t, runDown([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--state", statePath,
	}))
	require.Equal(t, []string{"app-live", "app-dev", "app-shared"}, destroyed)
}

func TestRunDown_PurgeArtifacts(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousEnsureTool := ensureToolAvailableFn
	previousDestroy := cdkDestroyStackFn
	previousDeleteBucketObjects := deleteBucketObjectsFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		loadAWSConfigFromProfileFn = previousLoadAWS
		ensureToolAvailableFn = previousEnsureTool
		cdkDestroyStackFn = previousDestroy
		deleteBucketObjectsFn = previousDeleteBucketObjects
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	ensureToolAvailableFn = func(string) error { return nil }

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		[]naming.Stage{naming.StageDev, naming.StageLive},
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.SharedStack = "app-shared"
	receipt.Stages["dev"].StackName = "app-dev"
	receipt.Stages["live"].StackName = "app-live"
	receipt.Stages["dev"].StackOutputs = map[string]string{
		"ClientBucketName": "dev-client",
		"AuthUIBucketName": "dev-auth",
	}
	receipt.Stages["live"].StackOutputs = map[string]string{
		"ClientBucketName": "live-client",
		"AuthUIBucketName": "live-auth",
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	var purged []string
	deleteBucketObjectsFn = func(_ context.Context, _ s3BucketAPI, bucket string) error {
		purged = append(purged, bucket)
		return nil
	}
	cdkDestroyStackFn = func(context.Context, string, string, cdkDestroyRequest) error { return nil }

	require.NoError(t, runDown([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--state", statePath,
		"--purge-artifacts",
	}))
	require.Equal(t, []string{"dev-auth", "dev-client", "live-auth", "live-client"}, purged)
}

func TestRunDown_MissingStatePathErrors(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() { ensureToolAvailableFn = previousEnsureTool })
	ensureToolAvailableFn = func(string) error { return nil }

	err := runDown([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--state", filepath.Join(t.TempDir(), "missing-state.json"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "deployment receipt not found")
}

func TestRunDown_RejectsPartialOrMismatchedReceipt(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousEnsureTool := ensureToolAvailableFn
	previousDestroy := cdkDestroyStackFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		loadAWSConfigFromProfileFn = previousLoadAWS
		ensureToolAvailableFn = previousEnsureTool
		cdkDestroyStackFn = previousDestroy
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	ensureToolAvailableFn = func(string) error { return nil }
	cdkDestroyStackFn = func(context.Context, string, string, cdkDestroyRequest) error { return nil }

	t.Run("missing stage stack name", func(t *testing.T) {
		receipt := newUpReceipt(
			"app",
			"example.com",
			"profile",
			"123456789012",
			"us-east-1",
			[]naming.Stage{naming.StageDev},
			hostedZone{ID: "Z1", Name: "example.com"},
		)
		receipt.SharedStack = "app-shared"
		receipt.Stages["dev"].StackName = ""
		statePath := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, writeReceipt(statePath, receipt))

		err := runDown([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--state", statePath,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "receipt missing stack name for stage")
	})

	t.Run("app/base-domain mismatch", func(t *testing.T) {
		receipt := newUpReceipt(
			"other",
			"example.com",
			"profile",
			"123456789012",
			"us-east-1",
			[]naming.Stage{naming.StageDev},
			hostedZone{ID: "Z1", Name: "example.com"},
		)
		receipt.SharedStack = "other-shared"
		receipt.Stages["dev"].StackName = "other-dev"
		statePath := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, writeReceipt(statePath, receipt))

		err := runDown([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--state", statePath,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "receipt app/base-domain mismatch")
	})
}

func TestDeleteBucketObjectsIfPresent_SkipsNotFound(t *testing.T) {
	previousDeleteBucketObjects := deleteBucketObjectsFn
	t.Cleanup(func() { deleteBucketObjectsFn = previousDeleteBucketObjects })

	deleteBucketObjectsFn = func(context.Context, s3BucketAPI, string) error {
		return fakeSmithyAPIError{code: "NoSuchBucket"}
	}
	require.NoError(t, deleteBucketObjectsIfPresent(context.Background(), &fakeS3BucketClient{}, "bucket"))
}

func TestDeleteBucketObjectsIfPresent_WrapsErrors(t *testing.T) {
	previousDeleteBucketObjects := deleteBucketObjectsFn
	t.Cleanup(func() { deleteBucketObjectsFn = previousDeleteBucketObjects })

	deleteBucketObjectsFn = func(context.Context, s3BucketAPI, string) error {
		return errors.New("boom")
	}
	err := deleteBucketObjectsIfPresent(context.Background(), &fakeS3BucketClient{}, "bucket")
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}
