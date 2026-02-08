package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var captureCommandOutputFn = captureCommandOutput

func runDev(argv []string) error {
	if len(argv) == 0 {
		return runDevServer(nil)
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "init":
		return runDevInit(argv[1:])
	case "dynamodb":
		return runDevDynamoDB(argv[1:])
	case "seed-and-validate":
		return runDevSeedAndValidate(argv[1:])
	default:
		if argv[0] != "" && argv[0][0] == '-' {
			return runDevServer(argv)
		}
		return fmt.Errorf("unknown dev command %q", argv[0])
	}
}

func runDevInit(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}

	envPath := filepath.Join(repoRoot, ".env")
	if _, err := os.Stat(envPath); err == nil {
		fmt.Println(".env file already exists")
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat .env: %w", err)
	}

	secret, err := randomBase64(32)
	if err != nil {
		return err
	}

	content := strings.Join([]string{
		"DOMAIN=localhost",
		`INSTANCE_NAME="Lesser Dev"`,
		"AWS_REGION=us-east-1",
		"DYNAMO_TABLE_NAME=lesser-dev",
		"S3_BUCKET_NAME=lesser-dev-media",
		"JWT_SECRET=" + secret,
		"",
	}, "\n")

	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil { //nolint:gosec // file path is derived from repo root
		return fmt.Errorf("write .env: %w", err)
	}
	fmt.Println("✓ Created .env file with default values")
	return nil
}

func runDevServer(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}

	envPath := filepath.Join(repoRoot, ".env")
	if _, err := os.Stat(envPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .env file found; run `lesser dev init` first")
		}
		return fmt.Errorf("stat .env: %w", err)
	}

	if err := ensureToolAvailableFn("go"); err != nil {
		return err
	}
	if err := ensureToolAvailableFn("bash"); err != nil {
		return err
	}

	fmt.Println("Starting local development server...")
	return runCommandFn(context.Background(), "bash", []string{"-lc", "set -a; source ./.env; set +a; go run ./cmd/api"}, execOptions{
		Dir: repoRoot,
	})
}

func runDevDynamoDB(_ []string) error {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}
	_ = repoRoot

	if err := ensureToolAvailableFn("docker"); err != nil {
		return err
	}

	fmt.Println("Starting local DynamoDB (docker)...")
	return runCommandFn(context.Background(), "docker", []string{"run", "-p", "8000:8000", "amazon/dynamodb-local"}, execOptions{})
}

type seedValidateArgs struct {
	AWSProfile       string
	DynamoDBTable    string
	BaseURL          string
	GraphQLEndpoint  string
	GraphQLStage     string
	GraphQLDomain    string
	GraphQLTestDelay string
}

