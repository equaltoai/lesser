package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	notificationMigrationUserPrefix       = "USER#"
	notificationMigrationSortKeyPrefix    = "notif#"
	notificationMigrationGSI4IDPrefix     = "NOTIF_ID#"
	notificationMigrationGSI4UserPrefix   = "USER#"
	notificationMigrationSampleLimit      = 10
	notificationMigrationAttrPK           = "PK"
	notificationMigrationAttrSK           = "SK"
	notificationMigrationAttrID           = "id"
	notificationMigrationAttrUserID       = "userID"
	notificationMigrationAttrGSI4PK       = "gsi4PK"
	notificationMigrationAttrGSI4SK       = "gsi4SK"
	notificationMigrationProjectionFields = "#pk,#sk,#id,#userID,#gsi4pk,#gsi4sk"
)

type notificationGSI4MigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

type notificationGSI4MigrationSummary struct {
	ScannedNotifications int
	AlreadyIndexed       int
	PlannedUpdates       int
	AppliedUpdates       int
	ValidationErrors     int
	SampleNotifications  []string
}

type notificationGSI4MigrationCandidate struct {
	PK             string
	SK             string
	ID             string
	UserID         string
	ExpectedGSI4PK string
	ExpectedGSI4SK string
}

var newNotificationGSI4MigrationClientFn = func(cfg aws.Config) notificationGSI4MigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateNotificationGSI4(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-notification-gsi4",
		"maximum number of legacy notification rows to update (0 = all)",
		"write GSI4 notification ID lookup keys on existing notification rows",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (notificationGSI4MigrationSummary, error) {
			return executeNotificationGSI4Migration(
				ctx,
				newNotificationGSI4MigrationClientFn(awsCfg),
				tableName,
				apply,
				limit,
			)
		},
		printNotificationGSI4MigrationSummary,
	)
}

func printNotificationGSI4MigrationSummary(
	summary notificationGSI4MigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-notification-gsi4 %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_notifications: %d\n", summary.ScannedNotifications)
	fmt.Printf("already_indexed: %d\n", summary.AlreadyIndexed)
	fmt.Printf("planned_updates: %d\n", summary.PlannedUpdates)
	fmt.Printf("applied_updates: %d\n", summary.AppliedUpdates)
	fmt.Printf("validation_errors: %d\n", summary.ValidationErrors)
	printNotificationGSI4MigrationSamples(summary.SampleNotifications)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to backfill notification GSI4 lookup keys")
	}
}

func printNotificationGSI4MigrationSamples(samples []string) {
	if len(samples) == 0 {
		return
	}
	fmt.Println("sample_notifications:")
	for _, sample := range samples {
		fmt.Printf("  %s\n", sample)
	}
}

