package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// blockRecord represents a block in DynamoDB
type blockRecord struct {
	PK        string    `dynamodbav:"PK"`
	SK        string    `dynamodbav:"SK"`
	Type      string    `dynamodbav:"Type"`
	Actor     string    `dynamodbav:"Actor"`
	Object    string    `dynamodbav:"Object"`
	ID        string    `dynamodbav:"ID"`
	Published time.Time `dynamodbav:"Published"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
	// For reverse lookup - who blocked this actor
	GSI5PK string `dynamodbav:"GSI5PK"`
	GSI5SK string `dynamodbav:"GSI5SK"`
}

// extractUsername extracts the username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsername(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return actorID
}

// CreateBlock creates a new block relationship
func (s *dynamoDBStorage) CreateBlock(ctx context.Context, block *storage.Block) error {
	log := common.WithContext(ctx)

	// Generate ID if not provided
	if block.ID == "" {
		block.ID = fmt.Sprintf("%s/activities/block-%d", block.Actor, time.Now().Unix())
	}

	// Set timestamps
	if block.Published.IsZero() {
		block.Published = time.Now().UTC()
	}
	block.CreatedAt = time.Now().UTC()

	// Create the block record
	record := blockRecord{
		PK:        fmt.Sprintf("ACTOR#%s#BLOCKS", extractUsername(block.Actor)),
		SK:        fmt.Sprintf("BLOCKED#%s", extractUsername(block.Object)),
		Type:      "Block",
		Actor:     block.Actor,
		Object:    block.Object,
		ID:        block.ID,
		Published: block.Published,
		CreatedAt: block.CreatedAt,
		// For reverse lookup - to find who blocked this actor
		GSI5PK: fmt.Sprintf("BLOCKED#%s", extractUsername(block.Object)),
		GSI5SK: fmt.Sprintf("BLOCKER#%s", extractUsername(block.Actor)),
	}

	item, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %w", err)
	}

	// Use conditional expression to prevent duplicate blocks
	input := &dynamodb.PutItemInput{
		TableName:           &s.tableName,
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			log.Info("block already exists",
				zap.String("actor", block.Actor),
				zap.String("blocked", block.Object))
			return fmt.Errorf("block already exists")
		}
		return fmt.Errorf("failed to create block: %w", err)
	}

	log.Info("block created",
		zap.String("actor", block.Actor),
		zap.String("blocked", block.Object))

	return nil
}

// GetBlock retrieves a specific block relationship
func (s *dynamoDBStorage) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	input := &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#BLOCKS", extractUsername(actor))},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("BLOCKED#%s", extractUsername(blockedActor))},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("block not found")
	}

	var record blockRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &storage.Block{
		Actor:     record.Actor,
		Object:    record.Object,
		ID:        record.ID,
		Published: record.Published,
		CreatedAt: record.CreatedAt,
	}, nil
}

// DeleteBlock removes a block relationship
func (s *dynamoDBStorage) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#BLOCKS", extractUsername(actor))},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("BLOCKED#%s", extractUsername(blockedActor))},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete block: %w", err)
	}

	return nil
}

// GetBlockedActors returns a paginated list of actors blocked by the given actor
func (s *dynamoDBStorage) GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	log := common.WithContext(ctx)

	// Build key condition
	keyCondition := expression.Key("PK").Equal(expression.Value(
		fmt.Sprintf("ACTOR#%s#BLOCKS", extractUsername(actor)),
	))

	// Build the expression
	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		return nil, "", fmt.Errorf("failed to build expression: %w", err)
	}

	// Build query input
	input := &dynamodb.QueryInput{
		TableName:                 &s.tableName,
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     safeInt32(limit),
	}

	// Add cursor if provided
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			log.Warn("invalid cursor", zap.Error(err))
		} else {
			input.ExclusiveStartKey = startKey
		}
	}

	// Execute query
	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query blocks: %w", err)
	}

	// Unmarshal results
	blocks := make([]*storage.Block, 0, len(result.Items))
	for _, item := range result.Items {
		var record blockRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal block", zap.Error(err))
			continue
		}

		blocks = append(blocks, &storage.Block{
			Actor:     record.Actor,
			Object:    record.Object,
			ID:        record.ID,
			Published: record.Published,
			CreatedAt: record.CreatedAt,
		})
	}

	// Generate next cursor
	nextCursor := ""
	if result.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(result.LastEvaluatedKey)
	}

	log.Info("retrieved blocked actors",
		zap.String("actor", actor),
		zap.Int("count", len(blocks)),
		zap.Bool("has_more", nextCursor != ""))

	return blocks, nextCursor, nil
}

// GetBlockedByActors returns a paginated list of actors who have blocked the given actor
func (s *dynamoDBStorage) GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	log := common.WithContext(ctx)

	// Build key condition using GSI5
	keyCondition := expression.Key("GSI5PK").Equal(expression.Value(
		fmt.Sprintf("BLOCKED#%s", extractUsername(actor)),
	))

	// Build the expression
	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		return nil, "", fmt.Errorf("failed to build expression: %w", err)
	}

	// Build query input
	input := &dynamodb.QueryInput{
		TableName:                 &s.tableName,
		IndexName:                 aws.String("GSI5"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     safeInt32(limit),
	}

	// Add cursor if provided
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			log.Warn("invalid cursor", zap.Error(err))
		} else {
			input.ExclusiveStartKey = startKey
		}
	}

	// Execute query
	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query blocks: %w", err)
	}

	// Unmarshal results
	blocks := make([]*storage.Block, 0, len(result.Items))
	for _, item := range result.Items {
		var record blockRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal block", zap.Error(err))
			continue
		}

		blocks = append(blocks, &storage.Block{
			Actor:     record.Actor,
			Object:    record.Object,
			ID:        record.ID,
			Published: record.Published,
			CreatedAt: record.CreatedAt,
		})
	}

	// Generate next cursor
	nextCursor := ""
	if result.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(result.LastEvaluatedKey)
	}

	log.Info("retrieved actors who blocked",
		zap.String("blocked_actor", actor),
		zap.Int("count", len(blocks)),
		zap.Bool("has_more", nextCursor != ""))

	return blocks, nextCursor, nil
}

// IsBlocked checks if targetActor is blocked by actor
func (s *dynamoDBStorage) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	_, err := s.GetBlock(ctx, actor, targetActor)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsBlockedBidirectional checks if either actor has blocked the other
func (s *dynamoDBStorage) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	// Check if actor1 blocked actor2
	blocked1to2, err := s.IsBlocked(ctx, actor1, actor2)
	if err != nil {
		return false, err
	}
	if blocked1to2 {
		return true, nil
	}

	// Check if actor2 blocked actor1
	blocked2to1, err := s.IsBlocked(ctx, actor2, actor1)
	if err != nil {
		return false, err
	}

	return blocked2to1, nil
}
