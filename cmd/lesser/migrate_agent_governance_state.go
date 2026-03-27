package main

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

type agentGovernanceStateMigrationClient interface {
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

type agentGovernanceStateMigrationSummary struct {
	ScannedAgents          int
	AgentsWithLegacyState  int
	ExistingTypedRows      int
	GovernanceRowsPlanned  int
	GovernanceRowsUpserted int
	UserRowsCleanupPlanned int
	UserRowsUpdated        int
	ParityMatches          int
	ParityMismatches       int
	ValidationErrors       int
	SampleUsernames        []string
}

var newAgentGovernanceStateMigrationClientFn = func(cfg aws.Config) agentGovernanceStateMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateAgentGovernanceState(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-agent-governance-state",
		"maximum number of agent rows to process (0 = all)",
		"backfill typed agent governance rows and remove legacy governance metadata from user rows",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (agentGovernanceStateMigrationSummary, error) {
			return executeAgentGovernanceStateMigration(
				ctx,
				newAgentGovernanceStateMigrationClientFn(awsCfg),
				tableName,
				apply,
				limit,
			)
		},
		printAgentGovernanceStateMigrationSummary,
	)
}

func printAgentGovernanceStateMigrationSummary(
	summary agentGovernanceStateMigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-agent-governance-state %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_agents: %d\n", summary.ScannedAgents)
	fmt.Printf("agents_with_legacy_state: %d\n", summary.AgentsWithLegacyState)
	fmt.Printf("existing_typed_rows: %d\n", summary.ExistingTypedRows)
	fmt.Printf("governance_rows_planned: %d\n", summary.GovernanceRowsPlanned)
	fmt.Printf("governance_rows_upserted: %d\n", summary.GovernanceRowsUpserted)
	fmt.Printf("user_rows_cleanup_planned: %d\n", summary.UserRowsCleanupPlanned)
	fmt.Printf("user_rows_updated: %d\n", summary.UserRowsUpdated)
	fmt.Printf("parity_matches: %d\n", summary.ParityMatches)
	fmt.Printf("parity_mismatches: %d\n", summary.ParityMismatches)
	fmt.Printf("validation_errors: %d\n", summary.ValidationErrors)
	printMigrationSamples("sample_usernames", summary.SampleUsernames)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to backfill typed governance rows and remove legacy metadata keys")
	}
}

func executeAgentGovernanceStateMigration(
	ctx context.Context,
	client agentGovernanceStateMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (agentGovernanceStateMigrationSummary, error) {
	summary := agentGovernanceStateMigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String(models.IndexGSI6),
		KeyConditionExpression: aws.String("gsi6PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "ACCOUNT_TYPE#AGENT"},
		},
	}

	for {
		out, err := client.Query(ctx, queryInput)
		if err != nil {
			return summary, fmt.Errorf("query agent user rows: %w", err)
		}

		for _, item := range out.Items {
			if limit > 0 && summary.ScannedAgents >= limit {
				return summary, nil
			}
			if err := processAgentGovernanceMigrationItem(ctx, client, tableName, item, apply, &summary); err != nil {
				return summary, err
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		queryInput.ExclusiveStartKey = out.LastEvaluatedKey
	}

	return summary, nil
}

func processAgentGovernanceMigrationItem(
	ctx context.Context,
	client agentGovernanceStateMigrationClient,
	tableName string,
	userItem map[string]types.AttributeValue,
	apply bool,
	summary *agentGovernanceStateMigrationSummary,
) error {
	username := agentGovernanceMigrationUsername(userItem)
	if username == "" {
		summary.ValidationErrors++
		return nil
	}
	summary.ScannedAgents++

	legacyState, hasLegacyState, err := extractLegacyAgentGovernanceState(userItem)
	if err != nil {
		summary.ValidationErrors++
		recordAgentGovernanceMigrationSample(summary, username)
		return nil
	}
	if hasLegacyState {
		summary.AgentsWithLegacyState++
	}

	existingItem, existingState, err := loadExistingAgentGovernanceState(ctx, client, tableName, username)
	if err != nil {
		return err
	}
	if existingItem != nil {
		summary.ExistingTypedRows++
	}

	desiredState, hasDesiredState := desiredAgentGovernanceState(username, userItem, existingState, legacyState, hasLegacyState)
	if hasLegacyState {
		if existingState != nil && agentGovernanceStatesEqual(existingState, desiredState) {
			summary.ParityMatches++
		} else {
			summary.ParityMismatches++
		}
	}

	if hasDesiredState && !agentGovernanceStatesEqual(existingState, desiredState) {
		summary.GovernanceRowsPlanned++
		recordAgentGovernanceMigrationSample(summary, username)
		if apply {
			if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
				TableName: aws.String(tableName),
				Item:      buildAgentGovernanceStateItem(desiredState),
			}); err != nil {
				return fmt.Errorf("put agent governance row for %s: %w", username, err)
			}
			summary.GovernanceRowsUpserted++
		}
	}

	cleanedUserItem, removedLegacyKeys := stripLegacyAgentGovernanceMetadata(userItem)
	if removedLegacyKeys {
		summary.UserRowsCleanupPlanned++
		recordAgentGovernanceMigrationSample(summary, username)
		if apply {
			if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
				TableName: aws.String(tableName),
				Item:      cleanedUserItem,
			}); err != nil {
				return fmt.Errorf("update user metadata row for %s: %w", username, err)
			}
			summary.UserRowsUpdated++
		}
	}

	return nil
}

