package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

type conversationParticipantSnapshotMigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

type conversationParticipantSnapshotMigrationSummary struct {
	ScannedParticipantRows   int
	CorruptedParticipantRows int
	ReparableRows            int
	MissingCanonicalRows     int
	RepairedRows             int
	SampleConversationIDs    []string
}

type conversationParticipantSnapshotRepairCandidate struct {
	ConversationID   string
	ParticipantPK    string
	ParticipantSK    string
	MissingCanonical bool
	RepairedItem     map[string]types.AttributeValue
}

var newConversationParticipantSnapshotMigrationClientFn = func(cfg aws.Config) conversationParticipantSnapshotMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateConversationParticipantSnapshots(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-conversation-participant-snapshots",
		"maximum number of corrupted participant rows to process (0 = all)",
		"rewrite corrupted participant snapshots from canonical conversation metadata",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (conversationParticipantSnapshotMigrationSummary, error) {
			return executeConversationParticipantSnapshotMigration(
				ctx,
				newConversationParticipantSnapshotMigrationClientFn(awsCfg),
				tableName,
				apply,
				limit,
			)
		},
		printConversationParticipantSnapshotMigrationSummary,
	)
}

func printConversationParticipantSnapshotMigrationSummary(
	summary conversationParticipantSnapshotMigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-conversation-participant-snapshots %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_participant_rows: %d\n", summary.ScannedParticipantRows)
	fmt.Printf("corrupted_participant_rows: %d\n", summary.CorruptedParticipantRows)
	fmt.Printf("reparable_rows: %d\n", summary.ReparableRows)
	fmt.Printf("missing_canonical_rows: %d\n", summary.MissingCanonicalRows)
	fmt.Printf("repaired_rows: %d\n", summary.RepairedRows)

	printConversationMigrationSamples(summary.SampleConversationIDs)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to rebuild corrupted participant snapshots from canonical conversation metadata")
	}
}

func executeConversationParticipantSnapshotMigration(
	ctx context.Context,
	client conversationParticipantSnapshotMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (conversationParticipantSnapshotMigrationSummary, error) {
	summary := conversationParticipantSnapshotMigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationParticipantPartitionPrefix},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return summary, fmt.Errorf("scan participant rows: %w", err)
		}

		for _, item := range out.Items {
			summary.ScannedParticipantRows++

			candidate, ok, err := buildConversationParticipantSnapshotRepairCandidate(ctx, client, tableName, item)
			if err != nil {
				return summary, err
			}
			if !ok {
				continue
			}

			summary.CorruptedParticipantRows++
			appendConversationMigrationSample(&summary.SampleConversationIDs, candidate.ConversationID)

			if candidate.MissingCanonical {
				summary.MissingCanonicalRows++
			} else {
				summary.ReparableRows++
				if apply {
					if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
						TableName: aws.String(tableName),
						Item:      candidate.RepairedItem,
					}); err != nil {
						return summary, fmt.Errorf("repair participant snapshot %s/%s: %w", candidate.ParticipantPK, candidate.ParticipantSK, err)
					}
					summary.RepairedRows++
				}
			}

			if limit > 0 && summary.CorruptedParticipantRows >= limit {
				return summary, nil
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return summary, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildConversationParticipantSnapshotRepairCandidate(
	ctx context.Context,
	client conversationParticipantSnapshotMigrationClient,
	tableName string,
	item map[string]types.AttributeValue,
) (conversationParticipantSnapshotRepairCandidate, bool, error) {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationParticipantPartitionPrefix) {
		return conversationParticipantSnapshotRepairCandidate{}, false, nil
	}

	if !conversationParticipantSnapshotCorrupted(item) {
		return conversationParticipantSnapshotRepairCandidate{}, false, nil
	}

	sk, _ := attributeString(item["SK"])
	conversationID := participantSnapshotRepairConversationID(item)
	if conversationID == "" {
		return conversationParticipantSnapshotRepairCandidate{
			ParticipantPK:    pk,
			ParticipantSK:    sk,
			MissingCanonical: true,
		}, true, nil
	}

	canonicalItem, err := loadConversationMetadataItem(ctx, client, tableName, conversationID)
	if err != nil {
		return conversationParticipantSnapshotRepairCandidate{}, false, fmt.Errorf("load canonical conversation %q: %w", conversationID, err)
	}
	if len(canonicalItem) == 0 {
		return conversationParticipantSnapshotRepairCandidate{
			ConversationID:   conversationID,
			ParticipantPK:    pk,
			ParticipantSK:    sk,
			MissingCanonical: true,
		}, true, nil
	}

	repairedItem, err := rebuildConversationParticipantSnapshot(item, canonicalItem)
	if err != nil {
		return conversationParticipantSnapshotRepairCandidate{
			ConversationID:   conversationID,
			ParticipantPK:    pk,
			ParticipantSK:    sk,
			MissingCanonical: true,
		}, true, nil
	}

	return conversationParticipantSnapshotRepairCandidate{
		ConversationID: conversationID,
		ParticipantPK:  pk,
		ParticipantSK:  sk,
		RepairedItem:   repairedItem,
	}, true, nil
}

