package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

const (
	securityHashtagGSIPrimaryPrefix  = "HASHTAG#"
	securityStatusHashtagIndexPrefix = "HASHTAG_INDEX#"
	securityHashtagTimelinePrefix    = "HASHTAG_TIMELINE#"
)

func executeSecurityFindingsHashtagIndexCleanup(
	ctx context.Context,
	client securityFindingsMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (securityFindingsOperationSummary, error) {
	summary := securityFindingsOperationSummary{Name: "hashtag-indexes"}

	statusItems, err := scanSecurityFindingsItems(ctx, client, tableName,
		"attribute_exists(gsi5PK) AND begins_with(gsi5PK, :prefix)",
		map[string]types.AttributeValue{":prefix": &types.AttributeValueMemberS{Value: securityHashtagGSIPrimaryPrefix}},
	)
	if err != nil {
		return summary, fmt.Errorf("scan status hashtag GSI rows: %w", err)
	}
	if err := planAndApplyPrimaryHashtagGSIRepairs(ctx, client, tableName, statusItems, apply, limit, &summary); err != nil {
		return summary, err
	}

	legacyIndexItems, err := scanSecurityFindingsItems(ctx, client, tableName,
		"begins_with(PK, :prefix)",
		map[string]types.AttributeValue{":prefix": &types.AttributeValueMemberS{Value: securityStatusHashtagIndexPrefix}},
	)
	if err != nil {
		return summary, fmt.Errorf("scan legacy hashtag index rows: %w", err)
	}
	if err := planAndApplyHashtagIndexDeletes(ctx, client, tableName, legacyIndexItems, apply, limit, &summary); err != nil {
		return summary, err
	}

	timelineIndexItems, err := scanSecurityFindingsItems(ctx, client, tableName,
		"begins_with(PK, :prefix)",
		map[string]types.AttributeValue{":prefix": &types.AttributeValueMemberS{Value: securityHashtagTimelinePrefix}},
	)
	if err != nil {
		return summary, fmt.Errorf("scan hashtag timeline index rows: %w", err)
	}
	if err := planAndApplyHashtagIndexDeletes(ctx, client, tableName, timelineIndexItems, apply, limit, &summary); err != nil {
		return summary, err
	}

	return summary, nil
}

func scanSecurityFindingsItems(
	ctx context.Context,
	client securityFindingsMigrationClient,
	tableName string,
	filterExpression string,
	values map[string]types.AttributeValue,
) ([]map[string]types.AttributeValue, error) {
	input := &dynamodb.ScanInput{
		TableName:                 aws.String(tableName),
		FilterExpression:          aws.String(filterExpression),
		ExpressionAttributeValues: values,
	}

	var items []map[string]types.AttributeValue
	for {
		out, err := client.Scan(ctx, input)
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		input.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func planAndApplyPrimaryHashtagGSIRepairs(
	ctx context.Context,
	client securityFindingsMigrationClient,
	tableName string,
	items []map[string]types.AttributeValue,
	apply bool,
	limit int,
	summary *securityFindingsOperationSummary,
) error {
	for _, item := range items {
		summary.Scanned++
		if isPublicVisibilityAttribute(item["visibility"]) {
			continue
		}
		if securityFindingsCandidateLimitReached(summary, limit) {
			return nil
		}
		pk, sk, ok := migrationItemKey(item)
		if !ok {
			summary.Skipped++
			appendSecurityFindingsSample(summary, "primary hashtag row missing PK/SK")
			continue
		}

		summary.Candidates++
		summary.PlannedWrites++
		appendSecurityFindingsSample(summary, fmt.Sprintf("remove gsi5 from %s %s", pk, sk))
		if apply {
			_, err := client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName:        aws.String(tableName),
				Key:              migrationDynamoKey(pk, sk),
				UpdateExpression: aws.String("REMOVE gsi5PK, gsi5SK"),
				ConditionExpression: aws.String(
					"attribute_exists(PK) AND attribute_exists(SK)",
				),
			})
			if err != nil {
				return fmt.Errorf("remove hashtag GSI keys from %s %s: %w", pk, sk, err)
			}
			summary.AppliedWrites++
		}
	}
	return nil
}

func planAndApplyHashtagIndexDeletes(
	ctx context.Context,
	client securityFindingsMigrationClient,
	tableName string,
	items []map[string]types.AttributeValue,
	apply bool,
	limit int,
	summary *securityFindingsOperationSummary,
) error {
	for _, item := range items {
		summary.Scanned++
		if isPublicVisibilityAttribute(item["visibility"]) {
			continue
		}
		if securityFindingsCandidateLimitReached(summary, limit) {
			return nil
		}
		pk, sk, ok := migrationItemKey(item)
		if !ok {
			summary.Skipped++
			appendSecurityFindingsSample(summary, "hashtag index row missing PK/SK")
			continue
		}

		summary.Candidates++
		summary.PlannedWrites++
		appendSecurityFindingsSample(summary, fmt.Sprintf("delete stale hashtag index %s %s", pk, sk))
		if apply {
			if _, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(tableName),
				Key:       migrationDynamoKey(pk, sk),
			}); err != nil {
				return fmt.Errorf("delete stale hashtag index %s %s: %w", pk, sk, err)
			}
			summary.AppliedWrites++
		}
	}
	return nil
}

func isPublicVisibilityAttribute(value types.AttributeValue) bool {
	visibility, ok := attributeString(value)
	return ok && strings.EqualFold(strings.TrimSpace(visibility), models.VisibilityPublic)
}

func securityFindingsCandidateLimitReached(summary *securityFindingsOperationSummary, limit int) bool {
	return limit > 0 && summary != nil && summary.Candidates >= limit
}

func migrationItemKey(item map[string]types.AttributeValue) (string, string, bool) {
	pk, pkOK := attributeString(item["PK"])
	sk, skOK := attributeString(item["SK"])
	return pk, sk, pkOK && skOK && strings.TrimSpace(pk) != "" && strings.TrimSpace(sk) != ""
}

func migrationDynamoKey(pk string, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: pk},
		"SK": &types.AttributeValueMemberS{Value: sk},
	}
}
