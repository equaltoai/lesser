package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRunMigrateConversations_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newConversationMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newConversationMigrationClientFn = previousClientFactory
	})

	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"Arch", "Medic"}, createdAt, updatedAt, 0, "", time.Time{}),
			}},
			{Items: []map[string]types.AttributeValue{
				conversationParticipantRow("Arch", "conv-1", updatedAt, false, buildConversationSnapshot("conv-1", []string{"Arch", "Medic"}, createdAt, updatedAt, "", 0, time.Time{}, false)),
			}},
			{},
			{},
		},
		queryOutputs: []*dynamodb.QueryOutput{
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"Arch", "Medic"}, createdAt, updatedAt, 0, "", time.Time{}),
			}},
		},
	}

	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		require.Equal(t, "Sim", awsProfile)
		return aws.Config{}, "Sim", nil
	}
	newConversationMigrationClientFn = func(aws.Config) conversationMigrationClient { return client }

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateConversations([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Contains(t, output, "migrate-conversations dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "scanned_conversations: 1")
	require.Contains(t, output, "candidate_groups: 1")
	require.Contains(t, output, "candidate_conversations: 1")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 4)
	require.Len(t, client.queryInputs, 1)
	require.Equal(t, conversationMetadataPartitionPrefix, strAttr(t, client.scanInputs[0].ExpressionAttributeValues[":prefix"]))
	require.Equal(t, conversationParticipantPartitionPrefix, strAttr(t, client.scanInputs[1].ExpressionAttributeValues[":prefix"]))
	require.Equal(t, conversationStatusPartitionPrefix, strAttr(t, client.scanInputs[2].ExpressionAttributeValues[":prefix"]))
	require.Equal(t, conversationParticipantLookupPrefix, strAttr(t, client.scanInputs[3].ExpressionAttributeValues[":prefix"]))
}

func TestExecuteConversationMigration_ApplyDeduplicatesLegacyConversationData(t *testing.T) {
	legacyCreatedAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	legacyUpdatedAt := legacyCreatedAt.Add(5 * time.Minute)
	canonicalCreatedAt := legacyCreatedAt.Add(10 * time.Minute)
	canonicalUpdatedAt := canonicalCreatedAt.Add(5 * time.Minute)
	legacyMessageAt := legacyUpdatedAt.Add(2 * time.Minute)
	canonicalMessageAt := canonicalUpdatedAt.Add(2 * time.Minute)

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("canon-conv", []string{"arch", "medic"}, canonicalCreatedAt, canonicalUpdatedAt, 1, "status-canon", canonicalMessageAt),
				conversationMetadataItem("legacy-conv", []string{"Arch", "Medic"}, legacyCreatedAt, legacyUpdatedAt, 1, "status-legacy", legacyMessageAt),
			}},
			{Items: []map[string]types.AttributeValue{
				conversationParticipantRow("arch", "canon-conv", canonicalUpdatedAt, false, buildConversationSnapshot("canon-conv", []string{"arch", "medic"}, canonicalCreatedAt, canonicalUpdatedAt, "status-canon", 1, canonicalMessageAt, false)),
				conversationParticipantRow("medic", "canon-conv", canonicalUpdatedAt, true, buildConversationSnapshot("canon-conv", []string{"arch", "medic"}, canonicalCreatedAt, canonicalUpdatedAt, "status-canon", 1, canonicalMessageAt, true)),
				conversationParticipantRow("Arch", "legacy-conv", legacyUpdatedAt, false, buildConversationSnapshot("legacy-conv", []string{"Arch", "Medic"}, legacyCreatedAt, legacyUpdatedAt, "status-legacy", 1, legacyMessageAt, false)),
				conversationParticipantRow("Medic", "legacy-conv", legacyUpdatedAt, true, buildConversationSnapshot("legacy-conv", []string{"Arch", "Medic"}, legacyCreatedAt, legacyUpdatedAt, "status-legacy", 1, legacyMessageAt, true)),
			}},
			{Items: []map[string]types.AttributeValue{
				conversationStatusRow("legacy-conv", "Arch", true, legacyUpdatedAt),
			}},
			{Items: []map[string]types.AttributeValue{
				conversationLookupRow("Arch,Medic", "legacy-conv"),
			}},
		},
		queryOutputs: []*dynamodb.QueryOutput{
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("canon-conv", []string{"arch", "medic"}, canonicalCreatedAt, canonicalUpdatedAt, 1, "status-canon", canonicalMessageAt),
				conversationMessageRow("canon-conv", "status-canon", canonicalMessageAt),
			}},
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("legacy-conv", []string{"Arch", "Medic"}, legacyCreatedAt, legacyUpdatedAt, 1, "status-legacy", legacyMessageAt),
				conversationMessageRow("legacy-conv", "status-legacy", legacyMessageAt),
			}},
		},
	}

	summary, err := executeConversationMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 2, summary.ScannedConversations)
	require.Equal(t, 1, summary.CandidateGroups)
	require.Equal(t, 2, summary.CandidateConversations)
	require.Equal(t, 2, summary.ConversationRowsUpserted)
	require.Equal(t, 2, summary.ConversationRowsDeleted)
	require.Equal(t, 2, summary.ParticipantRowsUpserted)
	require.Equal(t, 4, summary.ParticipantRowsDeleted)
	require.Equal(t, 1, summary.StatusRowsUpserted)
	require.Equal(t, 1, summary.StatusRowsDeleted)
	require.Equal(t, 1, summary.LookupRowsUpserted)
	require.Equal(t, 1, summary.LookupRowsDeleted)

	metadataPut := findPutItem(t, client.putInputs, "CONVERSATION#canon-conv", "METADATA")
	participants, ok := attributeStringSlice(metadataPut[conversationParticipantsAttribute])
	require.True(t, ok)
	require.Equal(t, []string{"arch", "medic"}, participants)
	require.Equal(t, int64(2), numAttr(t, metadataPut[conversationTotalMessageCountAttribute]))
	require.Equal(t, "status-canon", strAttr(t, metadataPut[conversationLastStatusIDAttribute]))

	movedMessagePut := findPutItem(t, client.putInputs, "CONVERSATION#canon-conv", conversationMessageRowSK("status-legacy", legacyMessageAt))
	require.Equal(t, "canon-conv", strAttr(t, movedMessagePut[conversationIDAttribute]))

	archParticipantPut := findPutItem(t, client.putInputs, "USER_CONVERSATIONS#arch", conversationParticipantSortKey(canonicalMessageAt, "canon-conv"))
	require.Equal(t, "PARTICIPANT#arch", strAttr(t, archParticipantPut["gsi1SK"]))
	require.Equal(t, "canon-conv", conversationSnapshotID(t, archParticipantPut[conversationSnapshotAttribute]))

	statusPut := findPutItem(t, client.putInputs, "CONVERSATION_STATUS#canon-conv", "USER#arch")
	unread, ok := attributeBool(statusPut[conversationUnreadAttribute])
	require.True(t, ok)
	require.True(t, unread)
	require.Equal(t, "arch", strAttr(t, statusPut[conversationUserIDAttribute]))

	lookupPut := findPutItem(t, client.putInputs, "CONVERSATION_PARTICIPANTS#arch,medic", "LOOKUP")
	require.Equal(t, "canon-conv", strAttr(t, lookupPut[conversationLookupConversationIDAttr]))

	require.True(t, deleteExists(client.deleteInputs, "CONVERSATION#legacy-conv", "METADATA"))
	require.True(t, deleteExists(client.deleteInputs, "CONVERSATION#legacy-conv", conversationMessageRowSK("status-legacy", legacyMessageAt)))
	require.True(t, deleteExists(client.deleteInputs, "USER_CONVERSATIONS#Arch", conversationParticipantSortKey(legacyUpdatedAt, "legacy-conv")))
	require.True(t, deleteExists(client.deleteInputs, "USER_CONVERSATIONS#Medic", conversationParticipantSortKey(legacyUpdatedAt, "legacy-conv")))
	require.True(t, deleteExists(client.deleteInputs, "CONVERSATION_STATUS#legacy-conv", "USER#Arch"))
	require.True(t, deleteExists(client.deleteInputs, "CONVERSATION_PARTICIPANTS#Arch,Medic", "LOOKUP"))
}

