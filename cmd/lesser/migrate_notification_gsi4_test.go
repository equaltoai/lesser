package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

type fakeNotificationGSI4MigrationClient struct {
	scanOutputs []*dynamodb.ScanOutput
	scanErr     error
	updateErr   error
	scanCalls   int
	updates     []*dynamodb.UpdateItemInput
}

func (f *fakeNotificationGSI4MigrationClient) Scan(_ context.Context, input *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	f.scanCalls++
	if f.scanErr != nil {
		return nil, f.scanErr
	}
	if len(f.scanOutputs) == 0 {
		return &dynamodb.ScanOutput{}, nil
	}
	out := f.scanOutputs[0]
	f.scanOutputs = f.scanOutputs[1:]
	_ = input
	return out, nil
}

func (f *fakeNotificationGSI4MigrationClient) UpdateItem(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.updates = append(f.updates, input)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func TestExecuteNotificationGSI4Migration_DryRunClassifiesRows(t *testing.T) {
	client := &fakeNotificationGSI4MigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					notificationGSI4TestItem("alice", "n1", "", ""),
					notificationGSI4TestItem("alice", "n2", "NOTIF_ID#n2", "USER#alice"),
					notificationGSI4TestItem("alice", "n3", "WRONG", "USER#alice"),
					{
						notificationMigrationAttrPK: &types.AttributeValueMemberS{Value: "USER#alice"},
						notificationMigrationAttrSK: &types.AttributeValueMemberS{Value: "notif#bad"},
					},
				},
			},
		},
	}

	summary, err := executeNotificationGSI4Migration(context.Background(), client, "lesser-dev", false, 0)
	require.NoError(t, err)
	require.Equal(t, 4, summary.ScannedNotifications)
	require.Equal(t, 1, summary.AlreadyIndexed)
	require.Equal(t, 2, summary.PlannedUpdates)
	require.Equal(t, 0, summary.AppliedUpdates)
	require.Equal(t, 1, summary.ValidationErrors)
	require.Empty(t, client.updates)
	require.Len(t, summary.SampleNotifications, 2)
	require.Contains(t, summary.SampleNotifications[0], "n1")
	require.Contains(t, summary.SampleNotifications[1], "n3")
}

func TestExecuteNotificationGSI4Migration_ApplyUpdatesMissingKeys(t *testing.T) {
	client := &fakeNotificationGSI4MigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					{
						notificationMigrationAttrPK: &types.AttributeValueMemberS{Value: "USER#alice"},
						notificationMigrationAttrSK: &types.AttributeValueMemberS{Value: "notif#20260510120000#n1"},
					},
					notificationGSI4TestItem("alice", "n2", "", ""),
				},
			},
		},
	}

	summary, err := executeNotificationGSI4Migration(context.Background(), client, "lesser-dev", true, 1)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedNotifications)
	require.Equal(t, 1, summary.PlannedUpdates)
	require.Equal(t, 1, summary.AppliedUpdates)
	require.Len(t, client.updates, 1)

	update := client.updates[0]
	require.Equal(t, "lesser-dev", *update.TableName)
	require.Equal(t, "SET #gsi4pk = :gsi4pk, #gsi4sk = :gsi4sk", *update.UpdateExpression)
	require.Equal(t, "attribute_exists(#pk) AND attribute_exists(#sk)", *update.ConditionExpression)
	require.Equal(t, "USER#alice", stringAttrValue(t, update.Key[notificationMigrationAttrPK]))
	require.Equal(t, "notif#20260510120000#n1", stringAttrValue(t, update.Key[notificationMigrationAttrSK]))
	require.Equal(t, "NOTIF_ID#n1", stringAttrValue(t, update.ExpressionAttributeValues[":gsi4pk"]))
	require.Equal(t, "USER#alice", stringAttrValue(t, update.ExpressionAttributeValues[":gsi4sk"]))
}

func TestExecuteNotificationGSI4Migration_ErrorBranches(t *testing.T) {
	_, err := executeNotificationGSI4Migration(context.Background(), nil, "lesser-dev", false, 0)
	require.EqualError(t, err, "migration client is required")

	client := &fakeNotificationGSI4MigrationClient{}
	_, err = executeNotificationGSI4Migration(context.Background(), client, "", false, 0)
	require.EqualError(t, err, "table name is required")

	client = &fakeNotificationGSI4MigrationClient{scanErr: errors.New("scan failed")}
	_, err = executeNotificationGSI4Migration(context.Background(), client, "lesser-dev", false, 0)
	require.ErrorContains(t, err, "scan notification rows")

	client = &fakeNotificationGSI4MigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{{Items: []map[string]types.AttributeValue{notificationGSI4TestItem("alice", "n1", "", "")}}},
		updateErr:   errors.New("write failed"),
	}
	_, err = executeNotificationGSI4Migration(context.Background(), client, "lesser-dev", true, 0)
	require.ErrorContains(t, err, "update notification GSI4 keys")
}

