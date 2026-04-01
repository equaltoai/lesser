package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestUploadReleaseAssemblyAssets(t *testing.T) {
	previousNewS3Client := newS3ClientFn
	previousNewPresign := newS3PresignClientFn
	previousUploadFile := uploadReleaseAssemblyFileFn
	previousPresignURL := presignReleaseAssemblyURLFn
	t.Cleanup(func() {
		newS3ClientFn = previousNewS3Client
		newS3PresignClientFn = previousNewPresign
		uploadReleaseAssemblyFileFn = previousUploadFile
		presignReleaseAssemblyURLFn = previousPresignURL
	})

	var uploadedKeys []string
	newS3ClientFn = func(aws.Config, ...func(*s3.Options)) *s3.Client { return nil }
	newS3PresignClientFn = func(*s3.Client, ...func(*s3.PresignOptions)) *s3.PresignClient { return nil }
	uploadReleaseAssemblyFileFn = func(_ context.Context, _ *s3.Client, bucket, objectKey, localPath string) error {
		require.Equal(t, "bucket", bucket)
		uploadedKeys = append(uploadedKeys, objectKey+"="+filepath.Base(localPath))
		return nil
	}
	presignReleaseAssemblyURLFn = func(_ context.Context, _ *s3.PresignClient, _ string, templateKey string) (string, error) {
		return "https://example.com/" + templateKey, nil
	}

	assembly := releaseDeployAssemblyInstallResult{
		Assets: []releaseDeployAssemblyAsset{
			{ObjectKey: "assets/plain.txt", LocalPath: "/tmp/plain.txt"},
		},
		StageTemplates: map[naming.Stage]string{
			naming.StageDev:     "/tmp/dev.template.json",
			naming.StageStaging: "/tmp/staging.template.json",
			naming.StageLive:    "/tmp/live.template.json",
		},
	}

	result, err := uploadReleaseAssemblyAssets(context.Background(), aws.Config{}, "bucket", "v1.2.3", "abcdef", assembly)
	require.NoError(t, err)
	require.Equal(t, []string{
		"assets/plain.txt=plain.txt",
		"release-assembly/v1.2.3/abcdef/templates/dev.template.json=dev.template.json",
		"release-assembly/v1.2.3/abcdef/templates/staging.template.json=staging.template.json",
		"release-assembly/v1.2.3/abcdef/templates/live.template.json=live.template.json",
	}, uploadedKeys)
	require.Equal(t, "https://example.com/release-assembly/v1.2.3/abcdef/templates/dev.template.json", result.StageTemplateURLs[naming.StageDev])
	require.Equal(t, "https://example.com/release-assembly/v1.2.3/abcdef/templates/staging.template.json", result.StageTemplateURLs[naming.StageStaging])
	require.Equal(t, "https://example.com/release-assembly/v1.2.3/abcdef/templates/live.template.json", result.StageTemplateURLs[naming.StageLive])
}

func TestUploadReleaseAssemblyAssets_Errors(t *testing.T) {
	previousNewS3Client := newS3ClientFn
	previousNewPresign := newS3PresignClientFn
	previousUploadFile := uploadReleaseAssemblyFileFn
	previousPresignURL := presignReleaseAssemblyURLFn
	t.Cleanup(func() {
		newS3ClientFn = previousNewS3Client
		newS3PresignClientFn = previousNewPresign
		uploadReleaseAssemblyFileFn = previousUploadFile
		presignReleaseAssemblyURLFn = previousPresignURL
	})

	newS3ClientFn = func(aws.Config, ...func(*s3.Options)) *s3.Client { return nil }
	newS3PresignClientFn = func(*s3.Client, ...func(*s3.PresignOptions)) *s3.PresignClient { return nil }

	_, err := uploadReleaseAssemblyAssets(context.Background(), aws.Config{}, "", "v1.2.3", "abcdef", releaseDeployAssemblyInstallResult{})
	require.ErrorContains(t, err, "release asset bucket is required")

	uploadReleaseAssemblyFileFn = func(context.Context, *s3.Client, string, string, string) error { return errors.New("upload failed") }
	_, err = uploadReleaseAssemblyAssets(context.Background(), aws.Config{}, "bucket", "v1.2.3", "abcdef", releaseDeployAssemblyInstallResult{
		Assets: []releaseDeployAssemblyAsset{{ObjectKey: "assets/plain.txt", LocalPath: "/tmp/plain.txt"}},
		StageTemplates: map[naming.Stage]string{
			naming.StageDev:     "/tmp/dev.template.json",
			naming.StageStaging: "/tmp/staging.template.json",
			naming.StageLive:    "/tmp/live.template.json",
		},
	})
	require.ErrorContains(t, err, "upload failed")

	uploadReleaseAssemblyFileFn = func(context.Context, *s3.Client, string, string, string) error { return nil }
	presignReleaseAssemblyURLFn = func(context.Context, *s3.PresignClient, string, string) (string, error) {
		return "", errors.New("presign failed")
	}
	_, err = uploadReleaseAssemblyAssets(context.Background(), aws.Config{}, "bucket", "v1.2.3", "abcdef", releaseDeployAssemblyInstallResult{
		StageTemplates: map[naming.Stage]string{
			naming.StageDev:     "/tmp/dev.template.json",
			naming.StageStaging: "/tmp/staging.template.json",
			naming.StageLive:    "/tmp/live.template.json",
		},
	})
	require.ErrorContains(t, err, "presign failed")

	presignReleaseAssemblyURLFn = func(context.Context, *s3.PresignClient, string, string) (string, error) {
		return "https://example.com/template.json", nil
	}
	_, err = uploadReleaseAssemblyAssets(context.Background(), aws.Config{}, "bucket", "v1.2.3", "abcdef", releaseDeployAssemblyInstallResult{
		StageTemplates: map[naming.Stage]string{
			naming.StageDev:     "/tmp/dev.template.json",
			naming.StageStaging: "/tmp/staging.template.json",
		},
	})
	require.ErrorContains(t, err, "release assembly missing stage template for live")
}

