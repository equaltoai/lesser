package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

type fakeUserKeyMigrationClient struct {
	scanOutputs  []*dynamodb.ScanOutput
	putInputs    []*dynamodb.PutItemInput
	deleteInputs []*dynamodb.DeleteItemInput
}

func (f *fakeUserKeyMigrationClient) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if len(f.scanOutputs) == 0 {
		return &dynamodb.ScanOutput{}, nil
	}
	out := f.scanOutputs[0]
	f.scanOutputs = f.scanOutputs[1:]
	return out, nil
}

func (f *fakeUserKeyMigrationClient) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.putInputs = append(f.putInputs, input)
	return &dynamodb.PutItemOutput{}, nil
}

func (f *fakeUserKeyMigrationClient) DeleteItem(_ context.Context, input *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.deleteInputs = append(f.deleteInputs, input)
	return &dynamodb.DeleteItemOutput{}, nil
}

func TestBuildUserKeyMigrationItem_NormalizesLegacyMetadataAndGSIs(t *testing.T) {
	item, ok, err := buildUserKeyMigrationItem(map[string]types.AttributeValue{
		"PK":       sAttr("USER#Medic"),
		"SK":       sAttr("METADATA"),
		"username": sAttr("Medic"),
		"gsi1PK":   sAttr("USERS"),
		"gsi1SK":   sAttr("2026-03-24T11:28:58Z#Medic"),
		"gsi3PK":   sAttr("ROLE#user"),
		"gsi3SK":   sAttr("Medic"),
		"gsi4PK":   sAttr("STATUS#active"),
		"gsi4SK":   sAttr("Medic"),
		"gsi5PK":   sAttr("USER_HANDLE_PREFIX#me"),
		"gsi5SK":   sAttr("medic"),
		"gsi6PK":   sAttr("ACCOUNT_TYPE#AGENT"),
		"gsi6SK":   sAttr("2026-03-24T11:28:58Z#Medic"),
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "USER#Medic", item.OldPK)
	require.Equal(t, "USER#medic", item.NewPK)
	require.Equal(t, "METADATA", item.SK)
	require.Equal(t, "USER#medic", strAttr(t, item.Item["PK"]))
	require.Equal(t, "medic", strAttr(t, item.Item["username"]))
	require.Equal(t, "2026-03-24T11:28:58Z#medic", strAttr(t, item.Item["gsi1SK"]))
	require.Equal(t, "medic", strAttr(t, item.Item["gsi3SK"]))
	require.Equal(t, "medic", strAttr(t, item.Item["gsi4SK"]))
	require.Equal(t, "USER_HANDLE_PREFIX#me", strAttr(t, item.Item["gsi5PK"]))
	require.Equal(t, "medic", strAttr(t, item.Item["gsi5SK"]))
	require.Equal(t, "2026-03-24T11:28:58Z#medic", strAttr(t, item.Item["gsi6SK"]))
	require.Equal(t, []string{"gsi1SK", "gsi3SK", "gsi4SK", "gsi6SK"}, item.AuditedGSIFields)
}

func TestExecuteUserKeyMigration_DryRunCountsCandidatesWithoutWrites(t *testing.T) {
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					{
						"PK":       sAttr("USER#Medic"),
						"SK":       sAttr("METADATA"),
						"username": sAttr("Medic"),
						"gsi3SK":   sAttr("Medic"),
					},
					{
						"PK":       sAttr("USER#arch"),
						"SK":       sAttr("METADATA"),
						"username": sAttr("arch"),
					},
				},
			},
		},
	}

	summary, err := executeUserKeyMigration(context.Background(), client, "lesser-dev-main-table", false, 0)
	require.NoError(t, err)
	require.Equal(t, 2, summary.Scanned)
	require.Equal(t, 1, summary.LegacyPartitions)
	require.Equal(t, 0, summary.Migrated)
	require.Equal(t, 0, summary.Deleted)
	require.Equal(t, 1, summary.DryRunCandidates)
	require.Equal(t, 1, summary.AuditedGSIFields["gsi3SK"])
	require.Empty(t, client.putInputs)
	require.Empty(t, client.deleteInputs)
}

func TestExecuteUserKeyMigration_ApplyWritesLowercaseItemAndDeletesLegacyKey(t *testing.T) {
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					{
						"PK":      sAttr("USER#Arch"),
						"SK":      sAttr("notif#1"),
						"userID":  sAttr("Arch"),
						"message": sAttr("hello"),
						"gsi1PK":  sAttr("USER#Arch"),
						"gsi1SK":  sAttr("notif#Arch"),
					},
				},
			},
		},
	}

	summary, err := executeUserKeyMigration(context.Background(), client, "lesser-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.LegacyPartitions)
	require.Equal(t, 1, summary.Migrated)
	require.Equal(t, 1, summary.Deleted)
	require.Len(t, client.putInputs, 1)
	require.Len(t, client.deleteInputs, 1)
	require.Equal(t, "USER#arch", strAttr(t, client.putInputs[0].Item["PK"]))
	require.Equal(t, "arch", strAttr(t, client.putInputs[0].Item["userID"]))
	require.Equal(t, "USER#arch", strAttr(t, client.putInputs[0].Item["gsi1PK"]))
	require.Equal(t, "notif#arch", strAttr(t, client.putInputs[0].Item["gsi1SK"]))
	require.Equal(t, "USER#Arch", strAttr(t, client.deleteInputs[0].Key["PK"]))
	require.Equal(t, "notif#1", strAttr(t, client.deleteInputs[0].Key["SK"]))
}

func sAttr(value string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: value}
}

func strAttr(t *testing.T, value types.AttributeValue) string {
	t.Helper()
	typed, ok := value.(*types.AttributeValueMemberS)
	require.True(t, ok)
	return typed.Value
}
