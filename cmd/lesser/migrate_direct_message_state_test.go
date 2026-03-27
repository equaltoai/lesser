package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
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
		scanOutputs: []*dynamodb.ScanOutput{
			{},
			{},
			{},
			{},
			{},
			{
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
			},
		},
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
	require.Contains(t, output, "canonical_state_rows_planned: 2")
	require.Contains(t, output, "canonical_state_rows_upserted: 0")
	require.Contains(t, output, "lookup_rows_planned: 1")
	require.Contains(t, output, "lookup_rows_upserted: 0")
	require.Contains(t, output, "mention_repairs_planned: 0")
	require.Contains(t, output, "mention_repairs_applied: 0")
	require.Contains(t, output, "legacy_participant_rows_planned: 0")
	require.Contains(t, output, "legacy_participant_rows_deleted: 0")
	require.Contains(t, output, "legacy_read_state_rows_planned: 0")
	require.Contains(t, output, "legacy_read_state_rows_deleted: 0")
	require.Contains(t, output, "sample_conversation_ids:")
	require.Contains(t, output, "  conv-1")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 6)
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
		scanOutputs: []*dynamodb.ScanOutput{
			{},
			{},
			{},
			{},
			{},
			{
				Items: []map[string]types.AttributeValue{
					conversationMetadataItem("conv-1", []string{"arch", "scout"}, createdAt, createdAt, 0, "", time.Time{}),
					conversationMetadataItem("conv-2", []string{"arch", "medic"}, createdAt, createdAt, 0, "", time.Time{}),
				},
			},
		},
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
	require.Equal(t, 2, summary.CanonicalStateRowsPlanned)
	require.Zero(t, summary.CanonicalStateRowsUpserted)
	require.Equal(t, 1, summary.LookupRowsPlanned)
	require.Zero(t, summary.LookupRowsUpserted)
	require.Zero(t, summary.MentionRepairsPlanned)
	require.Zero(t, summary.MentionRepairsApplied)
	require.Zero(t, summary.LegacyParticipantRowsPlanned)
	require.Zero(t, summary.LegacyParticipantRowsDeleted)
	require.Zero(t, summary.LegacyReadStateRowsPlanned)
	require.Zero(t, summary.LegacyReadStateRowsDeleted)
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
	require.ErrorContains(t, err, "scan legacy participant rows")

	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	_, err = executeDirectMessageStateMigration(context.Background(), &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{},
			{},
			{},
			{},
			{},
			{
				Items: []map[string]types.AttributeValue{
					conversationMetadataItem("conv-1", []string{"arch", "scout"}, createdAt, createdAt, 0, "", time.Time{}),
				},
			},
		},
		queryErr: context.DeadlineExceeded,
	}, "simulacrum-dev-main-table", false, 0)
	require.ErrorContains(t, err, "load thread statuses for \"conv-1\"")
}

func TestExecuteDirectMessageStateMigration_ApplyBackfillsCanonicalStateRows(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	lastStatusTime := createdAt.Add(2 * time.Minute)
	archReadAt := createdAt.Add(3 * time.Minute)

	scoutRow := conversationParticipantRow("scout", "conv-1", createdAt, true, buildConversationSnapshot(
		"conv-1",
		[]string{"arch", "scout"},
		createdAt,
		createdAt,
		"status-1",
		1,
		lastStatusTime,
		true,
	))
	scoutRow["requestState"] = sAttr(string(models.DmRequestStatePending))
	scoutRow["requestedAt"] = sAttr(createdAt.Add(time.Minute).Format(time.RFC3339Nano))

	archRow := conversationParticipantRow("arch", "conv-1", createdAt, false, buildConversationSnapshot(
		"conv-1",
		[]string{"arch", "scout"},
		createdAt,
		createdAt,
		"status-1",
		1,
		lastStatusTime,
		false,
	))
	archRow["requestState"] = sAttr(string(models.DmRequestStateAccepted))
	archRow["acceptedAt"] = sAttr(createdAt.Format(time.RFC3339Nano))

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{Items: []map[string]types.AttributeValue{archRow, scoutRow}},
			{Items: []map[string]types.AttributeValue{
				conversationStatusRow("conv-1", "arch", false, archReadAt),
				conversationStatusRow("conv-1", "scout", true, time.Unix(0, 0).UTC()),
			}},
			{},
			{},
			{},
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"arch", "scout"}, createdAt, createdAt, 1, "status-1", lastStatusTime),
			}},
		},
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{
				conversationMetadataStatusItem("conv-1", "status-1", lastStatusTime),
			},
		}},
	}

	summary, err := executeDirectMessageStateMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 2, summary.CanonicalStateRowsPlanned)
	require.Equal(t, 2, summary.CanonicalStateRowsUpserted)
	require.Equal(t, 1, summary.LookupRowsPlanned)
	require.Equal(t, 1, summary.LookupRowsUpserted)
	require.Zero(t, summary.MentionRepairsPlanned)
	require.Zero(t, summary.MentionRepairsApplied)
	require.Equal(t, 2, summary.LegacyParticipantRowsPlanned)
	require.Equal(t, 2, summary.LegacyParticipantRowsDeleted)
	require.Equal(t, 2, summary.LegacyReadStateRowsPlanned)
	require.Equal(t, 2, summary.LegacyReadStateRowsDeleted)
	require.Len(t, client.putInputs, 5)
	require.Len(t, client.deleteInputs, 4)

	archState := findPutItem(t, client.putInputs, "USER_CONVERSATION_STATE#arch", "CONVERSATION#conv-1")
	require.Equal(t, "INBOX", strAttr(t, archState["folder"]))
	require.Equal(t, "ACCEPTED", strAttr(t, archState["requestState"]))
	require.Equal(t, "status-1", strAttr(t, archState["previewStatusID"]))
	require.Equal(t, archReadAt.Format(time.RFC3339Nano), strAttr(t, archState["lastReadAt"]))

	scoutState := findPutItem(t, client.putInputs, "USER_CONVERSATION_STATE#scout", "CONVERSATION#conv-1")
	require.Equal(t, "REQUESTS", strAttr(t, scoutState["folder"]))
	require.Equal(t, "PENDING", strAttr(t, scoutState["requestState"]))
	require.Equal(t, "status-1", strAttr(t, scoutState["previewStatusID"]))
	require.Equal(t, "USER_CONVERSATION_UNREAD#scout", strAttr(t, scoutState["gsi2PK"]))
	_, hasLastReadAt := scoutState["lastReadAt"]
	require.False(t, hasLastReadAt)

	lookupItem := findPutItem(t, client.putInputs, "CONVERSATION_PARTICIPANTS#arch,scout", "LOOKUP")
	require.Equal(t, "conv-1", strAttr(t, lookupItem[conversationLookupConversationIDAttr]))
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

