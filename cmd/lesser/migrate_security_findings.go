package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const securityFindingsMigrationName = "security-findings"

type securityFindingsMigrationClient interface {
	userKeyMigrationClient
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

type securityFindingsMigrationOptions struct {
	App        string
	BaseDomain string
	Stage      string
	AWSProfile string
	TableName  string
	Limit      int
	Apply      bool
	DryRunFlag bool
	AllowLive  bool
	Only       string
}

type securityFindingsMigrationContext struct {
	AWSConfig       aws.Config
	Client          securityFindingsMigrationClient
	TableName       string
	ResolvedProfile string
	AccountID       string
	Options         securityFindingsMigrationOptions
}

type securityFindingsMigrationSummary struct {
	OperationSummaries []securityFindingsOperationSummary
}

type securityFindingsOperationSummary struct {
	Name          string
	Scanned       int
	Candidates    int
	PlannedWrites int
	AppliedWrites int
	Skipped       int
	Samples       []string
}

type securityFindingsMigrationOperation struct {
	Name    string
	Execute func(context.Context, securityFindingsMigrationClient, string, bool, int) (securityFindingsOperationSummary, error)
}

var newSecurityFindingsMigrationClientFn = func(cfg aws.Config) securityFindingsMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

var runMigrateSecurityFindingsFn = runMigrateSecurityFindings

var securityFindingsMigrationOperations = []securityFindingsMigrationOperation{
	{Name: "numeric-ids", Execute: executeSecurityFindingsNumericIDBackfill},
	{Name: "hashtag-indexes", Execute: executeSecurityFindingsHashtagIndexCleanup},
}

func runMigrate(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("usage: lesser migrate security-findings [flags]")
	}

	switch argv[0] {
	case securityFindingsMigrationName:
		return runMigrateSecurityFindingsFn(argv[1:])
	case helpFlagShort, helpFlagLong, helpCommand:
		fmt.Fprintln(os.Stderr, "Usage: lesser migrate security-findings [--app <slug>] [--base-domain <domain>] [--stage dev|staging|live] [--aws-profile <profile>] [--table <name>] [--limit <n>] [--operation all|numeric-ids|hashtag-indexes|cms-publication-members] [--apply]")
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q", argv[0])
	}
}

func runMigrateSecurityFindings(argv []string) error {
	options, err := parseSecurityFindingsMigrationOptions(argv)
	if err != nil {
		return err
	}

	ctx := context.Background()
	migrationCtx, err := resolveSecurityFindingsMigrationContext(ctx, options)
	if err != nil {
		return err
	}

	summary, err := executeSecurityFindingsMigration(ctx, migrationCtx)
	if err != nil {
		return err
	}

	printSecurityFindingsMigrationSummary(migrationCtx, summary)
	return nil
}