func loadExistingAgentGovernanceState(
	ctx context.Context,
	client agentGovernanceStateMigrationClient,
	tableName string,
	username string,
) (map[string]types.AttributeValue, *storage.AgentGovernanceState, error) {
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf(models.KeyPatternUser, username)},
			"SK": &types.AttributeValueMemberS{Value: models.SKAgentGovernance},
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("get typed governance row for %s: %w", username, err)
	}
	if len(out.Item) == 0 {
		return nil, nil, nil
	}

	state, err := parseTypedAgentGovernanceState(out.Item)
	if err != nil {
		return nil, nil, fmt.Errorf("parse typed governance row for %s: %w", username, err)
	}
	return out.Item, state, nil
}

func desiredAgentGovernanceState(
	username string,
	userItem map[string]types.AttributeValue,
	existingState *storage.AgentGovernanceState,
	legacyState *storage.AgentGovernanceState,
	hasLegacyState bool,
) (*storage.AgentGovernanceState, bool) {
	if existingState == nil && (!hasLegacyState || legacyState == nil) {
		return nil, false
	}

	var desired storage.AgentGovernanceState
	if existingState != nil {
		desired = *cloneAgentGovernanceMigrationState(existingState)
	}
	if desired.Username == "" {
		desired.Username = username
	}

	if hasLegacyState && legacyState != nil {
		desired.QuarantineStatus = legacyState.QuarantineStatus
		desired.QuarantineStart = cloneAgentGovernanceMigrationTime(legacyState.QuarantineStart)
		desired.QuarantineEnd = cloneAgentGovernanceMigrationTime(legacyState.QuarantineEnd)
		desired.QuarantineApprovedBy = legacyState.QuarantineApprovedBy
		desired.QuarantineApprovedAt = cloneAgentGovernanceMigrationTime(legacyState.QuarantineApprovedAt)
		desired.DelegatedScopes = append([]string(nil), legacyState.DelegatedScopes...)
		desired.SelfScopes = append([]string(nil), legacyState.SelfScopes...)
		desired.SelfSovereign = legacyState.SelfSovereign
		desired.Verified = legacyState.Verified
		desired.VerifiedAt = cloneAgentGovernanceMigrationTime(legacyState.VerifiedAt)
		desired.VerifiedBy = legacyState.VerifiedBy
		desired.VerifiedReason = legacyState.VerifiedReason
		desired.UnverifiedAt = cloneAgentGovernanceMigrationTime(legacyState.UnverifiedAt)
		desired.UnverifiedBy = legacyState.UnverifiedBy
		desired.UnverifiedReason = legacyState.UnverifiedReason
		desired.KeyRotatedAt = cloneAgentGovernanceMigrationTime(legacyState.KeyRotatedAt)
	}

	userCreatedAt, _ := attributeTime(userItem["createdAt"])
	userUpdatedAt, _ := attributeTime(userItem["updatedAt"])
	if desired.CreatedAt.IsZero() {
		if !userCreatedAt.IsZero() {
			desired.CreatedAt = userCreatedAt.UTC()
		} else {
			desired.CreatedAt = time.Now().UTC()
		}
	}
	if desired.UpdatedAt.IsZero() {
		if !userUpdatedAt.IsZero() {
			desired.UpdatedAt = userUpdatedAt.UTC()
		} else {
			desired.UpdatedAt = desired.CreatedAt
		}
	}

	return normalizeAgentGovernanceMigrationState(&desired), true
}

