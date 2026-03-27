package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type commonMigrationCLIOptions struct {
	App        string
	Env        string
	AWSProfile string
	TableName  string
	Limit      int
	Apply      bool
}

func parseCommonMigrationCLIOptions(
	argv []string,
	commandName string,
	limitUsage string,
	applyUsage string,
) (commonMigrationCLIOptions, error) {
	fs := flag.NewFlagSet("lesser "+commandName, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	options := commonMigrationCLIOptions{}
	fs.StringVar(&options.App, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&options.Env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&options.AWSProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.StringVar(&options.TableName, "table", "", "explicit DynamoDB table name override")
	fs.IntVar(&options.Limit, "limit", 0, limitUsage)
	fs.BoolVar(&options.Apply, "apply", false, applyUsage)

	if err := fs.Parse(argv); err != nil {
		return commonMigrationCLIOptions{}, err
	}

	return options, nil
}

func resolveCommonMigrationCLIOptions(
	ctx context.Context,
	options commonMigrationCLIOptions,
) (aws.Config, string, string, error) {
	resolvedTableName, err := resolveUserKeyMigrationTableName(options.App, options.Env, options.TableName)
	if err != nil {
		return aws.Config{}, "", "", err
	}

	awsCfg, resolvedProfile, err := loadAWSConfigForCLIFn(ctx, options.AWSProfile)
	if err != nil {
		return aws.Config{}, "", "", err
	}

	return awsCfg, resolvedTableName, resolvedProfile, nil
}

func runCommonMigrationCLI[T any](
	argv []string,
	commandName string,
	limitUsage string,
	applyUsage string,
	execute func(context.Context, aws.Config, string, bool, int) (T, error),
	printSummary func(T, string, string, bool),
) error {
	options, err := parseCommonMigrationCLIOptions(argv, commandName, limitUsage, applyUsage)
	if err != nil {
		return err
	}

	ctx := context.Background()
	awsCfg, resolvedTableName, resolvedProfile, err := resolveCommonMigrationCLIOptions(ctx, options)
	if err != nil {
		return err
	}

	summary, err := execute(ctx, awsCfg, resolvedTableName, options.Apply, options.Limit)
	if err != nil {
		return err
	}

	printSummary(summary, resolvedTableName, resolvedProfile, options.Apply)
	return nil
}

func selectedMigrationMode(apply bool) string {
	if apply {
		return migrationModeApply
	}
	return migrationModeDryRun
}

func printConversationMigrationSamples(samples []string) {
	printMigrationSamples("sample_conversation_ids", samples)
}

func printMigrationSamples(label string, samples []string) {
	if len(samples) == 0 {
		return
	}

	fmt.Printf("%s:\n", label)
	for _, sample := range samples {
		fmt.Printf("  %s\n", sample)
	}
}
