package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func (e *upEnv) deployUIAssets(ctx context.Context, receipt *upReceipt) error {
	if receipt == nil {
		return fmt.Errorf("deployment receipt is nil")
	}

	authUIDist, err := buildAuthUI(e.repoRoot)
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

		clientBucket := strings.TrimSpace(stageReceipt.StackOutputs["ClientBucketName"])
		if clientBucket == "" {
			clientBucket = naming.S3BucketName(e.app, stage, "client", e.accountID, e.awsCfg.Region)
		}

		authBucket := strings.TrimSpace(stageReceipt.StackOutputs["AuthUIBucketName"])
		if authBucket == "" {
			authBucket = naming.S3BucketName(e.app, stage, "auth-ui", e.accountID, e.awsCfg.Region)
		}

		fmt.Printf("\nUploading UI assets (%s):\n", stageKey)
		fmt.Printf("  auth_ui:  s3://%s/\n", authBucket)
		fmt.Printf("  client:   s3://%s/\n", clientBucket)

		if err := replaceBucketWithDir(ctx, s3Client, authBucket, authUIDist); err != nil {
			return fmt.Errorf("upload auth UI (%s): %w", stageKey, err)
		}

		hasIndex, err := s3ObjectExists(ctx, s3Client, clientBucket, "index.html")
		if err != nil {
			return fmt.Errorf("inspect client bucket (%s): %w", stageKey, err)
		}
		if !hasIndex {
			if err := uploadClientPlaceholder(ctx, s3Client, clientBucket, stageReceipt.Domain); err != nil {
				return fmt.Errorf("upload client placeholder (%s): %w", stageKey, err)
			}
		}

		distID := strings.TrimSpace(stageReceipt.StackOutputs["FrontendDistributionId"])
		if distID == "" {
			continue
		}
		if err := invalidateFrontend(ctx, cfClient, distID); err != nil {
			return fmt.Errorf("cloudfront invalidation (%s): %w", stageKey, err)
		}
	}

	return nil
}

func buildAuthUI(repoRoot string) (string, error) {
	authDir := filepath.Join(repoRoot, "auth-ui")
	distDir := filepath.Join(authDir, "dist")

	if _, err := os.Stat(filepath.Join(authDir, "package.json")); err != nil {
		return "", fmt.Errorf("auth-ui not found at %s", authDir)
	}

	if _, err := os.Stat(filepath.Join(authDir, "node_modules")); err != nil {
		cmd := exec.Command("pnpm", "install", "--frozen-lockfile") //nolint:gosec // tool invocation
		cmd.Dir = authDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Println("\nInstalling auth UI dependencies...")
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("pnpm install (auth-ui): %w", err)
		}
	}

	cmd := exec.Command("pnpm", "-s", "build") //nolint:gosec // tool invocation
	cmd.Dir = authDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("\nBuilding auth UI...")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pnpm build (auth-ui): %w", err)
	}

	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		return "", fmt.Errorf("auth UI build missing %s", distDir)
	}
	return distDir, nil
}

func invalidateFrontend(ctx context.Context, client *cloudfront.Client, distributionID string) error {
	paths := []string{"/auth", "/auth/*", "/l", "/l/*"}
	quantity := int32(len(paths)) //nolint:gosec // safe: constant small slice for invalidation paths

	_, err := client.CreateInvalidation(ctx, &cloudfront.CreateInvalidationInput{
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

func uploadClientPlaceholder(ctx context.Context, client *s3.Client, bucket string, stageDomain string) error {
	content := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Lesser</title>
    <style>
      body { font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif; margin: 0; padding: 3rem 1.5rem; line-height: 1.5; }
      .card { max-width: 720px; margin: 0 auto; padding: 1.5rem; border: 1px solid #e5e7eb; border-radius: 12px; }
      code { background: #f3f4f6; padding: 0.15rem 0.35rem; border-radius: 6px; }
      a { color: #2563eb; text-decoration: none; }
      a:hover { text-decoration: underline; }
    </style>
  </head>
  <body>
    <div class="card">
      <h1>Lesser is deployed</h1>
      <p>The client UI has not been deployed yet.</p>
      <p>Auth UI: <a href="https://%s/auth">https://%s/auth</a></p>
      <p>API: <a href="https://%s/">https://%s/</a></p>
      <p>Setup status: <code>GET /setup/status</code></p>
    </div>
  </body>
</html>
`, stageDomain, stageDomain, stageDomain, stageDomain)

	return putObjectString(ctx, client, bucket, "index.html", content, "text/html; charset=utf-8", "public, max-age=60")
}
