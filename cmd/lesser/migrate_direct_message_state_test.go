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
	require.Contains(t, output, "legacy_message_rows_planned: 0")
	require.Contains(t, output, "legacy_message_rows_deleted: 0")
	require.Contains(t, output, "orphan_lookup_rows_planned: 0")
	require.Contains(t, output, "orphan_lookup_rows_deleted: 0")
	require.Contains(t, output, "validation_errors: 0")
	require.Contains(t, output, "sample_conversation_ids:")
	require.Contains(t, output, "  conv-1")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 7)
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
	require.Zero(t, summary.LegacyMessageRowsPlanned)
	require.Zero(t, summary.LegacyMessageRowsDeleted)
	require.Zero(t, summary.OrphanLookupRowsPlanned)
	require.Zero(t, summary.OrphanLookupRowsDeleted)
	require.Zero(t, summary.ValidationErrors)
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
	require.Zero(t, summary.LegacyMessageRowsPlanned)
	require.Zero(t, summary.LegacyMessageRowsDeleted)
	require.Zero(t, summary.OrphanLookupRowsPlanned)
	require.Zero(t, summary.OrphanLookupRowsDeleted)
	require.Zero(t, summary.ValidationErrors)
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

func TestExecuteDirectMessageStateMigration_ApplyDeletesLegacyConversationMessageRows(t *testing.T) {
	publishedAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{},
			{},
			{},
			{},
			{},
			{},
			{Items: []map[string]types.AttributeValue{
				conversationMessageRow("conv-1", "status-1", publishedAt),
			}},
		},
	}

	summary, err := executeDirectMessageStateMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.LegacyMessageRowsPlanned)
	require.Equal(t, 1, summary.LegacyMessageRowsDeleted)
	require.Len(t, client.deleteInputs, 1)
}

func TestExecuteDirectMessageStateMigration_ApplyDeletesOrphanLookupRows(t *testing.T) {
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{},
			{},
			{},
			{Items: []map[string]types.AttributeValue{
				conversationLookupRow("arch,scout", "conv-stale"),
			}},
			{},
			{},
			{},
		},
	}

	summary, err := executeDirectMessageStateMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.OrphanLookupRowsPlanned)
	require.Equal(t, 1, summary.OrphanLookupRowsDeleted)
	require.Len(t, client.deleteInputs, 1)
}

func TestBuildDirectMessageCanonicalStateRecord_ParsesCanonicalStateRows(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	lastReadAt := createdAt.Add(6 * time.Minute)
	item := map[string]types.AttributeValue{
		"PK":                       sAttr("USER_CONVERSATION_STATE#Arch"),
		"SK":                       sAttr("CONVERSATION#conv-1"),
		"counterpartID":            sAttr("scout"),
		"folder":                   sAttr(string(models.UserConversationFolderInbox)),
		"requestState":             sAttr(string(models.DmRequestStateAccepted)),
		"previewStatusID":          sAttr("status-1"),
		"previewStatusPublishedAt": sAttr(updatedAt.Format(time.RFC3339Nano)),
		"sortAt":                   sAttr(updatedAt.Format(time.RFC3339Nano)),
		"unread":                   &types.AttributeValueMemberBOOL{Value: true},
		"unreadCount":              &types.AttributeValueMemberN{Value: "7"},
		"lastReadAt":               sAttr(lastReadAt.Format(time.RFC3339Nano)),
		"createdAt":                sAttr(createdAt.Format(time.RFC3339Nano)),
		"updatedAt":                sAttr(updatedAt.Format(time.RFC3339Nano)),
	}

	record, ok := buildDirectMessageCanonicalStateRecord(item)
	require.True(t, ok)
	require.NotNil(t, record)
	require.Equal(t, "arch", record.State.ViewerID)
	require.Equal(t, "conv-1", record.State.ConversationID)
	require.Equal(t, "scout", record.State.CounterpartID)
	require.Equal(t, models.UserConversationFolderInbox, record.State.Folder)
	require.Equal(t, models.DmRequestStateAccepted, record.State.RequestState)
	require.Equal(t, "status-1", record.State.PreviewStatusID)
	require.Equal(t, updatedAt, record.State.SortAt)
	require.Equal(t, 7, record.State.UnreadCount)
	require.Equal(t, lastReadAt, *record.State.LastReadAt)

	record, ok = buildDirectMessageCanonicalStateRecord(map[string]types.AttributeValue{
		"PK": sAttr("USER_CONVERSATION_STATE#"),
		"SK": sAttr("CONVERSATION#"),
	})
	require.False(t, ok)
	require.Nil(t, record)
}

