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
	OldSK            string
	NewPK            string
	NewSK            string
	Item             map[string]types.AttributeValue
	AuditedGSIFields []string
}

type userKeyMigrationBuilder func(map[string]types.AttributeValue) (userKeyMigrationItem, bool, error)

type userKeyMigrationSpec struct {
	Prefix  string
	Builder userKeyMigrationBuilder
}

var newUserKeyMigrationClientFn = func(cfg aws.Config) userKeyMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

var userKeyMigrationSpecs = []userKeyMigrationSpec{
	{Prefix: "USER#", Builder: buildUserKeyMigrationItem},
	{Prefix: "SOUL_BODY_BINDING_USERNAME#", Builder: buildSoulBodyBindingUsernameMigrationItem},
	{Prefix: "ACTOR#", Builder: buildActorKeyMigrationItem},
	{Prefix: "MUTE#", Builder: buildMuteKeyMigrationItem},
	{Prefix: "follow#", Builder: buildFollowKeyMigrationItem},
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
	fs.IntVar(&limit, "limit", 0, "maximum number of legacy username-bearing partitions to process (0 = all)")
	fs.BoolVar(&apply, "apply", false, "write lowercase username-bearing items and delete legacy partitions")

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

	mode := migrationModeDryRun
	if apply {
		mode = migrationModeApply
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
		fmt.Println("no writes performed; re-run with --apply to copy lowercase username-bearing partitions and delete legacy keys")
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

	for _, spec := range userKeyMigrationSpecs {
		stop, err := executeUserKeyMigrationSpec(ctx, client, tableName, apply, limit, spec, &summary)
		if err != nil {
			return summary, err
		}
		if stop {
			return summary, nil
		}
	}

	return summary, nil
}

func executeUserKeyMigrationSpec(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	apply bool,
	limit int,
	spec userKeyMigrationSpec,
	summary *userKeyMigrationSummary,
) (bool, error) {
	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
		FilterExpression: aws.String(
			"begins_with(PK, :prefix)",
		),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: spec.Prefix},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return false, fmt.Errorf("scan legacy %s items: %w", spec.Prefix, err)
		}

		stop, err := processUserKeyMigrationPage(ctx, client, tableName, apply, limit, spec.Builder, out.Items, summary)
		if err != nil {
			return false, err
		}
		if stop {
			return true, nil
		}

		if len(out.LastEvaluatedKey) == 0 {
			return false, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func processUserKeyMigrationPage(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	apply bool,
	limit int,
	builder userKeyMigrationBuilder,
	items []map[string]types.AttributeValue,
	summary *userKeyMigrationSummary,
) (bool, error) {
	for _, item := range items {
		stop, err := processUserKeyMigrationItem(ctx, client, tableName, apply, limit, builder, item, summary)
		if err != nil {
			return false, err
		}
		if stop {
			return true, nil
		}
	}

	return false, nil
}

func processUserKeyMigrationItem(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	apply bool,
	limit int,
	builder userKeyMigrationBuilder,
	item map[string]types.AttributeValue,
	summary *userKeyMigrationSummary,
) (bool, error) {
	summary.Scanned++

	migrationItem, ok, err := builder(item)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	summary.LegacyPartitions++
	for _, field := range migrationItem.AuditedGSIFields {
		summary.AuditedGSIFields[field]++
	}

	if err := writeUserKeyMigrationItem(ctx, client, tableName, apply, migrationItem, summary); err != nil {
		return false, err
	}

	return limit > 0 && summary.LegacyPartitions >= limit, nil
}

func writeUserKeyMigrationItem(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	apply bool,
	migrationItem userKeyMigrationItem,
	summary *userKeyMigrationSummary,
) error {
	if !apply {
		summary.DryRunCandidates++
		return nil
	}

	if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      migrationItem.Item,
	}); err != nil {
		return fmt.Errorf("put migrated item %s %s: %w", migrationItem.NewPK, migrationItem.NewSK, err)
	}
	summary.Migrated++

	if _, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: migrationItem.OldPK},
			"SK": &types.AttributeValueMemberS{Value: migrationItem.OldSK},
		},
	}); err != nil {
		return fmt.Errorf("delete legacy item %s %s: %w", migrationItem.OldPK, migrationItem.OldSK, err)
	}
	summary.Deleted++

	return nil
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
		OldSK:            sk,
		NewPK:            "USER#" + newUsername,
		NewSK:            sk,
		Item:             newItem,
		AuditedGSIFields: auditedGSIFields,
	}, true, nil
}

