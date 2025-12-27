package main

import (
	"context"
	"errors"
	"flag"
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

type clientDeployArgs struct {
	App        string
	BaseDomain string
	AWSProfile string
	DistDir    string
	Stage      string
	StatePath  string
}

func runClientDeploy(argv []string) error {
	args, err := parseClientDeployArgs(argv)
	if err != nil {
		return err
	}

	app, err := naming.NormalizeAppName(args.App)
	if err != nil {
		return err
	}

	baseDomain, err := normalizeBaseDomain(args.BaseDomain)
	if err != nil {
		return err
	}

	awsProfile := strings.TrimSpace(args.AWSProfile)
	if awsProfile == "" {
		return errors.New("aws profile is required")
	}

	distDir := strings.TrimSpace(args.DistDir)
	if distDir == "" {
		return errors.New("dist directory is required")
	}
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		return fmt.Errorf("client dist is missing %s: %w", filepath.Join(distDir, "index.html"), err)
	}

	ctx := context.Background()
	if err := ensureAWSCLIToolAvailable(); err != nil {
		return err
	}

	awsCfg, err := loadAWSConfigFromProfile(ctx, awsProfile)
	if err != nil {
		return err
	}

	statePath, err := resolveReceiptPath(app, baseDomain, args.StatePath)
	if err != nil {
		return err
	}

	receipt, err := readReceipt(statePath)
	if err != nil {
		return err
	}

	stages, err := parseStageSelection(args.Stage)
	if err != nil {
		return err
	}

	s3Client := s3.NewFromConfig(awsCfg)
	cfClient := cloudfront.NewFromConfig(awsCfg)

	fmt.Println("\nDeploying client app:")
	fmt.Println("  dist:", distDir)
	fmt.Println("  receipt:", statePath)

	for _, stage := range stages {
		stageKey := string(stage)
		stageReceipt := receipt.Stages[stageKey]
		if stageReceipt == nil {
			return fmt.Errorf("receipt missing stage %q", stageKey)
		}

		clientBucket := strings.TrimSpace(stageReceipt.StackOutputs["ClientBucketName"])
		if clientBucket == "" {
			clientBucket = naming.S3BucketName(receipt.App, stage, "client", receipt.AccountID, receipt.Region)
		}

		distID := strings.TrimSpace(stageReceipt.StackOutputs["FrontendDistributionId"])
		if distID == "" {
			return fmt.Errorf("missing FrontendDistributionId in receipt for stage %q", stageKey)
		}

		fmt.Printf("\nUploading client assets (%s):\n", stageKey)
		fmt.Printf("  bucket: s3://%s/\n", clientBucket)
		if err := replaceBucketWithDir(ctx, s3Client, clientBucket, distDir); err != nil {
			return fmt.Errorf("upload client UI (%s): %w", stageKey, err)
		}

		if err := invalidateClientPaths(ctx, cfClient, distID); err != nil {
			return fmt.Errorf("cloudfront invalidation (%s): %w", stageKey, err)
		}
	}

	return nil
}

func parseClientDeployArgs(argv []string) (clientDeployArgs, error) {
	fs := flag.NewFlagSet("lesser client deploy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args clientDeployArgs
	fs.StringVar(&args.App, "app", "", "app name slug (e.g. my-lesser)")
	fs.StringVar(&args.BaseDomain, "base-domain", "", "base domain with an existing public hosted zone (e.g. example.com)")
	fs.StringVar(&args.AWSProfile, "aws-profile", "", "AWS profile name to use (sets AWS_PROFILE)")
	fs.StringVar(&args.DistDir, "dist", "", "path to client build output directory (must contain index.html)")
	fs.StringVar(&args.Stage, "stage", "both", "stage to deploy (dev|live|staging|both|all)")
	fs.StringVar(&args.StatePath, "state", "", "path to deployment receipt (defaults to ~/.lesser/<app>/<base-domain>/state.json)")

	if err := fs.Parse(argv); err != nil {
		return clientDeployArgs{}, err
	}

	if strings.TrimSpace(args.App) == "" || strings.TrimSpace(args.BaseDomain) == "" || strings.TrimSpace(args.AWSProfile) == "" || strings.TrimSpace(args.DistDir) == "" {
		return clientDeployArgs{}, errors.New("required flags: --app, --base-domain, --aws-profile, --dist")
	}

	return args, nil
}

func parseStageSelection(value string) ([]naming.Stage, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case valueDev:
		return []naming.Stage{naming.StageDev}, nil
	case valueStaging:
		return []naming.Stage{naming.StageStaging}, nil
	case "live":
		return []naming.Stage{naming.StageLive}, nil
	case "", "both":
		return []naming.Stage{naming.StageDev, naming.StageLive}, nil
	case valueAll:
		return []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive}, nil
	default:
		return nil, fmt.Errorf("invalid --stage %q (expected dev|live|staging|both|all)", value)
	}
}

func invalidateClientPaths(ctx context.Context, client *cloudfront.Client, distributionID string) error {
	paths := []string{"/l", "/l/*"}
	quantity := int32(len(paths)) // #nosec G115 -- len(paths) is bounded by static slice

	_, err := client.CreateInvalidation(ctx, &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(distributionID),
		InvalidationBatch: &cloudfronttypes.InvalidationBatch{
			CallerReference: aws.String(fmt.Sprintf("lesser-client-%d", time.Now().UnixNano())),
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
