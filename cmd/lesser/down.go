package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

type downArgs struct {
	App            string
	BaseDomain     string
	AWSProfile     string
	StatePath      string
	PurgeArtifacts bool
}

var deleteBucketObjectsFn = deleteAllObjects

func runDown(argv []string) error {
	args, err := parseDownArgs(argv)
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

	ctx := context.Background()
	statePath, err := resolveReceiptPath(app, baseDomain, args.StatePath)
	if err != nil {
		return err
	}
	receipt, err := readReceipt(statePath)
	if err != nil {
		return err
	}

	stages, err := validateDownReceipt(receipt, app, baseDomain)
	if err != nil {
		return err
	}

	if err := ensureAWSCLIToolAvailable(); err != nil {
		return err
	}
	if err := ensureToolAvailableFn("cdk"); err != nil {
		return err
	}

	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	awsCfg, err := loadAWSConfigFromProfileFn(ctx, awsProfile)
	if err != nil {
		return err
	}

	region := strings.TrimSpace(receipt.Region)
	if region == "" {
		region = strings.TrimSpace(awsCfg.Region)
	}
	if region == "" {
		return errors.New("receipt is missing AWS region")
	}

	if args.PurgeArtifacts {
		s3Client := s3.NewFromConfig(awsCfg)
		buckets := receiptArtifactBuckets(receipt, stages)
		for _, bucket := range buckets {
			fmt.Println("Purging deployment artifacts from:", bucket)
			if err := deleteBucketObjectsIfPresent(ctx, s3Client, bucket); err != nil {
				return fmt.Errorf("purge artifacts in bucket %s: %w", bucket, err)
			}
		}
	}

	for _, stage := range stages {
		stageReceipt := receipt.Stages[string(stage)]
		fmt.Println("Destroying stage stack:", stageReceipt.StackName)
		if err := cdkDestroyStackFn(ctx, repoRoot, awsProfile, cdkDestroyRequest{
			StackName:    strings.TrimSpace(stageReceipt.StackName),
			App:          app,
			BaseDomain:   baseDomain,
			HostedZoneID: strings.TrimSpace(receipt.HostedZone.ID),
			Region:       region,
			StageFilter:  string(stage),
		}); err != nil {
			return err
		}
	}

	fmt.Println("Destroying shared stack:", receipt.SharedStack)
	if err := cdkDestroyStackFn(ctx, repoRoot, awsProfile, cdkDestroyRequest{
		StackName:    strings.TrimSpace(receipt.SharedStack),
		App:          app,
		BaseDomain:   baseDomain,
		HostedZoneID: strings.TrimSpace(receipt.HostedZone.ID),
		Region:       region,
		StageFilter:  string(naming.StageShared),
	}); err != nil {
		return err
	}

	fmt.Println("Teardown complete:", statePath)
	return nil
}