func TestPrintNotificationGSI4MigrationSummary(t *testing.T) {
	output := captureStdout(t, func() {
		printNotificationGSI4MigrationSummary(notificationGSI4MigrationSummary{
			ScannedNotifications: 3,
			AlreadyIndexed:       1,
			PlannedUpdates:       2,
			AppliedUpdates:       0,
			ValidationErrors:     1,
			SampleNotifications:  []string{"USER#alice/notif#1#n1 (n1)"},
		}, "lesser-dev", "Theory", false)
	})

	require.Contains(t, output, "migrate-notification-gsi4 dry-run complete")
	require.Contains(t, output, "table: lesser-dev")
	require.Contains(t, output, "aws_profile: Theory")
	require.Contains(t, output, "planned_updates: 2")
	require.Contains(t, output, "sample_notifications:")
	require.Contains(t, output, "no writes performed")

	output = captureStdout(t, func() {
		printNotificationGSI4MigrationSummary(notificationGSI4MigrationSummary{AppliedUpdates: 2}, "lesser-dev", "", true)
	})
	require.Contains(t, output, "migrate-notification-gsi4 apply complete")
	require.NotContains(t, output, "no writes performed")
}

func TestBuildNotificationGSI4MigrationCandidate_InvalidRows(t *testing.T) {
	_, _, valid := buildNotificationGSI4MigrationCandidate(nil)
	require.False(t, valid)

	_, _, valid = buildNotificationGSI4MigrationCandidate(map[string]types.AttributeValue{
		notificationMigrationAttrPK: &types.AttributeValueMemberS{Value: "NOTE#1"},
		notificationMigrationAttrSK: &types.AttributeValueMemberS{Value: "notif#20260510120000#n1"},
	})
	require.False(t, valid)

	_, _, valid = buildNotificationGSI4MigrationCandidate(map[string]types.AttributeValue{
		notificationMigrationAttrPK: &types.AttributeValueMemberS{Value: "USER#alice"},
		notificationMigrationAttrSK: &types.AttributeValueMemberS{Value: "NOTE#1"},
	})
	require.False(t, valid)
}

func TestAppendNotificationGSI4MigrationSampleBoundsAndDeduplicates(t *testing.T) {
	candidate := notificationGSI4MigrationCandidate{PK: "USER#alice", SK: "notif#1#n1", ID: "n1"}
	appendNotificationGSI4MigrationSample(nil, candidate)

	var samples []string
	appendNotificationGSI4MigrationSample(&samples, candidate)
	appendNotificationGSI4MigrationSample(&samples, candidate)
	require.Equal(t, []string{"USER#alice/notif#1#n1 (n1)"}, samples)

	for i := 0; i < notificationMigrationSampleLimit+5; i++ {
		appendNotificationGSI4MigrationSample(&samples, notificationGSI4MigrationCandidate{
			PK: "USER#alice",
			SK: "notif#1#extra",
			ID: string(rune('a' + i)),
		})
	}
	require.Len(t, samples, notificationMigrationSampleLimit)
}

func TestNotificationIDFromSortKey(t *testing.T) {
	require.Equal(t, "n1", notificationIDFromSortKey("notif#20260510120000#n1"))
	require.Equal(t, "n#1", notificationIDFromSortKey("notif#20260510120000#n#1"))
	require.Empty(t, notificationIDFromSortKey("notif#bad"))
}

func notificationGSI4TestItem(userID, notificationID, gsi4PK, gsi4SK string) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		notificationMigrationAttrPK:     &types.AttributeValueMemberS{Value: "USER#" + userID},
		notificationMigrationAttrSK:     &types.AttributeValueMemberS{Value: "notif#20260510120000#" + notificationID},
		notificationMigrationAttrID:     &types.AttributeValueMemberS{Value: notificationID},
		notificationMigrationAttrUserID: &types.AttributeValueMemberS{Value: userID},
	}
	if gsi4PK != "" {
		item[notificationMigrationAttrGSI4PK] = &types.AttributeValueMemberS{Value: gsi4PK}
	}
	if gsi4SK != "" {
		item[notificationMigrationAttrGSI4SK] = &types.AttributeValueMemberS{Value: gsi4SK}
	}
	return item
}

func stringAttrValue(t *testing.T, value types.AttributeValue) string {
	t.Helper()
	typed, ok := value.(*types.AttributeValueMemberS)
	require.True(t, ok)
	return typed.Value
}