func TestDirectMessageMentionHelpers_CoverRecipientFallbacksAndTagFiltering(t *testing.T) {
	recipientID := "https://remote.example/users/medic"
	fallbackItem := map[string]types.AttributeValue{
		"note": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"BaseObject": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"To": mustMarshalAttributeValue([]string{recipientID}),
			}},
			"Tag": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"Type": sAttr("Mention"),
					"Href": sAttr(recipientID),
					"Name": sAttr("@medic@remote.example"),
				}},
				&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"Type": sAttr("Hashtag"),
					"Name": sAttr("#ops"),
				}},
				&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}},
			}},
		}},
	}

	require.Equal(t, []string{recipientID}, directMessageStatusRecipientActorIDs(map[string]types.AttributeValue{
		"toRecipients": mustMarshalAttributeValue([]string{recipientID}),
	}))
	require.Equal(t, []string{recipientID}, directMessageStatusRecipientActorIDs(fallbackItem))
	require.Nil(t, directMessageStatusRecipientActorIDs(map[string]types.AttributeValue{}))

	tags := directMessageStatusNonMentionTags(fallbackItem["note"].(*types.AttributeValueMemberM).Value)
	require.Len(t, tags, 1)
	require.Equal(t, "Hashtag", tags[0].Type)
	require.Equal(t, "#ops", tags[0].Name)

	mentionTags := directMessageMentionTags([]string{
		recipientID,
		" https://example.com/@arch ",
		"",
	}, "https://example.com/users/arch")
	require.Len(t, mentionTags, 2)
	require.Equal(t, "@medic@remote.example", mentionTags[0].Name)
	require.Equal(t, "@arch", mentionTags[1].Name)

	require.Equal(t, "@", directMessageMentionName("", "https://example.com/users/arch"))
	require.Equal(t, []string{"medic", "remote.example"}, pairToStrings(actorIDUsernameAndHost("https://remote.example/users/medic")))
	require.Equal(t, []string{"arch", "example.com"}, pairToStrings(actorIDUsernameAndHost("https://example.com/@arch")))
	require.Equal(t, []string{"leaf", "remote.example"}, pairToStrings(actorIDUsernameAndHost("https://remote.example/actors/leaf")))
	require.Equal(t, []string{"arch", "remote.example"}, pairToStrings(actorIDUsernameAndHost("@arch@remote.example")))
	require.Equal(t, []string{"arch", ""}, pairToStrings(actorIDUsernameAndHost("arch")))
	require.Equal(t, []string{"dup", "unique"}, uniqueNonEmptyStrings([]string{" dup ", "", "dup", "unique"}))
}

