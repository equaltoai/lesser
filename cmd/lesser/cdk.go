package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type cdkDeployRequest struct {
	StackName       string
	App             string
	BaseDomain      string
	HostedZoneID    string
	Region          string
	LambdaAssetRoot string
	OutputsPath     string
	StageFilter     string
	WithStaging     bool
	Contexts        map[string]string
}

type cdkDestroyRequest struct {
	StackName    string
	App          string
	BaseDomain   string
	HostedZoneID string
	Region       string
	StageFilter  string
	WithStaging  bool
}

type cdkDeployResult struct {
	StackName string
	Outputs   map[string]string
}

var (
	cdkBootstrapFn         = cdkBootstrap
	cdkDeployWithOutputsFn = cdkDeployWithOutputs
	cdkDestroyStackFn      = cdkDestroyStack
)

func cdkBootstrap(ctx context.Context, repoRoot string, awsProfile string, accountID string, region string) error {
	cdkDir := filepath.Join(repoRoot, "infra", "cdk")

	args := []string{
		"bootstrap",
		fmt.Sprintf("aws://%s/%s", accountID, region),
		// The CDK CLI executes the app for bootstrap in this repo; ensure it can synth
		// without stage-domain context by scoping to the shared stack.
		"--context",
		"stage=shared",
	}

	fmt.Println("\nEnsuring CDK bootstrap:", args[len(args)-1])
	env := map[string]string{
		"AWS_REGION":         region,
		"AWS_DEFAULT_REGION": region,
	}
	if strings.TrimSpace(awsProfile) != "" {
		env["AWS_PROFILE"] = awsProfile
	}
	if err := runCommandFn(ctx, "cdk", args, execOptions{
		Dir: cdkDir,
		Env: env,
	}); err != nil {
		return fmt.Errorf("cdk bootstrap: %w", err)
	}
	return nil
}

func cdkDeployWithOutputs(ctx context.Context, repoRoot string, awsProfile string, req cdkDeployRequest) (cdkDeployResult, error) {
	cdkDir := filepath.Join(repoRoot, "infra", "cdk")

	outputsPath := strings.TrimSpace(req.OutputsPath)
	if outputsPath == "" {
		outputsPath = filepath.Join(repoRoot, "tmp", "cdk-outputs.json")
	}
	if err := os.MkdirAll(filepath.Dir(outputsPath), 0o750); err != nil {
		return cdkDeployResult{}, err
	}

	args := []string{
		"deploy",
		req.StackName,
		"--require-approval",
		"never",
		"--outputs-file",
		outputsPath,
		"--context",
		fmt.Sprintf("app=%s", req.App),
		"--context",
		fmt.Sprintf("baseDomain=%s", req.BaseDomain),
		"--context",
		fmt.Sprintf("hostedZoneId=%s", req.HostedZoneID),
	}

	// Optional deployment configuration from environment variables.
	// This keeps `lesser up` ergonomic while allowing one-off instance config without editing CDK code.
	envContexts := map[string]string{
		"lesserHostUrl":             strings.TrimSpace(os.Getenv("LESSER_HOST_URL")),
		"lesserHostInstanceKeyArn":  strings.TrimSpace(os.Getenv("LESSER_HOST_INSTANCE_KEY_ARN")),
		"lesserHostAttestationsUrl": strings.TrimSpace(os.Getenv("LESSER_HOST_ATTESTATIONS_URL")),
		"bodyEnabled":               strings.TrimSpace(os.Getenv("BODY_ENABLED")),
		"translationEnabled":        strings.TrimSpace(os.Getenv("TRANSLATION_ENABLED")),
		"tipEnabled":                strings.TrimSpace(os.Getenv("TIP_ENABLED")),
		"tipChainId":                strings.TrimSpace(os.Getenv("TIP_CHAIN_ID")),
		"tipContractAddress":        strings.TrimSpace(os.Getenv("TIP_CONTRACT_ADDRESS")),
	}
	normalizeContext := func(key, value string) string {
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

	contexts := map[string]string{}
	for key, value := range envContexts {
		if v := normalizeContext(key, value); v != "" {
			contexts[key] = v
		}
	}
	for key, value := range req.Contexts {
		if v := normalizeContext(key, value); v != "" {
			contexts[key] = v
		}
	}

	if err := rejectLambdaFunctionURLHost(contexts["lesserHostUrl"]); err != nil {
		return cdkDeployResult{}, err
	}
	if v := strings.TrimSpace(req.LambdaAssetRoot); v != "" {
		contexts["lambdaAssetRoot"] = v
	}

	keys := make([]string, 0, len(contexts))
	for key := range contexts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--context", fmt.Sprintf("%s=%s", key, contexts[key]))
	}

	stage := strings.TrimSpace(strings.ToLower(req.StageFilter))
	if stage != "" {
		args = append(args, "--context", fmt.Sprintf("stage=%s", stage))
	}
	if req.WithStaging {
		args = append(args, "--context", "withStaging=true")
	}
	env := map[string]string{
		"AWS_REGION":         req.Region,
		"AWS_DEFAULT_REGION": req.Region,
	}
	if strings.TrimSpace(awsProfile) != "" {
		env["AWS_PROFILE"] = awsProfile
	}
	if err := runCommandFn(ctx, "cdk", args, execOptions{
		Dir: cdkDir,
		Env: env,
	}); err != nil {
		return cdkDeployResult{}, fmt.Errorf("cdk deploy %s: %w", req.StackName, err)
	}

	out, err := parseCdkOutputs(outputsPath)
	if err != nil {
		return cdkDeployResult{}, err
	}

	return cdkDeployResult{
		StackName: req.StackName,
		Outputs:   out[req.StackName],
	}, nil
}

func cdkDestroyStack(ctx context.Context, repoRoot string, awsProfile string, req cdkDestroyRequest) error {
	cdkDir := filepath.Join(repoRoot, "infra", "cdk")

	args := []string{
		"destroy",
		req.StackName,
		"--force",
		"--context",
		fmt.Sprintf("app=%s", req.App),
		"--context",
		fmt.Sprintf("baseDomain=%s", req.BaseDomain),
	}
	if strings.TrimSpace(req.HostedZoneID) != "" {
		args = append(args, "--context", fmt.Sprintf("hostedZoneId=%s", req.HostedZoneID))
	}

	stage := strings.TrimSpace(strings.ToLower(req.StageFilter))
	if stage != "" {
		args = append(args, "--context", fmt.Sprintf("stage=%s", stage))
	}
	if req.WithStaging {
		args = append(args, "--context", "withStaging=true")
	}

	env := map[string]string{
		"AWS_REGION":         req.Region,
		"AWS_DEFAULT_REGION": req.Region,
	}
	if strings.TrimSpace(awsProfile) != "" {
		env["AWS_PROFILE"] = awsProfile
	}
	if err := runCommandFn(ctx, "cdk", args, execOptions{
		Dir: cdkDir,
		Env: env,
	}); err != nil {
		return fmt.Errorf("cdk destroy %s: %w", req.StackName, err)
	}
	return nil
}

func parseCdkOutputs(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- file path is derived from repo root
	if err != nil {
		return nil, fmt.Errorf("read cdk outputs: %w", err)
	}

	var out map[string]map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse cdk outputs: %w", err)
	}
	return out, nil
}
