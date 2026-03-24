package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

type fakeUserKeyMigrationClient struct {
	scanOutputs  []*dynamodb.ScanOutput
	scanInputs   []*dynamodb.ScanInput
	putInputs    []*dynamodb.PutItemInput
	deleteInputs []*dynamodb.DeleteItemInput
	scanErr      error
	putErr       error
	deleteErr    error
}

func (f *fakeUserKeyMigrationClient) Scan(_ context.Context, input *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	f.scanInputs = append(f.scanInputs, input)
	if f.scanErr != nil {
		return nil, f.scanErr
	}
	if len(f.scanOutputs) == 0 {
		return &dynamodb.ScanOutput{}, nil
	}
	out := f.scanOutputs[0]
	f.scanOutputs = f.scanOutputs[1:]
	return out, nil
}

func (f *fakeUserKeyMigrationClient) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.putInputs = append(f.putInputs, input)
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (f *fakeUserKeyMigrationClient) DeleteItem(_ context.Context, input *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.deleteInputs = append(f.deleteInputs, input)
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	previousStdout := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer

	fn()

	require.NoError(t, writer.Close())
	os.Stdout = previousStdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	return buf.String()
}

func TestRunMigrateUserKeys_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newUserKeyMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newUserKeyMigrationClientFn = previousClientFactory
	})

	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{
			Items: []map[string]types.AttributeValue{
				{
					"PK":       sAttr("USER#Medic"),
					"SK":       sAttr("METADATA"),
					"username": sAttr("Medic"),
					"gsi3SK":   sAttr("Medic"),
				},
			},
		}},
	}

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{}, "Sim", nil
	}
	newUserKeyMigrationClientFn = func(aws.Config) userKeyMigrationClient { return client }

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateUserKeys([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Equal(t, "Sim", seenProfile)
	require.Contains(t, output, "migrate-user-keys dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "legacy_partitions: 1")
	require.Contains(t, output, "dry_run_candidates: 1")
	require.Contains(t, output, "audited_gsi_fields:")
	require.Contains(t, output, "gsi3SK: 1")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 2)
	require.Equal(t, "USER#", strAttr(t, client.scanInputs[0].ExpressionAttributeValues[":prefix"]))
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#", strAttr(t, client.scanInputs[1].ExpressionAttributeValues[":prefix"]))
}

func TestRunMigrateUserKeys_ApplyUsesExplicitTable(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newUserKeyMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newUserKeyMigrationClientFn = previousClientFactory
	})

	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		return aws.Config{}, "", nil
	}
	newUserKeyMigrationClientFn = func(aws.Config) userKeyMigrationClient {
		return &fakeUserKeyMigrationClient{}
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateUserKeys([]string{
			"--table", "custom-main-table",
			"--apply",
		}))
	})

	require.Contains(t, output, "migrate-user-keys apply complete")
	require.Contains(t, output, "table: custom-main-table")
	require.NotContains(t, output, "aws_profile:")
	require.NotContains(t, output, "no writes performed")
}

