package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudformationtypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

type cloudFormationDeployRequest struct {
	StackName    string
	TemplateBody string
	TemplateURL  string
	Parameters   map[string]string
}

var (
	deployCloudFormationStackFn     = deployCloudFormationStack
	newCloudFormationClientFn       = cloudformation.NewFromConfig
	cloudFormationStackExistsFn     = cloudFormationStackExists
	describeCloudFormationOutputsFn = describeCloudFormationOutputs
	createCloudFormationStackFn     = createCloudFormationStack
	updateCloudFormationStackFn     = updateCloudFormationStack
	waitCloudFormationStackCreateFn = waitCloudFormationStackCreate
	waitCloudFormationStackUpdateFn = waitCloudFormationStackUpdate
)

func deployCloudFormationStack(
	ctx context.Context,
	cfg aws.Config,
	req cloudFormationDeployRequest,
) (map[string]string, error) {
	if strings.TrimSpace(req.StackName) == "" {
		return nil, fmt.Errorf("cloudformation stack name is required")
	}
	if strings.TrimSpace(req.TemplateBody) == "" && strings.TrimSpace(req.TemplateURL) == "" {
		return nil, fmt.Errorf("cloudformation deploy requires template body or template URL")
	}
	if strings.TrimSpace(req.TemplateBody) != "" && strings.TrimSpace(req.TemplateURL) != "" {
		return nil, fmt.Errorf("cloudformation deploy must use either template body or template URL")
	}

	client := newCloudFormationClientFn(cfg)
	exists, err := cloudFormationStackExistsFn(ctx, client, req.StackName)
	if err != nil {
		return nil, err
	}

	parameters := makeCloudFormationParameters(req.Parameters)
	capabilities := []cloudformationtypes.Capability{
		cloudformationtypes.CapabilityCapabilityIam,
		cloudformationtypes.CapabilityCapabilityNamedIam,
	}

	if !exists {
		input := &cloudformation.CreateStackInput{
			StackName:    aws.String(req.StackName),
			Capabilities: capabilities,
			Parameters:   parameters,
		}
		if strings.TrimSpace(req.TemplateBody) != "" {
			input.TemplateBody = aws.String(req.TemplateBody)
		} else {
			input.TemplateURL = aws.String(req.TemplateURL)
		}
		if _, err := createCloudFormationStackFn(ctx, client, input); err != nil {
			return nil, fmt.Errorf("create cloudformation stack %s: %w", req.StackName, err)
		}
		if err := waitCloudFormationStackCreateFn(ctx, client, req.StackName); err != nil {
			return nil, fmt.Errorf("wait for cloudformation stack create %s: %w", req.StackName, err)
		}
	} else {
		input := &cloudformation.UpdateStackInput{
			StackName:    aws.String(req.StackName),
			Capabilities: capabilities,
			Parameters:   parameters,
		}
		if strings.TrimSpace(req.TemplateBody) != "" {
			input.TemplateBody = aws.String(req.TemplateBody)
		} else {
			input.TemplateURL = aws.String(req.TemplateURL)
		}
		if _, err := updateCloudFormationStackFn(ctx, client, input); err != nil {
			if isNoCloudFormationUpdatesError(err) {
				return describeCloudFormationOutputsFn(ctx, client, req.StackName)
			}
			return nil, fmt.Errorf("update cloudformation stack %s: %w", req.StackName, err)
		}
		if err := waitCloudFormationStackUpdateFn(ctx, client, req.StackName); err != nil {
			return nil, fmt.Errorf("wait for cloudformation stack update %s: %w", req.StackName, err)
		}
	}

	return describeCloudFormationOutputsFn(ctx, client, req.StackName)
}

func createCloudFormationStack(
	ctx context.Context,
	client *cloudformation.Client,
	input *cloudformation.CreateStackInput,
) (*cloudformation.CreateStackOutput, error) {
	return client.CreateStack(ctx, input)
}

func updateCloudFormationStack(
	ctx context.Context,
	client *cloudformation.Client,
	input *cloudformation.UpdateStackInput,
) (*cloudformation.UpdateStackOutput, error) {
	return client.UpdateStack(ctx, input)
}

func waitCloudFormationStackCreate(ctx context.Context, client *cloudformation.Client, stackName string) error {
	waiter := cloudformation.NewStackCreateCompleteWaiter(client)
	return waiter.Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	}, 30*time.Minute)
}

func waitCloudFormationStackUpdate(ctx context.Context, client *cloudformation.Client, stackName string) error {
	waiter := cloudformation.NewStackUpdateCompleteWaiter(client)
	return waiter.Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	}, 30*time.Minute)
}

func cloudFormationStackExists(ctx context.Context, client *cloudformation.Client, stackName string) (bool, error) {
	_, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err == nil {
		return true, nil
	}
	if isCloudFormationStackNotFoundError(err) {
		return false, nil
	}
	return false, fmt.Errorf("describe cloudformation stack %s: %w", stackName, err)
}

func describeCloudFormationOutputs(ctx context.Context, client *cloudformation.Client, stackName string) (map[string]string, error) {
	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return nil, fmt.Errorf("describe cloudformation stack %s: %w", stackName, err)
	}
	if len(out.Stacks) == 0 {
		return nil, fmt.Errorf("cloudformation stack %s not found after deploy", stackName)
	}

	outputs := map[string]string{}
	for _, output := range out.Stacks[0].Outputs {
		key := strings.TrimSpace(aws.ToString(output.OutputKey))
		if key == "" {
			continue
		}
		outputs[key] = aws.ToString(output.OutputValue)
	}
	return outputs, nil
}

func makeCloudFormationParameters(values map[string]string) []cloudformationtypes.Parameter {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]cloudformationtypes.Parameter, 0, len(keys))
	for _, key := range keys {
		params = append(params, cloudformationtypes.Parameter{
			ParameterKey:   aws.String(key),
			ParameterValue: aws.String(values[key]),
		})
	}
	return params
}

func isCloudFormationStackNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not exist")
}

func isNoCloudFormationUpdatesError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "No updates are to be performed")
}

func extractCloudFormationOutputs(outputs map[string]string) map[string]string {
	if outputs == nil {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(outputs))
	for key, value := range outputs {
		cloned[key] = value
	}
	return cloned
}

func resolveReleaseAssetBucket(sharedOutputs map[string]string, app, accountID, region string) string {
	if bucket := strings.TrimSpace(sharedOutputs["ReleaseAssetBucketName"]); bucket != "" {
		return bucket
	}
	return naming.S3BucketName(app, naming.StageShared, "release-assets", accountID, region)
}