func buildSoulBodyBindingUsernameMigrationItem(item map[string]types.AttributeValue) (userKeyMigrationItem, bool, error) {
	oldPK, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(oldPK, "SOUL_BODY_BINDING_USERNAME#") {
		return userKeyMigrationItem{}, false, nil
	}

	oldUsername := strings.TrimPrefix(oldPK, "SOUL_BODY_BINDING_USERNAME#")
	newUsername := strings.ToLower(strings.TrimSpace(oldUsername))
	if newUsername == "" || newUsername == oldUsername {
		return userKeyMigrationItem{}, false, nil
	}

	oldSK, ok := attributeString(item["SK"])
	if !ok || strings.TrimSpace(oldSK) == "" {
		return userKeyMigrationItem{}, false, fmt.Errorf("legacy soul body binding username item missing SK for PK %q", oldPK)
	}

	newItem := make(map[string]types.AttributeValue, len(item))
	for key, value := range item {
		newItem[key] = value
	}
	newItem["PK"] = &types.AttributeValueMemberS{Value: "SOUL_BODY_BINDING_USERNAME#" + newUsername}

	if usernameValue, ok := attributeString(item["username"]); ok && strings.EqualFold(strings.TrimSpace(usernameValue), oldUsername) {
		newItem["username"] = &types.AttributeValueMemberS{Value: newUsername}
	}

	return userKeyMigrationItem{
		OldPK: oldPK,
		OldSK: oldSK,
		NewPK: "SOUL_BODY_BINDING_USERNAME#" + newUsername,
		NewSK: oldSK,
		Item:  newItem,
	}, true, nil
}

func buildActorKeyMigrationItem(item map[string]types.AttributeValue) (userKeyMigrationItem, bool, error) {
	oldPK, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(oldPK, "ACTOR#") {
		return userKeyMigrationItem{}, false, nil
	}

	oldUsername, suffix, ok := splitActorKeyUsername(oldPK)
	if !ok {
		return userKeyMigrationItem{}, false, nil
	}
	newUsername := normalizedMigrationUsername(oldUsername)

	oldSK, ok := attributeString(item["SK"])
	if !ok || strings.TrimSpace(oldSK) == "" {
		return userKeyMigrationItem{}, false, fmt.Errorf("legacy actor item missing SK for PK %q", oldPK)
	}

	newPK := "ACTOR#" + newUsername + suffix
	newItem := cloneMigrationAttributes(item)
	audited := map[string]struct{}{}
	changed := setMigrationStringAttribute(newItem, "PK", newPK, audited)
	changed = normalizeActorUsernameAttributes(item, newItem, oldUsername, newUsername, audited) || changed
	changed = normalizeActorIndexAttributes(item, newItem, oldUsername, newUsername, audited) || changed

	newSK, skChanged := normalizeActorBlockedSK(newItem, oldSK, audited)
	changed = skChanged || changed
	changed = normalizeActorBlockedIndex(item, newItem, audited) || changed

	if !changed {
		return userKeyMigrationItem{}, false, nil
	}

	return userKeyMigrationItem{
		OldPK:            oldPK,
		OldSK:            oldSK,
		NewPK:            newPK,
		NewSK:            newSK,
		Item:             newItem,
		AuditedGSIFields: collectAuditedGSIFields(audited),
	}, true, nil
}

func normalizeActorUsernameAttributes(
	item map[string]types.AttributeValue,
	newItem map[string]types.AttributeValue,
	oldUsername string,
	newUsername string,
	audited map[string]struct{},
) bool {
	changed := false

	if usernameValue, ok := attributeString(item["username"]); ok && strings.EqualFold(strings.TrimSpace(usernameValue), oldUsername) {
		changed = setMigrationStringAttribute(newItem, "username", newUsername, audited) || changed
	}

	if value, ok := attributeString(item["gsi1SK"]); ok {
		updated := normalizedMigrationUsername(value)
		if updated != "" && updated != value {
			changed = setMigrationStringAttribute(newItem, "gsi1SK", updated, audited) || changed
		}
	}

	if value, ok := attributeString(item["gsi3SK"]); ok && strings.EqualFold(strings.TrimSpace(value), oldUsername) {
		changed = setMigrationStringAttribute(newItem, "gsi3SK", newUsername, audited) || changed
	}

	return changed
}

func normalizeActorIndexAttributes(
	item map[string]types.AttributeValue,
	newItem map[string]types.AttributeValue,
	oldUsername string,
	newUsername string,
	audited map[string]struct{},
) bool {
	changed := false

	if value, ok := attributeString(item["gsi1PK"]); ok {
		if updated, didUpdate := lowercasePrefixedUsername(value, "INBOX#"); didUpdate {
			changed = setMigrationStringAttribute(newItem, "gsi1PK", updated, audited) || changed
		}
	}

	for _, field := range []string{"gsi2SK", "gsi4SK", "gsi5SK"} {
		if updated, didUpdate := normalizedActorIndexField(item, field, oldUsername, newUsername); didUpdate {
			changed = setMigrationStringAttribute(newItem, field, updated, audited) || changed
		}
	}

	return changed
}

