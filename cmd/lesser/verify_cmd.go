package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runVerify(argv []string) error {
	if len(argv) == 0 {
		return runVerifyAll(nil)
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case valueAll:
		return runVerifyAll(argv[1:])
	case "docs":
		return runVerifyDocs(argv[1:])
	case "ai-training":
		return runVerifyAITraining(argv[1:])
	case valueSchema:
		return runVerifySchema(argv[1:])
	case "graphql-coverage":
		return runVerifyGraphQLCoverage(argv[1:])
	case "openapi":
		return runVerifyOpenAPI(argv[1:])
	case "inventory":
		return runVerifyInventory(argv[1:])
	case "lambda-set":
		return runVerifyLambdaSet(argv[1:])
	case "unit":
		return runVerifyUnit(argv[1:])
	case "smoke":
		return runVerifySmoke(argv[1:])
	case "cdk":
		return runVerifyCDK(argv[1:])
	default:
		if argv[0] != "" && argv[0][0] == '-' {
			return runVerifyAll(argv)
		}
		return fmt.Errorf("unknown verify command %q", argv[0])
	}
}

type verifyAllArgs struct {
	RunSmoke bool
	RunCDK   bool

	SmokeBaseURL        string
	SmokeToken          string
	SmokeUsername       string
	SmokeObjectID       string
	SmokeAcceptHeader   string
	SmokeTimeoutSeconds int
	SmokeInsecure       bool

	CDKAWSProfile string
	CDKRegion     string
}

