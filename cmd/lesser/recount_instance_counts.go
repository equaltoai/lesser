package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	lessertheorydb "github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"go.uber.org/zap"
)

// openRecountDBFn builds the tabletheory DB for the recount CLI.
var openRecountDBFn = func(awsCfg aws.Config) (core.DB, func() error, error) {
	db, err := tabletheoryNewFn(session.Config{
		Region:              awsCfg.Region,
		CredentialsProvider: awsCfg.Credentials,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := lessertheorydb.RegisterDefaultTypeConverters(db); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return db, db.Close, nil
}

type recountInstanceCountsSummary struct {
	Users               int64
	Domains             int64
	DomainCounters      int64
	StaleDomainCounters int64
}

// runRecountInstanceCounts is the offline drift remedy for the O(1) instance
// counters: it recomputes TOTAL_USERS and TOTAL_DOMAINS from bounded key-only
// reads and rewrites the counters. It never runs on a request path — it is a
// deliberately invoked maintenance command (`--apply` writes; the default is a
// dry-run report). See repositories.RecountInstanceCounts.
func runRecountInstanceCounts(argv []string) error {
	fs := flag.NewFlagSet("lesser recount-instance-counts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		app        string
		env        string
		awsProfile string
		tableName  string
		apply      bool
	)
	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.StringVar(&tableName, "table", "", "explicit DynamoDB table name override")
	fs.BoolVar(&apply, "apply", false, "rewrite the counters (default: dry-run report only)")

	if err := fs.Parse(argv); err != nil {
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

	prevTableName := models.MainTableName
	models.MainTableName = resolvedTableName
	defer func() {
		models.MainTableName = prevTableName
	}()

	db, closeDB, err := openRecountDBFn(awsCfg)
	if err != nil {
		return err
	}
	if closeDB != nil {
		defer func() { _ = closeDB() }()
	}

	result, err := repositories.RecountInstanceCounts(ctx, db, zap.NewNop(), apply)
	if err != nil {
		return err
	}

	summary := recountInstanceCountsSummary{
		Users:               result.Users,
		Domains:             result.Domains,
		DomainCounters:      result.DomainCounters,
		StaleDomainCounters: result.StaleDomainCounters,
	}
	printRecountInstanceCountsSummary(summary, resolvedTableName, resolvedProfile, apply)
	return nil
}

func printRecountInstanceCountsSummary(summary recountInstanceCountsSummary, tableName, profile string, apply bool) {
	mode := selectedMigrationMode(apply)
	fmt.Printf("recount-instance-counts %s complete\n", mode)
	fmt.Printf("  table:        %s\n", tableName)
	if profile != "" {
		fmt.Printf("  aws_profile:  %s\n", profile)
	}
	fmt.Printf("  total_users:    %d\n", summary.Users)
	fmt.Printf("  total_domains:  %d\n", summary.Domains)
	fmt.Printf("  domain counters upserted: %d\n", summary.DomainCounters)
	fmt.Printf("  stale domain counters removed: %d\n", summary.StaleDomainCounters)
	if !apply {
		fmt.Println("  dry-run: pass --apply to rewrite the counters")
	}
}