func TestExecuteConversationMigration_SkipsConformantConversationGroup(t *testing.T) {
	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"arch", "medic"}, createdAt, updatedAt, 0, "", time.Time{}),
			}},
			{Items: []map[string]types.AttributeValue{
				conversationParticipantRow("arch", "conv-1", updatedAt, false, buildConversationSnapshot("conv-1", []string{"arch", "medic"}, createdAt, updatedAt, "", 0, time.Time{}, false)),
				conversationParticipantRow("medic", "conv-1", updatedAt, false, buildConversationSnapshot("conv-1", []string{"arch", "medic"}, createdAt, updatedAt, "", 0, time.Time{}, false)),
			}},
			{},
			{Items: []map[string]types.AttributeValue{
				conversationLookupRow("arch,medic", "conv-1"),
			}},
		},
		queryOutputs: []*dynamodb.QueryOutput{
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"arch", "medic"}, createdAt, updatedAt, 0, "", time.Time{}),
			}},
		},
	}

	summary, err := executeConversationMigration(context.Background(), client, "simulacrum-dev-main-table", false, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedConversations)
	require.Equal(t, 0, summary.CandidateGroups)
	require.Equal(t, 0, summary.CandidateConversations)
	require.Empty(t, client.putInputs)
	require.Empty(t, client.deleteInputs)
}

