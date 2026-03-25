package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
)

func TestRunCommonMigrationCLI_UsesExecuteAndPrintSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
	})

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{Region: "us-east-1"}, "Sim", nil
	}

	var (
		executeCalled bool
		printed       bool
	)
	err := runCommonMigrationCLI(
		[]string{"--table", "custom-main-table", "--aws-profile", "Sim", "--limit", "7", "--apply"},
		"demo-migration",
		"limit usage",
		"apply usage",
		func(_ context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (int, error) {
			executeCalled = true
			require.Equal(t, "us-east-1", awsCfg.Region)
			require.Equal(t, "custom-main-table", tableName)
			require.True(t, apply)
			require.Equal(t, 7, limit)
			return 41, nil
		},
		func(summary int, tableName string, resolvedProfile string, apply bool) {
			printed = true
			require.Equal(t, 41, summary)
			require.Equal(t, "custom-main-table", tableName)
			require.Equal(t, "Sim", resolvedProfile)
			require.True(t, apply)
		},
	)
	require.NoError(t, err)
	require.Equal(t, "Sim", seenProfile)
	require.True(t, executeCalled)
	require.True(t, printed)
}

func TestRunCommonMigrationCLI_ReturnsExecuteError(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
	})

	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		require.Equal(t, "Sim", awsProfile)
		return aws.Config{}, "Sim", nil
	}

	printCalled := false
	err := runCommonMigrationCLI(
		[]string{"--table", "custom-main-table", "--aws-profile", "Sim"},
		"demo-migration",
		"limit usage",
		"apply usage",
		func(context.Context, aws.Config, string, bool, int) (int, error) {
			return 0, errors.New("boom")
		},
		func(int, string, string, bool) {
			printCalled = true
		},
	)
	require.EqualError(t, err, "boom")
	require.False(t, printCalled)
}

func TestParseCommonMigrationCLIOptions_UsesEnvironmentDefaults(t *testing.T) {
	t.Setenv("AWS_PROFILE", "FromEnv")

	options, err := parseCommonMigrationCLIOptions([]string{"--app", "simulacrum", "--limit", "3"}, "demo-migration", "limit usage", "apply usage")
	require.NoError(t, err)
	require.Equal(t, "simulacrum", options.App)
	require.Equal(t, valueDev, options.Env)
	require.Equal(t, "FromEnv", options.AWSProfile)
	require.Equal(t, 3, options.Limit)
	require.False(t, options.Apply)
}

func TestPrintConversationMigrationSamples_DeduplicatesOutputShapes(t *testing.T) {
	output := captureStdout(t, func() {
		printConversationMigrationSamples(nil)
		printConversationMigrationSamples([]string{"conv-1", "conv-2"})
	})

	require.NotContains(t, output, "sample_conversation_ids:\n\n")
	require.Contains(t, output, "sample_conversation_ids:")
	require.Contains(t, output, "  conv-1")
	require.Contains(t, output, "  conv-2")
}

func TestResolveCommonMigrationCLIOptions_ReturnsErrors(t *testing.T) {
	_, _, _, err := resolveCommonMigrationCLIOptions(context.Background(), commonMigrationCLIOptions{
		App: "lesser",
		Env: "qa",
	})
	require.Error(t, err)

	previousLoadAWSConfig := loadAWSConfigForCLIFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
	})

	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		require.Equal(t, "Sim", awsProfile)
		return aws.Config{}, "", errors.New("load aws")
	}

	_, _, _, err = resolveCommonMigrationCLIOptions(context.Background(), commonMigrationCLIOptions{
		App:        "simulacrum",
		Env:        valueDev,
		AWSProfile: "Sim",
	})
	require.EqualError(t, err, "load aws")
}
