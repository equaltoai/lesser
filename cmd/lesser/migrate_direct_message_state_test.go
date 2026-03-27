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

func TestRunMigrateDirectMessageState_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newDirectMessageStateMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newDirectMessageStateMigrationClientFn = previousClientFactory
	})

	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	lastStatusTime := createdAt.Add(2 * time.Minute)
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataItem(
					"conv-1",
					[]string{"arch", "scout"},
					createdAt,
					createdAt,
					1,
					"status-1",
					lastStatusTime,
				),
			},
		}},
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataStatusItem("conv-1", "status-1", lastStatusTime),
			},
		}},
	}

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{}, "Sim", nil
	}
	newDirectMessageStateMigrationClientFn = func(aws.Config) directMessageStateMigrationClient {
		return client
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateDirectMessageState([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Equal(t, "Sim", seenProfile)
	require.Contains(t, output, "migrate-direct-message-state dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "scanned_conversations: 1")
	require.Contains(t, output, "active_conversations: 1")
	require.Contains(t, output, "status_backed_conversations: 1")
	require.Contains(t, output, "thread_statuses_scanned: 1")
	require.Contains(t, output, "sample_conversation_ids:")
	require.Contains(t, output, "  conv-1")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 1)
	require.Len(t, client.queryInputs, 1)
	require.Equal(
		t,
		conversationMetadataPartitionPrefix+"conv-1",
		strAttr(t, client.queryInputs[0].ExpressionAttributeValues[":conversation"]),
	)
}

func TestExecuteDirectMessageStateMigration_EnumeratesThreadRealityAndLimit(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	firstStatusTime := createdAt.Add(time.Minute)
	secondStatusTime := createdAt.Add(2 * time.Minute)

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"arch", "scout"}, createdAt, createdAt, 0, "", time.Time{}),
				conversationMetadataItem("conv-2", []string{"arch", "medic"}, createdAt, createdAt, 0, "", time.Time{}),
			},
		}},
		queryOutputs: []*dynamodb.QueryOutput{
			{
				Items: []map[string]types.AttributeValue{
					conversationMetadataStatusItem("conv-1", "status-1", firstStatusTime),
					conversationMetadataStatusItem("conv-1", "status-2", secondStatusTime),
				},
			},
		},
	}

	summary, err := executeDirectMessageStateMigration(context.Background(), client, "simulacrum-dev-main-table", false, 1)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedConversations)
	require.Equal(t, 1, summary.ActiveConversations)
	require.Equal(t, 1, summary.StatusBackedConversations)
	require.Equal(t, 2, summary.ThreadStatusesScanned)
	require.Equal(t, []string{"conv-1"}, summary.SampleConversationIDs)
	require.Len(t, client.queryInputs, 1)
}

func TestExecuteDirectMessageStateMigration_ValidatesInputsAndQueryErrors(t *testing.T) {
	_, err := executeDirectMessageStateMigration(context.Background(), nil, "simulacrum-dev-main-table", false, 0)
	require.EqualError(t, err, "migration client is required")

	_, err = executeDirectMessageStateMigration(context.Background(), &fakeUserKeyMigrationClient{}, "   ", false, 0)
	require.EqualError(t, err, "table name is required")

	_, err = executeDirectMessageStateMigration(context.Background(), &fakeUserKeyMigrationClient{
		scanErr: context.DeadlineExceeded,
	}, "simulacrum-dev-main-table", false, 0)
	require.ErrorContains(t, err, "scan conversation metadata rows")

	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	_, err = executeDirectMessageStateMigration(context.Background(), &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"arch", "scout"}, createdAt, createdAt, 0, "", time.Time{}),
			},
		}},
		queryErr: context.DeadlineExceeded,
	}, "simulacrum-dev-main-table", false, 0)
	require.ErrorContains(t, err, "load thread statuses for \"conv-1\"")
}

func TestBuildDirectMessageMigrationConversation_SkipsInvalidRows(t *testing.T) {
	conversation, ok := buildDirectMessageMigrationConversation(map[string]types.AttributeValue{
		"PK": sAttr("USER#arch"),
	})
	require.False(t, ok)
	require.Equal(t, directMessageMigrationConversation{}, conversation)

	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	conversation, ok = buildDirectMessageMigrationConversation(conversationMetadataItem(
		"conv-1",
		[]string{"arch", "scout"},
		createdAt,
		createdAt.Add(time.Minute),
		3,
		"status-3",
		createdAt.Add(2*time.Minute),
	))
	require.True(t, ok)
	require.Equal(t, "conv-1", conversation.ConversationID)
	require.Equal(t, []string{"arch", "scout"}, conversation.Participants)
	require.EqualValues(t, 3, conversation.TotalMessageCount)
	require.Equal(t, "status-3", conversation.LastStatusID)
}

func TestLoadDirectMessageThreadStatuses_PaginatesAndSkipsInvalidRows(t *testing.T) {
	firstStatusTime := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	secondStatusTime := firstStatusTime.Add(time.Minute)

	client := &fakeUserKeyMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{
			{
				Items: []map[string]types.AttributeValue{
					{"publishedAt": sAttr(firstStatusTime.Format(time.RFC3339Nano))},
					conversationMetadataStatusItem("conv-1", "status-1", firstStatusTime),
				},
				LastEvaluatedKey: map[string]types.AttributeValue{"PK": sAttr("next")},
			},
			{
				Items: []map[string]types.AttributeValue{
					conversationMetadataStatusItem("conv-1", "status-2", secondStatusTime),
				},
			},
		},
	}

	items, count, lastStatusID, lastStatusTime, err := loadDirectMessageThreadStatuses(
		context.Background(),
		client,
		"simulacrum-dev-main-table",
		"conv-1",
	)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 2, count)
	require.Equal(t, "status-2", lastStatusID)
	require.Equal(t, secondStatusTime, lastStatusTime)
	require.Len(t, client.queryInputs, 2)
	require.Equal(t, "next", strAttr(t, client.queryInputs[1].ExclusiveStartKey["PK"]))
}