func TestDirectMessageLookupPlanSelection_PrefersNewestThenLexicalID(t *testing.T) {
	older := directMessageLookupPlan{
		PK:             "CONVERSATION_PARTICIPANTS#arch,scout",
		ConversationID: "conv-b",
		SortTime:       time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC),
	}
	newer := directMessageLookupPlan{
		PK:             older.PK,
		ConversationID: "conv-c",
		SortTime:       older.SortTime.Add(time.Minute),
	}
	tieBreaker := directMessageLookupPlan{
		PK:             older.PK,
		ConversationID: "conv-a",
		SortTime:       older.SortTime,
	}

	require.True(t, shouldReplaceDirectMessageLookupPlan(older, newer))
	require.True(t, shouldReplaceDirectMessageLookupPlan(older, tieBreaker))
	require.False(t, shouldReplaceDirectMessageLookupPlan(newer, older))
}

func TestBuildDirectMessageMentionRepairItem_RepairsMissingMentionsAndTags(t *testing.T) {
	publishedAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	item := directMessageStatusRepairCandidateItem(
		"status-1",
		"https://dev.simulacrum.greater.website/users/arch",
		"https://dev.simulacrum.greater.website/users/medic",
		publishedAt,
	)

	repaired, changed, err := buildDirectMessageMentionRepairItem(item)
	require.NoError(t, err)
	require.True(t, changed)

	mentions, ok := attributeStringSlice(repaired["mentions"])
	require.True(t, ok)
	require.Equal(t, []string{"https://dev.simulacrum.greater.website/users/medic"}, mentions)

	noteValue, ok := repaired["note"].(*types.AttributeValueMemberM)
	require.True(t, ok)
	tagValue, ok := noteValue.Value["Tag"].(*types.AttributeValueMemberL)
	require.True(t, ok)
	require.Len(t, tagValue.Value, 1)
	tagMap, ok := tagValue.Value[0].(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Equal(t, "Mention", strAttr(t, tagMap.Value["Type"]))
	require.Equal(t, "https://dev.simulacrum.greater.website/users/medic", strAttr(t, tagMap.Value["Href"]))
	require.Equal(t, "@medic", strAttr(t, tagMap.Value["Name"]))
}

func TestExecuteDirectMessageStateMigration_ApplyRepairsDirectStatusMentions(t *testing.T) {
	publishedAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{},
			{},
			{},
			{},
			{Items: []map[string]types.AttributeValue{
				directMessageStatusRepairCandidateItem(
					"status-1",
					"https://dev.simulacrum.greater.website/users/arch",
					"https://dev.simulacrum.greater.website/users/medic",
					publishedAt,
				),
			}},
			{},
		},
	}

	summary, err := executeDirectMessageStateMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.MentionRepairsPlanned)
	require.Equal(t, 1, summary.MentionRepairsApplied)
	require.Len(t, client.putInputs, 3)

	repaired := findPutItem(t, client.putInputs, "status#status-1", "status#status-1")
	mentions, ok := attributeStringSlice(repaired["mentions"])
	require.True(t, ok)
	require.Equal(t, []string{"https://dev.simulacrum.greater.website/users/medic"}, mentions)
}

func directMessageStatusRepairCandidateItem(
	statusID string,
	authorActorID string,
	recipientActorID string,
	publishedAt time.Time,
) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":          sAttr("status#" + statusID),
		"SK":          sAttr("status#" + statusID),
		"statusID":    sAttr(statusID),
		"visibility":  sAttr(models.VisibilityDirect),
		"authorID":    sAttr(authorActorID),
		"publishedAt": sAttr(publishedAt.Format(time.RFC3339Nano)),
		"toRecipients": mustMarshalAttributeValue([]string{
			recipientActorID,
		}),
		"mentions": &types.AttributeValueMemberNULL{Value: true},
		"note": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"AttributedTo": sAttr(authorActorID),
			"BaseObject": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"To": mustMarshalAttributeValue([]string{recipientActorID}),
			}},
		}},
	}
}