func buildAgentGovernanceStateItem(state *storage.AgentGovernanceState) map[string]types.AttributeValue {
	state = normalizeAgentGovernanceMigrationState(state)

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf(models.KeyPatternUser, state.Username)},
		"SK":        &types.AttributeValueMemberS{Value: models.SKAgentGovernance},
		"username":  &types.AttributeValueMemberS{Value: state.Username},
		"createdAt": &types.AttributeValueMemberS{Value: state.CreatedAt.UTC().Format(time.RFC3339Nano)},
		"updatedAt": &types.AttributeValueMemberS{Value: state.UpdatedAt.UTC().Format(time.RFC3339Nano)},
	}

	if value := strings.TrimSpace(state.QuarantineStatus); value != "" {
		item["quarantineStatus"] = &types.AttributeValueMemberS{Value: value}
	}
	setAgentGovernanceMigrationTimeAttribute(item, "quarantineStart", state.QuarantineStart)
	setAgentGovernanceMigrationTimeAttribute(item, "quarantineEnd", state.QuarantineEnd)
	if value := strings.TrimSpace(state.QuarantineApprovedBy); value != "" {
		item["quarantineApprovedBy"] = &types.AttributeValueMemberS{Value: value}
	}
	setAgentGovernanceMigrationTimeAttribute(item, "quarantineApprovedAt", state.QuarantineApprovedAt)
	setAgentGovernanceMigrationStringListAttribute(item, "delegatedScopes", state.DelegatedScopes)
	setAgentGovernanceMigrationStringListAttribute(item, "selfScopes", state.SelfScopes)
	if state.SelfSovereign {
		item["selfSovereign"] = &types.AttributeValueMemberBOOL{Value: true}
	}
	if state.Verified {
		item["verified"] = &types.AttributeValueMemberBOOL{Value: true}
	}
	setAgentGovernanceMigrationTimeAttribute(item, "verifiedAt", state.VerifiedAt)
	if value := strings.TrimSpace(state.VerifiedBy); value != "" {
		item["verifiedBy"] = &types.AttributeValueMemberS{Value: value}
	}
	if value := strings.TrimSpace(state.VerifiedReason); value != "" {
		item["verifiedReason"] = &types.AttributeValueMemberS{Value: value}
	}
	setAgentGovernanceMigrationTimeAttribute(item, "unverifiedAt", state.UnverifiedAt)
	if value := strings.TrimSpace(state.UnverifiedBy); value != "" {
		item["unverifiedBy"] = &types.AttributeValueMemberS{Value: value}
	}
	if value := strings.TrimSpace(state.UnverifiedReason); value != "" {
		item["unverifiedReason"] = &types.AttributeValueMemberS{Value: value}
	}
	setAgentGovernanceMigrationTimeAttribute(item, "keyRotatedAt", state.KeyRotatedAt)

	return item
}

