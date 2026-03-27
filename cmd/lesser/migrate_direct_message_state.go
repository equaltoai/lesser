package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type directMessageStateMigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type directMessageStateMigrationSummary struct {
	ScannedConversations      int
	ActiveConversations       int
	StatusBackedConversations int
	ThreadStatusesScanned     int
	SampleConversationIDs     []string
}

type directMessageMigrationConversation struct {
	ConversationID     string
	Participants       []string
	MetadataItem       map[string]types.AttributeValue
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastStatusID       string
	LastMessageTime    time.Time
	TotalMessageCount  int64
	ThreadStatusItems  []map[string]types.AttributeValue
	ThreadStatusCount  int
	ThreadLastStatusID string
	ThreadLastTime     time.Time
}

var newDirectMessageStateMigrationClientFn = func(cfg aws.Config) directMessageStateMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateDirectMessageState(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-direct-message-state",
		"maximum number of DM conversations to process (0 = all)",
		"rebuild canonical DM state, repair direct-message mentions, and retire legacy DM rows",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (directMessageStateMigrationSummary, error) {
			return executeDirectMessageStateMigration(
				ctx,
				newDirectMessageStateMigrationClientFn(awsCfg),
				tableName,
				apply,
				limit,
			)
		},
		printDirectMessageStateMigrationSummary,
	)
}

func printDirectMessageStateMigrationSummary(
	summary directMessageStateMigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-direct-message-state %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_conversations: %d\n", summary.ScannedConversations)
	fmt.Printf("active_conversations: %d\n", summary.ActiveConversations)
	fmt.Printf("status_backed_conversations: %d\n", summary.StatusBackedConversations)
	fmt.Printf("thread_statuses_scanned: %d\n", summary.ThreadStatusesScanned)
	printConversationMigrationSamples(summary.SampleConversationIDs)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to backfill canonical DM state, repair missing mentions, and retire legacy DM rows")
	}
}

func executeDirectMessageStateMigration(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
	_ bool,
	limit int,
) (directMessageStateMigrationSummary, error) {
	summary := directMessageStateMigrationSummary{}

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

			conversation, ok := buildDirectMessageMigrationConversation(item)
			if !ok {
				continue
			}

			threadStatuses, statusCount, lastStatusID, lastStatusTime, err := loadDirectMessageThreadStatuses(ctx, client, tableName, conversation.ConversationID)
			if err != nil {
				return summary, fmt.Errorf("load thread statuses for %q: %w", conversation.ConversationID, err)
			}

			conversation.ThreadStatusItems = threadStatuses
			conversation.ThreadStatusCount = statusCount
			conversation.ThreadLastStatusID = lastStatusID
			conversation.ThreadLastTime = lastStatusTime

			summary.ActiveConversations++
			if statusCount > 0 {
				summary.StatusBackedConversations++
				summary.ThreadStatusesScanned += statusCount
			}
			appendConversationMigrationSample(&summary.SampleConversationIDs, conversation.ConversationID)

			if limit > 0 && summary.ActiveConversations >= limit {
				return summary, nil
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return summary, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildDirectMessageMigrationConversation(item map[string]types.AttributeValue) (directMessageMigrationConversation, bool) {
	conversationID, ok := firstAttributeString(item, "id", "ID")
	if !ok {
		pk, _ := attributeString(item["PK"])
		conversationID = strings.TrimSpace(strings.TrimPrefix(pk, conversationMetadataPartitionPrefix))
	}
	participants, ok := attributeStringSlice(item[conversationParticipantsAttribute])
	if !ok || strings.TrimSpace(conversationID) == "" || len(participants) == 0 {
		return directMessageMigrationConversation{}, false
	}

	return directMessageMigrationConversation{
		ConversationID:    conversationID,
		Participants:      append([]string(nil), participants...),
		MetadataItem:      cloneAttributeMap(item),
		CreatedAt:         firstConversationTime(item, conversationCreatedAtAttribute, "CreatedAt"),
		UpdatedAt:         firstConversationTime(item, conversationUpdatedAtAttribute, "UpdatedAt"),
		LastStatusID:      firstConversationString(item, conversationLastStatusIDAttribute, "LastStatusID"),
		LastMessageTime:   firstConversationTime(item, conversationLastMessageTimeAttribute, "LastMessageTime"),
		TotalMessageCount: firstConversationInt64(item, conversationTotalMessageCountAttribute, "TotalMessageCount"),
	}, true
}

func loadDirectMessageThreadStatuses(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
	conversationID string,
) ([]map[string]types.AttributeValue, int, string, time.Time, error) {
	statusItems := make([]map[string]types.AttributeValue, 0)
	statusCount := 0
	lastStatusID := ""
	lastStatusTime := time.Time{}

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
			return nil, 0, "", time.Time{}, err
		}

		for _, item := range out.Items {
			statusID, ok := firstAttributeString(item, conversationMessageStatusIDAttribute, "statusID")
			if !ok {
				continue
			}
			publishedAt := firstConversationTime(item, "publishedAt", "PublishedAt")
			if publishedAt.IsZero() {
				continue
			}

			statusItems = append(statusItems, cloneAttributeMap(item))
			statusCount++
			if publishedAt.After(lastStatusTime) || (publishedAt.Equal(lastStatusTime) && statusID > lastStatusID) {
				lastStatusTime = publishedAt
				lastStatusID = statusID
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return statusItems, statusCount, lastStatusID, lastStatusTime, nil
		}
		queryInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func firstConversationString(item map[string]types.AttributeValue, keys ...string) string {
	value, _ := firstAttributeString(item, keys...)
	return value
}

func firstConversationInt64(item map[string]types.AttributeValue, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := attributeInt64(item[key]); ok {
			return value
		}
	}
	return 0
}

func firstConversationTime(item map[string]types.AttributeValue, keys ...string) time.Time {
	for _, key := range keys {
		if value, ok := attributeTime(item[key]); ok {
			return value
		}
	}
	return time.Time{}
}
