package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestRunMigrateConversationMetadata_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newConversationMetadataMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newConversationMetadataMigrationClientFn = previousClientFactory
	})

	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)
	newestStatusTime := createdAt.Add(30 * time.Second)
	staleConversation := conversationMetadataItem(
		"conv-1",
		[]string{"arch", "scout"},
		createdAt,
		updatedAt,
		0,
		"status-1",
		time.Time{},
	)

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{staleConversation},
		}},
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataStatusItem("conv-1", "status-1", newestStatusTime),
			},
		}},
	}

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{}, "Sim", nil
	}
	newConversationMetadataMigrationClientFn = func(aws.Config) conversationMetadataMigrationClient {
		return client
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateConversationMetadata([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Equal(t, "Sim", seenProfile)
	require.Contains(t, output, "migrate-conversation-metadata dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "scanned_conversations: 1")
	require.Contains(t, output, "flagged_conversations: 1")
	require.Contains(t, output, "reparable_conversations: 1")
	require.Contains(t, output, "manual_review_conversations: 0")
	require.Contains(t, output, "repaired_conversations: 0")
	require.Contains(t, output, "sample_conversation_ids:")
	require.Contains(t, output, "  conv-1")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 1)
	require.Len(t, client.queryInputs, 1)
	require.Equal(t, "gsi3", aws.ToString(client.queryInputs[0].IndexName))
	require.Equal(
		t,
		conversationMetadataPartitionPrefix+"conv-1",
		strAttr(t, client.queryInputs[0].ExpressionAttributeValues[":conversation"]),
	)
}

func TestExecuteConversationMetadataMigration_ApplyRepairsStaleRows(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)
	firstStatusTime := createdAt.Add(30 * time.Second)
	secondStatusTime := createdAt.Add(90 * time.Second)

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataItem(
					"conv-1",
					[]string{"arch", "scout"},
					createdAt,
					updatedAt,
					0,
					"status-0",
					time.Time{},
				),
			},
		}},
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataStatusItem("conv-1", "status-1", firstStatusTime),
				conversationMetadataStatusItem("conv-1", "status-2", secondStatusTime),
			},
		}},
	}

	summary, err := executeConversationMetadataMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedConversations)
	require.Equal(t, 1, summary.FlaggedConversations)
	require.Equal(t, 1, summary.ReparableConversations)
	require.Equal(t, 0, summary.ManualReviewConversations)
	require.Equal(t, 1, summary.RepairedConversations)
	require.Equal(t, []string{"conv-1"}, summary.SampleConversationIDs)

	require.Len(t, client.putInputs, 1)
	repaired := client.putInputs[0].Item
	require.Equal(t, "status-2", strAttr(t, repaired[conversationLastStatusIDAttribute]))
	require.EqualValues(t, 2, numAttr(t, repaired[conversationTotalMessageCountAttribute]))
	require.Equal(t, secondStatusTime.UTC().Format(time.RFC3339Nano), strAttr(t, repaired[conversationLastMessageTimeAttribute]))
	repairedUpdatedAt, ok := attributeTime(repaired[conversationUpdatedAtAttribute])
	require.True(t, ok)
	require.False(t, repairedUpdatedAt.IsZero())
	require.True(t, repairedUpdatedAt.After(updatedAt) || repairedUpdatedAt.Equal(updatedAt))
}

func TestExecuteConversationMetadataMigration_ManualReviewAndLimit(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataItem(
					"conv-1",
					[]string{"arch", "scout"},
					createdAt,
					updatedAt,
					0,
					"status-1",
					time.Time{},
				),
				conversationMetadataItem(
					"conv-2",
					[]string{"arch", "medic"},
					createdAt,
					updatedAt,
					0,
					"status-2",
					time.Time{},
				),
			},
		}},
		queryOutputs: []*dynamodb.QueryOutput{{Items: nil}},
	}

	summary, err := executeConversationMetadataMigration(context.Background(), client, "simulacrum-dev-main-table", false, 1)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedConversations)
	require.Equal(t, 1, summary.FlaggedConversations)
	require.Equal(t, 0, summary.ReparableConversations)
	require.Equal(t, 1, summary.ManualReviewConversations)
	require.Equal(t, 0, summary.RepairedConversations)
	require.Equal(t, []string{"conv-1"}, summary.SampleConversationIDs)
	require.Len(t, client.queryInputs, 1)
	require.Empty(t, client.putInputs)
}