func TestUploadReleaseAssemblyFile(t *testing.T) {
	previousPutObject := putS3ObjectFn
	t.Cleanup(func() {
		putS3ObjectFn = previousPutObject
	})

	localPath := filepath.Join(t.TempDir(), "plain.txt")
	require.NoError(t, os.WriteFile(localPath, []byte("plain"), 0o644))

	var uploadedBucket string
	var uploadedKey string
	var uploadedBody string
	putS3ObjectFn = func(_ context.Context, _ *s3.Client, input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		uploadedBucket = aws.ToString(input.Bucket)
		uploadedKey = aws.ToString(input.Key)
		body, err := io.ReadAll(input.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		return &s3.PutObjectOutput{}, nil
	}

	require.NoError(t, uploadReleaseAssemblyFile(context.Background(), nil, "bucket", "assets/plain.txt", localPath))
	require.Equal(t, "bucket", uploadedBucket)
	require.Equal(t, "assets/plain.txt", uploadedKey)
	require.Equal(t, "plain", uploadedBody)
}

func TestUploadReleaseAssemblyFile_Errors(t *testing.T) {
	previousPutObject := putS3ObjectFn
	t.Cleanup(func() {
		putS3ObjectFn = previousPutObject
	})

	err := uploadReleaseAssemblyFile(context.Background(), nil, "bucket", "assets/plain.txt", filepath.Join(t.TempDir(), "missing.txt"))
	require.ErrorContains(t, err, "open release assembly file")

	localPath := filepath.Join(t.TempDir(), "plain.txt")
	require.NoError(t, os.WriteFile(localPath, []byte("plain"), 0o644))

	putS3ObjectFn = func(context.Context, *s3.Client, *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		return nil, errors.New("put failed")
	}
	err = uploadReleaseAssemblyFile(context.Background(), nil, "bucket", "assets/plain.txt", localPath)
	require.ErrorContains(t, err, "put failed")
}

func TestUpEnvDeployFromReleaseAssembly_RequiresPreparedAssembly(t *testing.T) {
	env := &upEnv{}

	_, err := env.deployFromReleaseAssembly(context.Background())
	require.ErrorContains(t, err, "release deploy assembly is not prepared")
}

func TestUpEnvDeployFromReleaseAssembly_ErrorsWhenSharedTemplateMissing(t *testing.T) {
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	t.Cleanup(func() {
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
	})

	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }

	env := &upEnv{
		app:        "app",
		baseDomain: "example.com",
		awsProfile: "profile",
		accountID:  "123456789012",
		awsCfg:     aws.Config{Region: "us-east-1"},
		hostedZone: hostedZone{ID: "Z1", Name: "example.com"},
		stages:     []naming.Stage{naming.StageDev},
		releaseAssembly: &releaseDeployAssemblyInstallResult{
			SharedTemplate: filepath.Join(t.TempDir(), "missing.template.json"),
		},
	}

	_, err := env.deployFromReleaseAssembly(context.Background())
	require.ErrorContains(t, err, "read shared deploy assembly template")
}

func TestUpEnvDeployFromReleaseAssembly_ErrorsWhenStageTemplateURLMissing(t *testing.T) {
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousDeploy := deployCloudFormationStackFn
	previousUpload := uploadReleaseAssemblyAssetsFn
	t.Cleanup(func() {
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		deployCloudFormationStackFn = previousDeploy
		uploadReleaseAssemblyAssetsFn = previousUpload
	})

	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }
	deployCloudFormationStackFn = func(_ context.Context, _ aws.Config, req cloudFormationDeployRequest) (map[string]string, error) {
		if req.StackName == naming.SharedStackName("app") {
			return map[string]string{"ReleaseAssetBucketName": "release-assets"}, nil
		}
		t.Fatal("stage deploy should not run without a template URL")
		return nil, nil
	}
	uploadReleaseAssemblyAssetsFn = func(context.Context, aws.Config, string, string, string, releaseDeployAssemblyInstallResult) (releaseAssemblyUploadResult, error) {
		return releaseAssemblyUploadResult{
			StageTemplateURLs: map[naming.Stage]string{},
		}, nil
	}

	workspaceRoot := t.TempDir()
	assets := stubReleaseDeployAssetsInstallResult(t, workspaceRoot)
	env := &upEnv{
		args:            upArgs{},
		app:             "app",
		baseDomain:      "example.com",
		awsProfile:      "profile",
		accountID:       "123456789012",
		awsCfg:          aws.Config{Region: "us-east-1"},
		hostedZone:      hostedZone{ID: "Z1", Name: "example.com"},
		stages:          []naming.Stage{naming.StageDev},
		releaseVersion:  assets.Version,
		releaseGitSHA:   assets.GitSHA,
		releaseAssembly: &assets.Assembly,
	}

	_, err := env.deployFromReleaseAssembly(context.Background())
	require.ErrorContains(t, err, "release deploy assembly missing template URL for dev")
}
