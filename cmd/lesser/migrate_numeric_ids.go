package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
)

const (
	actorPartitionPrefix    = "ACTOR#"
	numericMappingPartition = "NUMERIC_ID#"
	numericMetadataSortKey  = "METADATA"
	actorProfileSortKey     = "PROFILE"
	numericMappingTypeName  = "NumericIDMapping"
)

type numericIDMigrationSummary struct {
	ScannedActors         int
	Candidates            int
	ActorRowsUpdated      int
	MappingsUpserted      int
	LegacyMappingsDeleted int
}

type numericIDMigrationCandidate struct {
	ActorPK         string
	ActorSK         string
	ActorItem       map[string]types.AttributeValue
	PutActor        bool
	MappingItem     map[string]types.AttributeValue
	PutMapping      bool
	LegacyMappingPK string
	DeleteLegacy    bool
}

func runMigrateNumericIDs(argv []string) error {
	fs := flag.NewFlagSet("lesser migrate-numeric-ids", flag.ContinueOnError)
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
	fs.IntVar(&limit, "limit", 0, "maximum number of legacy actor rows to process (0 = all)")
	fs.BoolVar(&apply, "apply", false, "rewrite actor numeric IDs and upsert lowercase NUMERIC_ID mappings")

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

	summary, err := executeNumericIDMigration(ctx, newUserKeyMigrationClientFn(awsCfg), resolvedTableName, apply, limit)
	if err != nil {
		return err
	}

	mode := migrationModeDryRun
	if apply {
		mode = migrationModeApply
	}

	fmt.Printf("migrate-numeric-ids %s complete\n", mode)
	fmt.Printf("table: %s\n", resolvedTableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_actors: %d\n", summary.ScannedActors)
	fmt.Printf("candidates: %d\n", summary.Candidates)
	fmt.Printf("actor_rows_updated: %d\n", summary.ActorRowsUpdated)
	fmt.Printf("mappings_upserted: %d\n", summary.MappingsUpserted)
	fmt.Printf("legacy_mappings_deleted: %d\n", summary.LegacyMappingsDeleted)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to rewrite actor numeric IDs and conformant NUMERIC_ID mappings")
	}

	return nil
}

func executeNumericIDMigration(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (numericIDMigrationSummary, error) {
	summary := numericIDMigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	actors, err := scanMigrationItems(ctx, client, tableName, actorPartitionPrefix, actorProfileSortKey)
	if err != nil {
		return summary, fmt.Errorf("scan actor profiles: %w", err)
	}
	mappings, err := scanMigrationItems(ctx, client, tableName, numericMappingPartition, numericMetadataSortKey)
	if err != nil {
		return summary, fmt.Errorf("scan numeric ID mappings: %w", err)
	}

	mappingsByPK := make(map[string]map[string]types.AttributeValue, len(mappings))
	for _, item := range mappings {
		if pk, ok := attributeString(item["PK"]); ok {
			mappingsByPK[pk] = item
		}
	}

	for _, actorItem := range actors {
		summary.ScannedActors++

		candidate, ok, err := buildNumericIDMigrationCandidate(actorItem, mappingsByPK)
		if err != nil {
			return summary, err
		}
		if !ok {
			continue
		}

		summary.Candidates++
		if apply {
			if err := applyNumericIDMigrationCandidate(ctx, client, tableName, candidate, &summary); err != nil {
				return summary, err
			}
		}

		if limit > 0 && summary.Candidates >= limit {
			break
		}
	}

	return summary, nil
}

func scanMigrationItems(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	pkPrefix string,
	sk string,
) ([]map[string]types.AttributeValue, error) {
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: pkPrefix},
			":sk":     &types.AttributeValueMemberS{Value: sk},
		},
	}

	var items []map[string]types.AttributeValue
	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildNumericIDMigrationCandidate(
	actorItem map[string]types.AttributeValue,
	mappingsByPK map[string]map[string]types.AttributeValue,
) (numericIDMigrationCandidate, bool, error) {
	actorPK, ok := attributeString(actorItem["PK"])
	if !ok || !strings.HasPrefix(actorPK, actorPartitionPrefix) {
		return numericIDMigrationCandidate{}, false, nil
	}
	actorSK, ok := attributeString(actorItem["SK"])
	if !ok || actorSK != actorProfileSortKey {
		return numericIDMigrationCandidate{}, false, nil
	}

	canonicalUsername := canonicalMigrationUsername(actorItem, actorPK)
	if canonicalUsername == "" {
		return numericIDMigrationCandidate{}, false, fmt.Errorf("actor profile %q is missing username", actorPK)
	}

	desiredNumericID := common.GenerateNumericID(canonicalUsername)
	currentNumericID, _ := attributeString(actorItem["numericID"])
	updatedActorItem := cloneAttributeMap(actorItem)
	putActor := false

	if currentNumericID != desiredNumericID {
		updatedActorItem["numericID"] = &types.AttributeValueMemberS{Value: desiredNumericID}
		putActor = true
	}
	if usernameValue, ok := attributeString(updatedActorItem["username"]); !ok || usernameValue != canonicalUsername {
		updatedActorItem["username"] = &types.AttributeValueMemberS{Value: canonicalUsername}
		putActor = true
	}
	if normalizeActorPayloadIdentity(updatedActorItem, canonicalUsername) {
		putActor = true
	}

	legacyMappingPK := ""
	if strings.TrimSpace(currentNumericID) != "" && currentNumericID != desiredNumericID {
		legacyMappingPK = numericMappingPartition + currentNumericID
	}

	desiredMappingPK := numericMappingPartition + desiredNumericID
	existingDesiredMapping := mappingsByPK[desiredMappingPK]
	legacyMapping := mappingsByPK[legacyMappingPK]
	desiredActorID := desiredActorIDForMapping(updatedActorItem, existingDesiredMapping, legacyMapping, canonicalUsername)
	desiredCreatedAt := desiredCreatedAtAttribute(existingDesiredMapping, legacyMapping)
	desiredMappingItem := buildNumericIDMappingItem(desiredNumericID, canonicalUsername, desiredActorID, desiredCreatedAt)
	putMapping := !mappingItemsEquivalent(existingDesiredMapping, desiredMappingItem)
	deleteLegacy := legacyMappingPK != "" && legacyMapping != nil

	if !putActor && !putMapping && !deleteLegacy {
		return numericIDMigrationCandidate{}, false, nil
	}

	return numericIDMigrationCandidate{
		ActorPK:         actorPK,
		ActorSK:         actorSK,
		ActorItem:       updatedActorItem,
		PutActor:        putActor,
		MappingItem:     desiredMappingItem,
		PutMapping:      putMapping,
		LegacyMappingPK: legacyMappingPK,
		DeleteLegacy:    deleteLegacy,
	}, true, nil
}

