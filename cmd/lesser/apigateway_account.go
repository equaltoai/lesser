package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const apiGatewayLogsRoleName = "lesser-apigateway-cloudwatch-logs"

var ensureAPIGatewayCloudWatchLogsRoleFn = ensureAPIGatewayCloudWatchLogsRole
var sleepFn = time.Sleep

type apiGatewayAccountAPI interface {
	GetAccount(ctx context.Context, params *apigateway.GetAccountInput, optFns ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error)
	UpdateAccount(ctx context.Context, params *apigateway.UpdateAccountInput, optFns ...func(*apigateway.Options)) (*apigateway.UpdateAccountOutput, error)
}

type iamRoleAPI interface {
	GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	CreateRole(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	AttachRolePolicy(ctx context.Context, params *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
}

var newAPIGatewayClientFn = func(cfg aws.Config) apiGatewayAccountAPI {
	return apigateway.NewFromConfig(cfg)
}

var newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
	return iam.NewFromConfig(cfg)
}

func ensureAPIGatewayCloudWatchLogsRole(ctx context.Context, cfg aws.Config) error {
	client := newAPIGatewayClientFn(cfg)

	account, err := client.GetAccount(ctx, &apigateway.GetAccountInput{})
	if err != nil {
		return fmt.Errorf("apigateway:GetAccount: %w", err)
	}
	if arn := strings.TrimSpace(aws.ToString(account.CloudwatchRoleArn)); arn != "" {
		return nil
	}

	roleArn, err := ensureIAMRoleForAPIGatewayLogs(ctx, cfg)
	if err != nil {
		return err
	}

	patch := []apigwtypes.PatchOperation{
		{
			Op:    apigwtypes.OpReplace,
			Path:  aws.String("/cloudwatchRoleArn"),
			Value: aws.String(roleArn),
		},
	}

	var lastErr error
	for attempt := 1; attempt <= 6; attempt++ {
		_, err = client.UpdateAccount(ctx, &apigateway.UpdateAccountInput{PatchOperations: patch})
		if err == nil {
			return nil
		}
		lastErr = err
		sleepFn(time.Duration(attempt) * 2 * time.Second)
	}

	return fmt.Errorf("apigateway:UpdateAccount (set cloudwatchRoleArn): %w", lastErr)
}

func ensureIAMRoleForAPIGatewayLogs(ctx context.Context, cfg aws.Config) (string, error) {
	client := newIAMClientFn(cfg)

	existing, err := client.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(apiGatewayLogsRoleName)})
	if err == nil && existing.Role != nil && strings.TrimSpace(aws.ToString(existing.Role.Arn)) != "" {
		if err := ensureRoleHasAPIGatewayLogsPolicy(ctx, client); err != nil {
			return "", err
		}
		return aws.ToString(existing.Role.Arn), nil
	}
	var notFound *iamtypes.NoSuchEntityException
	if err != nil && !errors.As(err, &notFound) {
		return "", fmt.Errorf("iam:GetRole %q: %w", apiGatewayLogsRoleName, err)
	}

	assumeRolePolicy, err := json.Marshal(map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect": "Allow",
				"Principal": map[string]string{
					"Service": "apigateway.amazonaws.com",
				},
				"Action": "sts:AssumeRole",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal API Gateway assume role policy: %w", err)
	}

	out, err := client.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(apiGatewayLogsRoleName),
		AssumeRolePolicyDocument: aws.String(string(assumeRolePolicy)),
		Description:              aws.String("Allows API Gateway to push logs to CloudWatch Logs (required for access logging)"),
	})
	if err != nil {
		return "", fmt.Errorf("iam:CreateRole %q: %w", apiGatewayLogsRoleName, err)
	}

	if err := ensureRoleHasAPIGatewayLogsPolicy(ctx, client); err != nil {
		return "", err
	}

	roleArn := strings.TrimSpace(aws.ToString(out.Role.Arn))
	if roleArn == "" {
		return "", fmt.Errorf("iam:CreateRole %q returned empty role ARN", apiGatewayLogsRoleName)
	}
	return roleArn, nil
}

func ensureRoleHasAPIGatewayLogsPolicy(ctx context.Context, client iamRoleAPI) error {
	const policyArn = "arn:aws:iam::aws:policy/service-role/AmazonAPIGatewayPushToCloudWatchLogs"
	_, err := client.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(apiGatewayLogsRoleName),
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		return fmt.Errorf("iam:AttachRolePolicy %q (%s): %w", apiGatewayLogsRoleName, policyArn, err)
	}
	return nil
}