func parseSecurityFindingsMigrationOptions(argv []string) (securityFindingsMigrationOptions, error) {
	fs := flag.NewFlagSet("lesser migrate security-findings", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	options := securityFindingsMigrationOptions{Stage: valueDev, Only: "all"}
	fs.StringVar(&options.App, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&options.BaseDomain, "base-domain", os.Getenv("LESSER_BASE_DOMAIN"), "base domain for operator confirmation output")
	fs.StringVar(&options.Stage, "stage", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&options.Stage, "env", valueDev, "deployment stage alias (dev|staging|live)")
	fs.StringVar(&options.AWSProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.StringVar(&options.TableName, "table", "", "explicit DynamoDB table name override")
	fs.IntVar(&options.Limit, "limit", 0, "maximum candidates per operation to process (0 = all)")
	fs.StringVar(&options.Only, "operation", "all", "operation to run: all|numeric-ids|hashtag-indexes|cms-publication-members")
	fs.BoolVar(&options.Apply, "apply", false, "apply planned security-finding data repairs")
	fs.BoolVar(&options.DryRunFlag, "dry-run", false, "force dry-run mode; this is the default when --apply is omitted")
	fs.BoolVar(&options.AllowLive, "allow-live", false, "permit --apply against live stage after explicit operator authorization")

	if err := fs.Parse(argv); err != nil {
		return securityFindingsMigrationOptions{}, err
	}
	if fs.NArg() > 0 {
		return securityFindingsMigrationOptions{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if options.Apply && options.DryRunFlag {
		return securityFindingsMigrationOptions{}, fmt.Errorf("--apply and --dry-run are mutually exclusive")
	}
	options.Stage = strings.TrimSpace(options.Stage)
	if options.Stage == "" {
		options.Stage = valueDev
	}
	if options.Apply && strings.EqualFold(options.Stage, "live") && !options.AllowLive {
		return securityFindingsMigrationOptions{}, fmt.Errorf("refusing to apply security findings migration to live without --allow-live")
	}
	options.Only = strings.ToLower(strings.TrimSpace(options.Only))
	if options.Only == "" {
		options.Only = "all"
	}
	if !validSecurityFindingsOperationName(options.Only) {
		return securityFindingsMigrationOptions{}, fmt.Errorf("unknown security findings migration operation %q", options.Only)
	}

	return options, nil
}

func validSecurityFindingsOperationName(name string) bool {
	switch name {
	case "all", "numeric-ids", "hashtag-indexes", "cms-publication-members":
		return true
	default:
		return false
	}
}

func resolveSecurityFindingsMigrationContext(ctx context.Context, options securityFindingsMigrationOptions) (securityFindingsMigrationContext, error) {
	resolvedTableName, err := resolveUserKeyMigrationTableName(options.App, options.Stage, options.TableName)
	if err != nil {
		return securityFindingsMigrationContext{}, err
	}

	awsCfg, resolvedProfile, err := loadAWSConfigForCLIFn(ctx, options.AWSProfile)
	if err != nil {
		return securityFindingsMigrationContext{}, err
	}

	accountID, err := resolveAWSAccountIDFn(ctx, awsCfg)
	if err != nil {
		return securityFindingsMigrationContext{}, err
	}

	return securityFindingsMigrationContext{
		AWSConfig:       awsCfg,
		Client:          newSecurityFindingsMigrationClientFn(awsCfg),
		TableName:       resolvedTableName,
		ResolvedProfile: resolvedProfile,
		AccountID:       accountID,
		Options:         options,
	}, nil
}

func executeSecurityFindingsMigration(ctx context.Context, migrationCtx securityFindingsMigrationContext) (securityFindingsMigrationSummary, error) {
	if migrationCtx.Client == nil {
		return securityFindingsMigrationSummary{}, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(migrationCtx.TableName) == "" {
		return securityFindingsMigrationSummary{}, fmt.Errorf("table name is required")
	}

	operations := selectedSecurityFindingsMigrationOperations(migrationCtx.Options.Only)
	summary := securityFindingsMigrationSummary{OperationSummaries: make([]securityFindingsOperationSummary, 0, len(operations))}
	for _, operation := range operations {
		operationSummary, err := operation.Execute(ctx, migrationCtx.Client, migrationCtx.TableName, migrationCtx.Options.Apply, migrationCtx.Options.Limit)
		if err != nil {
			return summary, fmt.Errorf("%s: %w", operation.Name, err)
		}
		if operationSummary.Name == "" {
			operationSummary.Name = operation.Name
		}
		summary.OperationSummaries = append(summary.OperationSummaries, operationSummary)
	}
	return summary, nil
}

func selectedSecurityFindingsMigrationOperations(only string) []securityFindingsMigrationOperation {
	if only == "all" {
		return append([]securityFindingsMigrationOperation(nil), securityFindingsMigrationOperations...)
	}
	selected := make([]securityFindingsMigrationOperation, 0, 1)
	for _, operation := range securityFindingsMigrationOperations {
		if operation.Name == only {
			selected = append(selected, operation)
		}
	}
	return selected
}

func printSecurityFindingsMigrationSummary(migrationCtx securityFindingsMigrationContext, summary securityFindingsMigrationSummary) {
	mode := selectedMigrationMode(migrationCtx.Options.Apply)
	fmt.Printf("migrate security-findings %s complete\n", mode)
	fmt.Printf("app: %s\n", printableMigrationValue(migrationCtx.Options.App, "lesser"))
	fmt.Printf("base_domain: %s\n", printableMigrationValue(migrationCtx.Options.BaseDomain, "(not provided)"))
	fmt.Printf("stage: %s\n", printableMigrationValue(migrationCtx.Options.Stage, valueDev))
	fmt.Printf("table: %s\n", migrationCtx.TableName)
	if migrationCtx.ResolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", migrationCtx.ResolvedProfile)
	}
	fmt.Printf("aws_account_id: %s\n", printableMigrationValue(migrationCtx.AccountID, "(unknown)"))
	fmt.Printf("aws_region: %s\n", printableMigrationValue(migrationCtx.AWSConfig.Region, "(unknown)"))
	fmt.Printf("operation_filter: %s\n", migrationCtx.Options.Only)

	totals := totalSecurityFindingsOperationSummary(summary.OperationSummaries)
	fmt.Printf("operations: %d\n", len(summary.OperationSummaries))
	fmt.Printf("scanned_items: %d\n", totals.Scanned)
	fmt.Printf("candidates: %d\n", totals.Candidates)
	fmt.Printf("planned_writes: %d\n", totals.PlannedWrites)
	fmt.Printf("applied_writes: %d\n", totals.AppliedWrites)
	fmt.Printf("skipped: %d\n", totals.Skipped)

	for _, operationSummary := range sortedSecurityFindingsOperationSummaries(summary.OperationSummaries) {
		fmt.Printf("operation.%s.scanned: %d\n", operationSummary.Name, operationSummary.Scanned)
		fmt.Printf("operation.%s.candidates: %d\n", operationSummary.Name, operationSummary.Candidates)
		fmt.Printf("operation.%s.planned_writes: %d\n", operationSummary.Name, operationSummary.PlannedWrites)
		fmt.Printf("operation.%s.applied_writes: %d\n", operationSummary.Name, operationSummary.AppliedWrites)
		fmt.Printf("operation.%s.skipped: %d\n", operationSummary.Name, operationSummary.Skipped)
		printMigrationSamples("operation."+operationSummary.Name+".samples", operationSummary.Samples)
	}

	if !migrationCtx.Options.Apply {
		fmt.Println("no writes performed; re-run with --apply only after reviewing this dry-run output for the intended dev instance")
	}
}

func printableMigrationValue(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func totalSecurityFindingsOperationSummary(summaries []securityFindingsOperationSummary) securityFindingsOperationSummary {
	total := securityFindingsOperationSummary{Name: "total"}
	for _, summary := range summaries {
		total.Scanned += summary.Scanned
		total.Candidates += summary.Candidates
		total.PlannedWrites += summary.PlannedWrites
		total.AppliedWrites += summary.AppliedWrites
		total.Skipped += summary.Skipped
	}
	return total
}

func sortedSecurityFindingsOperationSummaries(summaries []securityFindingsOperationSummary) []securityFindingsOperationSummary {
	sorted := append([]securityFindingsOperationSummary(nil), summaries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}