func parseDownArgs(argv []string) (downArgs, error) {
	fs := flag.NewFlagSet("lesser down", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args downArgs
	fs.StringVar(&args.App, "app", "", "app name slug (e.g. my-lesser)")
	fs.StringVar(&args.BaseDomain, "base-domain", "", "base domain with an existing public hosted zone (e.g. example.com)")
	fs.StringVar(&args.AWSProfile, "aws-profile", "", "AWS profile name to use (sets AWS_PROFILE)")
	fs.StringVar(&args.StatePath, "state", "", "path to deployment receipt (defaults to ~/.lesser/<app>/<base-domain>/state.json)")
	fs.BoolVar(&args.PurgeArtifacts, "purge-artifacts", false, "purge known deployment S3 buckets before destroying stacks")

	if err := fs.Parse(argv); err != nil {
		return downArgs{}, err
	}
	if strings.TrimSpace(args.App) == "" || strings.TrimSpace(args.BaseDomain) == "" || strings.TrimSpace(args.AWSProfile) == "" {
		return downArgs{}, errors.New("required flags: --app, --base-domain, --aws-profile")
	}
	return args, nil
}

func validateDownReceipt(receipt *upReceipt, app, baseDomain string) ([]naming.Stage, error) {
	if receipt == nil {
		return nil, errors.New("deployment receipt is nil")
	}

	receiptApp := strings.TrimSpace(receipt.App)
	receiptDomain := strings.TrimSpace(receipt.BaseDomain)
	if !strings.EqualFold(receiptApp, app) || !strings.EqualFold(receiptDomain, baseDomain) {
		return nil, fmt.Errorf("receipt app/base-domain mismatch (receipt has %q / %q, expected %q / %q)", receiptApp, receiptDomain, app, baseDomain)
	}

	if strings.TrimSpace(receipt.SharedStack) == "" {
		return nil, errors.New("receipt is missing shared stack name")
	}

	stages, err := receiptDestroyStages(receipt.Stages)
	if err != nil {
		return nil, err
	}

	for _, stage := range stages {
		stageReceipt := receipt.Stages[string(stage)]
		if stageReceipt == nil {
			return nil, fmt.Errorf("receipt missing stage %q", stage)
		}
		if strings.TrimSpace(stageReceipt.StackName) == "" {
			return nil, fmt.Errorf("receipt missing stack name for stage %q", stage)
		}
	}

	return stages, nil
}

func receiptDestroyStages(stageMap map[string]*stageReceipt) ([]naming.Stage, error) {
	if len(stageMap) == 0 {
		return nil, errors.New("receipt has no stage entries")
	}

	known := map[string]naming.Stage{
		string(naming.StageDev):     naming.StageDev,
		string(naming.StageStaging): naming.StageStaging,
		string(naming.StageLive):    naming.StageLive,
	}
	for key := range stageMap {
		if _, ok := known[key]; !ok {
			return nil, fmt.Errorf("receipt has unsupported stage entry %q", key)
		}
	}

	order := []naming.Stage{naming.StageLive, naming.StageStaging, naming.StageDev}
	stages := make([]naming.Stage, 0, len(stageMap))
	for _, stage := range order {
		if _, ok := stageMap[string(stage)]; ok {
			stages = append(stages, stage)
		}
	}
	if len(stages) == 0 {
		return nil, errors.New("receipt has no deployable stages")
	}
	return stages, nil
}

func receiptArtifactBuckets(receipt *upReceipt, stages []naming.Stage) []string {
	unique := map[string]struct{}{}
	for _, stage := range stages {
		stageReceipt := receipt.Stages[string(stage)]
		if stageReceipt == nil {
			continue
		}

		clientBucket := strings.TrimSpace(stageReceipt.StackOutputs["ClientBucketName"])
		if clientBucket == "" && strings.TrimSpace(receipt.AccountID) != "" && strings.TrimSpace(receipt.Region) != "" {
			clientBucket = naming.S3BucketName(receipt.App, stage, "client", receipt.AccountID, receipt.Region)
		}
		if clientBucket != "" {
			unique[clientBucket] = struct{}{}
		}

		clientArtifactBucket := strings.TrimSpace(stageReceipt.StackOutputs["ClientArtifactBucketName"])
		if clientArtifactBucket == "" && strings.TrimSpace(receipt.AccountID) != "" && strings.TrimSpace(receipt.Region) != "" {
			clientArtifactBucket = naming.S3BucketName(receipt.App, stage, "client-artifacts", receipt.AccountID, receipt.Region)
		}
		if clientArtifactBucket != "" {
			unique[clientArtifactBucket] = struct{}{}
		}

		authBucket := strings.TrimSpace(stageReceipt.StackOutputs["AuthUIBucketName"])
		if authBucket == "" && strings.TrimSpace(receipt.AccountID) != "" && strings.TrimSpace(receipt.Region) != "" {
			authBucket = naming.S3BucketName(receipt.App, stage, "auth-ui", receipt.AccountID, receipt.Region)
		}
		if authBucket != "" {
			unique[authBucket] = struct{}{}
		}
	}

	buckets := make([]string, 0, len(unique))
	for bucket := range unique {
		buckets = append(buckets, bucket)
	}
	sort.Strings(buckets)
	return buckets
}

func deleteBucketObjectsIfPresent(ctx context.Context, client s3BucketAPI, bucket string) error {
	if err := deleteBucketObjectsFn(ctx, client, bucket); err != nil {
		if isS3BucketNotFoundError(err) {
			fmt.Println("  s3: bucket not found, skipping:", bucket)
			return nil
		}
		return err
	}
	return nil
}

func isS3BucketNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchBucket", "NotFound", "ResourceNotFoundException":
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nosuchbucket") || strings.Contains(msg, "bucket not found")
}