func parseTypedAgentGovernanceState(item map[string]types.AttributeValue) (*storage.AgentGovernanceState, error) {
	username, ok := agentGovernanceMigrationAttributeString(item["username"])
	if !ok {
		username = agentGovernanceMigrationUsername(item)
	}

	createdAt, _ := attributeTime(item["createdAt"])
	updatedAt, _ := attributeTime(item["updatedAt"])

	state := &storage.AgentGovernanceState{
		Username:             strings.ToLower(strings.TrimSpace(username)),
		QuarantineStatus:     agentGovernanceMigrationAttributeStringValue(item["quarantineStatus"]),
		QuarantineStart:      agentGovernanceMigrationAttributeTimeValue(item["quarantineStart"]),
		QuarantineEnd:        agentGovernanceMigrationAttributeTimeValue(item["quarantineEnd"]),
		QuarantineApprovedBy: agentGovernanceMigrationAttributeStringValue(item["quarantineApprovedBy"]),
		QuarantineApprovedAt: agentGovernanceMigrationAttributeTimeValue(item["quarantineApprovedAt"]),
		DelegatedScopes:      agentGovernanceMigrationAttributeStringSliceValue(item["delegatedScopes"]),
		SelfScopes:           agentGovernanceMigrationAttributeStringSliceValue(item["selfScopes"]),
		SelfSovereign:        agentGovernanceMigrationAttributeBoolValue(item["selfSovereign"]),
		Verified:             agentGovernanceMigrationAttributeBoolValue(item["verified"]),
		VerifiedAt:           agentGovernanceMigrationAttributeTimeValue(item["verifiedAt"]),
		VerifiedBy:           agentGovernanceMigrationAttributeStringValue(item["verifiedBy"]),
		VerifiedReason:       agentGovernanceMigrationAttributeStringValue(item["verifiedReason"]),
		UnverifiedAt:         agentGovernanceMigrationAttributeTimeValue(item["unverifiedAt"]),
		UnverifiedBy:         agentGovernanceMigrationAttributeStringValue(item["unverifiedBy"]),
		UnverifiedReason:     agentGovernanceMigrationAttributeStringValue(item["unverifiedReason"]),
		KeyRotatedAt:         agentGovernanceMigrationAttributeTimeValue(item["keyRotatedAt"]),
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}

	return normalizeAgentGovernanceMigrationState(state), nil
}