func runDevSeedAndValidate(argv []string) error {
	fs := flag.NewFlagSet("lesser dev seed-and-validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args seedValidateArgs
	fs.StringVar(&args.AWSProfile, "aws-profile", envOrDefault("AWS_PROFILE", "Lesser"), "AWS profile for AWS CLI calls")
	fs.StringVar(&args.DynamoDBTable, "dynamodb-table", envOrDefault("DYNAMODB_TABLE", "lesser-development"), "DynamoDB table name to clear (env: DYNAMODB_TABLE)")
	fs.StringVar(&args.BaseURL, "base-url", envOrDefault("LESSER_BASE_URL", "https://dev.lesser.host"), "base URL to seed/validate (env: LESSER_BASE_URL)")
	fs.StringVar(&args.GraphQLEndpoint, "graphql-endpoint", os.Getenv("LESSER_GRAPHQL_ENDPOINT"), "GraphQL endpoint (defaults to <base-url>/api/graphql)")
	fs.StringVar(&args.GraphQLStage, "graphql-stage", envOrDefault("GRAPHQL_STAGE", valueDev), "GraphQL stage for validation scripts (env: GRAPHQL_STAGE)")
	fs.StringVar(&args.GraphQLDomain, "graphql-domain", envOrDefault("GRAPHQL_DOMAIN", "lesser.host"), "GraphQL domain for validation scripts (env: GRAPHQL_DOMAIN)")
	fs.StringVar(&args.GraphQLTestDelay, "graphql-test-delay", envOrDefault("GRAPHQL_TEST_DELAY", "0.5"), "GraphQL test delay (env: GRAPHQL_TEST_DELAY)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if strings.TrimSpace(args.GraphQLEndpoint) == "" {
		args.GraphQLEndpoint = strings.TrimRight(args.BaseURL, "/") + "/api/graphql"
	}

	repoRoot, err := findRepoRootFn()
	if err != nil {
		return err
	}

	if err := ensureToolAvailableFn("python3"); err != nil {
		return err
	}
	if err := ensureToolAvailableFn("aws"); err != nil {
		return err
	}
	if err := ensureToolAvailableFn("bash"); err != nil {
		return err
	}

	fmt.Println("=== Step 1: Clearing existing data ===")
	if err := runCommandFn(context.Background(), "python3", []string{"scripts/clear_all_data.py"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"AWS_PROFILE":    args.AWSProfile,
			"DYNAMODB_TABLE": args.DynamoDBTable,
		},
	}); err != nil {
		return err
	}

	fmt.Println("\n=== Step 2: Seeding fresh data ===")
	if err := runCommandFn(context.Background(), "python3", []string{"scripts/seed_runner/main.py"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"LESSER_BASE_URL":         args.BaseURL,
			"LESSER_GRAPHQL_ENDPOINT": args.GraphQLEndpoint,
		},
	}); err != nil {
		return err
	}

	token, err := captureCommandOutputFn(context.Background(), repoRoot, map[string]string{
		"LESSER_BASE_URL": args.BaseURL,
	}, "python3", "scripts/seed_runner/main.py", "get_token")
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("seed runner returned empty token")
	}

	fmt.Println("\n=== Step 3: Running GraphQL validation tests ===")
	if err := runCommandFn(context.Background(), "python3", []string{"tests/system/test_graphql.py"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GRAPHQL_STAGE":    args.GraphQLStage,
			"GRAPHQL_DOMAIN":   args.GraphQLDomain,
			"GRAPHQL_ENDPOINT": args.GraphQLEndpoint,
			"GRAPHQL_TOKEN":    token,
		},
	}); err != nil {
		return err
	}

	fmt.Println("\n=== Step 4: Running GraphQL read validation tests ===")
	if err := runCommandFn(context.Background(), "python3", []string{"tests/system/test_graphql_reads.py"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GRAPHQL_STAGE":    args.GraphQLStage,
			"GRAPHQL_DOMAIN":   args.GraphQLDomain,
			"GRAPHQL_ENDPOINT": args.GraphQLEndpoint,
			"GRAPHQL_TOKEN":    token,
		},
	}); err != nil {
		return err
	}

	fmt.Println("\n=== Step 5: Running comprehensive GraphQL validation ===")
	if err := runCommandFn(context.Background(), "bash", []string{"scripts/run_graphql_validation.sh"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GRAPHQL_STAGE":      args.GraphQLStage,
			"GRAPHQL_DOMAIN":     args.GraphQLDomain,
			"GRAPHQL_ENDPOINT":   args.GraphQLEndpoint,
			"ADMIN_TOKEN":        token,
			"GRAPHQL_TEST_DELAY": args.GraphQLTestDelay,
		},
	}); err != nil {
		return err
	}

	fmt.Println("\n=== Step 6: Running expanded GraphQL validation ===")
	if err := runCommandFn(context.Background(), "python3", []string{"scripts/validate_graphql_expanded.py"}, execOptions{
		Dir: repoRoot,
		Env: map[string]string{
			"GRAPHQL_STAGE":      args.GraphQLStage,
			"GRAPHQL_DOMAIN":     args.GraphQLDomain,
			"GRAPHQL_ENDPOINT":   args.GraphQLEndpoint,
			"ADMIN_TOKEN":        token,
			"GRAPHQL_TEST_DELAY": args.GraphQLTestDelay,
		},
	}); err != nil {
		return err
	}

	fmt.Println("\n✓ seed-and-validate complete")
	return nil
}

func randomBase64(size int) (string, error) {
	b := make([]byte, size)
	// Avoid crypto/rand.Read which can panic on failure in newer Go versions.
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func captureCommandOutput(ctx context.Context, dir string, env map[string]string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // tool invocation
	cmd.Dir = dir
	cmd.Env = mergeEnv(os.Environ(), env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
