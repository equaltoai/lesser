package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// CollectionItemRecord represents a collection item stored in DynamoDB
type CollectionItemRecord struct {
	PK             string                  `dynamodbav:"PK"`     // COLLECTION#{collection}
	SK             string                  `dynamodbav:"SK"`     // ITEM#{itemID}
	GSI1PK         string                  `dynamodbav:"GSI1PK"` // ITEM#{itemID}
	GSI1SK         string                  `dynamodbav:"GSI1SK"` // COLLECTION#{collection}
	CollectionItem *storage.CollectionItem `dynamodbav:"CollectionItem"`
	CreatedAt      time.Time               `dynamodbav:"CreatedAt"`
	TTL            *int64                  `dynamodbav:"TTL,omitempty"` // Optional TTL for cleanup
}

// AddToCollection adds an item to a collection
func (s *dynamoDBStorage) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	log := common.WithContext(ctx).With(
		zap.String("collection", collection),
		zap.String("item_id", item.ItemID),
		zap.String("item_type", item.ItemType),
		zap.String("added_by", item.AddedBy),
	)

	item.Collection = collection
	item.AddedAt = time.Now()

	// Create the collection item record
	record := &CollectionItemRecord{
		PK:             fmt.Sprintf("COLLECTION#%s", collection),
		SK:             fmt.Sprintf("ITEM#%s", item.ItemID),
		GSI1PK:         fmt.Sprintf("ITEM#%s", item.ItemID),
		GSI1SK:         fmt.Sprintf("COLLECTION#%s", collection),
		CollectionItem: item,
		CreatedAt:      item.AddedAt,
	}

	itemData, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal collection item record", zap.Error(err))
		return fmt.Errorf("failed to marshal collection item record: %w", err)
	}

	// Use condition to prevent duplicate additions
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                itemData,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			log.Info("item already in collection",
				zap.String("item_id", item.ItemID),
				zap.String("collection", collection))
			return nil // Not an error to add something already in collection
		}
		log.Error("failed to add to collection", zap.Error(err))
		return fmt.Errorf("failed to add to collection: %w", err)
	}

	log.Info("item added to collection successfully")
	return nil
}

// RemoveFromCollection removes an item from a collection
func (s *dynamoDBStorage) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	log := common.WithContext(ctx).With(
		zap.String("collection", collection),
		zap.String("item_id", itemID),
	)

	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("COLLECTION#%s", collection)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ITEM#%s", itemID)},
		},
	})
	if err != nil {
		log.Error("failed to remove from collection", zap.Error(err))
		return fmt.Errorf("failed to remove from collection: %w", err)
	}

	log.Info("item removed from collection successfully")
	return nil
}

// GetCollectionItems retrieves items from a collection with pagination
func (s *dynamoDBStorage) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	log := common.WithContext(ctx).With(
		zap.String("collection", collection),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Build the query
	keyCondition := expression.Key("PK").Equal(expression.Value(fmt.Sprintf("COLLECTION#%s", collection)))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return nil, "", fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(int32(limit)),
		ScanIndexForward:          aws.Bool(false), // Most recent first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("COLLECTION#%s", collection)},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query collection items", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query collection items: %w", err)
	}

	items := make([]*storage.CollectionItem, 0, len(result.Items))
	for _, item := range result.Items {
		var record CollectionItemRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal collection item record", zap.Error(err))
			continue
		}
		items = append(items, record.CollectionItem)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	log.Info("retrieved collection items",
		zap.Int("count", len(items)),
		zap.Bool("has_more", nextCursor != ""),
	)

	return items, nextCursor, nil
}

// IsInCollection checks if an item is in a collection
func (s *dynamoDBStorage) IsInCollection(ctx context.Context, collection string, itemID string) (bool, error) {
	log := common.WithContext(ctx).With(
		zap.String("collection", collection),
		zap.String("item_id", itemID),
	)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("COLLECTION#%s", collection)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ITEM#%s", itemID)},
		},
	})
	if err != nil {
		log.Error("failed to check collection membership", zap.Error(err))
		return false, fmt.Errorf("failed to check collection membership: %w", err)
	}

	exists := result.Item != nil
	log.Info("checked collection membership", zap.Bool("exists", exists))
	return exists, nil
}

// CountCollectionItems returns the count of items in a collection
func (s *dynamoDBStorage) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	log := common.WithContext(ctx).With(zap.String("collection", collection))

	keyCondition := expression.Key("PK").Equal(expression.Value(fmt.Sprintf("COLLECTION#%s", collection)))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return 0, fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Select:                    types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to count collection items", zap.Error(err))
		return 0, fmt.Errorf("failed to count collection items: %w", err)
	}

	count := int(result.Count)
	log.Info("counted collection items", zap.Int("count", count))

	return count, nil
}
