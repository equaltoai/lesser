package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestExecuteSecurityFindingsNumericIDBackfill_DryRunReportsMissingOnly(t *testing.T) {
	mappedID := common.GenerateNumericID("mapped")
	missingID := common.GenerateNumericID("missing")
	conflictID := common.GenerateNumericID("conflict")
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{
				{Items: []map[string]types.AttributeValue{
					legacyActorMigrationItem("mapped", mappedID, "mapped"),
					legacyActorMigrationItem("missing", missingID, "missing"),
					legacyActorMigrationItem("conflict", conflictID, "conflict"),
				}},
				{Items: []map[string]types.AttributeValue{
					legacyNumericMappingItem(mappedID, "mapped", "https://example.com/users/mapped"),
					legacyNumericMappingItem(conflictID, "other", "https://example.com/users/other"),
				}},
			},
		},
	}

	summary, err := executeSecurityFindingsNumericIDBackfill(context.Background(), client, "simulacrum-dev-main-table", false, 0)
	require.NoError(t, err)
	require.Equal(t, "numeric-ids", summary.Name)
	require.Equal(t, 3, summary.Scanned)
	require.Equal(t, 1, summary.Candidates)
	require.Equal(t, 1, summary.PlannedWrites)
	require.Equal(t, 0, summary.AppliedWrites)
	require.Equal(t, 1, summary.Skipped)
	require.Empty(t, client.putInputs)
	require.Contains(t, summary.Samples[0], "NUMERIC_ID#"+missingID)
	require.Contains(t, summary.Samples[1], "conflicts with existing username")
}

func TestExecuteSecurityFindingsNumericIDBackfill_ApplyCreatesAbsentMappingConditionally(t *testing.T) {
	numericID := common.GenerateNumericID("agent-0")
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{
				{Items: []map[string]types.AttributeValue{
					legacyActorMigrationItem("agent-0", numericID, "agent-0"),
				}},
				{Items: nil},
			},
		},
	}

	summary, err := executeSecurityFindingsNumericIDBackfill(context.Background(), client, "theory-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Candidates)
	require.Equal(t, 1, summary.PlannedWrites)
	require.Equal(t, 1, summary.AppliedWrites)
	require.Len(t, client.putInputs, 1)
	put := client.putInputs[0]
	require.Equal(t, "attribute_not_exists(PK)", *put.ConditionExpression)
	require.Equal(t, "NUMERIC_ID#"+numericID, strAttr(t, put.Item["PK"]))
	require.Equal(t, numericMetadataSortKey, strAttr(t, put.Item["SK"]))
	require.Equal(t, "agent-0", strAttr(t, put.Item["username"]))
	require.Equal(t, numericMappingTypeName, strAttr(t, put.Item["type"]))
}

func TestExecuteSecurityFindingsNumericIDBackfill_RespectsCandidateLimit(t *testing.T) {
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{
				{Items: []map[string]types.AttributeValue{
					legacyActorMigrationItem("one", common.GenerateNumericID("one"), "one"),
					legacyActorMigrationItem("two", common.GenerateNumericID("two"), "two"),
				}},
				{Items: nil},
			},
		},
	}

	summary, err := executeSecurityFindingsNumericIDBackfill(context.Background(), client, "theory-dev-main-table", true, 1)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Candidates)
	require.Equal(t, 1, summary.AppliedWrites)
	require.Len(t, client.putInputs, 1)
}
