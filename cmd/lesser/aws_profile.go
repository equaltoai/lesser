package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

type exportedCredentials struct {
	Version         int    `json:"Version"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken"`
	Expiration      string `json:"Expiration"`
}

func loadAWSConfigFromProfile(ctx context.Context, awsProfile string) (aws.Config, error) {
	profile := strings.TrimSpace(awsProfile)
	if profile == "" {
		return aws.Config{}, fmt.Errorf("aws profile is required")
	}

	region, err := awsCLIConfigureGet(ctx, profile, "region")
	if err != nil {
		return aws.Config{}, err
	}
	if strings.TrimSpace(region) == "" {
		return aws.Config{}, fmt.Errorf("AWS profile %q has no default region configured", profile)
	}

	creds, err := awsCLIExportCredentials(ctx, profile)
	if err != nil {
		return aws.Config{}, err
	}

	cfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		)),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load AWS config: %w", err)
	}

	return cfg, nil
}

func awsCLIConfigureGet(ctx context.Context, profile string, key string) (string, error) {
	args := []string{"configure", "get", key, "--profile", profile}
	cmd := exec.CommandContext(ctx, "aws", args...) //nolint:gosec // tool invocation
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("aws %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func awsCLIExportCredentials(ctx context.Context, profile string) (exportedCredentials, error) {
	args := []string{"configure", "export-credentials", "--profile", profile, "--format", "process"}
	cmd := exec.CommandContext(ctx, "aws", args...) //nolint:gosec // tool invocation
	out, err := cmd.CombinedOutput()
	if err != nil {
		return exportedCredentials{}, fmt.Errorf("aws %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	var creds exportedCredentials
	if err := json.Unmarshal(out, &creds); err != nil {
		return exportedCredentials{}, fmt.Errorf("parse aws export-credentials output: %w", err)
	}
	if strings.TrimSpace(creds.AccessKeyID) == "" || strings.TrimSpace(creds.SecretAccessKey) == "" {
		return exportedCredentials{}, fmt.Errorf("aws export-credentials returned empty keys for profile %q", profile)
	}
	return creds, nil
}