func TestExecuteConversationMigration_ValidatesInputsAndQueryErrors(t *testing.T) {
	_, err := executeConversationMigration(context.Background(), nil, "simulacrum-dev-main-table", false, 0)
	require.Error(t, err)

	_, err = executeConversationMigration(context.Background(), &fakeUserKeyMigrationClient{}, "", false, 0)
	require.Error(t, err)

	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{Items: []map[string]types.AttributeValue{
				conversationMetadataItem("conv-1", []string{"arch", "medic"}, createdAt, updatedAt, 0, "", time.Time{}),
			}},
			{},
			{},
			{},
		},
		queryErr: context.DeadlineExceeded,
	}

	_, err = executeConversationMigration(context.Background(), client, "simulacrum-dev-main-table", false, 0)
	require.Error(t, err)
	require.ErrorContains(t, err, "query conversation partition")
}

func TestLoadConversationMigrationDataset_PaginatesAndBuildsGroups(t *testing.T) {
	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	paginationKey := map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION#conv-1"),
		"SK": sAttr("METADATA"),
	}
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					conversationMetadataItem("conv-1", []string{"Arch", "Medic"}, createdAt, updatedAt, 1, "status-1", updatedAt),
				},
				LastEvaluatedKey: paginationKey,
			},
			{},
			{Items: []map[string]types.AttributeValue{
				conversationParticipantRow("Arch", "conv-1", updatedAt, true, buildConversationSnapshot("conv-1", []string{"Arch", "Medic"}, createdAt, updatedAt, "status-1", 1, updatedAt, true)),
			}},
			{Items: []map[string]types.AttributeValue{
				conversationStatusRow("conv-1", "Medic", false, updatedAt),
			}},
			{Items: []map[string]types.AttributeValue{
				{
					"PK":             sAttr("CONVERSATION_PARTICIPANTS#Arch,Medic"),
					"SK":             sAttr("LOOKUP"),
					"gsi1PK":         sAttr("CONVERSATION_PARTICIPANTS#Arch,Medic"),
					"ConversationID": sAttr("conv-1"),
				},
			}},
		},
	}

	dataset, err := loadConversationMigrationDataset(context.Background(), client, "simulacrum-dev-main-table")
	require.NoError(t, err)
	require.Equal(t, 1, dataset.ScannedConversations)
	require.Len(t, client.scanInputs, 5)
	require.Equal(t, paginationKey, client.scanInputs[1].ExclusiveStartKey)

	group := dataset.GroupsByKey["arch,medic"]
	require.NotNil(t, group)
	require.Len(t, group.Conversations, 1)
	require.Len(t, group.ParticipantRows, 1)
	require.Len(t, group.StatusRows, 1)
	require.Len(t, group.LookupRows, 1)
	require.Equal(t, "conv-1", group.LookupRows[0].ConversationID)
	require.Equal(t, "Arch,Medic", group.LookupRows[0].ParticipantKey)
}