func TestDirectMessageMigrationDecisionHelpers_CoverFallbackSelection(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(10 * time.Minute)
	lastMessageTime := createdAt.Add(20 * time.Minute)
	previewTime := createdAt.Add(30 * time.Minute)
	requestedAt := createdAt.Add(40 * time.Minute)
	deletedAt := createdAt.Add(50 * time.Minute)
	lastReadAt := createdAt.Add(60 * time.Minute)

	existing := &directMessageCanonicalStateRecord{
		State: &models.UserConversationState{
			PreviewStatusID:          "status-existing",
			PreviewStatusPublishedAt: previewTime,
			SortAt:                   previewTime,
			CreatedAt:                createdAt,
			UpdatedAt:                updatedAt,
			RequestState:             models.DmRequestStateAccepted,
			Folder:                   models.UserConversationFolderInbox,
			Unread:                   true,
			LastReadAt:               &lastReadAt,
			DeletedAt:                &deletedAt,
			RequestedAt:              &requestedAt,
			AcceptedAt:               conversationTimePtr(createdAt.Add(70 * time.Minute)),
			DeclinedAt:               conversationTimePtr(createdAt.Add(80 * time.Minute)),
		},
	}
	participant := &directMessageLegacyParticipantRow{
		RequestState: models.DmRequestStatePending,
		Unread:       true,
		LastReadAt:   &lastReadAt,
		DeletedAt:    &deletedAt,
		RequestedAt:  &requestedAt,
	}
	legacyReadState := &directMessageLegacyReadState{
		Unread:     true,
		LastReadAt: &lastReadAt,
	}
	conversation := directMessageMigrationConversation{
		ConversationID:     "conv-1",
		Participants:       []string{"arch", "scout"},
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
		LastStatusID:       "status-meta",
		LastMessageTime:    lastMessageTime,
		ThreadLastStatusID: "status-thread",
		ThreadLastTime:     previewTime,
	}

	t.Run("preview prefers thread then metadata then existing", func(t *testing.T) {
		statusID, publishedAt := directMessagePreviewForMigration(conversation, existing)
		require.Equal(t, "status-thread", statusID)
		require.Equal(t, previewTime, publishedAt)

		conversation.ThreadLastStatusID = ""
		statusID, publishedAt = directMessagePreviewForMigration(conversation, existing)
		require.Equal(t, "status-meta", statusID)
		require.Equal(t, lastMessageTime, publishedAt)

		conversation.LastStatusID = ""
		statusID, publishedAt = directMessagePreviewForMigration(conversation, existing)
		require.Equal(t, "status-existing", statusID)
		require.Equal(t, previewTime, publishedAt)
	})

	t.Run("sort and created timestamps use stable fallbacks", func(t *testing.T) {
		require.Equal(t, previewTime, directMessageMigrationSortAt(previewTime, conversation, nil))
		require.Equal(t, existing.State.SortAt, directMessageMigrationSortAt(time.Time{}, conversation, existing))
		require.Equal(t, lastMessageTime, directMessageMigrationSortAt(time.Time{}, conversation, nil))

		require.Equal(t, existing.State.CreatedAt, directMessageMigrationCreatedAt(conversation, existing))
		require.Equal(t, createdAt, directMessageMigrationCreatedAt(conversation, nil))
		require.False(t, directMessageMigrationCreatedAt(directMessageMigrationConversation{}, nil).IsZero())
	})

	t.Run("request folder read and update helpers prefer canonical state", func(t *testing.T) {
		require.Equal(t, models.DmRequestStateAccepted, directMessageMigrationRequestState(existing, participant))
		require.Equal(t, models.UserConversationFolderHidden, directMessageMigrationFolder(nil, models.DmRequestStateAccepted, &deletedAt))
		require.Equal(t, models.UserConversationFolderInbox, directMessageMigrationFolder(existing, models.DmRequestStatePending, nil))
		require.Equal(t, models.UserConversationFolderRequests, directMessageMigrationFolder(nil, models.DmRequestStatePending, nil))
		require.Equal(t, models.UserConversationFolderDeclined, directMessageMigrationFolder(nil, models.DmRequestStateDeclined, nil))

		require.Equal(t, models.DmRequestStatePending, directMessageMigrationDefaultRequestState(models.UserConversationFolderRequests))
		require.Equal(t, models.DmRequestStateDeclined, directMessageMigrationDefaultRequestState(models.UserConversationFolderDeclined))
		require.Equal(t, models.DmRequestStateAccepted, directMessageMigrationDefaultRequestState(models.UserConversationFolderInbox))

		unread, readAt := directMessageMigrationReadState(existing, legacyReadState, participant)
		require.True(t, unread)
		require.Equal(t, existing.State.LastReadAt, readAt)

		existingOnlyRead := &directMessageCanonicalStateRecord{}
		unread, readAt = directMessageMigrationReadState(existingOnlyRead, legacyReadState, participant)
		require.True(t, unread)
		require.Equal(t, legacyReadState.LastReadAt, readAt)

		unread, readAt = directMessageMigrationReadState(nil, nil, participant)
		require.True(t, unread)
		require.Equal(t, participant.LastReadAt, readAt)

		unread, readAt = directMessageMigrationReadState(nil, nil, nil)
		require.False(t, unread)
		require.Nil(t, readAt)
	})

	t.Run("normalization updated-at and counterpart helpers cover edge cases", func(t *testing.T) {
		require.Nil(t, normalizeLegacyMigrationLastReadAt(nil, false))
		require.Nil(t, normalizeLegacyMigrationLastReadAt(conversationTimePtr(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)), false))
		require.Nil(t, normalizeLegacyMigrationLastReadAt(conversationTimePtr(time.Unix(0, 0).UTC()), true))
		require.Equal(t, lastReadAt, *normalizeLegacyMigrationLastReadAt(&lastReadAt, false))

		state := &models.UserConversationState{
			CreatedAt:                createdAt,
			SortAt:                   updatedAt,
			PreviewStatusPublishedAt: previewTime,
			LastReadAt:               &lastReadAt,
		}
		require.Equal(t, lastReadAt, directMessageMigrationUpdatedAt(state, conversation, existing))
		require.False(t, directMessageMigrationUpdatedAt(&models.UserConversationState{}, directMessageMigrationConversation{}, nil).IsZero())

		require.Equal(t, "scout", directMessageCounterpartIDForMigration("arch", []string{"arch", "scout"}))
		require.Empty(t, directMessageCounterpartIDForMigration("arch", []string{"arch"}))
		require.Equal(t, previewTime, directMessageLookupPlanSortTime(conversation))
		require.True(t, directMessageLookupPlanSortTime(directMessageMigrationConversation{}).IsZero())
	})
}

