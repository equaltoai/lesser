package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type conversationMetadataMigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

type conversationMetadataMigrationSummary struct {
	ScannedConversations      int
	FlaggedConversations      int
	ReparableConversations    int
	ManualReviewConversations int
	RepairedConversations     int
	SampleConversationIDs     []string
}

type conversationMetadataRepairCandidate struct {
	ConversationID string
	ManualReview   bool
	RepairedItem   map[string]types.AttributeValue
}

type conversationMetadataStatusSummary struct {
	StatusCount    int64
	NewestStatusID string
	NewestTime     time.Time
}

var newConversationMetadataMigrationClientFn = func(cfg aws.Config) conversationMetadataMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateConversationMetadata(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-conversation-metadata",
		"maximum number of stale conversation metadata rows to process (0 = all)",
		"rewrite stale conversation metadata from canonical status rows",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (conversationMetadataMigrationSummary, error) {
			return executeConversationMetadataMigration(
				ctx,
				newConversationMetadataMigrationClientFn(awsCfg),
				tableName,
				apply,
				limit,
			)
		},
		printConversationMetadataMigrationSummary,
	)
}

func printConversationMetadataMigrationSummary(
	summary conversationMetadataMigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-conversation-metadata %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_conversations: %d\n", summary.ScannedConversations)
	fmt.Printf("flagged_conversations: %d\n", summary.FlaggedConversations)
	fmt.Printf("reparable_conversations: %d\n", summary.ReparableConversations)
	fmt.Printf("manual_review_conversations: %d\n", summary.ManualReviewConversations)
	fmt.Printf("repaired_conversations: %d\n", summary.RepairedConversations)

	printConversationMigrationSamples(summary.SampleConversationIDs)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to rebuild stale conversation metadata from canonical status rows")
	}
}

func executeConversationMetadataMigration(
	ctx context.Context,
	client conversationMetadataMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (conversationMetadataMigrationSummary, error) {
	summary := conversationMetadataMigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationMetadataPartitionPrefix},
			":sk":     &types.AttributeValueMemberS{Value: conversationMetadataSortKey},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return summary, fmt.Errorf("scan conversation metadata rows: %w", err)
		}

		for _, item := range out.Items {
			summary.ScannedConversations++

			candidate, ok, err := buildConversationMetadataRepairCandidate(ctx, client, tableName, item)
			if err != nil {
				return summary, err
			}
			if !ok {
				continue
			}

			summary.FlaggedConversations++
			appendConversationMigrationSample(&summary.SampleConversationIDs, candidate.ConversationID)

			if candidate.ManualReview {
				summary.ManualReviewConversations++
			} else {
				summary.ReparableConversations++
				if apply {
					if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
						TableName: aws.String(tableName),
						Item:      candidate.RepairedItem,
					}); err != nil {
						return summary, fmt.Errorf("repair stale conversation metadata %q: %w", candidate.ConversationID, err)
					}
					summary.RepairedConversations++
				}
			}

			if limit > 0 && summary.FlaggedConversations >= limit {
				return summary, nil
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return summary, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildConversationMetadataRepairCandidate(
	ctx context.Context,
	client conversationMetadataMigrationClient,
	tableName string,
	item map[string]types.AttributeValue,
) (conversationMetadataRepairCandidate, bool, error) {
	if !conversationMetadataRowIsStale(item) {
		return conversationMetadataRepairCandidate{}, false, nil
	}

	conversationID, ok := firstAttributeString(item, "id", "ID")
	if !ok {
		pk, _ := attributeString(item["PK"])
		conversationID = strings.TrimSpace(strings.TrimPrefix(pk, conversationMetadataPartitionPrefix))
	}
	if conversationID == "" {
		return conversationMetadataRepairCandidate{ManualReview: true}, true, nil
	}

	statusSummary, err := loadConversationStatusSummary(ctx, client, tableName, conversationID)
	if err != nil {
		return conversationMetadataRepairCandidate{}, false, fmt.Errorf("load canonical statuses for %q: %w", conversationID, err)
	}
	if statusSummary.StatusCount == 0 || strings.TrimSpace(statusSummary.NewestStatusID) == "" || statusSummary.NewestTime.IsZero() {
		return conversationMetadataRepairCandidate{
			ConversationID: conversationID,
			ManualReview:   true,
		}, true, nil
	}

	repairedItem := cloneAttributeMap(item)
	repairedItem[conversationTotalMessageCountAttribute] = &types.AttributeValueMemberN{
		Value: strconv.FormatInt(statusSummary.StatusCount, 10),
	}
	repairedItem[conversationLastStatusIDAttribute] = &types.AttributeValueMemberS{Value: statusSummary.NewestStatusID}
	repairedItem[conversationLastMessageTimeAttribute] = &types.AttributeValueMemberS{
		Value: statusSummary.NewestTime.UTC().Format(time.RFC3339Nano),
	}
	repairedItem[conversationUpdatedAtAttribute] = &types.AttributeValueMemberS{
		Value: time.Now().UTC().Format(time.RFC3339Nano),
	}

	return conversationMetadataRepairCandidate{
		ConversationID: conversationID,
		RepairedItem:   repairedItem,
	}, true, nil
}

func conversationMetadataRowIsStale(item map[string]types.AttributeValue) bool {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationMetadataPartitionPrefix) {
		return false
	}
	sk, ok := attributeString(item["SK"])
	if !ok || sk != conversationMetadataSortKey {
		return false
	}

	lastStatusID, ok := firstAttributeString(item, conversationLastStatusIDAttribute, "LastStatusID")
	if !ok || strings.TrimSpace(lastStatusID) == "" {
		return false
	}

	totalMessageCount, _ := attributeInt64(item[conversationTotalMessageCountAttribute])
	lastMessageTime, ok := attributeTime(item[conversationLastMessageTimeAttribute])
	return totalMessageCount == 0 || !ok || lastMessageTime.IsZero()
}

func loadConversationStatusSummary(
	ctx context.Context,
	client conversationMetadataMigrationClient,
	tableName string,
	conversationID string,
) (conversationMetadataStatusSummary, error) {
	summary := conversationMetadataStatusSummary{}

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("gsi3"),
		KeyConditionExpression: aws.String("gsi3PK = :conversation"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":conversation": &types.AttributeValueMemberS{Value: conversationMetadataPartitionPrefix + conversationID},
		},
	}

	for {
		out, err := client.Query(ctx, queryInput)
		if err != nil {
			return summary, err
		}

		for _, item := range out.Items {
			statusID, ok := firstAttributeString(item, conversationMessageStatusIDAttribute, "statusID")
			if !ok {
				continue
			}
			publishedAt, ok := attributeTime(item["publishedAt"])
			if !ok || publishedAt.IsZero() {
				continue
			}

			summary.StatusCount++
			if summary.NewestTime.IsZero() || publishedAt.After(summary.NewestTime) {
				summary.NewestTime = publishedAt.UTC()
				summary.NewestStatusID = statusID
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return summary, nil
		}
		queryInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}