func TestBuildConversationMigrationDataset_ErrorsOnUnknownConversationRows(t *testing.T) {
	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)

	_, err := buildConversationMigrationDataset(conversationMigrationScanData{
		ParticipantItems: []map[string]types.AttributeValue{
			conversationParticipantRow("Arch", "missing", createdAt, false, buildConversationSnapshot("missing", []string{"Arch"}, createdAt, createdAt, "", 0, time.Time{}, false)),
		},
	})
	require.ErrorContains(t, err, `references unknown conversation "missing"`)

	_, err = buildConversationMigrationDataset(conversationMigrationScanData{
		StatusItems: []map[string]types.AttributeValue{
			conversationStatusRow("missing", "Arch", true, createdAt),
		},
	})
	require.ErrorContains(t, err, `references unknown conversation "missing"`)
}

func TestConversationMigrationHelpers_BranchCoverage(t *testing.T) {
	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	later := createdAt.Add(10 * time.Minute)

	recordA := conversationMetadataRecord{
		ConversationID:  "a",
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		LastMessageTime: later,
	}
	recordB := conversationMetadataRecord{
		ConversationID:  "b",
		CreatedAt:       updatedAt,
		UpdatedAt:       later,
		LastMessageTime: later,
	}
	require.True(t, conversationMetadataLess(recordA, recordB))
	require.False(t, conversationMetadataLess(recordB, recordA))

	participantTime := updatedAt
	participantA := conversationParticipantRecordItem{
		ConversationID: "legacy-conv",
		SortTime:       participantTime,
		Item: map[string]types.AttributeValue{
			"PK": sAttr("USER_CONVERSATIONS#arch"),
			"SK": sAttr(conversationParticipantSortKey(participantTime, "legacy-conv")),
		},
	}
	participantB := conversationParticipantRecordItem{
		ConversationID: "canon-conv",
		SortTime:       participantTime,
		Item: map[string]types.AttributeValue{
			"PK": sAttr("USER_CONVERSATIONS#arch"),
			"SK": sAttr(conversationParticipantSortKey(participantTime, "canon-conv")),
		},
	}
	require.True(t, participantRecordPreferred(participantB, participantA, "canon-conv"))
	require.False(t, participantRecordPreferred(participantA, participantB, "canon-conv"))

	statusTime := updatedAt
	statusA := conversationStatusRecord{
		ConversationID: "legacy-conv",
		LastReadAt:     statusTime,
		Unread:         false,
		Item: map[string]types.AttributeValue{
			"PK": sAttr("CONVERSATION_STATUS#legacy-conv"),
			"SK": sAttr("USER#arch"),
		},
	}
	statusB := conversationStatusRecord{
		ConversationID: "canon-conv",
		LastReadAt:     statusTime,
		Unread:         true,
		Item: map[string]types.AttributeValue{
			"PK": sAttr("CONVERSATION_STATUS#canon-conv"),
			"SK": sAttr("USER#arch"),
		},
	}
	require.True(t, statusRecordPreferred(statusB, statusA, "canon-conv"))
	require.False(t, statusRecordPreferred(statusA, statusB, "canon-conv"))
	require.True(t, anyStatusUnread([]conversationStatusRecord{statusA, statusB}))
	require.False(t, anyStatusUnread([]conversationStatusRecord{statusA}))

	require.Equal(t, "conv-gsi", conversationIDFromParticipantItem(map[string]types.AttributeValue{
		"gsi1PK": sAttr("CONVERSATION#conv-gsi"),
	}, "ignored"))
	require.Equal(t, "conv-upper", conversationIDFromParticipantItem(map[string]types.AttributeValue{
		conversationSnapshotAttribute: &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"ID": sAttr("conv-upper"),
		}},
	}, "ignored"))
	require.Equal(t, "conv-lower", conversationIDFromParticipantItem(map[string]types.AttributeValue{
		conversationSnapshotAttribute: &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id": sAttr("conv-lower"),
		}},
	}, "ignored"))
	require.Equal(t, "conv-sk", conversationIDFromParticipantItem(nil, conversationParticipantSortKey(updatedAt, "conv-sk")))
	require.Empty(t, conversationIDFromParticipantItem(nil, "invalid"))

	require.Equal(t, later, conversationMessageCreatedAt(map[string]types.AttributeValue{
		conversationCreatedAtAttribute: sAttr(later.UTC().Format(time.RFC3339Nano)),
	}))
	require.Equal(t, later, conversationMessageCreatedAt(map[string]types.AttributeValue{
		"SK": sAttr(conversationMessageRowSK("status-1", later)),
	}))
	require.True(t, conversationMessageCreatedAt(map[string]types.AttributeValue{
		"SK": sAttr("STATUS#invalid"),
	}).IsZero())

	parsedInt, ok := attributeInt64(&types.AttributeValueMemberN{Value: "42"})
	require.True(t, ok)
	require.EqualValues(t, 42, parsedInt)
	parsedInt, ok = attributeInt64(&types.AttributeValueMemberS{Value: "43"})
	require.True(t, ok)
	require.EqualValues(t, 43, parsedInt)
	_, ok = attributeInt64(&types.AttributeValueMemberS{Value: "nope"})
	require.False(t, ok)

	parsedTime, ok := parseFlexibleTime(later.UTC().Format(time.RFC3339Nano))
	require.True(t, ok)
	require.Equal(t, later, parsedTime)
	parsedTime, ok = parseFlexibleTime(updatedAt.UTC().Format(time.RFC3339))
	require.True(t, ok)
	require.Equal(t, updatedAt, parsedTime)
	parsedTime, ok = parseFlexibleTime("1711281600")
	require.True(t, ok)
	require.Equal(t, time.Unix(1711281600, 0).UTC(), parsedTime)
	parsedTime, ok = parseFlexibleTime("1711281600000")
	require.True(t, ok)
	require.Equal(t, time.UnixMilli(1711281600000).UTC(), parsedTime)
	parsedTime, ok = parseFlexibleTime("1711281600000000000")
	require.True(t, ok)
	require.Equal(t, time.Unix(0, 1711281600000000000).UTC(), parsedTime)
	_, ok = parseFlexibleTime(" ")
	require.False(t, ok)
}

