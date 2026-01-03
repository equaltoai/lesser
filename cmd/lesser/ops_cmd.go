package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func runLogs(argv []string) error {
	fs := flag.NewFlagSet("lesser logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var function string
	var env string
	var awsProfile string
	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&function, "function", "", "lambda function name (e.g. api)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if strings.TrimSpace(function) == "" {
		return fmt.Errorf("--function is required")
	}

	if err := ensureAWSCLIToolAvailable(); err != nil {
		return err
	}

	appName := strings.TrimSpace(app)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		return err
	}

	stage := naming.StageForEnvironment(env)
	switch stage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return fmt.Errorf("invalid env %q (expected dev|staging|live)", env)
	}

	functionName := naming.StageResourceName(normalizedApp, stage, strings.TrimSpace(function))

	logGroup := fmt.Sprintf("/aws/lambda/%s", functionName)
	fmt.Println("Tailing logs for", functionName)

	overrides := map[string]string{}
	if strings.TrimSpace(awsProfile) != "" {
		overrides["AWS_PROFILE"] = awsProfile
	}

	return runCommandFn(context.Background(), "aws", []string{"logs", "tail", logGroup, "--follow"}, execOptions{
		Env: overrides,
	})
}

func runMetrics(argv []string) error {
	fs := flag.NewFlagSet("lesser metrics", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var function string
	var env string
	var awsProfile string
	var region string
	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&function, "function", "", "lambda function name (e.g. api)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.StringVar(&region, "region", envOrDefault("AWS_REGION", "us-east-1"), "AWS region (sets AWS_REGION/AWS_DEFAULT_REGION)")

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if strings.TrimSpace(function) == "" {
		return fmt.Errorf("--function is required")
	}

	if err := ensureAWSCLIToolAvailable(); err != nil {
		return err
	}

	appName := strings.TrimSpace(app)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		return err
	}

	stage := naming.StageForEnvironment(env)
	switch stage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return fmt.Errorf("invalid env %q (expected dev|staging|live)", env)
	}

	functionName := naming.StageResourceName(normalizedApp, stage, strings.TrimSpace(function))

	fmt.Println("Fetching metrics for", functionName)

	end := time.Now().UTC()
	start := end.Add(-1 * time.Hour)
	format := "2006-01-02T15:04:05"

	overrides := map[string]string{
		"AWS_REGION":         region,
		"AWS_DEFAULT_REGION": region,
	}
	if strings.TrimSpace(awsProfile) != "" {
		overrides["AWS_PROFILE"] = awsProfile
	}

	return runCommandFn(context.Background(), "aws", []string{
		"cloudwatch", "get-metric-statistics",
		"--namespace", "AWS/Lambda",
		"--metric-name", "Invocations",
		"--dimensions", "Name=FunctionName,Value=" + functionName,
		"--start-time", start.Format(format),
		"--end-time", end.Format(format),
		"--period", "300",
		"--statistics", "Sum,Average",
	}, execOptions{
		Env: overrides,
	})
}

func runErrors(argv []string) error {
	fs := flag.NewFlagSet("lesser errors", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var env string
	var awsProfile string
	var function string
	var maxItems int
	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&function, "function", "api", "lambda function name to scan for errors (default: api)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.IntVar(&maxItems, "max-items", 10, "maximum log events to return")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if err := ensureAWSCLIToolAvailable(); err != nil {
		return err
	}

	appName := strings.TrimSpace(app)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		return err
	}

	stage := naming.StageForEnvironment(env)
	switch stage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return fmt.Errorf("invalid env %q (expected dev|staging|live)", env)
	}

	functionName := naming.StageResourceName(normalizedApp, stage, strings.TrimSpace(function))

	logGroup := fmt.Sprintf("/aws/lambda/%s", functionName)
	fmt.Println("Recent errors in", string(stage), "stage (log group:", logGroup+"):")

	overrides := map[string]string{}
	if strings.TrimSpace(awsProfile) != "" {
		overrides["AWS_PROFILE"] = awsProfile
	}

	return runCommandFn(context.Background(), "aws", []string{
		"logs", "filter-log-events",
		"--log-group-name", logGroup,
		"--filter-pattern", "ERROR",
		"--max-items", fmt.Sprintf("%d", maxItems),
		"--output", "text",
	}, execOptions{
		Env: overrides,
	})
}

func runDashboard(argv []string) error {
	fs := flag.NewFlagSet("lesser dashboard", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var env string
	var region string
	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&region, "region", envOrDefault("AWS_REGION", "us-east-1"), "AWS region for console URL")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	appName := strings.TrimSpace(app)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		return err
	}

	stage := naming.StageForEnvironment(env)
	switch stage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return fmt.Errorf("invalid env %q (expected dev|staging|live)", env)
	}

	dashboardName := naming.StageResourceName(normalizedApp, stage, "dashboard")

	fmt.Println("CloudWatch Dashboard URL:")
	fmt.Printf("https://console.aws.amazon.com/cloudwatch/home?region=%s#dashboards:name=%s\n", region, dashboardName)
	return nil
}