func normalizedActorIndexField(
	item map[string]types.AttributeValue,
	field string,
	oldUsername string,
	newUsername string,
) (string, bool) {
	value, ok := attributeString(item[field])
	if !ok {
		return "", false
	}

	updated := replaceDelimitedUsernameToken(value, oldUsername, newUsername)
	if strings.HasPrefix(value, "BLOCKER#") {
		if prefixed, didUpdate := lowercasePrefixedUsername(value, "BLOCKER#"); didUpdate {
			updated = prefixed
		}
	}

	return updated, updated != value
}

func normalizeActorBlockedSK(
	newItem map[string]types.AttributeValue,
	oldSK string,
	audited map[string]struct{},
) (string, bool) {
	updated, didUpdate := lowercasePrefixedUsername(oldSK, "BLOCKED#")
	if !didUpdate {
		return oldSK, false
	}

	return updated, setMigrationStringAttribute(newItem, "SK", updated, audited)
}

func normalizeActorBlockedIndex(
	item map[string]types.AttributeValue,
	newItem map[string]types.AttributeValue,
	audited map[string]struct{},
) bool {
	value, ok := attributeString(item["gsi5PK"])
	if !ok {
		return false
	}

	updated, didUpdate := lowercasePrefixedUsername(value, "BLOCKED#")
	if !didUpdate {
		return false
	}

	return setMigrationStringAttribute(newItem, "gsi5PK", updated, audited)
}

func buildMuteKeyMigrationItem(item map[string]types.AttributeValue) (userKeyMigrationItem, bool, error) {
	oldPK, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(oldPK, "MUTE#") {
		return userKeyMigrationItem{}, false, nil
	}

	oldMuter := strings.TrimPrefix(oldPK, "MUTE#")
	newMuter := normalizedMigrationUsername(oldMuter)
	oldSK, ok := attributeString(item["SK"])
	if !ok || strings.TrimSpace(oldSK) == "" {
		return userKeyMigrationItem{}, false, fmt.Errorf("legacy mute item missing SK for PK %q", oldPK)
	}

	newPK := "MUTE#" + newMuter
	newSK, changedSK := lowercasedPrefixedUsernameOrSame(oldSK, "MUTED#")
	newItem := cloneMigrationAttributes(item)
	audited := map[string]struct{}{}
	changed := setMigrationStringAttribute(newItem, "PK", newPK, audited)
	if changedSK {
		changed = setMigrationStringAttribute(newItem, "SK", newSK, audited) || changed
	}

	if value, ok := attributeString(item["gsi1PK"]); ok {
		if updated, didUpdate := lowercasePrefixedUsername(value, "MUTED#"); didUpdate {
			changed = setMigrationStringAttribute(newItem, "gsi1PK", updated, audited) || changed
		}
	}
	if value, ok := attributeString(item["gsi1SK"]); ok {
		if updated, didUpdate := lowercasePrefixedUsername(value, "MUTER#"); didUpdate {
			changed = setMigrationStringAttribute(newItem, "gsi1SK", updated, audited) || changed
		}
	}

	if !changed {
		return userKeyMigrationItem{}, false, nil
	}

	return userKeyMigrationItem{
		OldPK:            oldPK,
		OldSK:            oldSK,
		NewPK:            newPK,
		NewSK:            newSK,
		Item:             newItem,
		AuditedGSIFields: collectAuditedGSIFields(audited),
	}, true, nil
}