func TestConversationMigrationParsersAndHelpers_HandleInvalidRows(t *testing.T) {
	_, ok, err := parseConversationMetadataRecord(map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION#conv-1"),
		"SK": sAttr("METADATA"),
		conversationParticipantsAttribute: &types.AttributeValueMemberS{
			Value: "not-a-list",
		},
	})
	require.ErrorContains(t, err, "missing participants")
	require.False(t, ok)

	_, ok, err = parseConversationParticipantRecordItem(map[string]types.AttributeValue{
		"PK": sAttr("USER_CONVERSATIONS#Arch"),
	})
	require.ErrorContains(t, err, "missing SK")
	require.False(t, ok)

	_, ok, err = parseConversationParticipantRecordItem(map[string]types.AttributeValue{
		"PK": sAttr("USER_CONVERSATIONS#"),
		"SK": sAttr("bad"),
	})
	require.ErrorContains(t, err, "missing participant ID")
	require.False(t, ok)

	_, ok, err = parseConversationParticipantRecordItem(map[string]types.AttributeValue{
		"PK": sAttr("USER_CONVERSATIONS#Arch"),
		"SK": sAttr("bad"),
	})
	require.ErrorContains(t, err, "missing conversation ID")
	require.False(t, ok)

	_, ok, err = parseConversationStatusRecord(map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION_STATUS#conv-1"),
		"SK": sAttr("bad"),
	})
	require.ErrorContains(t, err, "invalid SK")
	require.False(t, ok)

	_, ok, err = parseConversationStatusRecord(map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION_STATUS#"),
		"SK": sAttr("USER#"),
	})
	require.ErrorContains(t, err, "missing conversation or user ID")
	require.False(t, ok)

	_, ok, err = parseConversationLookupRecord(map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION_PARTICIPANTS#"),
		"SK": sAttr("LOOKUP"),
	})
	require.ErrorContains(t, err, "missing participant key")
	require.False(t, ok)

	_, ok, err = parseConversationLookupRecord(map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION_PARTICIPANTS#Arch,Medic"),
		"SK": sAttr("LOOKUP"),
	})
	require.ErrorContains(t, err, "missing ConversationID")
	require.False(t, ok)

	record, ok, err := parseConversationLookupRecord(map[string]types.AttributeValue{
		"PK":                                 sAttr("CONVERSATION_PARTICIPANTS#Arch,Medic"),
		"SK":                                 sAttr("LOOKUP"),
		conversationLookupConversationIDAttr: sAttr("conv-1"),
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "arch,medic", record.CanonicalParticipantKey)

	require.Nil(t, selectCanonicalParticipantRecord(nil, "canon-conv"))
	require.Nil(t, selectCanonicalStatusRecord(nil, "canon-conv"))

	noUnread, lastReadAt := mergeStatusBucket(nil)
	require.False(t, noUnread)
	require.True(t, lastReadAt.IsZero())

	noUnread, lastReadAt = mergeStatusBucket([]conversationStatusRecord{
		{Unread: false, LastReadAt: time.Unix(10, 0).UTC()},
		{Unread: false, LastReadAt: time.Unix(20, 0).UTC()},
	})
	require.False(t, noUnread)
	require.Equal(t, time.Unix(20, 0).UTC(), lastReadAt)

	require.True(t, conversationParticipantSortTime("bad").IsZero())
	require.Contains(t, conversationParticipantSortKey(time.Time{}, "conv-zero"), "#conv-zero")

	snapshotItem := map[string]types.AttributeValue{}
	require.NoError(t, setConversationSnapshotAttribute(snapshotItem, conversationCanonicalState{
		ID:                "conv-1",
		Participants:      []string{"arch", "medic"},
		LastStatusID:      "status-1",
		CreatedAt:         time.Unix(10, 0).UTC(),
		UpdatedAt:         time.Unix(20, 0).UTC(),
		TotalMessageCount: 2,
		LastMessageTime:   time.Unix(30, 0).UTC(),
	}, true))
	require.NotNil(t, snapshotItem[conversationSnapshotAttribute])

	values, ok := attributeStringSlice(&types.AttributeValueMemberSS{Value: []string{"arch", "medic"}})
	require.True(t, ok)
	require.Equal(t, []string{"arch", "medic"}, values)
	_, ok = attributeStringSlice(&types.AttributeValueMemberL{Value: []types.AttributeValue{
		sAttr("arch"),
		&types.AttributeValueMemberN{Value: "1"},
	}})
	require.False(t, ok)

	_, ok = firstAttributeString(map[string]types.AttributeValue{
		"empty": sAttr(" "),
	}, "empty", "missing")
	require.False(t, ok)

	_, ok = attributeBool(&types.AttributeValueMemberS{Value: "true"})
	require.False(t, ok)

	parsedTime, ok := attributeTime(&types.AttributeValueMemberN{Value: "1711281600"})
	require.True(t, ok)
	require.Equal(t, time.Unix(1711281600, 0).UTC(), parsedTime)
	_, ok = attributeTime(&types.AttributeValueMemberBOOL{Value: true})
	require.False(t, ok)

	plan := newConversationMigrationPlan(1)
	plan.putIfChanged("lookup", nil, map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION_PARTICIPANTS#arch,medic"),
		"SK": sAttr("LOOKUP"),
	})
	plan.deleteWithKey("lookup", map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION_PARTICIPANTS#arch,medic"),
		"SK": sAttr("LOOKUP"),
	})
	require.Empty(t, plan.Deletes)

	original := map[string]types.AttributeValue{
		"PK": sAttr("CONVERSATION#conv-1"),
		"SK": sAttr("METADATA"),
	}
	plan.putIfChanged("conversation", original, cloneAttributeMap(original))
	require.Len(t, plan.Puts, 1)
}

