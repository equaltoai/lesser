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

func TestRunMigrateConversationParticipantSnapshots_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newConversationParticipantSnapshotMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newConversationParticipantSnapshotMigrationClientFn = previousClientFactory
	})

	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)
	lastMessageTime := createdAt.Add(30 * time.Second)
	corruptedParticipant := conversationParticipantRow(
		"arch",
		"conv-1",
		updatedAt,
		true,
		&legacyConversationSnapshot{},
	)

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{corruptedParticipant},
		}},
		getItemMap: map[string]map[string]types.AttributeValue{
			"CONVERSATION#conv-1\x00METADATA": conversationMetadataItem(
				"conv-1",
				[]string{"arch", "scout"},
				createdAt,
				updatedAt,
				3,
				"status-1",
				lastMessageTime,
			),
		},
	}

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{}, "Sim", nil
	}
	newConversationParticipantSnapshotMigrationClientFn = func(aws.Config) conversationParticipantSnapshotMigrationClient {
		return client
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateConversationParticipantSnapshots([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Equal(t, "Sim", seenProfile)
	require.Contains(t, output, "migrate-conversation-participant-snapshots dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "scanned_participant_rows: 1")
	require.Contains(t, output, "corrupted_participant_rows: 1")
	require.Contains(t, output, "reparable_rows: 1")
	require.Contains(t, output, "missing_canonical_rows: 0")
	require.Contains(t, output, "repaired_rows: 0")
	require.Contains(t, output, "sample_conversation_ids:")
	require.Contains(t, output, "  conv-1")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 1)
	require.Len(t, client.getItemInputs, 1)
}

func TestExecuteConversationParticipantSnapshotMigration_ApplyRepairsCorruptedRows(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)
	lastMessageTime := createdAt.Add(30 * time.Second)

	corruptedParticipant := conversationParticipantRow(
		"arch",
		"conv-1",
		updatedAt,
		true,
		&legacyConversationSnapshot{},
	)
	corruptedParticipant["requestState"] = sAttr(string(models.DmRequestStatePending))

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{corruptedParticipant},
		}},
		getItemMap: map[string]map[string]types.AttributeValue{
			"CONVERSATION#conv-1\x00METADATA": conversationMetadataItem(
				"conv-1",
				[]string{"arch", "scout"},
				createdAt,
				updatedAt,
				3,
				"status-1",
				lastMessageTime,
			),
		},
	}

	summary, err := executeConversationParticipantSnapshotMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedParticipantRows)
	require.Equal(t, 1, summary.CorruptedParticipantRows)
	require.Equal(t, 1, summary.ReparableRows)
	require.Equal(t, 0, summary.MissingCanonicalRows)
	require.Equal(t, 1, summary.RepairedRows)
	require.Equal(t, []string{"conv-1"}, summary.SampleConversationIDs)

	require.Len(t, client.putInputs, 1)
	repaired := client.putInputs[0].Item
	require.Equal(t, "PENDING", strAttr(t, repaired["requestState"]))
	unread, ok := attributeBool(repaired[conversationUnreadAttribute])
	require.True(t, ok)
	require.True(t, unread)

	snapshotValue, ok := repaired[conversationSnapshotAttribute].(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Equal(t, "conv-1", strAttr(t, snapshotValue.Value["ID"]))
	participants, ok := attributeStringSlice(snapshotValue.Value["Participants"])
	require.True(t, ok)
	require.Equal(t, []string{"arch", "scout"}, participants)
	require.Equal(t, "status-1", strAttr(t, snapshotValue.Value["LastStatusID"]))
	require.EqualValues(t, 3, numAttr(t, snapshotValue.Value["TotalMessageCount"]))
	require.Equal(t, lastMessageTime.UTC().Format(time.RFC3339Nano), strAttr(t, snapshotValue.Value["LastMessageTime"]))
	snapshotUnread, ok := attributeBool(snapshotValue.Value["Unread"])
	require.True(t, ok)
	require.True(t, snapshotUnread)
}

func TestExecuteConversationParticipantSnapshotMigration_MissingCanonicalAndLimit(t *testing.T) {
	corruptedParticipant := conversationParticipantRow(
		"arch",
		"conv-1",
		time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC),
		false,
		&legacyConversationSnapshot{},
	)

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				corruptedParticipant,
				corruptedParticipant,
			},
		}},
	}

	summary, err := executeConversationParticipantSnapshotMigration(context.Background(), client, "simulacrum-dev-main-table", false, 1)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedParticipantRows)
	require.Equal(t, 1, summary.CorruptedParticipantRows)
	require.Equal(t, 0, summary.ReparableRows)
	require.Equal(t, 1, summary.MissingCanonicalRows)
	require.Equal(t, 0, summary.RepairedRows)
	require.Equal(t, []string{"conv-1"}, summary.SampleConversationIDs)
	require.Len(t, client.getItemInputs, 1)
	require.Empty(t, client.putInputs)
}