func extractLegacyAgentGovernanceState(item map[string]types.AttributeValue) (*storage.AgentGovernanceState, bool, error) {
	metadataMap, ok := item["metadata"].(*types.AttributeValueMemberM)
	if !ok || metadataMap == nil || len(metadataMap.Value) == 0 {
		return nil, false, nil
	}

	values := metadataMap.Value
	legacyKeysSeen := false
	state := &storage.AgentGovernanceState{}
	var parseErrs []string

	for _, key := range legacyAgentGovernanceMetadataKeys {
		if _, ok := values[key]; ok {
			legacyKeysSeen = true
			break
		}
	}
	if !legacyKeysSeen {
		return nil, false, nil
	}

	state.QuarantineStatus = agentGovernanceMigrationAttributeStringValue(values["agent_quarantine_status"])
	if value, err := agentGovernanceMigrationLegacyTime(values["agent_quarantine_start"], "agent_quarantine_start"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.QuarantineStart = value
	}
	if value, err := agentGovernanceMigrationLegacyTime(values["agent_quarantine_end"], "agent_quarantine_end"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.QuarantineEnd = value
	}
	state.QuarantineApprovedBy = agentGovernanceMigrationAttributeStringValue(values["agent_quarantine_approved_by"])
	if value, err := agentGovernanceMigrationLegacyTime(values["agent_quarantine_approved_at"], "agent_quarantine_approved_at"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.QuarantineApprovedAt = value
	}
	if value, err := agentGovernanceMigrationLegacyStringSlice(values["agent_delegated_scopes"], "agent_delegated_scopes"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.DelegatedScopes = value
	}
	if value, err := agentGovernanceMigrationLegacyStringSlice(values["agent_self_scopes"], "agent_self_scopes"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.SelfScopes = value
	}
	if value, err := agentGovernanceMigrationLegacyBool(values["agent_self_sovereign"], "agent_self_sovereign"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.SelfSovereign = value
	}
	if value, err := agentGovernanceMigrationLegacyBool(values["agent_verified"], "agent_verified"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.Verified = value
	}
	if value, err := agentGovernanceMigrationLegacyTime(values["agent_verified_at"], "agent_verified_at"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.VerifiedAt = value
	}
	state.VerifiedBy = agentGovernanceMigrationAttributeStringValue(values["agent_verified_by"])
	state.VerifiedReason = agentGovernanceMigrationAttributeStringValue(values["agent_verified_reason"])
	if value, err := agentGovernanceMigrationLegacyTime(values["agent_unverified_at"], "agent_unverified_at"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.UnverifiedAt = value
	}
	state.UnverifiedBy = agentGovernanceMigrationAttributeStringValue(values["agent_unverified_by"])
	state.UnverifiedReason = agentGovernanceMigrationAttributeStringValue(values["agent_unverified_reason"])
	if value, err := agentGovernanceMigrationLegacyTime(values["agent_key_rotated_at"], "agent_key_rotated_at"); err != nil {
		parseErrs = append(parseErrs, err.Error())
	} else {
		state.KeyRotatedAt = value
	}

	if len(parseErrs) > 0 {
		return nil, true, fmt.Errorf("%s", strings.Join(parseErrs, "; "))
	}

	return normalizeAgentGovernanceMigrationState(state), true, nil
}

func stripLegacyAgentGovernanceMetadata(item map[string]types.AttributeValue) (map[string]types.AttributeValue, bool) {
	cloned := cloneAttributeValueMap(item)
	metadataAttr, ok := cloned["metadata"].(*types.AttributeValueMemberM)
	if !ok || metadataAttr == nil || len(metadataAttr.Value) == 0 {
		return cloned, false
	}

	metadataClone := cloneAttributeValueMap(metadataAttr.Value)
	removed := false
	for _, key := range legacyAgentGovernanceMetadataKeys {
		if _, ok := metadataClone[key]; ok {
			delete(metadataClone, key)
			removed = true
		}
	}
	if !removed {
		return cloned, false
	}

	if len(metadataClone) == 0 {
		delete(cloned, "metadata")
		return cloned, true
	}

	cloned["metadata"] = &types.AttributeValueMemberM{Value: metadataClone}
	return cloned, true
}

var legacyAgentGovernanceMetadataKeys = []string{
	"agent_quarantine_status",
	"agent_quarantine_start",
	"agent_quarantine_end",
	"agent_quarantine_approved_by",
	"agent_quarantine_approved_at",
	"agent_delegated_scopes",
	"agent_self_scopes",
	"agent_self_sovereign",
	"agent_verified",
	"agent_verified_at",
	"agent_verified_by",
	"agent_verified_reason",
	"agent_unverified_at",
	"agent_unverified_by",
	"agent_unverified_reason",
	"agent_key_rotated_at",
}

