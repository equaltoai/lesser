package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

type releaseAssemblyUploadResult struct {
	StageTemplateURLs map[naming.Stage]string
}

var uploadReleaseAssemblyAssetsFn = uploadReleaseAssemblyAssets
var (
	newS3ClientFn               = s3.NewFromConfig
	newS3PresignClientFn        = s3.NewPresignClient
	uploadReleaseAssemblyFileFn = uploadReleaseAssemblyFile
	presignReleaseAssemblyURLFn = presignReleaseAssemblyURL
	putS3ObjectFn               = putS3Object
)

func (e *upEnv) deployFromReleaseAssembly(ctx context.Context) (*upReceipt, error) {
	if e.releaseAssembly == nil {
		return nil, fmt.Errorf("release deploy assembly is not prepared")
	}

	receipt := newUpReceipt(e.app, e.baseDomain, e.awsProfile, e.accountID, e.awsCfg.Region, e.stages, e.hostedZone)
	receipt.Integration = resolveIntegrationReceipt(e.args)

	if err := ensureAPIGatewayCloudWatchLogsRoleFn(ctx, e.awsCfg); err != nil {
		return nil, err
	}

	sharedTemplate, err := os.ReadFile(e.releaseAssembly.SharedTemplate) // #nosec G304 -- shared template path comes from verified release assembly extraction
	if err != nil {
		return nil, fmt.Errorf("read shared deploy assembly template: %w", err)
	}

	sharedStack := naming.SharedStackName(e.app)
	fmt.Println("\nDeploying shared stack from release assembly:", sharedStack)
	sharedOutputs, err := deployCloudFormationStackFn(ctx, e.awsCfg, cloudFormationDeployRequest{
		StackName:    sharedStack,
		TemplateBody: string(sharedTemplate),
		Parameters: map[string]string{
			"AppSlug": e.app,
		},
	})
	if err != nil {
		return nil, err
	}
	receipt.SharedOutputs = extractCloudFormationOutputs(sharedOutputs)

	releaseAssetBucket := resolveReleaseAssetBucket(sharedOutputs, e.app, e.accountID, e.awsCfg.Region)
	fmt.Println("Shared release asset bucket:", releaseAssetBucket)

	uploadResult, err := uploadReleaseAssemblyAssetsFn(
		ctx,
		e.awsCfg,
		releaseAssetBucket,
		e.releaseVersion,
		e.releaseGitSHA,
		*e.releaseAssembly,
	)
	if err != nil {
		return nil, err
	}

	for _, stage := range e.stages {
		templateURL := strings.TrimSpace(uploadResult.StageTemplateURLs[stage])
		if templateURL == "" {
			return nil, fmt.Errorf("release deploy assembly missing template URL for %s", stage)
		}

		stackName := naming.StageStackName(e.app, stage)
		fmt.Println("\nDeploying stage stack from release assembly:", stackName)
		outputs, err := deployCloudFormationStackFn(ctx, e.awsCfg, cloudFormationDeployRequest{
			StackName:   stackName,
			TemplateURL: templateURL,
			Parameters:  e.releaseStageTemplateParameters(releaseAssetBucket),
		})
		if err != nil {
			return nil, err
		}

		receipt.Stages[string(stage)].StackName = stackName
		receipt.Stages[string(stage)].Domain = naming.StageDomain(stage, e.baseDomain)
		receipt.Stages[string(stage)].StackOutputs = extractCloudFormationOutputs(outputs)
	}

	return receipt, nil
}