func conversationMetadataItem(
	conversationID string,
	participants []string,
	createdAt time.Time,
	updatedAt time.Time,
	totalMessageCount int64,
	lastStatusID string,
	lastMessageTime time.Time,
) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":                              sAttr("CONVERSATION#" + conversationID),
		"SK":                              sAttr("METADATA"),
		"id":                              sAttr(conversationID),
		conversationParticipantsAttribute: mustMarshalAttributeValue(participants),
		conversationCreatedAtAttribute:    sAttr(createdAt.UTC().Format(time.RFC3339Nano)),
		conversationUpdatedAtAttribute:    sAttr(updatedAt.UTC().Format(time.RFC3339Nano)),
		conversationTotalMessageCountAttribute: &types.AttributeValueMemberN{
			Value: strconv.FormatInt(totalMessageCount, 10),
		},
	}
	if lastStatusID != "" {
		item[conversationLastStatusIDAttribute] = sAttr(lastStatusID)
	}
	if !lastMessageTime.IsZero() {
		item[conversationLastMessageTimeAttribute] = sAttr(lastMessageTime.UTC().Format(time.RFC3339Nano))
	}
	return item
}

func conversationParticipantRow(
	participantID string,
	conversationID string,
	updatedAt time.Time,
	unread bool,
	snapshot *models.ConversationSnapshot,
) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":                          sAttr("USER_CONVERSATIONS#" + participantID),
		"SK":                          sAttr(conversationParticipantSortKey(updatedAt, conversationID)),
		"gsi1PK":                      sAttr("CONVERSATION#" + conversationID),
		"gsi1SK":                      sAttr("PARTICIPANT#" + participantID),
		conversationUnreadAttribute:   &types.AttributeValueMemberBOOL{Value: unread},
		conversationSnapshotAttribute: mustMarshalAttributeValue(snapshot),
	}
}

