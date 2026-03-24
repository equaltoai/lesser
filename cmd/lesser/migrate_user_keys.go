package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

type userKeyMigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type userKeyMigrationSummary struct {
	Scanned          int
	LegacyPartitions int
	Migrated         int
	Deleted          int
	DryRunCandidates int
	AuditedGSIFields map[string]int
}

type userKeyMigrationItem struct {
	OldPK            string
	NewPK            string
	SK               string
	Item             map[string]types.AttributeValue
	AuditedGSIFields []string
}

var newUserKeyMigrationClientFn = func(cfg aws.Config) userKeyMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateUserKeys(argv []string) error {
	fs := flag.NewFlagSet("lesser migrate-user-keys", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		app        string
		env        string
		awsProfile string
		tableName  string
		limit      int
		apply      bool
	)
	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.StringVar(&tableName, "table", "", "explicit DynamoDB table name override")
	fs.IntVar(&limit, "limit", 0, "maximum number of legacy USER# partitions to process (0 = all)")
	fs.BoolVar(&apply, "apply", false, "write lowercase USER# items and delete legacy partitions")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	resolvedTableName, err := resolveUserKeyMigrationTableName(app, env, tableName)
	if err != nil {
		return err
	}

	ctx := context.Background()
	awsCfg, resolvedProfile, err := loadAWSConfigForCLIFn(ctx, awsProfile)
	if err != nil {
		return err
	}

	summary, err := executeUserKeyMigration(ctx, newUserKeyMigrationClientFn(awsCfg), resolvedTableName, apply, limit)
	if err != nil {
		return err
	}

	mode := "dry-run"
	if apply {
		mode = "apply"
	}

	fmt.Printf("migrate-user-keys %s complete\n", mode)
	fmt.Printf("table: %s\n", resolvedTableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_items: %d\n", summary.Scanned)
	fmt.Printf("legacy_partitions: %d\n", summary.LegacyPartitions)
	fmt.Printf("migrated_items: %d\n", summary.Migrated)
	fmt.Printf("deleted_legacy_items: %d\n", summary.Deleted)
	fmt.Printf("dry_run_candidates: %d\n", summary.DryRunCandidates)

	if len(summary.AuditedGSIFields) > 0 {
		fmt.Println("audited_gsi_fields:")
		keys := make([]string, 0, len(summary.AuditedGSIFields))
		for key := range summary.AuditedGSIFields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Printf("  %s: %d\n", key, summary.AuditedGSIFields[key])
		}
	}

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to copy lowercase USER# partitions and delete legacy keys")
	}

	return nil
}

func resolveUserKeyMigrationTableName(app, env, explicitTableName string) (string, error) {
	if strings.TrimSpace(explicitTableName) != "" {
		return strings.TrimSpace(explicitTableName), nil
	}

	appName := strings.TrimSpace(app)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		return "", err
	}

	stage := naming.StageForEnvironment(env)
	switch stage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return "", fmt.Errorf("invalid env %q (expected dev|staging|live)", env)
	}

	return naming.ResourceNameWithApp(normalizedApp, "main-table", string(stage)), nil
}

func executeUserKeyMigration(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (userKeyMigrationSummary, error) {
	summary := userKeyMigrationSummary{
		AuditedGSIFields: map[string]int{},
	}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
		FilterExpression: aws.String(
			"begins_with(PK, :user_prefix)",
		),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":user_prefix": &types.AttributeValueMemberS{Value: "USER#"},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return summary, fmt.Errorf("scan legacy USER# items: %w", err)
		}

		for _, item := range out.Items {
			summary.Scanned++

			migrationItem, ok, err := buildUserKeyMigrationItem(item)
			if err != nil {
				return summary, err
			}
			if !ok {
				continue
			}

			summary.LegacyPartitions++
			for _, field := range migrationItem.AuditedGSIFields {
				summary.AuditedGSIFields[field]++
			}

			if apply {
				if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
					TableName: aws.String(tableName),
					Item:      migrationItem.Item,
				}); err != nil {
					return summary, fmt.Errorf("put migrated item %s %s: %w", migrationItem.NewPK, migrationItem.SK, err)
				}
				summary.Migrated++

				if _, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
					TableName: aws.String(tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: migrationItem.OldPK},
						"SK": &types.AttributeValueMemberS{Value: migrationItem.SK},
					},
				}); err != nil {
					return summary, fmt.Errorf("delete legacy item %s %s: %w", migrationItem.OldPK, migrationItem.SK, err)
				}
				summary.Deleted++
			} else {
				summary.DryRunCandidates++
			}

			if limit > 0 && summary.LegacyPartitions >= limit {
				return summary, nil
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}

	return summary, nil
}

func buildUserKeyMigrationItem(item map[string]types.AttributeValue) (userKeyMigrationItem, bool, error) {
	oldPK, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(oldPK, "USER#") {
		return userKeyMigrationItem{}, false, nil
	}

	oldUsername := strings.TrimPrefix(oldPK, "USER#")
	newUsername := strings.ToLower(strings.TrimSpace(oldUsername))
	if newUsername == "" || newUsername == oldUsername {
		return userKeyMigrationItem{}, false, nil
	}

	sk, ok := attributeString(item["SK"])
	if !ok || strings.TrimSpace(sk) == "" {
		return userKeyMigrationItem{}, false, fmt.Errorf("legacy user partition item missing SK for PK %q", oldPK)
	}

	newItem, auditedGSIFields := normalizeLegacyUserPartitionItem(item, oldUsername, newUsername)

	return userKeyMigrationItem{
		OldPK:            oldPK,
		NewPK:            "USER#" + newUsername,
		SK:               sk,
		Item:             newItem,
		AuditedGSIFields: auditedGSIFields,
	}, true, nil
}

