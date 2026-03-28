package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

var (
	replaceBucketWithDirFn         = replaceBucketWithDir
	invalidateClientPathsFn        = invalidateClientPaths
	createCloudfrontInvalidationFn = func(ctx context.Context, client *cloudfront.Client, input *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error) {
		return client.CreateInvalidation(ctx, input)
	}
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
	_, _ = argv, parseClientDeployArgs
	return errors.New("`lesser client deploy` is retired for SSR-first /l/ hosting; use `lesser client install` from the FaceTheory app repo")
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

	_, err := createCloudfrontInvalidationFn(ctx, client, &cloudfront.CreateInvalidationInput{
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