func runVerifyAll(argv []string) error {
	fs := flag.NewFlagSet("lesser verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args verifyAllArgs
	fs.BoolVar(&args.RunSmoke, "smoke", false, "also run smoke scripts (core + federation)")
	fs.BoolVar(&args.RunCDK, "cdk", false, "also run a CDK synth check")
	fs.StringVar(&args.CDKAWSProfile, "cdk-aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile for CDK synth (env: AWS_PROFILE)")
	fs.StringVar(&args.CDKRegion, "cdk-region", envOrDefault("AWS_REGION", "us-east-1"), "AWS region for CDK synth (env: AWS_REGION)")

	fs.StringVar(&args.SmokeBaseURL, "smoke-base-url", os.Getenv("SMOKE_BASE_URL"), "smoke base url (env: SMOKE_BASE_URL)")
	fs.StringVar(&args.SmokeToken, "smoke-token", os.Getenv("SMOKE_TOKEN"), "smoke auth token (env: SMOKE_TOKEN)")
	fs.StringVar(&args.SmokeUsername, "smoke-username", os.Getenv("SMOKE_USERNAME"), "smoke federation username (env: SMOKE_USERNAME)")
	fs.StringVar(&args.SmokeObjectID, "smoke-object-id", os.Getenv("SMOKE_OBJECT_ID"), "smoke federation object id (env: SMOKE_OBJECT_ID)")
	fs.StringVar(&args.SmokeAcceptHeader, "smoke-accept-header", envOrDefault("SMOKE_ACCEPT_HEADER", "application/activity+json"), "smoke federation accept header (env: SMOKE_ACCEPT_HEADER)")
	fs.IntVar(&args.SmokeTimeoutSeconds, "smoke-timeout-seconds", 15, "smoke timeout seconds (env: SMOKE_TIMEOUT_SECONDS)")
	fs.BoolVar(&args.SmokeInsecure, "smoke-insecure", envOrDefault("SMOKE_INSECURE", "0") == "1", "allow insecure TLS for smoke")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if err := runVerifyLambdaSet(nil); err != nil {
		return err
	}
	if err := runVerifyInventory(nil); err != nil {
		return err
	}
	if err := runVerifyDocs(nil); err != nil {
		return err
	}
	if err := runVerifyAITraining(nil); err != nil {
		return err
	}
	if err := runVerifySchema(nil); err != nil {
		return err
	}
	if err := runVerifyGraphQLCoverage([]string{"--strict"}); err != nil {
		return err
	}
	if err := runVerifyOpenAPI(nil); err != nil {
		return err
	}
	if err := runVerifyUnit(nil); err != nil {
		return err
	}

	if args.RunSmoke {
		if err := runSmokeCore(smokeArgs{
			BaseURL:        args.SmokeBaseURL,
			Token:          args.SmokeToken,
			TimeoutSeconds: args.SmokeTimeoutSeconds,
			Insecure:       args.SmokeInsecure,
		}); err != nil {
			return err
		}
		if err := runSmokeFederation(smokeArgs{
			BaseURL:        args.SmokeBaseURL,
			Username:       args.SmokeUsername,
			ObjectID:       args.SmokeObjectID,
			AcceptHeader:   args.SmokeAcceptHeader,
			TimeoutSeconds: args.SmokeTimeoutSeconds,
			Insecure:       args.SmokeInsecure,
		}); err != nil {
			return err
		}
	}

	if args.RunCDK {
		if err := runCDKSynth(args.CDKAWSProfile, args.CDKRegion); err != nil {
			return err
		}
	}

	fmt.Println("✓ verify complete (lambda set, inventory, docs, ai-training docs, graphql schema, graphql coverage, openapi, unit tests)")
	return nil
}

func runVerifyDocs(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	return runCommand(context.Background(), "bash", []string{"scripts/verify_docs.sh"}, execOptions{
		Dir: repoRoot,
	})
}

func runVerifyAITraining(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	return runCommand(context.Background(), "bash", []string{"scripts/verify_ai_training.sh"}, execOptions{
		Dir: repoRoot,
	})
}

func runVerifySchema(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	return runCommand(context.Background(), "bash", []string{"scripts/verify_schema.sh"}, execOptions{
		Dir: repoRoot,
	})
}

func runVerifyGraphQLCoverage(argv []string) error {
	fs := flag.NewFlagSet("lesser verify graphql-coverage", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var strict bool
	fs.BoolVar(&strict, "strict", false, "fail if any graphql_required route remains missing")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := ensureToolAvailable("go"); err != nil {
		return err
	}
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	args := []string{"run", "./tools/graphql_coverage", "--check"}
	if strict {
		args = append(args, "--strict")
	}

	return runCommand(context.Background(), "go", args, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runVerifyOpenAPI(argv []string) error {
	fs := flag.NewFlagSet("lesser verify openapi", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var strict bool
	fs.BoolVar(&strict, "strict", false, "strict verification (no placeholders)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := ensureToolAvailable("go"); err != nil {
		return err
	}
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}

	args := []string{"run", "./tools/openapi", "--check"}
	if strict {
		args = append(args, "--strict")
	}
	return runCommand(context.Background(), "go", args, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runVerifyInventory(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}
	return runCommand(context.Background(), "bash", []string{"scripts/verify_inventory.sh"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GOCACHE": goCache,
		},
	})
}

func runVerifyLambdaSet(_ []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	return runCommand(context.Background(), "bash", []string{"scripts/verify_lambda_set.sh"}, execOptions{
		Dir: repoRoot,
	})
}

func runVerifyUnit(_ []string) error {
	return runGoTests(testArgs{Environment: "test", Stage: "test"}, []string{"test", "-short", "-v", "./..."}, nil)
}

func runVerifySmoke(argv []string) error {
	fs := flag.NewFlagSet("lesser verify smoke", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var baseURL string
	var token string
	var username string
	var objectID string
	var acceptHeader string
	var timeoutSeconds int
	var insecure bool

	fs.StringVar(&baseURL, "base-url", os.Getenv("SMOKE_BASE_URL"), "base url (env: SMOKE_BASE_URL)")
	fs.StringVar(&token, "token", os.Getenv("SMOKE_TOKEN"), "auth token (env: SMOKE_TOKEN)")
	fs.StringVar(&username, "username", os.Getenv("SMOKE_USERNAME"), "federation username (env: SMOKE_USERNAME)")
	fs.StringVar(&objectID, "object-id", os.Getenv("SMOKE_OBJECT_ID"), "federation object id (env: SMOKE_OBJECT_ID)")
	fs.StringVar(&acceptHeader, "accept-header", envOrDefault("SMOKE_ACCEPT_HEADER", "application/activity+json"), "federation accept header")
	fs.IntVar(&timeoutSeconds, "timeout-seconds", 15, "timeout seconds")
	fs.BoolVar(&insecure, "insecure", envOrDefault("SMOKE_INSECURE", "0") == "1", "allow insecure TLS")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if err := runSmokeCore(smokeArgs{
		BaseURL:        baseURL,
		Token:          token,
		TimeoutSeconds: timeoutSeconds,
		Insecure:       insecure,
	}); err != nil {
		return err
	}

	return runSmokeFederation(smokeArgs{
		BaseURL:        baseURL,
		Username:       username,
		ObjectID:       objectID,
		AcceptHeader:   acceptHeader,
		TimeoutSeconds: timeoutSeconds,
		Insecure:       insecure,
	})
}

func runVerifyCDK(argv []string) error {
	fs := flag.NewFlagSet("lesser verify cdk", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var awsProfile string
	var region string
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile (env: AWS_PROFILE)")
	fs.StringVar(&region, "region", envOrDefault("AWS_REGION", "us-east-1"), "AWS region (env: AWS_REGION)")

	if err := fs.Parse(argv); err != nil {
		return err
	}
	return runCDKSynth(awsProfile, region)
}

func runCDKSynth(awsProfile string, region string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := ensureToolAvailable("cdk"); err != nil {
		return err
	}
	if strings.TrimSpace(awsProfile) == "" {
		return fmt.Errorf("--aws-profile is required (or set AWS_PROFILE)")
	}

	goCache, err := ensureGoCacheDir(repoRoot)
	if err != nil {
		return err
	}
	xdgCache, err := ensureXDGCacheDir(repoRoot)
	if err != nil {
		return err
	}

	return runCommand(context.Background(), "cdk", []string{"synth", "--context", "stage=shared"}, execOptions{
		Dir: filepath.Join(repoRoot, "infra", "cdk"),
		Env: map[string]string{
			"AWS_PROFILE":        awsProfile,
			"AWS_REGION":         region,
			"AWS_DEFAULT_REGION": region,
			"GOCACHE":            goCache,
			"XDG_CACHE_HOME":     xdgCache,
		},
	})
}
