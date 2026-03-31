package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

var (
	invalidateFrontendFn         = invalidateFrontend
	replaceBucketWithDirPrefixFn = replaceBucketWithDirPrefix
)

func (e *upEnv) deployUIAssets(ctx context.Context, receipt *upReceipt) error {
	if receipt == nil {
		return fmt.Errorf("deployment receipt is nil")
	}

	authUIDist, err := e.resolveAuthUIDist()
	if err != nil {
		return err
	}

	s3Client := s3.NewFromConfig(e.awsCfg)
	cfClient := cloudfront.NewFromConfig(e.awsCfg)

	for _, stage := range e.stages {
		stageKey := string(stage)
		stageReceipt := receipt.Stages[stageKey]
		if stageReceipt == nil {
			continue
		}

		authBucket := strings.TrimSpace(stageReceipt.StackOutputs["AuthUIBucketName"])
		if authBucket == "" {
			authBucket = naming.S3BucketName(e.app, stage, "auth-ui", e.accountID, e.awsCfg.Region)
		}

		fmt.Printf("\nUploading UI assets (%s):\n", stageKey)
		fmt.Printf("  auth_ui:  s3://%s/\n", authBucket)

		if err := replaceBucketWithDirPrefixFn(ctx, s3Client, authBucket, "auth", authUIDist); err != nil {
			return fmt.Errorf("upload auth UI (%s): %w", stageKey, err)
		}

		distID := strings.TrimSpace(stageReceipt.StackOutputs["FrontendDistributionId"])
		if distID == "" {
			continue
		}
		if err := invalidateFrontendFn(ctx, cfClient, distID); err != nil {
			return fmt.Errorf("cloudfront invalidation (%s): %w", stageKey, err)
		}
	}

	return nil
}

func (e *upEnv) resolveAuthUIDist() (string, error) {
	if e.usesReleaseArtifacts() {
		if strings.TrimSpace(e.releaseAuthUIDir) == "" {
			return "", fmt.Errorf("release auth-ui bundle is not prepared")
		}
		if _, err := os.Stat(filepath.Join(e.releaseAuthUIDir, "index.html")); err != nil {
			return "", fmt.Errorf("release auth-ui bundle missing index.html at %s", e.releaseAuthUIDir)
		}
		return e.releaseAuthUIDir, nil
	}
	return buildAuthUIFn(e.repoRoot)
}

func buildAuthUI(repoRoot string) (string, error) {
	authDir := filepath.Join(repoRoot, "auth-ui")
	distDir := filepath.Join(authDir, "dist")

	if _, err := os.Stat(filepath.Join(authDir, "package.json")); err != nil {
		return "", fmt.Errorf("auth-ui not found at %s", authDir)
	}

	if _, err := os.Stat(filepath.Join(authDir, "node_modules")); err != nil {
		fmt.Println("\nInstalling auth UI dependencies...")
		if err := runCommandFn(context.Background(), "pnpm", []string{"install", "--frozen-lockfile"}, execOptions{Dir: authDir}); err != nil {
			return "", fmt.Errorf("pnpm install (auth-ui): %w", err)
		}
	}

	fmt.Println("\nBuilding auth UI...")
	if err := runCommandFn(context.Background(), "pnpm", []string{"-s", "build"}, execOptions{Dir: authDir}); err != nil {
		return "", fmt.Errorf("pnpm build (auth-ui): %w", err)
	}

	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		return "", fmt.Errorf("auth UI build missing %s", distDir)
	}
	return distDir, nil
}

func invalidateFrontend(ctx context.Context, client *cloudfront.Client, distributionID string) error {
	paths := []string{"/auth", "/auth/*"}
	quantity := int32(len(paths)) // #nosec G115 -- len(paths) is bounded by static slice

	_, err := createCloudfrontInvalidationFn(ctx, client, &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(distributionID),
		InvalidationBatch: &cloudfronttypes.InvalidationBatch{
			CallerReference: aws.String(fmt.Sprintf("lesser-%d", time.Now().UnixNano())),
			Paths: &cloudfronttypes.Paths{
				Quantity: aws.Int32(quantity),
				Items:    paths,
			},
		},
	})
	if err != nil {
		return err
	}
	fmt.Println("  cloudfront: invalidation created")
	return nil
}