func TestBuildMigratedUserConversationStateItem_CoversExistingAndLegacyFallbacks(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	previewTime := createdAt.Add(5 * time.Minute)
	conversation := directMessageMigrationConversation{
		ConversationID:     "conv-1",
		Participants:       []string{"arch", "scout"},
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt.Add(10 * time.Minute),
		ThreadLastStatusID: "status-1",
		ThreadLastTime:     previewTime,
	}
	legacyParticipant := &directMessageLegacyParticipantRow{
		RequestState: models.DmRequestStatePending,
		Unread:       true,
		RequestedAt:  conversationTimePtr(createdAt.Add(time.Minute)),
	}
	legacyReadState := &directMessageLegacyReadState{
		Unread:     true,
		LastReadAt: conversationTimePtr(time.Unix(0, 0).UTC()),
	}

	state, item, changed, err := buildMigratedUserConversationStateItem(conversation, "arch", nil, legacyParticipant, legacyReadState)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.True(t, changed)
	require.Equal(t, "scout", state.CounterpartID)
	require.Equal(t, models.UserConversationFolderRequests, state.Folder)
	require.Equal(t, models.DmRequestStatePending, state.RequestState)
	require.True(t, state.Unread)
	require.Nil(t, state.LastReadAt)
	require.Equal(t, "status-1", strAttr(t, item["previewStatusID"]))
	require.Equal(t, "2026-03-25T10:44:00.000000000Z#conv-1", strAttr(t, item["gsi1SK"]))

	state.UnreadCount = 7
	item["unreadCount"] = &types.AttributeValueMemberN{Value: "7"}
	existing := &directMessageCanonicalStateRecord{
		State: state,
		Item:  cloneAttributeMap(item),
	}
	state, item, changed, err = buildMigratedUserConversationStateItem(conversation, "arch", existing, legacyParticipant, legacyReadState)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.False(t, changed)
	require.Equal(t, models.UserConversationFolderRequests, state.Folder)
	require.Equal(t, 7, state.UnreadCount)

	state, item, changed, err = buildMigratedUserConversationStateItem(conversation, "   ", nil, legacyParticipant, legacyReadState)
	require.NoError(t, err)
	require.Nil(t, state)
	require.Nil(t, item)
	require.False(t, changed)
}

