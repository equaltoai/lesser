package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
)

func executeSecurityFindingsNumericIDBackfill(
	ctx context.Context,
	client securityFindingsMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (securityFindingsOperationSummary, error) {
	summary := securityFindingsOperationSummary{Name: "numeric-ids"}

	actors, err := scanMigrationItems(ctx, client, tableName, actorPartitionPrefix, actorProfileSortKey)
	if err != nil {
		return summary, fmt.Errorf("scan actor profiles: %w", err)
	}
	mappings, err := scanMigrationItems(ctx, client, tableName, numericMappingPartition, numericMetadataSortKey)
	if err != nil {
		return summary, fmt.Errorf("scan numeric ID mappings: %w", err)
	}

	mappingsByPK := make(map[string]map[string]types.AttributeValue, len(mappings))
	for _, mapping := range mappings {
		if pk, ok := attributeString(mapping["PK"]); ok {
			mappingsByPK[pk] = mapping
		}
	}

	for _, actorItem := range actors {
		summary.Scanned++
		mappingItem, sample, ok, skipped, err := buildSecurityFindingsNumericIDBackfill(actorItem, mappingsByPK)
		if err != nil {
			return summary, err
		}
		if skipped {
			summary.Skipped++
			appendSecurityFindingsSample(&summary, sample)
			continue
		}
		if !ok {
			continue
		}

		summary.Candidates++
		summary.PlannedWrites++
		appendSecurityFindingsSample(&summary, sample)
		if apply {
			if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
				TableName:           aws.String(tableName),
				Item:                mappingItem,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}); err != nil {
				pk, _ := attributeString(mappingItem["PK"])
				return summary, fmt.Errorf("put numeric ID mapping %s: %w", pk, err)
			}
			summary.AppliedWrites++
		}

		if limit > 0 && summary.Candidates >= limit {
			break
		}
	}

	return summary, nil
}

func buildSecurityFindingsNumericIDBackfill(
	actorItem map[string]types.AttributeValue,
	mappingsByPK map[string]map[string]types.AttributeValue,
) (map[string]types.AttributeValue, string, bool, bool, error) {
	actorPK, ok := attributeString(actorItem["PK"])
	if !ok || !strings.HasPrefix(actorPK, actorPartitionPrefix) {
		return nil, "", false, false, nil
	}
	actorSK, ok := attributeString(actorItem["SK"])
	if !ok || actorSK != actorProfileSortKey {
		return nil, "", false, false, nil
	}

	canonicalUsername := canonicalMigrationUsername(actorItem, actorPK)
	if canonicalUsername == "" {
		return nil, "", false, false, fmt.Errorf("actor profile %q is missing username", actorPK)
	}

	numericID := common.GenerateNumericID(canonicalUsername)
	mappingPK := numericMappingPartition + numericID
	if existing := mappingsByPK[mappingPK]; existing != nil {
		existingUsername, _ := attributeString(existing["username"])
		if strings.EqualFold(strings.TrimSpace(existingUsername), canonicalUsername) {
			return nil, "", false, false, nil
		}
		return nil, fmt.Sprintf("%s conflicts with existing username %q for actor %s", mappingPK, existingUsername, canonicalUsername), false, true, nil
	}

	actorID := desiredActorIDForMapping(actorItem, nil, nil, canonicalUsername)
	mapping := buildNumericIDMappingItem(
		numericID,
		canonicalUsername,
		actorID,
		&types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)},
	)
	return mapping, fmt.Sprintf("%s -> %s", mappingPK, canonicalUsername), true, false, nil
}

func appendSecurityFindingsSample(summary *securityFindingsOperationSummary, sample string) {
	if summary == nil || strings.TrimSpace(sample) == "" || len(summary.Samples) >= 10 {
		return
	}
	summary.Samples = append(summary.Samples, sample)
}
