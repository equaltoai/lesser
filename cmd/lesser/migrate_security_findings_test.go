package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/require"
)

type fakeSecurityFindingsMigrationClient struct {
	fakeUserKeyMigrationClient
	updateInputs []*dynamodb.UpdateItemInput
	updateErr    error
}

func (f *fakeSecurityFindingsMigrationClient) UpdateItem(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.updateInputs = append(f.updateInputs, input)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func TestRunMigrateSecurityFindings_PrintsDryRunContext(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousResolveAccountID := resolveAWSAccountIDFn
	previousClientFactory := newSecurityFindingsMigrationClientFn
	previousOperations := securityFindingsMigrationOperations
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		resolveAWSAccountIDFn = previousResolveAccountID
		newSecurityFindingsMigrationClientFn = previousClientFactory
		securityFindingsMigrationOperations = previousOperations
	})

	securityFindingsMigrationOperations = []securityFindingsMigrationOperation{
		{
			Name: "numeric-ids",
			Execute: func(context.Context, securityFindingsMigrationClient, string, bool, int) (securityFindingsOperationSummary, error) {
				return securityFindingsOperationSummary{Name: "numeric-ids", Scanned: 2, Candidates: 1, PlannedWrites: 1}, nil
			},
		},
	}
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		require.Equal(t, "Sim", awsProfile)
		return aws.Config{Region: "us-west-2"}, "Sim", nil
	}
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) {
		return "123456789012", nil
	}
	newSecurityFindingsMigrationClientFn = func(aws.Config) securityFindingsMigrationClient {
		return &fakeSecurityFindingsMigrationClient{}
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateSecurityFindings([]string{
			"--app", "simulacrum",
			"--base-domain", "sim.example",
			"--stage", "dev",
			"--aws-profile", "Sim",
			"--operation", "numeric-ids",
		}))
	})

	require.Contains(t, output, "migrate security-findings dry-run complete")
	require.Contains(t, output, "app: simulacrum")
	require.Contains(t, output, "base_domain: sim.example")
	require.Contains(t, output, "stage: dev")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "aws_account_id: 123456789012")
	require.Contains(t, output, "aws_region: us-west-2")
	require.Contains(t, output, "planned_writes: 1")
	require.Contains(t, output, "no writes performed")
}

func TestRunMigrateSecurityFindings_RejectsUnsafeFlags(t *testing.T) {
	_, err := parseSecurityFindingsMigrationOptions([]string{"--apply", "--dry-run"})
	require.EqualError(t, err, "--apply and --dry-run are mutually exclusive")

	_, err = parseSecurityFindingsMigrationOptions([]string{"--stage", "live", "--apply"})
	require.EqualError(t, err, "refusing to apply security findings migration to live without --allow-live")

	_, err = parseSecurityFindingsMigrationOptions([]string{"--operation", "unknown"})
	require.EqualError(t, err, "unknown security findings migration operation \"unknown\"")
}

func TestRunMigrate_DispatchesSecurityFindings(t *testing.T) {
	previousRunner := runMigrateSecurityFindingsFn
	t.Cleanup(func() { runMigrateSecurityFindingsFn = previousRunner })

	var seen []string
	runMigrateSecurityFindingsFn = func(argv []string) error {
		seen = append([]string(nil), argv...)
		return nil
	}

	require.NoError(t, runMigrate([]string{"security-findings", "--dry-run"}))
	require.Equal(t, []string{"--dry-run"}, seen)
}

func TestResolveSecurityFindingsMigrationContext_PropagatesAccountError(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousResolveAccountID := resolveAWSAccountIDFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		resolveAWSAccountIDFn = previousResolveAccountID
	})

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "Theory", nil
	}
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) {
		return "", errors.New("sts down")
	}

	_, err := resolveSecurityFindingsMigrationContext(context.Background(), securityFindingsMigrationOptions{App: "theory", Stage: valueDev})
	require.EqualError(t, err, "sts down")
}