func TestLoadDirectMessageCanonicalAndLookupRows_PaginatesAndSkipsInvalidRows(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)

	canonicalClient := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					{
						"PK":        sAttr("USER_CONVERSATION_STATE#Arch"),
						"SK":        sAttr("CONVERSATION#conv-1"),
						"folder":    sAttr(string(models.UserConversationFolderInbox)),
						"createdAt": sAttr(createdAt.Format(time.RFC3339Nano)),
						"updatedAt": sAttr(updatedAt.Format(time.RFC3339Nano)),
					},
					{
						"PK": sAttr("USER_CONVERSATION_STATE#"),
						"SK": sAttr("CONVERSATION#"),
					},
				},
				LastEvaluatedKey: map[string]types.AttributeValue{"PK": sAttr("cursor")},
			},
			{},
		},
	}

	rows, err := loadDirectMessageCanonicalStates(context.Background(), canonicalClient, "simulacrum-dev-main-table")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, rows["conv-1"], "arch")
	require.Len(t, canonicalClient.scanInputs, 2)
	require.Equal(t, "cursor", strAttr(t, canonicalClient.scanInputs[1].ExclusiveStartKey["PK"]))

	lookupClient := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				conversationLookupRow("arch,scout", "conv-1"),
				{"SK": sAttr("LOOKUP")},
			},
		}},
	}

	lookupRows, err := loadDirectMessageLookupRows(context.Background(), lookupClient, "simulacrum-dev-main-table")
	require.NoError(t, err)
	require.Len(t, lookupRows, 1)
	require.Equal(t, "conv-1", strAttr(t, lookupRows["CONVERSATION_PARTICIPANTS#arch,scout"][conversationLookupConversationIDAttr]))
}

func TestDirectMessageMigrationScalarHelpers_CoverFalseyInputs(t *testing.T) {
	item := map[string]types.AttributeValue{
		"boolValue": &types.AttributeValueMemberBOOL{Value: true},
		"intValue":  &types.AttributeValueMemberN{Value: "42"},
	}

	require.True(t, firstConversationBool(item, "missing", "boolValue"))
	require.False(t, firstConversationBool(map[string]types.AttributeValue{}, "missing"))
	require.EqualValues(t, 42, firstConversationInt64(item, "missing", "intValue"))
	require.Zero(t, firstConversationInt64(map[string]types.AttributeValue{}, "missing"))
	require.Equal(t, 42, firstConversationInt(item, "missing", "intValue"))
	require.Equal(t, 7, firstConversationInt(map[string]types.AttributeValue{"stringValue": sAttr("7")}, "stringValue"))
	require.Zero(t, firstConversationInt(map[string]types.AttributeValue{"negative": &types.AttributeValueMemberN{Value: "-1"}}, "negative"))
	require.Zero(t, firstConversationInt(map[string]types.AttributeValue{"overflow": &types.AttributeValueMemberN{Value: "999999999999999999999999999999"}}, "overflow"))
	require.Nil(t, conversationTimePtr(time.Time{}))
}

func pairToStrings(first string, second string) []string {
	return []string{first, second}
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