func TestParticipantSnapshotRepairConversationID_FallsBackFromGSIToSortKey(t *testing.T) {
	item := map[string]types.AttributeValue{
		"gsi1PK": sAttr("CONVERSATION#"),
		"SK":     sAttr("2026-03-25T10:39:00Z#conv-1"),
	}
	require.Equal(t, "conv-1", participantSnapshotRepairConversationID(item))
	require.Empty(t, participantSnapshotRepairConversationID(map[string]types.AttributeValue{
		"SK": sAttr("missing-delimiter"),
	}))
}

func TestRebuildConversationParticipantSnapshot_ValidatesCanonicalConversation(t *testing.T) {
	item := conversationParticipantRow(
		"arch",
		"conv-1",
		time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC),
		true,
		&legacyConversationSnapshot{},
	)

	_, err := rebuildConversationParticipantSnapshot(item, map[string]types.AttributeValue{
		conversationParticipantsAttribute: mustMarshalAttributeValue([]string{"arch"}),
	})
	require.EqualError(t, err, "canonical conversation ID missing")

	_, err = rebuildConversationParticipantSnapshot(item, map[string]types.AttributeValue{
		"id": sAttr("conv-1"),
	})
	require.EqualError(t, err, "canonical participants missing")
}

func TestPrintConversationParticipantSnapshotMigrationSummary_ApplyOmitsDryRunNote(t *testing.T) {
	output := captureStdout(t, func() {
		printConversationParticipantSnapshotMigrationSummary(conversationParticipantSnapshotMigrationSummary{
			ScannedParticipantRows:   1,
			CorruptedParticipantRows: 1,
			ReparableRows:            1,
			MissingCanonicalRows:     0,
			RepairedRows:             1,
			SampleConversationIDs:    []string{"conv-1"},
		}, "custom-main-table", "", true)
	})

	require.Contains(t, output, "migrate-conversation-participant-snapshots apply complete")
	require.Contains(t, output, "table: custom-main-table")
	require.Contains(t, output, "sample_conversation_ids:")
	require.NotContains(t, output, "aws_profile:")
	require.NotContains(t, output, "no writes performed")
}

func TestExecuteConversationParticipantSnapshotMigration_ValidatesInputsAndScanErrors(t *testing.T) {
	_, err := executeConversationParticipantSnapshotMigration(context.Background(), nil, "simulacrum-dev-main-table", false, 0)
	require.EqualError(t, err, "migration client is required")

	_, err = executeConversationParticipantSnapshotMigration(context.Background(), &fakeUserKeyMigrationClient{}, "   ", false, 0)
	require.EqualError(t, err, "table name is required")

	_, err = executeConversationParticipantSnapshotMigration(context.Background(), &fakeUserKeyMigrationClient{
		scanErr: context.DeadlineExceeded,
	}, "simulacrum-dev-main-table", false, 0)
	require.ErrorContains(t, err, "scan participant rows")
}

func TestBuildConversationParticipantSnapshotRepairCandidate_HandlesBadRowsAndCanonicalErrors(t *testing.T) {
	candidate, ok, err := buildConversationParticipantSnapshotRepairCandidate(context.Background(), &fakeUserKeyMigrationClient{}, "simulacrum-dev-main-table", map[string]types.AttributeValue{
		"PK": sAttr("USER#arch"),
	})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, conversationParticipantSnapshotRepairCandidate{}, candidate)

	corruptedParticipant := conversationParticipantRow(
		"arch",
		"conv-1",
		time.Date(2026, 3, 25, 10, 39, 0, 0, time.UTC),
		false,
		&legacyConversationSnapshot{},
	)

	_, _, err = buildConversationParticipantSnapshotRepairCandidate(context.Background(), &fakeUserKeyMigrationClient{
		getItemErr: context.DeadlineExceeded,
	}, "simulacrum-dev-main-table", corruptedParticipant)
	require.ErrorContains(t, err, "load canonical conversation")

	candidate, ok, err = buildConversationParticipantSnapshotRepairCandidate(context.Background(), &fakeUserKeyMigrationClient{}, "simulacrum-dev-main-table", corruptedParticipant)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, candidate.MissingCanonical)

	candidate, ok, err = buildConversationParticipantSnapshotRepairCandidate(context.Background(), &fakeUserKeyMigrationClient{
		getItemMap: map[string]map[string]types.AttributeValue{
			"CONVERSATION#conv-1\x00METADATA": {"id": sAttr("conv-1")},
		},
	}, "simulacrum-dev-main-table", corruptedParticipant)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, candidate.MissingCanonical)
	require.Equal(t, "conv-1", candidate.ConversationID)
}