func applyNumericIDMigrationCandidate(
	ctx context.Context,
	client userKeyMigrationClient,
	tableName string,
	candidate numericIDMigrationCandidate,
	summary *numericIDMigrationSummary,
) error {
	if candidate.PutActor {
		if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      candidate.ActorItem,
		}); err != nil {
			return fmt.Errorf("put actor profile %s %s: %w", candidate.ActorPK, candidate.ActorSK, err)
		}
		summary.ActorRowsUpdated++
	}

	if candidate.PutMapping {
		if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      candidate.MappingItem,
		}); err != nil {
			mappingPK, _ := attributeString(candidate.MappingItem["PK"])
			return fmt.Errorf("put numeric ID mapping %s: %w", mappingPK, err)
		}
		summary.MappingsUpserted++
	}

	if candidate.DeleteLegacy {
		if _, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: candidate.LegacyMappingPK},
				"SK": &types.AttributeValueMemberS{Value: numericMetadataSortKey},
			},
		}); err != nil {
			return fmt.Errorf("delete legacy numeric ID mapping %s: %w", candidate.LegacyMappingPK, err)
		}
		summary.LegacyMappingsDeleted++
	}

	return nil
}

func canonicalMigrationUsername(actorItem map[string]types.AttributeValue, actorPK string) string {
	if username, ok := attributeString(actorItem["username"]); ok && strings.TrimSpace(username) != "" {
		return strings.ToLower(strings.TrimSpace(username))
	}

	if actorValue, ok := actorItem["actor"].(*types.AttributeValueMemberM); ok {
		if preferredUsername, ok := attributeString(actorValue.Value["preferredUsername"]); ok && strings.TrimSpace(preferredUsername) != "" {
			return strings.ToLower(strings.TrimSpace(preferredUsername))
		}
	}

	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(actorPK, actorPartitionPrefix)))
}

func normalizeActorPayloadIdentity(actorItem map[string]types.AttributeValue, canonicalUsername string) bool {
	actorValue, ok := actorItem["actor"].(*types.AttributeValueMemberM)
	if !ok || actorValue == nil {
		return false
	}

	changed := setNestedStringAttribute(actorValue.Value, "preferredUsername", canonicalUsername)

	for _, key := range []string{"id", "url", "inbox", "outbox", "followers", "following", "liked"} {
		if normalizeNestedActorReference(actorValue.Value, key, canonicalUsername) {
			changed = true
		}
	}

	return changed
}

func desiredActorIDForMapping(
	actorItem map[string]types.AttributeValue,
	existingDesiredMapping map[string]types.AttributeValue,
	legacyMapping map[string]types.AttributeValue,
	canonicalUsername string,
) string {
	if actorValue, ok := actorItem["actor"].(*types.AttributeValueMemberM); ok && actorValue != nil {
		if actorID, ok := attributeString(actorValue.Value["id"]); ok && strings.TrimSpace(actorID) != "" {
			return normalizeActorReference(actorID, canonicalUsername)
		}
	}

	for _, item := range []map[string]types.AttributeValue{existingDesiredMapping, legacyMapping} {
		if item == nil {
			continue
		}
		if actorID, ok := attributeString(item["actorID"]); ok && strings.TrimSpace(actorID) != "" {
			return normalizeActorReference(actorID, canonicalUsername)
		}
	}

	return ""
}