func (e *upEnv) releaseStageTemplateParameters(releaseAssetBucket string) map[string]string {
	params := map[string]string{
		"AppSlug":                e.app,
		"BaseDomain":             e.baseDomain,
		"HostedZoneId":           e.hostedZone.ID,
		"ReleaseAssetBucketName": releaseAssetBucket,
		"LesserHostUrl":          normalizeReleaseStageParameter("lesserHostUrl", e.args.LesserHostURL),
		"LesserHostAttestationsUrl": normalizeReleaseStageParameter(
			"lesserHostAttestationsUrl",
			e.args.LesserHostAttestationsURL,
		),
		"LesserHostInstanceKeyArn": strings.TrimSpace(e.args.LesserHostInstanceKeyARN),
		"TranslationEnabled":       optionalBoolString(e.args.TranslationEnabled),
		"TipEnabled":               optionalBoolString(e.args.TipEnabled),
		"TipChainId":               optionalIntString(e.args.TipChainID),
		"TipContractAddress":       strings.TrimSpace(e.args.TipContractAddress),
	}
	return params
}

func normalizeReleaseStageParameter(key string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch key {
	case "lesserHostUrl", "lesserHostAttestationsUrl":
		return strings.TrimRight(value, "/")
	default:
		return value
	}
}

func optionalBoolString(value *bool) string {
	if value == nil {
		return ""
	}
	return strconv.FormatBool(*value)
}

func optionalIntString(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func uploadReleaseAssemblyAssets(
	ctx context.Context,
	cfg aws.Config,
	bucket string,
	version string,
	gitSHA string,
	assembly releaseDeployAssemblyInstallResult,
) (releaseAssemblyUploadResult, error) {
	if strings.TrimSpace(bucket) == "" {
		return releaseAssemblyUploadResult{}, fmt.Errorf("release asset bucket is required")
	}

	s3Client := newS3ClientFn(cfg)
	presignClient := newS3PresignClientFn(s3Client)
	uploaded := releaseAssemblyUploadResult{
		StageTemplateURLs: map[naming.Stage]string{},
	}

	for _, asset := range assembly.Assets {
		if err := uploadReleaseAssemblyFileFn(ctx, s3Client, bucket, asset.ObjectKey, asset.LocalPath); err != nil {
			return releaseAssemblyUploadResult{}, err
		}
	}
	fmt.Printf("  s3: uploaded %d deploy assembly asset(s)\n", len(assembly.Assets))

	templatePrefix := path.Join("release-assembly", version, gitSHA, "templates")
	for _, stage := range []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive} {
		templatePath := strings.TrimSpace(assembly.StageTemplates[stage])
		if templatePath == "" {
			return releaseAssemblyUploadResult{}, fmt.Errorf("release assembly missing stage template for %s", stage)
		}
		templateKey := path.Join(templatePrefix, path.Base(templatePath))
		if err := uploadReleaseAssemblyFileFn(ctx, s3Client, bucket, templateKey, templatePath); err != nil {
			return releaseAssemblyUploadResult{}, err
		}
		presigned, err := presignReleaseAssemblyURLFn(ctx, presignClient, bucket, templateKey)
		if err != nil {
			return releaseAssemblyUploadResult{}, err
		}
		uploaded.StageTemplateURLs[stage] = presigned
	}
	fmt.Printf("  s3: uploaded %d stage template(s)\n", len(uploaded.StageTemplateURLs))

	return uploaded, nil
}

func presignReleaseAssemblyURL(
	ctx context.Context,
	client *s3.PresignClient,
	bucket string,
	templateKey string,
) (string, error) {
	presigned, err := client.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(templateKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = time.Hour
	})
	if err != nil {
		return "", fmt.Errorf("presign deploy assembly template %s: %w", templateKey, err)
	}
	return presigned.URL, nil
}

func uploadReleaseAssemblyFile(
	ctx context.Context,
	client *s3.Client,
	bucket string,
	objectKey string,
	localPath string,
) error {
	f, err := os.Open(localPath) // #nosec G304 -- local path comes from verified release assembly extraction
	if err != nil {
		return fmt.Errorf("open release assembly file %s: %w", localPath, err)
	}
	defer func() { _ = f.Close() }()

	_, err = putS3ObjectFn(ctx, client, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("upload release assembly file to s3://%s/%s: %w", bucket, objectKey, err)
	}
	return nil
}

func putS3Object(ctx context.Context, client *s3.Client, input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	return client.PutObject(ctx, input)
}
