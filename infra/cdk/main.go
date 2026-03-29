package main

import (
	"fmt"
	"os"
	"strings"

	"cdk/stacks"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	appName := getContextString(app, "app")
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	appName = normalizedApp

	stageFilter := strings.TrimSpace(strings.ToLower(getContextString(app, "stage")))
	if stageFilter == "all" {
		stageFilter = ""
	}
	if stageFilter != "" && stageFilter != "shared" && stageFilter != string(naming.StageDev) && stageFilter != string(naming.StageStaging) && stageFilter != string(naming.StageLive) {
		fmt.Printf("Error: invalid stage %q (expected dev, staging, live, shared)\n", stageFilter)
		os.Exit(1)
	}
	withStaging := getContextBool(app, "withStaging")

	baseDomain := normalizeDomain(getContextString(app, "baseDomain"))
	if baseDomain == "" && stageFilter != "shared" {
		baseDomain = normalizeDomain(getContextString(app, "rootDomain"))
	}
	if baseDomain == "" && stageFilter != "shared" {
		fmt.Println("Error: base domain is required (pass --context baseDomain=example.com)")
		os.Exit(1)
	}

	awsAccount := strings.TrimSpace(os.Getenv("CDK_DEFAULT_ACCOUNT"))
	awsRegion := strings.TrimSpace(os.Getenv("CDK_DEFAULT_REGION"))
	if awsRegion == "" {
		awsRegion = strings.TrimSpace(os.Getenv("AWS_REGION"))
	}
	if awsAccount == "" {
		fmt.Println("Error: CDK_DEFAULT_ACCOUNT is not set (run via the CDK CLI)")
		os.Exit(1)
	}
	if awsRegion == "" {
		fmt.Println("Error: CDK_DEFAULT_REGION is not set (run via the CDK CLI)")
		os.Exit(1)
	}

	env := &awscdk.Environment{
		Region: jsii.String(awsRegion),
	}
	env.Account = jsii.String(awsAccount)

	sharedStackName := naming.SharedStackName(appName)
	_ = stacks.NewSharedStack(app, sharedStackName, &stacks.SharedStackProps{
		StackProps: awscdk.StackProps{
			Env:         env,
			Description: jsii.String("Lesser shared resources - KMS keys, secrets, roles"),
		},
		AppName: appName,
	})

	if stageFilter == "shared" {
		app.Synth(nil)
		return
	}

	hostedZoneID := strings.TrimSpace(getContextString(app, "hostedZoneId"))

	// Stage stacks.
	// Always include dev + live; include staging if requested.
	stages := []string{"dev", "live"}
	if stageFilter == string(naming.StageStaging) {
		withStaging = true
	}
	if withStaging {
		stages = []string{"dev", "staging", "live"}
	}

	for _, stage := range stages {
		if stageFilter != "" && stageFilter != stage {
			continue
		}

		environment := canonicalEnvironment(stage)
		stageDomain := naming.StageDomain(naming.Stage(stage), baseDomain)
		if stageDomain == "" {
			fmt.Printf("Error: unable to derive stage domain for %s\n", stage)
			os.Exit(1)
		}

		config := map[string]interface{}{
			"environment": environment,
			"appName":     appName,
			"domain":      stageDomain,
		}
		// Optional per-deployment config (passed via CDK context).
		for _, key := range []string{
			"lambdaAssetRoot",
			"lesserHostUrl",
			"lesserHostInstanceKeyArn",
			"lesserHostAttestationsUrl",
			"bodyEnabled",
			"soulEnabled",
			"translationEnabled",
			"tipEnabled",
			"tipChainId",
			"tipContractAddress",
		} {
			if v := getContextString(app, key); v != "" {
				config[key] = v
			}
		}

		stackName := naming.StageStackName(appName, naming.Stage(stage))
		_ = stacks.NewLesserApiStack(app, stackName, &stacks.LesserApiStackProps{
			StackProps: awscdk.StackProps{
				Env:         env,
				Description: jsii.String(fmt.Sprintf("Lesser stage stack - %s", stage)),
			},
			Environment:      environment,
			Domain:           stageDomain,
			Config:           config,
			HostedZoneDomain: baseDomain,
			HostedZoneId:     hostedZoneID,
			CloudFrontDomain: "",
			AppName:          appName,
			AccountID:        awsAccount,
			Region:           awsRegion,
		})
	}

	app.Synth(nil)
}

func getContextString(app awscdk.App, key string) string {
	value := app.Node().TryGetContext(jsii.String(key))
	if value == nil {
		return ""
	}
	out := strings.TrimSpace(fmt.Sprintf("%v", value))
	if out == "" || strings.EqualFold(out, "<nil>") {
		return ""
	}
	return out
}

func getContextBool(app awscdk.App, key string) bool {
	value := app.Node().TryGetContext(jsii.String(key))
	switch v := value.(type) {
	case bool:
		return v
	case string:
		clean := strings.TrimSpace(strings.ToLower(v))
		return clean == "true" || clean == "1" || clean == "yes" || clean == "y"
	default:
		return false
	}
}

func normalizeDomain(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
}

func canonicalEnvironment(env string) string {
	clean := strings.ToLower(strings.TrimSpace(env))
	switch clean {
	case "", "<nil>":
		return ""
	case "dev", "development":
		return "development"
	case "test", "stage", "staging":
		return "staging"
	case "prod", "production", "live":
		return "production"
	default:
		return clean
	}
}
