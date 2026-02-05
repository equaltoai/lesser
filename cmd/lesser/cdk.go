package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type cdkDeployRequest struct {
	StackName    string
	App          string
	BaseDomain   string
	HostedZoneID string
	Region       string
	StageFilter  string
	WithStaging  bool
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
	if err := runCommandFn(ctx, "cdk", args, execOptions{
		Dir: cdkDir,
		Env: map[string]string{
			"AWS_PROFILE":        awsProfile,
			"AWS_REGION":         region,
			"AWS_DEFAULT_REGION": region,
		},
	}); err != nil {
		return fmt.Errorf("cdk bootstrap: %w", err)
	}
	return nil
}

func cdkDeployWithOutputs(ctx context.Context, repoRoot string, awsProfile string, req cdkDeployRequest) (cdkDeployResult, error) {
	cdkDir := filepath.Join(repoRoot, "infra", "cdk")

	outputsPath := filepath.Join(repoRoot, "tmp", "cdk-outputs.json")
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

	stage := strings.TrimSpace(strings.ToLower(req.StageFilter))
	if stage != "" {
		args = append(args, "--context", fmt.Sprintf("stage=%s", stage))
	}
	if req.WithStaging {
		args = append(args, "--context", "withStaging=true")
	}
	if err := runCommandFn(ctx, "cdk", args, execOptions{
		Dir: cdkDir,
		Env: map[string]string{
			"AWS_PROFILE":        awsProfile,
			"AWS_REGION":         req.Region,
			"AWS_DEFAULT_REGION": req.Region,
		},
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

	if err := runCommandFn(ctx, "cdk", args, execOptions{
		Dir: cdkDir,
		Env: map[string]string{
			"AWS_PROFILE":        awsProfile,
			"AWS_REGION":         req.Region,
			"AWS_DEFAULT_REGION": req.Region,
		},
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