func normalizeAgentGovernanceMigrationState(state *storage.AgentGovernanceState) *storage.AgentGovernanceState {
	if state == nil {
		return nil
	}

	normalized := *cloneAgentGovernanceMigrationState(state)
	normalized.Username = strings.ToLower(strings.TrimSpace(normalized.Username))
	normalized.QuarantineStatus = strings.TrimSpace(normalized.QuarantineStatus)
	normalized.QuarantineApprovedBy = strings.TrimSpace(normalized.QuarantineApprovedBy)
	normalized.VerifiedBy = strings.TrimSpace(normalized.VerifiedBy)
	normalized.VerifiedReason = strings.TrimSpace(normalized.VerifiedReason)
	normalized.UnverifiedBy = strings.TrimSpace(normalized.UnverifiedBy)
	normalized.UnverifiedReason = strings.TrimSpace(normalized.UnverifiedReason)
	normalized.QuarantineStart = cloneAgentGovernanceMigrationTime(normalized.QuarantineStart)
	normalized.QuarantineEnd = cloneAgentGovernanceMigrationTime(normalized.QuarantineEnd)
	normalized.QuarantineApprovedAt = cloneAgentGovernanceMigrationTime(normalized.QuarantineApprovedAt)
	normalized.VerifiedAt = cloneAgentGovernanceMigrationTime(normalized.VerifiedAt)
	normalized.UnverifiedAt = cloneAgentGovernanceMigrationTime(normalized.UnverifiedAt)
	normalized.KeyRotatedAt = cloneAgentGovernanceMigrationTime(normalized.KeyRotatedAt)
	normalized.DelegatedScopes = normalizeAgentGovernanceMigrationScopes(normalized.DelegatedScopes)
	normalized.SelfScopes = normalizeAgentGovernanceMigrationScopes(normalized.SelfScopes)
	if !normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = normalized.CreatedAt.UTC()
	}
	if !normalized.UpdatedAt.IsZero() {
		normalized.UpdatedAt = normalized.UpdatedAt.UTC()
	}
	return &normalized
}

func normalizeAgentGovernanceMigrationScopes(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}

func cloneAgentGovernanceMigrationState(state *storage.AgentGovernanceState) *storage.AgentGovernanceState {
	if state == nil {
		return nil
	}

	cloned := *state
	cloned.QuarantineStart = cloneAgentGovernanceMigrationTime(state.QuarantineStart)
	cloned.QuarantineEnd = cloneAgentGovernanceMigrationTime(state.QuarantineEnd)
	cloned.QuarantineApprovedAt = cloneAgentGovernanceMigrationTime(state.QuarantineApprovedAt)
	cloned.VerifiedAt = cloneAgentGovernanceMigrationTime(state.VerifiedAt)
	cloned.UnverifiedAt = cloneAgentGovernanceMigrationTime(state.UnverifiedAt)
	cloned.KeyRotatedAt = cloneAgentGovernanceMigrationTime(state.KeyRotatedAt)
	cloned.DelegatedScopes = append([]string(nil), state.DelegatedScopes...)
	cloned.SelfScopes = append([]string(nil), state.SelfScopes...)
	return &cloned
}

func cloneAgentGovernanceMigrationTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	timestamp := value.UTC()
	return &timestamp
}

func agentGovernanceStatesEqual(left, right *storage.AgentGovernanceState) bool {
	if left == nil || right == nil {
		return left == right
	}
	return reflect.DeepEqual(normalizeAgentGovernanceMigrationState(left), normalizeAgentGovernanceMigrationState(right))
}

func recordAgentGovernanceMigrationSample(summary *agentGovernanceStateMigrationSummary, username string) {
	if summary == nil {
		return
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return
	}
	for _, existing := range summary.SampleUsernames {
		if existing == username {
			return
		}
	}
	if len(summary.SampleUsernames) >= 10 {
		return
	}
	summary.SampleUsernames = append(summary.SampleUsernames, username)
}

func agentGovernanceMigrationUsername(item map[string]types.AttributeValue) string {
	if username, ok := agentGovernanceMigrationAttributeString(item["username"]); ok {
		return strings.ToLower(strings.TrimSpace(username))
	}
	if pk, ok := agentGovernanceMigrationAttributeString(item["PK"]); ok && strings.HasPrefix(pk, "USER#") {
		return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(pk, "USER#")))
	}
	return ""
}