func conversationParticipantSnapshotCorrupted(item map[string]types.AttributeValue) bool {
	conversationValue, ok := item[conversationSnapshotAttribute].(*types.AttributeValueMemberM)
	if !ok {
		return false
	}

	conversationID, _ := firstAttributeString(conversationValue.Value, "ID", "id")
	return strings.TrimSpace(conversationID) == ""
}

func participantSnapshotRepairConversationID(item map[string]types.AttributeValue) string {
	if gsi1PK, ok := attributeString(item["gsi1PK"]); ok && strings.HasPrefix(gsi1PK, conversationMetadataPartitionPrefix) {
		conversationID := strings.TrimSpace(strings.TrimPrefix(gsi1PK, conversationMetadataPartitionPrefix))
		if conversationID != "" {
			return conversationID
		}
	}

	sk, ok := attributeString(item["SK"])
	if !ok {
		return ""
	}
	idx := strings.LastIndex(sk, "#")
	if idx < 0 || idx+1 >= len(sk) {
		return ""
	}
	return strings.TrimSpace(sk[idx+1:])
}

func loadConversationMetadataItem(
	ctx context.Context,
	client conversationParticipantSnapshotMigrationClient,
	tableName string,
	conversationID string,
) (map[string]types.AttributeValue, error) {
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: conversationMetadataPartitionPrefix + conversationID},
			"SK": &types.AttributeValueMemberS{Value: conversationMetadataSortKey},
		},
	})
	if err != nil {
		return nil, err
	}
	return out.Item, nil
}

func rebuildConversationParticipantSnapshot(
	item map[string]types.AttributeValue,
	canonicalConversationItem map[string]types.AttributeValue,
) (map[string]types.AttributeValue, error) {
	conversationID, ok := firstAttributeString(canonicalConversationItem, "id", "ID")
	if !ok {
		return nil, fmt.Errorf("canonical conversation ID missing")
	}
	participants, ok := attributeStringSlice(canonicalConversationItem[conversationParticipantsAttribute])
	if !ok || len(participants) == 0 {
		return nil, fmt.Errorf("canonical participants missing")
	}

	unread, _ := attributeBool(item[conversationUnreadAttribute])
	createdAt, _ := attributeTime(canonicalConversationItem[conversationCreatedAtAttribute])
	updatedAt, _ := attributeTime(canonicalConversationItem[conversationUpdatedAtAttribute])
	totalMessageCount, _ := attributeInt64(canonicalConversationItem[conversationTotalMessageCountAttribute])
	lastMessageTime, _ := attributeTime(canonicalConversationItem[conversationLastMessageTimeAttribute])
	lastStatusID, _ := firstAttributeString(canonicalConversationItem, conversationLastStatusIDAttribute, "LastStatusID")

	snapshotValue, err := attributevalue.Marshal(models.ConversationSnapshot{
		ID:                conversationID,
		Participants:      participants,
		LastStatusID:      lastStatusID,
		Unread:            unread,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		TotalMessageCount: totalMessageCount,
		LastMessageTime:   lastMessageTime,
	})
	if err != nil {
		return nil, err
	}

	repairedItem := cloneAttributeMap(item)
	repairedItem[conversationSnapshotAttribute] = snapshotValue
	return repairedItem, nil
}

func appendConversationMigrationSample(samples *[]string, conversationID string) {
	if samples == nil || strings.TrimSpace(conversationID) == "" {
		return
	}
	for _, existing := range *samples {
		if existing == conversationID {
			return
		}
	}
	if len(*samples) >= 5 {
		return
	}
	*samples = append(*samples, conversationID)
}