func buildFollowKeyMigrationItem(item map[string]types.AttributeValue) (userKeyMigrationItem, bool, error) {
	oldPK, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(oldPK, "follow#") {
		return userKeyMigrationItem{}, false, nil
	}

	oldFollower := strings.TrimPrefix(oldPK, "follow#")
	newFollower := normalizedMigrationUsername(oldFollower)
	oldSK, ok := attributeString(item["SK"])
	if !ok || strings.TrimSpace(oldSK) == "" {
		return userKeyMigrationItem{}, false, fmt.Errorf("legacy follow item missing SK for PK %q", oldPK)
	}

	newPK := "follow#" + newFollower
	oldFollowed, ok := prefixedUsernameValue(oldSK, "following#")
	if !ok {
		return userKeyMigrationItem{}, false, nil
	}
	newFollowed := normalizedMigrationUsername(oldFollowed)
	newSK := "following#" + newFollowed

	newItem := cloneMigrationAttributes(item)
	audited := map[string]struct{}{}
	changed := setMigrationStringAttribute(newItem, "PK", newPK, audited)
	changed = setMigrationStringAttribute(newItem, "SK", newSK, audited) || changed

	if value, ok := attributeString(item["gsi1PK"]); ok {
		updated, didUpdate := lowercasePrefixedUsername(value, "follow#")
		if didUpdate {
			changed = setMigrationStringAttribute(newItem, "gsi1PK", updated, audited) || changed
		}
	}
	if value, ok := attributeString(item["gsi1SK"]); ok {
		updated, didUpdate := lowercasePrefixedUsername(value, "follower#")
		if didUpdate {
			changed = setMigrationStringAttribute(newItem, "gsi1SK", updated, audited) || changed
		}
	}
	if value, ok := attributeString(item["gsi2SK"]); ok {
		updated := replaceDelimitedUsernameToken(value, oldFollower, newFollower)
		updated = replaceDelimitedUsernameToken(updated, oldFollowed, newFollowed)
		if updated != value {
			changed = setMigrationStringAttribute(newItem, "gsi2SK", updated, audited) || changed
		}
	}
	if value, ok := attributeString(item["followerUsername"]); ok && strings.EqualFold(strings.TrimSpace(value), oldFollower) {
		changed = setMigrationStringAttribute(newItem, "followerUsername", newFollower, audited) || changed
	}
	if value, ok := attributeString(item["followedUsername"]); ok && strings.EqualFold(strings.TrimSpace(value), oldFollowed) {
		changed = setMigrationStringAttribute(newItem, "followedUsername", newFollowed, audited) || changed
	}

	if !changed {
		return userKeyMigrationItem{}, false, nil
	}

	return userKeyMigrationItem{
		OldPK:            oldPK,
		OldSK:            oldSK,
		NewPK:            newPK,
		NewSK:            newSK,
		Item:             newItem,
		AuditedGSIFields: collectAuditedGSIFields(audited),
	}, true, nil
}

func cloneMigrationAttributes(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	cloned := make(map[string]types.AttributeValue, len(item))
	for key, value := range item {
		cloned[key] = value
	}
	return cloned
}

func setMigrationStringAttribute(
	item map[string]types.AttributeValue,
	field string,
	value string,
	audited map[string]struct{},
) bool {
	current, ok := attributeString(item[field])
	if !ok {
		item[field] = &types.AttributeValueMemberS{Value: value}
		if isGSIField(field) {
			audited[field] = struct{}{}
		}
		return true
	}
	if current == value {
		return false
	}
	item[field] = &types.AttributeValueMemberS{Value: value}
	if isGSIField(field) {
		audited[field] = struct{}{}
	}
	return true
}

func collectAuditedGSIFields(audited map[string]struct{}) []string {
	if len(audited) == 0 {
		return nil
	}
	fields := make([]string, 0, len(audited))
	for field := range audited {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func splitActorKeyUsername(pk string) (string, string, bool) {
	if !strings.HasPrefix(pk, "ACTOR#") {
		return "", "", false
	}
	rest := strings.TrimPrefix(pk, "ACTOR#")
	if rest == "" {
		return "", "", false
	}
	if idx := strings.Index(rest, "#"); idx != -1 {
		return rest[:idx], rest[idx:], true
	}
	return rest, "", true
}

func prefixedUsernameValue(value string, prefix string) (string, bool) {
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	return strings.TrimPrefix(value, prefix), true
}

func lowercasePrefixedUsername(value string, prefix string) (string, bool) {
	current, ok := prefixedUsernameValue(value, prefix)
	if !ok {
		return value, false
	}
	updated := prefix + normalizedMigrationUsername(current)
	return updated, updated != value
}

func lowercasedPrefixedUsernameOrSame(value string, prefix string) (string, bool) {
	updated, changed := lowercasePrefixedUsername(value, prefix)
	if !changed {
		return value, false
	}
	return updated, true
}

func normalizedMigrationUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func replaceDelimitedUsernameToken(value string, oldUsername string, newUsername string) string {
	if oldUsername == "" || newUsername == "" || oldUsername == newUsername {
		return value
	}
	updated := strings.ReplaceAll(value, "#"+oldUsername+"#", "#"+newUsername+"#")
	if strings.HasPrefix(updated, oldUsername+"#") {
		updated = newUsername + strings.TrimPrefix(updated, oldUsername)
	}
	return replaceUsernameSuffixToken(updated, oldUsername, newUsername)
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