func TestResolveUserKeyMigrationTableName(t *testing.T) {
	resolved, err := resolveUserKeyMigrationTableName("", "staging", "")
	require.NoError(t, err)
	require.Equal(
		t,
		naming.ResourceNameWithApp(naming.DefaultAppName, "main-table", string(naming.StageStaging)),
		resolved,
	)

	explicit, err := resolveUserKeyMigrationTableName("ignored", "live", "  explicit-table  ")
	require.NoError(t, err)
	require.Equal(t, "explicit-table", explicit)

	_, err = resolveUserKeyMigrationTableName("lesser", "qa", "")
	require.Error(t, err)
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
	require.Equal(t, "METADATA", item.NewSK)
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

func TestExecuteUserKeyMigration_ApplyWritesLowercaseSoulBodyBindingUsernameItem(t *testing.T) {
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{},
			{
				Items: []map[string]types.AttributeValue{
					{
						"PK":       sAttr("SOUL_BODY_BINDING_USERNAME#Medic"),
						"SK":       sAttr("SOUL_BODY_BINDING"),
						"username": sAttr("Medic"),
						"agentId":  sAttr("0xmedic"),
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
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#medic", strAttr(t, client.putInputs[0].Item["PK"]))
	require.Equal(t, "medic", strAttr(t, client.putInputs[0].Item["username"]))
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#Medic", strAttr(t, client.deleteInputs[0].Key["PK"]))
	require.Equal(t, "SOUL_BODY_BINDING", strAttr(t, client.deleteInputs[0].Key["SK"]))
}

func TestExecuteUserKeyMigration_ValidatesInputsAndScanErrors(t *testing.T) {
	_, err := executeUserKeyMigration(context.Background(), nil, "lesser-dev-main-table", false, 0)
	require.Error(t, err)

	_, err = executeUserKeyMigration(context.Background(), &fakeUserKeyMigrationClient{}, "", false, 0)
	require.Error(t, err)

	client := &fakeUserKeyMigrationClient{scanErr: errors.New("scan failed")}
	_, err = executeUserKeyMigration(context.Background(), client, "lesser-dev-main-table", false, 0)
	require.Error(t, err)
	require.ErrorContains(t, err, "scan legacy USER# items")
}

func TestExecuteUserKeyMigration_UsesPaginationCursorAndLimit(t *testing.T) {
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					{
						"PK": sAttr("USER#arch"),
						"SK": sAttr("METADATA"),
					},
				},
				LastEvaluatedKey: map[string]types.AttributeValue{"PK": sAttr("cursor")},
			},
			{
				Items: []map[string]types.AttributeValue{
					{
						"PK":       sAttr("USER#Medic"),
						"SK":       sAttr("METADATA"),
						"username": sAttr("Medic"),
					},
				},
			},
		},
	}

	summary, err := executeUserKeyMigration(context.Background(), client, "lesser-dev-main-table", false, 1)
	require.NoError(t, err)
	require.Equal(t, 2, summary.Scanned)
	require.Equal(t, 1, summary.LegacyPartitions)
	require.Len(t, client.scanInputs, 2)
	require.Equal(t, "cursor", strAttr(t, client.scanInputs[1].ExclusiveStartKey["PK"]))
}

func TestWriteUserKeyMigrationItem_PropagatesWriteErrors(t *testing.T) {
	item := userKeyMigrationItem{
		OldPK: "USER#Arch",
		OldSK: "METADATA",
		NewPK: "USER#arch",
		NewSK: "METADATA",
		Item: map[string]types.AttributeValue{
			"PK": sAttr("USER#arch"),
			"SK": sAttr("METADATA"),
		},
	}

	putSummary := &userKeyMigrationSummary{}
	putClient := &fakeUserKeyMigrationClient{putErr: errors.New("put failed")}
	err := writeUserKeyMigrationItem(context.Background(), putClient, "lesser-dev-main-table", true, item, putSummary)
	require.Error(t, err)
	require.ErrorContains(t, err, "put migrated item")
	require.Zero(t, putSummary.Migrated)

	deleteSummary := &userKeyMigrationSummary{}
	deleteClient := &fakeUserKeyMigrationClient{deleteErr: errors.New("delete failed")}
	err = writeUserKeyMigrationItem(context.Background(), deleteClient, "lesser-dev-main-table", true, item, deleteSummary)
	require.Error(t, err)
	require.ErrorContains(t, err, "delete legacy item")
	require.Equal(t, 1, deleteSummary.Migrated)
	require.Zero(t, deleteSummary.Deleted)
}

func TestBuildUserKeyMigrationItem_SkipsLowercaseAndErrorsOnMissingSK(t *testing.T) {
	_, ok, err := buildUserKeyMigrationItem(map[string]types.AttributeValue{
		"PK": sAttr("NOTE#123"),
		"SK": sAttr("METADATA"),
	})
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = buildUserKeyMigrationItem(map[string]types.AttributeValue{
		"PK": sAttr("USER#arch"),
		"SK": sAttr("METADATA"),
	})
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = buildUserKeyMigrationItem(map[string]types.AttributeValue{
		"PK": sAttr("USER#Arch"),
	})
	require.Error(t, err)
	require.False(t, ok)
	require.ErrorContains(t, err, "missing SK")
}