func desiredCreatedAtAttribute(existingDesiredMapping map[string]types.AttributeValue, legacyMapping map[string]types.AttributeValue) types.AttributeValue {
	for _, item := range []map[string]types.AttributeValue{existingDesiredMapping, legacyMapping} {
		if item == nil {
			continue
		}
		if createdAt, ok := item["createdAt"]; ok {
			return cloneAttributeValue(createdAt)
		}
	}

	return &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)}
}

func buildNumericIDMappingItem(
	numericID string,
	username string,
	actorID string,
	createdAt types.AttributeValue,
) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: numericMappingPartition + numericID},
		"SK":        &types.AttributeValueMemberS{Value: numericMetadataSortKey},
		"numericID": &types.AttributeValueMemberS{Value: numericID},
		"username":  &types.AttributeValueMemberS{Value: username},
		"type":      &types.AttributeValueMemberS{Value: numericMappingTypeName},
		"createdAt": cloneAttributeValue(createdAt),
	}
	if strings.TrimSpace(actorID) != "" {
		item["actorID"] = &types.AttributeValueMemberS{Value: actorID}
	}
	return item
}

func mappingItemsEquivalent(existing map[string]types.AttributeValue, desired map[string]types.AttributeValue) bool {
	if existing == nil {
		return false
	}

	for _, key := range []string{"PK", "SK", "numericID", "username", "type", "actorID"} {
		existingValue, existingOK := attributeString(existing[key])
		desiredValue, desiredOK := attributeString(desired[key])
		if existingOK != desiredOK {
			return false
		}
		if existingValue != desiredValue {
			return false
		}
	}

	_, existingCreatedAt := attributeString(existing["createdAt"])
	_, desiredCreatedAt := attributeString(desired["createdAt"])
	return existingCreatedAt == desiredCreatedAt
}

func setNestedStringAttribute(item map[string]types.AttributeValue, key string, value string) bool {
	current, ok := attributeString(item[key])
	if ok && current == value {
		return false
	}
	item[key] = &types.AttributeValueMemberS{Value: value}
	return true
}

func normalizeNestedActorReference(item map[string]types.AttributeValue, key string, canonicalUsername string) bool {
	current, ok := attributeString(item[key])
	if !ok || strings.TrimSpace(current) == "" {
		return false
	}

	normalized := normalizeActorReference(current, canonicalUsername)
	if normalized == current {
		return false
	}
	item[key] = &types.AttributeValueMemberS{Value: normalized}
	return true
}

func normalizeActorReference(value string, canonicalUsername string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	trimmed = strings.TrimRight(trimmed, "/")
	lowerTrimmed := strings.ToLower(trimmed)
	if idx := strings.LastIndex(lowerTrimmed, "/users/"); idx >= 0 {
		suffix := trimmed[idx+len("/users/"):]
		if suffix != "" && !strings.Contains(suffix, "/") {
			return trimmed[:idx+len("/users/")] + canonicalUsername
		}
	}
	if idx := strings.LastIndex(lowerTrimmed, "/@"); idx >= 0 {
		suffix := trimmed[idx+len("/@"):]
		if suffix != "" && !strings.Contains(suffix, "/") {
			return trimmed[:idx+len("/@")] + canonicalUsername
		}
	}
	return trimmed
}

func cloneAttributeMap(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	cloned := make(map[string]types.AttributeValue, len(item))
	for key, value := range item {
		cloned[key] = cloneAttributeValue(value)
	}
	return cloned
}

func cloneAttributeValue(value types.AttributeValue) types.AttributeValue {
	switch typed := value.(type) {
	case *types.AttributeValueMemberB:
		return &types.AttributeValueMemberB{Value: append([]byte(nil), typed.Value...)}
	case *types.AttributeValueMemberBOOL:
		return &types.AttributeValueMemberBOOL{Value: typed.Value}
	case *types.AttributeValueMemberBS:
		cloned := make([][]byte, len(typed.Value))
		for i := range typed.Value {
			cloned[i] = append([]byte(nil), typed.Value[i]...)
		}
		return &types.AttributeValueMemberBS{Value: cloned}
	case *types.AttributeValueMemberL:
		cloned := make([]types.AttributeValue, len(typed.Value))
		for i := range typed.Value {
			cloned[i] = cloneAttributeValue(typed.Value[i])
		}
		return &types.AttributeValueMemberL{Value: cloned}
	case *types.AttributeValueMemberM:
		return &types.AttributeValueMemberM{Value: cloneAttributeMap(typed.Value)}
	case *types.AttributeValueMemberN:
		return &types.AttributeValueMemberN{Value: typed.Value}
	case *types.AttributeValueMemberNS:
		return &types.AttributeValueMemberNS{Value: append([]string(nil), typed.Value...)}
	case *types.AttributeValueMemberNULL:
		return &types.AttributeValueMemberNULL{Value: typed.Value}
	case *types.AttributeValueMemberS:
		return &types.AttributeValueMemberS{Value: typed.Value}
	case *types.AttributeValueMemberSS:
		return &types.AttributeValueMemberSS{Value: append([]string(nil), typed.Value...)}
	default:
		return value
	}
}