func cloneAttributeValueMap(values map[string]types.AttributeValue) map[string]types.AttributeValue {
	if len(values) == 0 {
		return map[string]types.AttributeValue{}
	}
	cloned := make(map[string]types.AttributeValue, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func setAgentGovernanceMigrationTimeAttribute(item map[string]types.AttributeValue, key string, value *time.Time) {
	if item == nil || value == nil || value.IsZero() {
		return
	}
	item[key] = &types.AttributeValueMemberS{Value: value.UTC().Format(time.RFC3339Nano)}
}

func setAgentGovernanceMigrationStringListAttribute(item map[string]types.AttributeValue, key string, values []string) {
	if item == nil || len(values) == 0 {
		return
	}
	list := make([]types.AttributeValue, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		list = append(list, &types.AttributeValueMemberS{Value: trimmed})
	}
	if len(list) == 0 {
		return
	}
	item[key] = &types.AttributeValueMemberL{Value: list}
}

func agentGovernanceMigrationAttributeString(value types.AttributeValue) (string, bool) {
	switch typed := value.(type) {
	case *types.AttributeValueMemberS:
		return strings.TrimSpace(typed.Value), strings.TrimSpace(typed.Value) != ""
	default:
		return "", false
	}
}

func agentGovernanceMigrationAttributeStringValue(value types.AttributeValue) string {
	out, _ := agentGovernanceMigrationAttributeString(value)
	return out
}

func agentGovernanceMigrationAttributeBoolValue(value types.AttributeValue) bool {
	out, _ := agentGovernanceMigrationLegacyBool(value, "")
	return out
}

func agentGovernanceMigrationAttributeTimeValue(value types.AttributeValue) *time.Time {
	out, _ := agentGovernanceMigrationLegacyTime(value, "")
	return out
}

func agentGovernanceMigrationAttributeStringSliceValue(value types.AttributeValue) []string {
	out, _ := agentGovernanceMigrationLegacyStringSlice(value, "")
	return out
}

func agentGovernanceMigrationLegacyBool(value types.AttributeValue, field string) (bool, error) {
	if value == nil {
		return false, nil
	}
	switch typed := value.(type) {
	case *types.AttributeValueMemberBOOL:
		return typed.Value, nil
	case *types.AttributeValueMemberS:
		switch strings.ToLower(strings.TrimSpace(typed.Value)) {
		case "":
			return false, nil
		case "true":
			return true, nil
		case flagFalse:
			return false, nil
		default:
			return false, fmt.Errorf("%s is not a boolean", field)
		}
	default:
		return false, fmt.Errorf("%s is not a boolean", field)
	}
}

func agentGovernanceMigrationLegacyTime(value types.AttributeValue, field string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	raw, ok := agentGovernanceMigrationAttributeString(value)
	if !ok {
		return nil, fmt.Errorf("%s is not a timestamp", field)
	}
	if raw == "" {
		return nil, nil
	}
	parsed, ok := parseFlexibleTime(raw)
	if !ok {
		return nil, fmt.Errorf("%s is not a valid timestamp", field)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func agentGovernanceMigrationLegacyStringSlice(value types.AttributeValue, field string) ([]string, error) {
	if value == nil {
		return nil, nil
	}

	switch typed := value.(type) {
	case *types.AttributeValueMemberL:
		out := make([]string, 0, len(typed.Value))
		for _, entry := range typed.Value {
			s, ok := agentGovernanceMigrationAttributeString(entry)
			if !ok {
				return nil, fmt.Errorf("%s contains a non-string scope", field)
			}
			if s != "" {
				out = append(out, s)
			}
		}
		return normalizeAgentGovernanceMigrationScopes(out), nil
	case *types.AttributeValueMemberSS:
		return normalizeAgentGovernanceMigrationScopes(append([]string(nil), typed.Value...)), nil
	case *types.AttributeValueMemberS:
		raw := strings.TrimSpace(typed.Value)
		if raw == "" {
			return nil, nil
		}
		return normalizeAgentGovernanceMigrationScopes(strings.Fields(raw)), nil
	default:
		return nil, fmt.Errorf("%s is not a string list", field)
	}
}