func TestBuildSoulBodyBindingUsernameMigrationItem_NormalizesLegacyKey(t *testing.T) {
	item, ok, err := buildSoulBodyBindingUsernameMigrationItem(map[string]types.AttributeValue{
		"PK":       sAttr("SOUL_BODY_BINDING_USERNAME#Medic"),
		"SK":       sAttr("SOUL_BODY_BINDING"),
		"username": sAttr("Medic"),
		"agentId":  sAttr("0xmedic"),
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#Medic", item.OldPK)
	require.Equal(t, "SOUL_BODY_BINDING", item.OldSK)
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#medic", item.NewPK)
	require.Equal(t, "SOUL_BODY_BINDING", item.NewSK)
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#medic", strAttr(t, item.Item["PK"]))
	require.Equal(t, "medic", strAttr(t, item.Item["username"]))
}

func TestBuildSoulBodyBindingUsernameMigrationItem_SkipsLowercaseAndMissingSK(t *testing.T) {
	_, ok, err := buildSoulBodyBindingUsernameMigrationItem(map[string]types.AttributeValue{
		"PK": sAttr("SOUL_BODY_BINDING_USERNAME#medic"),
		"SK": sAttr("SOUL_BODY_BINDING"),
	})
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = buildSoulBodyBindingUsernameMigrationItem(map[string]types.AttributeValue{
		"PK": sAttr("SOUL_BODY_BINDING_USERNAME#Medic"),
	})
	require.Error(t, err)
	require.False(t, ok)
	require.ErrorContains(t, err, "missing SK")
}

func TestNormalizeLegacyUserPartitionAttribute_HandlesNestedMapsAndLists(t *testing.T) {
	root := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"preferred_username": sAttr("Arch"),
		"gsi1SK": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			sAttr("USER#Arch"),
			&types.AttributeValueMemberBOOL{Value: true},
		}},
	}}

	updated, changed := normalizeLegacyUserPartitionAttribute("metadata", root, "Arch", "arch")
	require.True(t, changed)

	updatedMap := updated.(*types.AttributeValueMemberM).Value
	require.Equal(t, "arch", strAttr(t, updatedMap["preferred_username"]))

	updatedList := updatedMap["gsi1SK"].(*types.AttributeValueMemberL).Value
	require.Equal(t, "USER#arch", strAttr(t, updatedList[0]))
	_, ok := updatedList[1].(*types.AttributeValueMemberBOOL)
	require.True(t, ok)

	unchanged, changed := normalizeLegacyUserPartitionAttribute("count", &types.AttributeValueMemberN{Value: "3"}, "Arch", "arch")
	require.False(t, changed)
	_, ok = unchanged.(*types.AttributeValueMemberN)
	require.True(t, ok)
}

func TestNormalizeLegacyUserPartitionStringField_SpecialCases(t *testing.T) {
	updated, changed := normalizeLegacyUserPartitionStringField("PK", "USER#Arch", "Arch", "arch")
	require.True(t, changed)
	require.Equal(t, "USER#arch", updated)

	updated, changed = normalizeLegacyUserPartitionStringField("preferred_username", "  ARCH  ", "Arch", "arch")
	require.True(t, changed)
	require.Equal(t, "arch", updated)

	updated, changed = normalizeLegacyUserPartitionStringField("user_pk", "USER#Arch", "Arch", "arch")
	require.True(t, changed)
	require.Equal(t, "USER#arch", updated)

	updated, changed = normalizeLegacyUserPartitionStringField("gsi5PK", "ignored", "Arch", "arch")
	require.True(t, changed)
	require.Equal(t, "USER_HANDLE_PREFIX#ar", updated)

	updated, changed = normalizeLegacyUserPartitionStringField("gsi5SK", "ARCH", "Arch", "arch")
	require.True(t, changed)
	require.Equal(t, "arch", updated)

	updated, changed = normalizeLegacyUserPartitionStringField("messageSK", "notif#Arch", "Arch", "arch")
	require.True(t, changed)
	require.Equal(t, "notif#arch", updated)

	updated, changed = normalizeLegacyUserPartitionStringField("message", "hello", "Arch", "arch")
	require.False(t, changed)
	require.Equal(t, "hello", updated)
}

func TestAttributeString_ReturnsFalseForNonStrings(t *testing.T) {
	value, ok := attributeString(&types.AttributeValueMemberN{Value: "1"})
	require.False(t, ok)
	require.Empty(t, value)
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