func normalizeLegacyUserPartitionItem(
	item map[string]types.AttributeValue,
	oldUsername string,
	newUsername string,
) (map[string]types.AttributeValue, []string) {
	newItem := make(map[string]types.AttributeValue, len(item))
	audited := map[string]struct{}{}

	for key, value := range item {
		updated, changed := normalizeLegacyUserPartitionAttribute(key, value, oldUsername, newUsername)
		newItem[key] = updated
		if changed && isGSIField(key) {
			audited[key] = struct{}{}
		}
	}

	newItem["PK"] = &types.AttributeValueMemberS{Value: "USER#" + newUsername}

	auditedFields := make([]string, 0, len(audited))
	for key := range audited {
		auditedFields = append(auditedFields, key)
	}
	sort.Strings(auditedFields)

	return newItem, auditedFields
}

func normalizeLegacyUserPartitionAttribute(
	field string,
	value types.AttributeValue,
	oldUsername string,
	newUsername string,
) (types.AttributeValue, bool) {
	switch typed := value.(type) {
	case *types.AttributeValueMemberS:
		updated, changed := normalizeLegacyUserPartitionStringField(field, typed.Value, oldUsername, newUsername)
		if !changed {
			return value, false
		}
		return &types.AttributeValueMemberS{Value: updated}, true
	case *types.AttributeValueMemberM:
		changed := false
		updatedMap := make(map[string]types.AttributeValue, len(typed.Value))
		for key, nested := range typed.Value {
			updatedValue, valueChanged := normalizeLegacyUserPartitionAttribute(key, nested, oldUsername, newUsername)
			updatedMap[key] = updatedValue
			changed = changed || valueChanged
		}
		if !changed {
			return value, false
		}
		return &types.AttributeValueMemberM{Value: updatedMap}, true
	case *types.AttributeValueMemberL:
		changed := false
		updatedList := make([]types.AttributeValue, len(typed.Value))
		for idx, nested := range typed.Value {
			updatedValue, valueChanged := normalizeLegacyUserPartitionAttribute(field, nested, oldUsername, newUsername)
			updatedList[idx] = updatedValue
			changed = changed || valueChanged
		}
		if !changed {
			return value, false
		}
		return &types.AttributeValueMemberL{Value: updatedList}, true
	default:
		return value, false
	}
}

func normalizeLegacyUserPartitionStringField(field string, value string, oldUsername string, newUsername string) (string, bool) {
	oldPK := "USER#" + oldUsername
	newPK := "USER#" + newUsername

	normalizedField := strings.ToLower(strings.TrimSpace(field))
	updated := value
	changed := false

	switch normalizedField {
	case "pk":
		return newPK, value != newPK
	case "username", "userid", "user_id", "preferredusername", "preferred_username":
		if strings.EqualFold(strings.TrimSpace(updated), oldUsername) {
			updated = newUsername
			changed = true
		}
	case "userpk", "user_pk":
		if updated == oldPK {
			updated = newPK
			changed = true
		}
	}

	if isKeyField(normalizedField) {
		replaced := strings.ReplaceAll(updated, oldPK, newPK)
		if replaced != updated {
			updated = replaced
			changed = true
		}

		replaced = replaceUsernameSuffixToken(updated, oldUsername, newUsername)
		if replaced != updated {
			updated = replaced
			changed = true
		}

		if normalizedField == "gsi5pk" {
			target := buildUserHandlePrefixKey(newUsername)
			if updated != target {
				updated = target
				changed = true
			}
		}
		if normalizedField == "gsi5sk" && updated != newUsername {
			updated = newUsername
			changed = true
		}
	}

	return updated, changed
}

func attributeString(value types.AttributeValue) (string, bool) {
	typed, ok := value.(*types.AttributeValueMemberS)
	if !ok {
		return "", false
	}
	return typed.Value, true
}

func buildUserHandlePrefixKey(username string) string {
	normalized := strings.ToLower(strings.TrimSpace(username))
	if len(normalized) > 2 {
		normalized = normalized[:2]
	}
	return fmt.Sprintf("USER_HANDLE_PREFIX#%s", normalized)
}

func isGSIField(field string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(field)), "gsi")
}

func isKeyField(field string) bool {
	normalized := strings.ToLower(strings.TrimSpace(field))
	return isGSIField(normalized) || strings.HasSuffix(normalized, "pk") || strings.HasSuffix(normalized, "sk")
}

func replaceUsernameSuffixToken(value string, oldUsername string, newUsername string) string {
	if value == oldUsername {
		return newUsername
	}

	suffix := "#" + oldUsername
	if strings.HasSuffix(value, suffix) {
		return strings.TrimSuffix(value, suffix) + "#" + newUsername
	}

	return value
}
