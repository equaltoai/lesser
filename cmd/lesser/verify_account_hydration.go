package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	lessertheorydb "github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"go.uber.org/zap"
)

const accountHydrationFixturePath = "testdata/account_hydration/live_agents.json"

type accountHydrationVerifier interface {
	GetUser(ctx context.Context, username string) (*storage.User, error)
	GetAccount(ctx context.Context, username string) (*storage.Account, error)
	GetAgentGovernanceState(ctx context.Context, username string) (*storage.AgentGovernanceState, error)
}

type accountHydrationVerificationSummary struct {
	Checked       int
	AgentRows     int
	TableName     string
	ResolvedStage string
	ResolvedApp   string
	ResolvedEnv   string
	ResolvedNames []string
}

var newAccountHydrationVerifierFn = func(db core.DB, tableName string, domain string) accountHydrationVerifier {
	return repositories.NewAccountRepository(db, tableName, domain, zap.NewNop())
}

func runVerifyAccountHydration(argv []string) error {
	fs := flag.NewFlagSet("lesser verify account-hydration", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var env string
	var awsProfile string
	var tableName string
	var baseDomain string
	var usernamesCSV string

	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (env: AWS_PROFILE)")
	fs.StringVar(&tableName, "table", "", "explicit DynamoDB table name override")
	fs.StringVar(&baseDomain, "base-domain", envOrDefault("LESSER_BASE_DOMAIN", ""), "base domain used to normalize local actor urls")
	fs.StringVar(&usernamesCSV, "usernames", "", "comma-separated usernames to verify (default: fixture usernames)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	usernames, err := resolveAccountHydrationUsernames(usernamesCSV)
	if err != nil {
		return err
	}

	ctx := context.Background()
	awsCfg, resolvedTableName, resolvedProfile, err := resolveCommonMigrationCLIOptions(ctx, commonMigrationCLIOptions{
		App:        app,
		Env:        env,
		AWSProfile: awsProfile,
		TableName:  tableName,
	})
	if err != nil {
		return err
	}

	db, err := tabletheoryNewFn(session.Config{
		Region:              awsCfg.Region,
		CredentialsProvider: awsCfg.Credentials,
	})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := lessertheorydb.RegisterDefaultTypeConverters(db); err != nil {
		return err
	}

	prevTableName := models.MainTableName
	models.MainTableName = resolvedTableName
	defer func() {
		models.MainTableName = prevTableName
	}()

	verifier := newAccountHydrationVerifierFn(db, resolvedTableName, resolveAccountHydrationDomain(env, baseDomain))
	summary, err := executeAccountHydrationVerification(ctx, verifier, usernames)
	if err != nil {
		return err
	}

	summary.TableName = resolvedTableName
	summary.ResolvedApp = strings.TrimSpace(app)
	if summary.ResolvedApp == "" {
		summary.ResolvedApp = naming.DefaultAppName
	}
	summary.ResolvedEnv = strings.TrimSpace(env)

	fmt.Println("verify account-hydration complete")
	fmt.Println("table:", summary.TableName)
	fmt.Println("env:", summary.ResolvedEnv)
	if resolvedProfile != "" {
		fmt.Println("aws_profile:", resolvedProfile)
	}
	if domain := resolveAccountHydrationDomain(env, baseDomain); domain != "" {
		fmt.Println("domain:", domain)
	}
	fmt.Println("checked:", summary.Checked)
	fmt.Println("agent_rows:", summary.AgentRows)
	fmt.Println("usernames:")
	for _, username := range summary.ResolvedNames {
		fmt.Printf("  %s\n", username)
	}

	return nil
}

func executeAccountHydrationVerification(
	ctx context.Context,
	verifier accountHydrationVerifier,
	usernames []string,
) (accountHydrationVerificationSummary, error) {
	summary := accountHydrationVerificationSummary{
		ResolvedNames: append([]string(nil), usernames...),
	}

	if verifier == nil {
		return summary, fmt.Errorf("account hydration verifier is required")
	}
	if len(usernames) == 0 {
		return summary, fmt.Errorf("at least one username is required")
	}

	for _, username := range usernames {
		user, err := verifier.GetUser(ctx, username)
		if err != nil {
			return summary, fmt.Errorf("get user %s: %w", username, err)
		}
		if user == nil {
			return summary, fmt.Errorf("get user %s: returned nil user", username)
		}

		account, err := verifier.GetAccount(ctx, username)
		if err != nil {
			return summary, fmt.Errorf("get account %s: %w", username, err)
		}
		if account == nil || account.User == nil {
			return summary, fmt.Errorf("get account %s: returned nil account user", username)
		}
		if !strings.EqualFold(strings.TrimSpace(account.User.Username), strings.TrimSpace(user.Username)) {
			return summary, fmt.Errorf("get account %s: user mismatch %q != %q", username, account.User.Username, user.Username)
		}
		if account.Actor == nil {
			return summary, fmt.Errorf("get account %s: missing actor", username)
		}

		summary.Checked++
		if user.IsAgent {
			governance, err := verifier.GetAgentGovernanceState(ctx, username)
			if err != nil {
				return summary, fmt.Errorf("get typed governance %s: %w", username, err)
			}
			if governance == nil {
				return summary, fmt.Errorf("get typed governance %s: returned nil state", username)
			}
			summary.AgentRows++
		}
	}

	return summary, nil
}

func resolveAccountHydrationUsernames(usernamesCSV string) ([]string, error) {
	if parsed := parseAccountHydrationUsernames(usernamesCSV); len(parsed) > 0 {
		return parsed, nil
	}

	repoRoot, err := findRepoRootFn()
	if err != nil {
		return nil, err
	}
	return loadAccountHydrationFixtureUsernames(filepath.Join(repoRoot, accountHydrationFixturePath))
}

func parseAccountHydrationUsernames(usernamesCSV string) []string {
	if strings.TrimSpace(usernamesCSV) == "" {
		return nil
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, part := range strings.Split(usernamesCSV, ",") {
		username := strings.TrimSpace(part)
		if username == "" {
			continue
		}
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}
		out = append(out, username)
	}
	return out
}

func loadAccountHydrationFixtureUsernames(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read account hydration fixture: %w", err)
	}

	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		return nil, fmt.Errorf("parse account hydration fixture: %w", err)
	}

	usernames := make([]string, 0, len(fixtures))
	for username := range fixtures {
		username = strings.TrimSpace(username)
		if username == "" {
			continue
		}
		usernames = append(usernames, username)
	}
	slices.Sort(usernames)
	if len(usernames) == 0 {
		return nil, fmt.Errorf("account hydration fixture %s contained no usernames", path)
	}
	return usernames, nil
}

func resolveAccountHydrationDomain(env string, baseDomain string) string {
	baseDomain = strings.TrimSpace(baseDomain)
	if baseDomain == "" {
		return ""
	}
	return naming.StageDomain(naming.StageForEnvironment(env), baseDomain)
}
