package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	browsercors "github.com/equaltoai/lesser/pkg/security/cors"
)

const releaseAssemblyAppSlugPlaceholder = "appslugplaceholder"

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
	if err := validateSoulBindingIntegrationDeployContexts(map[string]string{
		"bodyEnabled":                  "true",
		"instancePlaneEnabled":         "true",
		"soulBindingIntegrationKeyArn": e.args.SoulBindingIntegrationKeyARN,
	}); err != nil {
		return nil, err
	}

	if err := ensureAPIGatewayCloudWatchLogsRoleFn(ctx, e.awsCfg); err != nil {
		return nil, err
	}

	sharedTemplateRaw, err := os.ReadFile(e.releaseAssembly.SharedTemplate) // #nosec G304 -- shared template path comes from verified release assembly extraction
	if err != nil {
		return nil, fmt.Errorf("read shared deploy assembly template: %w", err)
	}
	sharedTemplate, err := resolveReleaseAssemblyTemplate(sharedTemplateRaw, e.app)
	if err != nil {
		return nil, fmt.Errorf("resolve shared deploy assembly template: %w", err)
	}

	sharedStack := naming.SharedStackName(e.app)
	fmt.Println("\nDeploying shared stack from release assembly:", sharedStack)
	sharedOutputs, err := deployCloudFormationStackFn(ctx, e.awsCfg, cloudFormationDeployRequest{
		StackName:    sharedStack,
		TemplateBody: sharedTemplate,
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
		e.app,
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
		"LesserHostInstanceKeyArn":     strings.TrimSpace(e.args.LesserHostInstanceKeyARN),
		"SoulBindingIntegrationKeyArn": strings.TrimSpace(e.args.SoulBindingIntegrationKeyARN),
		"TranslationEnabled":           optionalBoolString(e.args.TranslationEnabled),
		"AllowAgents":                  optionalBoolString(e.args.AllowAgents),
		"AllowAgentRegistration":       optionalBoolString(e.args.AllowAgentRegistration),
		"TipEnabled":                   optionalBoolString(e.args.TipEnabled),
		"TipChainId":                   optionalIntString(e.args.TipChainID),
		"TipContractAddress":           strings.TrimSpace(e.args.TipContractAddress),
		"ApiCorsAllowedOrigins":        normalizeReleaseStageParameter("apiCorsAllowedOrigins", e.args.APICORSAllowedOrigins),
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
	case "apiCorsAllowedOrigins":
		return browsercors.NormalizeAllowedOriginsForDeploy(value)
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
	appSlug string,
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
		// Read the template, replace the synthesis placeholder with the concrete app
		// slug so logical IDs, SSM paths, and all embedded references match the target
		// instance, then upload the resolved template.
		rawTemplate, err := os.ReadFile(templatePath) // #nosec G304 -- template path from verified assembly
		if err != nil {
			return releaseAssemblyUploadResult{}, fmt.Errorf("read stage template %s: %w", templatePath, err)
		}
		resolvedTemplate, err := resolveReleaseAssemblyTemplate(rawTemplate, appSlug)
		if err != nil {
			return releaseAssemblyUploadResult{}, fmt.Errorf("resolve stage template %s: %w", templatePath, err)
		}
		templateKey := path.Join(templatePrefix, path.Base(templatePath))
		if _, err := putS3ObjectFn(ctx, s3Client, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(templateKey),
			Body:   strings.NewReader(resolvedTemplate),
		}); err != nil {
			return releaseAssemblyUploadResult{}, fmt.Errorf("upload resolved stage template to s3://%s/%s: %w", bucket, templateKey, err)
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

func resolveReleaseAssemblyTemplate(rawTemplate []byte, appSlug string) (string, error) {
	logicalIDSegment, err := releaseAssemblyLogicalIDSegment(appSlug)
	if err != nil {
		return "", err
	}

	var template any
	if err := json.Unmarshal(rawTemplate, &template); err != nil {
		return "", fmt.Errorf("parse template JSON: %w", err)
	}

	resolved, err := resolveReleaseAssemblyTemplateValue(template, nil, logicalIDSegment)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal resolved template JSON: %w", err)
	}
	return string(append(data, '\n')), nil
}

func releaseAssemblyLogicalIDSegment(appSlug string) (string, error) {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(appSlug) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "", fmt.Errorf("release app slug %q has no alphanumeric characters for CloudFormation logical IDs", appSlug)
	}
	return builder.String(), nil
}

func resolveReleaseAssemblyTemplateValue(value any, path []string, logicalIDSegment string) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			resolvedKey := strings.ReplaceAll(key, releaseAssemblyAppSlugPlaceholder, logicalIDSegment)
			if _, exists := out[resolvedKey]; exists {
				return nil, fmt.Errorf("release assembly template logical ID collision at %s", strings.Join(append(path, key), "."))
			}
			resolvedChild, err := resolveReleaseAssemblyTemplateValue(child, append(path, key), logicalIDSegment)
			if err != nil {
				return nil, err
			}
			out[resolvedKey] = resolvedChild
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			resolvedChild, err := resolveReleaseAssemblyTemplateValue(child, append(path, fmt.Sprintf("[%d]", i)), logicalIDSegment)
			if err != nil {
				return nil, err
			}
			out[i] = resolvedChild
		}
		return out, nil
	case string:
		if !strings.Contains(v, releaseAssemblyAppSlugPlaceholder) {
			return v, nil
		}
		if releaseAssemblyTemplateStringIsLogicalRef(path) {
			return strings.ReplaceAll(v, releaseAssemblyAppSlugPlaceholder, logicalIDSegment), nil
		}
		return nil, fmt.Errorf("release assembly template contains unresolved app slug placeholder at %s", strings.Join(path, "."))
	default:
		return value, nil
	}
}

func releaseAssemblyTemplateStringIsLogicalRef(path []string) bool {
	for _, part := range path {
		switch part {
		case "DependsOn", "Fn::GetAtt":
			return true
		}
	}
	if len(path) == 0 {
		return false
	}
	return path[len(path)-1] == "Ref"
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
