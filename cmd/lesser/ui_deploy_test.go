package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestBuildAuthUI_HappyPathWithStubs(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }

	repoRoot := t.TempDir()
	authDir := filepath.Join(repoRoot, "auth-ui")
	require.NoError(t, os.MkdirAll(filepath.Join(authDir, "node_modules"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(authDir, "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "package.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "dist", "index.html"), []byte("<html/>"), 0o644))

	dist, err := buildAuthUI(repoRoot)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(authDir, "dist"), dist)
}

func TestBuildAuthUI_InstallsWhenNodeModulesMissing(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	var calls []string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		calls = append(calls, name+" "+firstArgOrEmpty(args))
		require.Contains(t, opts.Dir, "auth-ui")
		return nil
	}

	repoRoot := t.TempDir()
	authDir := filepath.Join(repoRoot, "auth-ui")
	require.NoError(t, os.MkdirAll(filepath.Join(authDir, "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "package.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "dist", "index.html"), []byte("<html/>"), 0o644))

	dist, err := buildAuthUI(repoRoot)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(authDir, "dist"), dist)
	require.Contains(t, calls, "pnpm install")
	require.Contains(t, calls, "pnpm -s")
}

func TestBuildAuthUI_ErrorBranches(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	repoRoot := t.TempDir()

	t.Run("missing auth-ui package.json is error", func(t *testing.T) {
		_, err := buildAuthUI(repoRoot)
		require.Error(t, err)
	})

	t.Run("pnpm install error is wrapped", func(t *testing.T) {
		authDir := filepath.Join(repoRoot, "auth-ui")
		require.NoError(t, os.MkdirAll(filepath.Join(authDir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(authDir, "package.json"), []byte("{}"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(authDir, "dist", "index.html"), []byte("<html/>"), 0o644))

		runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
			if len(args) > 0 && args[0] == "install" {
				return errSentinel
			}
			return nil
		}

		_, err := buildAuthUI(repoRoot)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "pnpm install (auth-ui)")
	})

	t.Run("pnpm build error is wrapped", func(t *testing.T) {
		authDir := filepath.Join(repoRoot, "auth-ui")
		require.NoError(t, os.MkdirAll(filepath.Join(authDir, "dist"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(authDir, "node_modules"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(authDir, "package.json"), []byte("{}"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(authDir, "dist", "index.html"), []byte("<html/>"), 0o644))

		runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
			if len(args) > 0 && args[0] == "-s" {
				return errSentinel
			}
			return nil
		}

		_, err := buildAuthUI(repoRoot)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "pnpm build (auth-ui)")
	})

	t.Run("missing dist index is error", func(t *testing.T) {
		authDir := filepath.Join(repoRoot, "auth-ui")
		require.NoError(t, os.MkdirAll(filepath.Join(authDir, "node_modules"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(authDir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(authDir, "package.json"), []byte("{}"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(authDir, "dist", "index.html"), []byte("<html/>"), 0o644))
		require.NoError(t, os.Remove(filepath.Join(authDir, "dist", "index.html")))

		runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }
		_, err := buildAuthUI(repoRoot)
		require.Error(t, err)
	})
}

func TestDeployUIAssets_WiresStagesAndCallsDeps(t *testing.T) {
	previousReplace := replaceBucketWithDirFn
	previousS3Exists := s3ObjectExistsFn
	previousPlaceholder := uploadClientPlaceholderFn
	previousInvalidate := invalidateFrontendFn
	previousBuildAuth := buildAuthUIFn
	t.Cleanup(func() {
		replaceBucketWithDirFn = previousReplace
		s3ObjectExistsFn = previousS3Exists
		uploadClientPlaceholderFn = previousPlaceholder
		invalidateFrontendFn = previousInvalidate
		buildAuthUIFn = previousBuildAuth
	})

	buildAuthUIFn = func(string) (string, error) { return t.TempDir(), nil }

	var replaced int
	replaceBucketWithDirFn = func(_ context.Context, _ s3BucketUploaderAPI, bucket string, _ string) error {
		require.NotEmpty(t, bucket)
		replaced++
		return nil
	}

	s3ObjectExistsFn = func(context.Context, s3HeadObjectAPI, string, string) (bool, error) {
		return false, nil
	}

	var placeholders int
	uploadClientPlaceholderFn = func(context.Context, *s3.Client, string, string) error {
		placeholders++
		return nil
	}

	var invalidations int
	invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error {
		invalidations++
		return nil
	}

	env := &upEnv{
		repoRoot:   t.TempDir(),
		app:        "app",
		baseDomain: "example.com",
		awsProfile: "profile",
		awsCfg:     aws.Config{Region: "us-east-1"},
		accountID:  "123456789012",
		stages:     []naming.Stage{naming.StageDev},
	}

	receipt := newUpReceipt("app", "example.com", "profile", env.accountID, env.awsCfg.Region, []naming.Stage{naming.StageDev}, hostedZone{ID: "Z1", Name: "example.com"})
	receipt.Stages["dev"].Domain = "dev.example.com"
	receipt.Stages["dev"].StackOutputs = map[string]string{
		"FrontendDistributionId": "DIST",
	}

	require.NoError(t, env.deployUIAssets(context.Background(), receipt))
	require.Equal(t, 1, replaced)
	require.Equal(t, 1, placeholders)
	require.Equal(t, 1, invalidations)
}

func TestDeployUIAssets_ErrorAndSkipBranches(t *testing.T) {
	previousReplace := replaceBucketWithDirFn
	previousS3Exists := s3ObjectExistsFn
	previousPlaceholder := uploadClientPlaceholderFn
	previousInvalidate := invalidateFrontendFn
	previousBuildAuth := buildAuthUIFn
	t.Cleanup(func() {
		replaceBucketWithDirFn = previousReplace
		s3ObjectExistsFn = previousS3Exists
		uploadClientPlaceholderFn = previousPlaceholder
		invalidateFrontendFn = previousInvalidate
		buildAuthUIFn = previousBuildAuth
	})

	env := &upEnv{
		repoRoot:   t.TempDir(),
		app:        "app",
		baseDomain: "example.com",
		awsProfile: "profile",
		awsCfg:     aws.Config{Region: "us-east-1"},
		accountID:  "123456789012",
		stages:     []naming.Stage{naming.StageDev, naming.StageLive},
	}

	receipt := newUpReceipt("app", "example.com", "profile", env.accountID, env.awsCfg.Region, []naming.Stage{naming.StageDev}, hostedZone{ID: "Z1", Name: "example.com"})
	receipt.Stages["dev"].Domain = "dev.example.com"
	receipt.Stages["dev"].StackOutputs = map[string]string{
		"FrontendDistributionId": "DIST",
	}

	t.Run("nil receipt is error", func(t *testing.T) {
		require.Error(t, env.deployUIAssets(context.Background(), nil))
	})

	t.Run("buildAuthUI error is propagated", func(t *testing.T) {
		buildAuthUIFn = func(string) (string, error) { return "", errSentinel }
		require.ErrorIs(t, env.deployUIAssets(context.Background(), receipt), errSentinel)
	})

	buildAuthUIFn = func(string) (string, error) { return t.TempDir(), nil }

	t.Run("upload auth UI error is wrapped", func(t *testing.T) {
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return errSentinel }
		s3ObjectExistsFn = func(context.Context, s3HeadObjectAPI, string, string) (bool, error) { return true, nil }
		uploadClientPlaceholderFn = func(context.Context, *s3.Client, string, string) error { return nil }
		invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }

		err := env.deployUIAssets(context.Background(), receipt)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "upload auth UI")
	})

	t.Run("inspect client bucket error is wrapped", func(t *testing.T) {
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return nil }
		s3ObjectExistsFn = func(context.Context, s3HeadObjectAPI, string, string) (bool, error) { return false, errSentinel }
		uploadClientPlaceholderFn = func(context.Context, *s3.Client, string, string) error { return nil }
		invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }

		err := env.deployUIAssets(context.Background(), receipt)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "inspect client bucket")
	})

	t.Run("placeholder upload error is wrapped", func(t *testing.T) {
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return nil }
		s3ObjectExistsFn = func(context.Context, s3HeadObjectAPI, string, string) (bool, error) { return false, nil }
		uploadClientPlaceholderFn = func(context.Context, *s3.Client, string, string) error { return errSentinel }
		invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }

		err := env.deployUIAssets(context.Background(), receipt)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "upload client placeholder")
	})

	t.Run("skips placeholder when index exists and wraps invalidation errors", func(t *testing.T) {
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return nil }
		s3ObjectExistsFn = func(context.Context, s3HeadObjectAPI, string, string) (bool, error) { return true, nil }
		uploadClientPlaceholderFn = func(context.Context, *s3.Client, string, string) error { return nil }
		invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return errSentinel }

		err := env.deployUIAssets(context.Background(), receipt)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "cloudfront invalidation")
	})

	t.Run("skips stages missing receipts", func(t *testing.T) {
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return nil }
		s3ObjectExistsFn = func(context.Context, s3HeadObjectAPI, string, string) (bool, error) { return true, nil }
		uploadClientPlaceholderFn = func(context.Context, *s3.Client, string, string) error { return nil }
		invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }

		require.NoError(t, env.deployUIAssets(context.Background(), receipt))
	})

	t.Run("skips invalidation when distribution id missing", func(t *testing.T) {
		receipt.Stages["dev"].StackOutputs = map[string]string{}
		replaceBucketWithDirFn = func(context.Context, s3BucketUploaderAPI, string, string) error { return nil }
		s3ObjectExistsFn = func(context.Context, s3HeadObjectAPI, string, string) (bool, error) { return true, nil }
		uploadClientPlaceholderFn = func(context.Context, *s3.Client, string, string) error { return nil }
		invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }

		require.NoError(t, env.deployUIAssets(context.Background(), receipt))
	})
}

