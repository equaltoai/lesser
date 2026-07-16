package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	browsercors "github.com/equaltoai/lesser/pkg/security/cors"
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
	resolveStackOutputsFn  = resolveStackOutputs
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
		"lesserVersion":                firstNonEmpty(os.Getenv("LESSER_VERSION"), os.Getenv("VERSION")),
		"lesserHostUrl":                strings.TrimSpace(os.Getenv("LESSER_HOST_URL")),
		"lesserHostInstanceKeyArn":     strings.TrimSpace(os.Getenv("LESSER_HOST_INSTANCE_KEY_ARN")),
		"lesserHostAttestationsUrl":    strings.TrimSpace(os.Getenv("LESSER_HOST_ATTESTATIONS_URL")),
		"soulBindingIntegrationKeyArn": strings.TrimSpace(os.Getenv("SOUL_BINDING_INTEGRATION_KEY_ARN")),
		"bodyEnabled":                  strings.TrimSpace(os.Getenv("BODY_ENABLED")),
		"instancePlaneEnabled":         strings.TrimSpace(os.Getenv("INSTANCE_PLANE_ENABLED")),
		"translationEnabled":           strings.TrimSpace(os.Getenv("TRANSLATION_ENABLED")),
		"allowAgents":                  strings.TrimSpace(os.Getenv("ALLOW_AGENTS")),
		"allowAgentRegistration":       strings.TrimSpace(os.Getenv("ALLOW_AGENT_REGISTRATION")),
		"tipEnabled":                   strings.TrimSpace(os.Getenv("TIP_ENABLED")),
		"tipChainId":                   strings.TrimSpace(os.Getenv("TIP_CHAIN_ID")),
		"tipContractAddress":           strings.TrimSpace(os.Getenv("TIP_CONTRACT_ADDRESS")),
		"apiCorsAllowedOrigins":        firstNonEmpty(os.Getenv("API_CORS_ALLOWED_ORIGINS"), os.Getenv("CORS_ALLOWED_ORIGINS")),
	}
	normalizeContext := func(key, value string) string {
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

	outputs, err := resolveStackOutputsFn(ctx, awsProfile, req)
	if err != nil {
		return cdkDeployResult{}, fmt.Errorf("resolve stack outputs for %s: %w", req.StackName, err)
	}
	if err := writeCdkOutputs(outputsPath, req.StackName, outputs); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: persist cdk outputs for %s: %v\n", req.StackName, err)
	}

	return cdkDeployResult{
		StackName: req.StackName,
		Outputs:   outputs,
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

func writeCdkOutputs(path string, stackName string, outputs map[string]string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("cdk outputs path is required")
	}
	if strings.TrimSpace(stackName) == "" {
		return fmt.Errorf("cdk stack name is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	serialized, err := json.MarshalIndent(map[string]map[string]string{
		stackName: cloneStringMap(outputs),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cdk outputs: %w", err)
	}
	serialized = append(serialized, '\n')
	if err := os.WriteFile(path, serialized, 0o600); err != nil {
		return fmt.Errorf("write cdk outputs: %w", err)
	}
	return nil
}

func resolveStackOutputs(ctx context.Context, awsProfile string, req cdkDeployRequest) (map[string]string, error) {
	cfg, _, err := loadAWSConfigForCLIFn(ctx, awsProfile)
	if err != nil {
		return nil, err
	}
	if region := strings.TrimSpace(req.Region); region != "" {
		cfg.Region = region
	}

	outputs, err := describeCloudFormationOutputsFn(ctx, newCloudFormationClientFn(cfg), req.StackName)
	if err != nil {
		return nil, err
	}
	return cloneStringMap(outputs), nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