func executeNotificationGSI4Migration(
	ctx context.Context,
	client notificationGSI4MigrationClient,
	tableName string,
	apply bool,
	limit int,
) (notificationGSI4MigrationSummary, error) {
	summary := notificationGSI4MigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	scanInput := &dynamodb.ScanInput{
		TableName:            aws.String(tableName),
		FilterExpression:     aws.String("begins_with(#pk, :userPrefix) AND begins_with(#sk, :notifPrefix)"),
		ProjectionExpression: aws.String(notificationMigrationProjectionFields),
		ExpressionAttributeNames: map[string]string{
			"#pk":     notificationMigrationAttrPK,
			"#sk":     notificationMigrationAttrSK,
			"#id":     notificationMigrationAttrID,
			"#userID": notificationMigrationAttrUserID,
			"#gsi4pk": notificationMigrationAttrGSI4PK,
			"#gsi4sk": notificationMigrationAttrGSI4SK,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":userPrefix":  &types.AttributeValueMemberS{Value: notificationMigrationUserPrefix},
			":notifPrefix": &types.AttributeValueMemberS{Value: notificationMigrationSortKeyPrefix},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return summary, fmt.Errorf("scan notification rows: %w", err)
		}

		for _, item := range out.Items {
			summary.ScannedNotifications++

			candidate, needsUpdate, valid := buildNotificationGSI4MigrationCandidate(item)
			if !valid {
				summary.ValidationErrors++
				continue
			}
			if !needsUpdate {
				summary.AlreadyIndexed++
				continue
			}

			summary.PlannedUpdates++
			appendNotificationGSI4MigrationSample(&summary.SampleNotifications, candidate)

			if apply {
				if err := updateNotificationGSI4Keys(ctx, client, tableName, candidate); err != nil {
					return summary, fmt.Errorf("update notification GSI4 keys for %s/%s: %w", candidate.PK, candidate.SK, err)
				}
				summary.AppliedUpdates++
			}

			if limit > 0 && summary.PlannedUpdates >= limit {
				return summary, nil
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return summary, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildNotificationGSI4MigrationCandidate(item map[string]types.AttributeValue) (notificationGSI4MigrationCandidate, bool, bool) {
	pk, ok := attributeString(item[notificationMigrationAttrPK])
	if !ok || !strings.HasPrefix(pk, notificationMigrationUserPrefix) {
		return notificationGSI4MigrationCandidate{}, false, false
	}
	sk, ok := attributeString(item[notificationMigrationAttrSK])
	if !ok || !strings.HasPrefix(sk, notificationMigrationSortKeyPrefix) {
		return notificationGSI4MigrationCandidate{}, false, false
	}

	userID, _ := firstAttributeString(item, notificationMigrationAttrUserID, "UserID")
	if strings.TrimSpace(userID) == "" {
		userID = strings.TrimSpace(strings.TrimPrefix(pk, notificationMigrationUserPrefix))
	}

	notificationID, _ := firstAttributeString(item, notificationMigrationAttrID, "ID")
	if strings.TrimSpace(notificationID) == "" {
		notificationID = notificationIDFromSortKey(sk)
	}

	userID = strings.TrimSpace(userID)
	notificationID = strings.TrimSpace(notificationID)
	if userID == "" || notificationID == "" {
		return notificationGSI4MigrationCandidate{}, false, false
	}

	candidate := notificationGSI4MigrationCandidate{
		PK:             pk,
		SK:             sk,
		ID:             notificationID,
		UserID:         userID,
		ExpectedGSI4PK: notificationMigrationGSI4IDPrefix + notificationID,
		ExpectedGSI4SK: notificationMigrationGSI4UserPrefix + userID,
	}

	currentGSI4PK, _ := attributeString(item[notificationMigrationAttrGSI4PK])
	currentGSI4SK, _ := attributeString(item[notificationMigrationAttrGSI4SK])
	needsUpdate := currentGSI4PK != candidate.ExpectedGSI4PK || currentGSI4SK != candidate.ExpectedGSI4SK
	return candidate, needsUpdate, true
}

func notificationIDFromSortKey(sk string) string {
	parts := strings.SplitN(sk, "#", 3)
	if len(parts) != 3 {
		return ""
	}
	return strings.TrimSpace(parts[2])
}

func updateNotificationGSI4Keys(
	ctx context.Context,
	client notificationGSI4MigrationClient,
	tableName string,
	candidate notificationGSI4MigrationCandidate,
) error {
	_, err := client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			notificationMigrationAttrPK: &types.AttributeValueMemberS{Value: candidate.PK},
			notificationMigrationAttrSK: &types.AttributeValueMemberS{Value: candidate.SK},
		},
		UpdateExpression:    aws.String("SET #gsi4pk = :gsi4pk, #gsi4sk = :gsi4sk"),
		ConditionExpression: aws.String("attribute_exists(#pk) AND attribute_exists(#sk)"),
		ExpressionAttributeNames: map[string]string{
			"#pk":     notificationMigrationAttrPK,
			"#sk":     notificationMigrationAttrSK,
			"#gsi4pk": notificationMigrationAttrGSI4PK,
			"#gsi4sk": notificationMigrationAttrGSI4SK,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gsi4pk": &types.AttributeValueMemberS{Value: candidate.ExpectedGSI4PK},
			":gsi4sk": &types.AttributeValueMemberS{Value: candidate.ExpectedGSI4SK},
		},
	})
	return err
}

func appendNotificationGSI4MigrationSample(samples *[]string, candidate notificationGSI4MigrationCandidate) {
	if samples == nil || len(*samples) >= notificationMigrationSampleLimit {
		return
	}
	sample := fmt.Sprintf("%s/%s (%s)", candidate.PK, candidate.SK, candidate.ID)
	for _, existing := range *samples {
		if existing == sample {
			return
		}
	}
	*samples = append(*samples, sample)
}
