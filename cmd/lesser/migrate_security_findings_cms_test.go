package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestExecuteSecurityFindingsCMSPublicationMemberRepair_DryRunPlansOnlyBrokenRows(t *testing.T) {
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{{Items: []map[string]types.AttributeValue{
				publicationMemberItem("PUBLICATION#pub1#MEMBER", "USER#alice", "", "", "", ""),
				publicationMemberItem("PUBLICATION#pub2#MEMBER", "USER#bob", "pub2", "bob", "USER#bob#PUBLICATION", "PUBLICATION#pub2"),
				publicationMemberItem("PUBLICATION#bad#extra#MEMBER", "USER#carol", "", "", "", ""),
			}}},
		},
	}

	summary, err := executeSecurityFindingsCMSPublicationMemberRepair(context.Background(), client, "theory-dev-main-table", false, 0)
	require.NoError(t, err)
	require.Equal(t, "cms-publication-members", summary.Name)
	require.Equal(t, 3, summary.Scanned)
	require.Equal(t, 1, summary.Candidates)
	require.Equal(t, 1, summary.PlannedWrites)
	require.Equal(t, 0, summary.AppliedWrites)
	require.Equal(t, 1, summary.Skipped)
	require.Empty(t, client.updateInputs)
	require.Contains(t, summary.Samples[0], "repair PUBLICATION#pub1#MEMBER USER#alice")
	require.Contains(t, summary.Samples[1], "unexpected key")
}

func TestExecuteSecurityFindingsCMSPublicationMemberRepair_ApplyUpdatesGSIKeys(t *testing.T) {
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{{Items: []map[string]types.AttributeValue{
				publicationMemberItem("PUBLICATION#pub1#MEMBER", "USER#alice", "", "", "", ""),
			}}},
		},
	}

	summary, err := executeSecurityFindingsCMSPublicationMemberRepair(context.Background(), client, "theory-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Candidates)
	require.Equal(t, 1, summary.AppliedWrites)
	require.Len(t, client.updateInputs, 1)
	update := client.updateInputs[0]
	require.Equal(t, "PUBLICATION#pub1#MEMBER", strAttr(t, update.Key["PK"]))
	require.Equal(t, "USER#alice", strAttr(t, update.Key["SK"]))
	require.Contains(t, *update.UpdateExpression, "gsi1PK")
	require.Equal(t, "USER#alice#PUBLICATION", strAttr(t, update.ExpressionAttributeValues[":gsi1pk"]))
	require.Equal(t, "PUBLICATION#pub1", strAttr(t, update.ExpressionAttributeValues[":gsi1sk"]))
	require.Equal(t, "pub1", strAttr(t, update.ExpressionAttributeValues[":publicationID"]))
	require.Equal(t, "alice", strAttr(t, update.ExpressionAttributeValues[":userID"]))
}

func TestParseCMSPublicationMemberKey(t *testing.T) {
	pub, user, ok := parseCMSPublicationMemberKey("PUBLICATION#news#MEMBER", "USER#alice")
	require.True(t, ok)
	require.Equal(t, "news", pub)
	require.Equal(t, "alice", user)

	_, _, ok = parseCMSPublicationMemberKey("PUBLICATION#news", "USER#alice")
	require.False(t, ok)
	_, _, ok = parseCMSPublicationMemberKey("PUBLICATION#news#extra#MEMBER", "USER#alice")
	require.False(t, ok)
	_, _, ok = parseCMSPublicationMemberKey("PUBLICATION#news#MEMBER", "TEAM#alice")
	require.False(t, ok)
}

func publicationMemberItem(pk, sk, publicationID, userID, gsi1PK, gsi1SK string) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK": sAttr(pk),
		"SK": sAttr(sk),
	}
	if publicationID != "" {
		item["publicationID"] = sAttr(publicationID)
	}
	if userID != "" {
		item["userID"] = sAttr(userID)
	}
	if gsi1PK != "" {
		item["gsi1PK"] = sAttr(gsi1PK)
	}
	if gsi1SK != "" {
		item["gsi1SK"] = sAttr(gsi1SK)
	}
	return item
}