func TestInvalidateFunctions_UseInjectedCreateInvalidation(t *testing.T) {
	previous := createCloudfrontInvalidationFn
	t.Cleanup(func() { createCloudfrontInvalidationFn = previous })

	var called int
	createCloudfrontInvalidationFn = func(_ context.Context, _ *cloudfront.Client, _ *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error) {
		called++
		return &cloudfront.CreateInvalidationOutput{}, nil
	}

	require.NoError(t, invalidateFrontend(context.Background(), &cloudfront.Client{}, "DIST"))
	require.NoError(t, invalidateClientPaths(context.Background(), &cloudfront.Client{}, "DIST"))
	require.Equal(t, 2, called)
}

func TestUploadClientPlaceholder_UsesInjectedPutObject(t *testing.T) {
	previous := putObjectStringFn
	t.Cleanup(func() { putObjectStringFn = previous })

	var gotBucket string
	var gotKey string
	putObjectStringFn = func(_ context.Context, _ s3PutObjectAPI, bucket, key, content, contentType, cacheControl string) error {
		gotBucket = bucket
		gotKey = key
		require.Contains(t, content, "Lesser is deployed")
		require.Equal(t, "text/html; charset=utf-8", contentType)
		require.Equal(t, "public, max-age=60", cacheControl)
		return nil
	}

	require.NoError(t, uploadClientPlaceholder(context.Background(), &s3.Client{}, "bucket", "dev.example.com"))
	require.Equal(t, "bucket", gotBucket)
	require.Equal(t, "index.html", gotKey)
}