func conversationStatusRow(conversationID string, userID string, unread bool, lastReadAt time.Time) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":                            sAttr("CONVERSATION_STATUS#" + conversationID),
		"SK":                            sAttr("USER#" + userID),
		conversationIDAttribute:         sAttr(conversationID),
		conversationUserIDAttribute:     sAttr(userID),
		conversationUnreadAttribute:     &types.AttributeValueMemberBOOL{Value: unread},
		conversationLastReadAtAttribute: sAttr(lastReadAt.UTC().Format(time.RFC3339Nano)),
	}
}

func conversationLookupRow(participantKey string, conversationID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":                                 sAttr("CONVERSATION_PARTICIPANTS#" + participantKey),
		"SK":                                 sAttr("LOOKUP"),
		"gsi1PK":                             sAttr("CONVERSATION_PARTICIPANTS#" + participantKey),
		conversationLookupConversationIDAttr: sAttr(conversationID),
	}
}

func conversationMessageRow(conversationID string, statusID string, createdAt time.Time) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":                                 sAttr("CONVERSATION#" + conversationID),
		"SK":                                 sAttr(conversationMessageRowSK(statusID, createdAt)),
		conversationIDAttribute:              sAttr(conversationID),
		conversationMessageStatusIDAttribute: sAttr(statusID),
		conversationCreatedAtAttribute:       sAttr(createdAt.UTC().Format(time.RFC3339Nano)),
	}
}

func conversationMessageRowSK(statusID string, createdAt time.Time) string {
	return "STATUS#" + createdAt.UTC().Format(time.RFC3339Nano) + "#" + statusID
}

func buildConversationSnapshot(
	conversationID string,
	participants []string,
	createdAt time.Time,
	updatedAt time.Time,
	lastStatusID string,
	totalMessageCount int64,
	lastMessageTime time.Time,
	unread bool,
) *models.ConversationSnapshot {
	return &models.ConversationSnapshot{
		ID:                conversationID,
		Participants:      participants,
		LastStatusID:      lastStatusID,
		Unread:            unread,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		TotalMessageCount: totalMessageCount,
		LastMessageTime:   lastMessageTime,
	}
}

func mustMarshalAttributeValue(value any) types.AttributeValue {
	encoded, err := attributevalue.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func findPutItem(t *testing.T, puts []*dynamodb.PutItemInput, pk string, sk string) map[string]types.AttributeValue {
	t.Helper()
	for _, put := range puts {
		if strAttr(t, put.Item["PK"]) == pk && strAttr(t, put.Item["SK"]) == sk {
			return put.Item
		}
	}
	t.Fatalf("put %s %s not found", pk, sk)
	return nil
}

func deleteExists(deletes []*dynamodb.DeleteItemInput, pk string, sk string) bool {
	for _, del := range deletes {
		deletePK, ok := attributeString(del.Key["PK"])
		if !ok || deletePK != pk {
			continue
		}
		deleteSK, ok := attributeString(del.Key["SK"])
		if ok && deleteSK == sk {
			return true
		}
	}
	return false
}

func numAttr(t *testing.T, value types.AttributeValue) int64 {
	t.Helper()
	typed, ok := value.(*types.AttributeValueMemberN)
	require.True(t, ok)
	require.NotEmpty(t, typed.Value)
	parsed, err := attributeInt64(value)
	require.True(t, err)
	return parsed
}

func conversationSnapshotID(t *testing.T, value types.AttributeValue) string {
	t.Helper()
	typed, ok := value.(*types.AttributeValueMemberM)
	require.True(t, ok)
	return strAttr(t, typed.Value["ID"])
}