func TestLoadConversationStatusSummary_PaginatesAndSkipsInvalidRows(t *testing.T) {
	firstPublishedAt := time.Date(2026, 3, 25, 10, 40, 0, 0, time.UTC)
	secondPublishedAt := firstPublishedAt.Add(5 * time.Minute)

	client := &fakeUserKeyMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{
			{
				Items: []map[string]types.AttributeValue{
					{"publishedAt": sAttr(firstPublishedAt.Format(time.RFC3339Nano))},
					conversationMetadataStatusItem("conv-1", "status-1", firstPublishedAt),
				},
				LastEvaluatedKey: map[string]types.AttributeValue{"PK": sAttr("next")},
			},
			{
				Items: []map[string]types.AttributeValue{
					{
						conversationMessageStatusIDAttribute: sAttr("status-missing-time"),
					},
					conversationMetadataStatusItem("conv-1", "status-2", secondPublishedAt),
				},
			},
		},
	}

	summary, err := loadConversationStatusSummary(context.Background(), client, "simulacrum-dev-main-table", "conv-1")
	require.NoError(t, err)
	require.EqualValues(t, 2, summary.StatusCount)
	require.Equal(t, "status-2", summary.NewestStatusID)
	require.Equal(t, secondPublishedAt, summary.NewestTime)
	require.Len(t, client.queryInputs, 2)
	require.Equal(t, "next", strAttr(t, client.queryInputs[1].ExclusiveStartKey["PK"]))
}

func TestConversationMetadataRowIsStale_RequiresConversationMetadataAndStatus(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)
	lastMessageTime := createdAt.Add(time.Minute)

	require.False(t, conversationMetadataRowIsStale(map[string]types.AttributeValue{
		"PK": sAttr("USER#arch"),
		"SK": sAttr("METADATA"),
	}))
	require.False(t, conversationMetadataRowIsStale(conversationMetadataItem("conv-1", []string{"arch"}, createdAt, updatedAt, 0, "", time.Time{})))
	require.False(t, conversationMetadataRowIsStale(conversationMetadataItem("conv-1", []string{"arch"}, createdAt, updatedAt, 2, "status-1", lastMessageTime)))
	require.True(t, conversationMetadataRowIsStale(conversationMetadataItem("conv-1", []string{"arch"}, createdAt, updatedAt, 0, "status-1", lastMessageTime)))
}

func TestPrintConversationMetadataMigrationSummary_ApplyOmitsDryRunNote(t *testing.T) {
	output := captureStdout(t, func() {
		printConversationMetadataMigrationSummary(conversationMetadataMigrationSummary{
			ScannedConversations:      1,
			FlaggedConversations:      1,
			ReparableConversations:    1,
			ManualReviewConversations: 0,
			RepairedConversations:     1,
			SampleConversationIDs:     []string{"conv-1"},
		}, "custom-main-table", "", true)
	})

	require.Contains(t, output, "migrate-conversation-metadata apply complete")
	require.Contains(t, output, "table: custom-main-table")
	require.Contains(t, output, "sample_conversation_ids:")
	require.NotContains(t, output, "aws_profile:")
	require.NotContains(t, output, "no writes performed")
}

func TestExecuteConversationMetadataMigration_ValidatesInputsAndScanErrors(t *testing.T) {
	_, err := executeConversationMetadataMigration(context.Background(), nil, "simulacrum-dev-main-table", false, 0)
	require.EqualError(t, err, "migration client is required")

	_, err = executeConversationMetadataMigration(context.Background(), &fakeUserKeyMigrationClient{}, "   ", false, 0)
	require.EqualError(t, err, "table name is required")

	_, err = executeConversationMetadataMigration(context.Background(), &fakeUserKeyMigrationClient{
		scanErr: context.DeadlineExceeded,
	}, "simulacrum-dev-main-table", false, 0)
	require.ErrorContains(t, err, "scan conversation metadata rows")
}

func TestBuildConversationMetadataRepairCandidate_HandlesHealthyRowsAndMissingIDs(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)

	candidate, ok, err := buildConversationMetadataRepairCandidate(context.Background(), &fakeUserKeyMigrationClient{}, "simulacrum-dev-main-table", conversationMetadataItem(
		"conv-1",
		[]string{"arch"},
		createdAt,
		updatedAt,
		2,
		"status-1",
		createdAt.Add(time.Minute),
	))
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, conversationMetadataRepairCandidate{}, candidate)

	candidate, ok, err = buildConversationMetadataRepairCandidate(context.Background(), &fakeUserKeyMigrationClient{}, "simulacrum-dev-main-table", map[string]types.AttributeValue{
		"PK":                              sAttr(conversationMetadataPartitionPrefix),
		"SK":                              sAttr(conversationMetadataSortKey),
		conversationLastStatusIDAttribute: sAttr("status-1"),
		conversationTotalMessageCountAttribute: &types.AttributeValueMemberN{
			Value: "0",
		},
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, candidate.ManualReview)
	require.Empty(t, candidate.ConversationID)
}

func conversationMetadataStatusItem(conversationID string, statusID string, publishedAt time.Time) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":                                 sAttr("status#" + statusID),
		"SK":                                 sAttr("status#" + statusID),
		"gsi3PK":                             sAttr(conversationMetadataPartitionPrefix + conversationID),
		conversationIDAttribute:              sAttr(conversationID),
		conversationMessageStatusIDAttribute: sAttr(statusID),
		"publishedAt":                        sAttr(publishedAt.UTC().Format(time.RFC3339Nano)),
	}
}
