package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// BookmarkRecord represents a bookmark stored in DynamoDB
type BookmarkRecord struct {
	PK        string    `dynamodbav:"PK"`        // BOOKMARK#username
	SK        string    `dynamodbav:"SK"`        // timestamp#objectID
	ObjectID  string    `dynamodbav:"ObjectID"`  // The bookmarked object ID
	CreatedAt time.Time `dynamodbav:"CreatedAt"` // When it was bookmarked
}

// CreateBookmark adds a bookmark for a user
func (s *dynamoDBStorage) CreateBookmark(ctx context.Context, username, objectID string) error {
	log := common.WithContext(ctx)

	// Create the bookmark record
	record := &BookmarkRecord{
		PK:        fmt.Sprintf("BOOKMARK#%s", username),
		SK:        fmt.Sprintf("%s#%s", time.Now().Format(time.RFC3339Nano), objectID),
		ObjectID:  objectID,
		CreatedAt: time.Now(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal bookmark",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to marshal bookmark: %w", err)
	}

	// Put with condition that the same bookmark doesn't exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			// Already bookmarked, not an error
			return nil
		}
		log.Error("failed to create bookmark",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to create bookmark: %w", err)
	}

	log.Info("bookmark created successfully",
		zap.String("username", username),
		zap.String("object_id", objectID))

	return nil
}

// RemoveBookmark removes a bookmark for a user
func (s *dynamoDBStorage) RemoveBookmark(ctx context.Context, username, objectID string) error {
	log := common.WithContext(ctx)

	// Query to find the bookmark with the specific objectID
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		FilterExpression:       aws.String("ObjectID = :oid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":  &types.AttributeValueMemberS{Value: fmt.Sprintf("BOOKMARK#%s", username)},
			":oid": &types.AttributeValueMemberS{Value: objectID},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		log.Error("failed to query bookmark",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to query bookmark: %w", err)
	}

	// Delete the bookmark if found
	for _, item := range result.Items {
		pk := item["PK"]
		sk := item["SK"]

		_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: s.getTableName(),
			Key: map[string]types.AttributeValue{
				"PK": pk,
				"SK": sk,
			},
		})
		if err != nil {
			log.Error("failed to delete bookmark",
				zap.String("username", username),
				zap.String("object_id", objectID),
				zap.Error(err))
			return fmt.Errorf("failed to delete bookmark: %w", err)
		}
	}

	log.Info("bookmark removed successfully",
		zap.String("username", username),
		zap.String("object_id", objectID))

	return nil
}

// GetBookmarks retrieves bookmarks for a user with pagination
func (s *dynamoDBStorage) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	log := common.WithContext(ctx)

	// Validate limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Build query input
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("BOOKMARK#%s", username)},
		},
		Limit:            aws.Int32(int32(limit + 1)), // Request one extra to determine if there's a next page
		ScanIndexForward: aws.Bool(false),             // Newest first
	}

	// Add cursor if provided
	if cursor != "" {
		lastEvaluatedKey := map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("BOOKMARK#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
		queryInput.ExclusiveStartKey = lastEvaluatedKey
	}

	// Execute query
	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		log.Error("failed to query bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query bookmarks: %w", err)
	}

	// Extract object IDs
	bookmarks := make([]string, 0, len(result.Items))
	for i, item := range result.Items {
		// Skip the extra item used for pagination
		if i >= limit {
			break
		}

		var record BookmarkRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			log.Error("failed to unmarshal bookmark record",
				zap.Error(err))
			continue
		}

		bookmarks = append(bookmarks, record.ObjectID)
	}

	// Determine next cursor
	var nextCursor string
	if len(result.Items) > limit && len(bookmarks) > 0 {
		// Use the SK of the last returned item as cursor
		lastItem := result.Items[limit-1]
		if sk, ok := lastItem["SK"].(*types.AttributeValueMemberS); ok {
			nextCursor = sk.Value
		}
	}

	return bookmarks, nextCursor, nil
}

// IsBookmarked checks if a user has bookmarked an object
func (s *dynamoDBStorage) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	// Query to find the bookmark
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		FilterExpression:       aws.String("ObjectID = :oid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":  &types.AttributeValueMemberS{Value: fmt.Sprintf("BOOKMARK#%s", username)},
			":oid": &types.AttributeValueMemberS{Value: objectID},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return false, fmt.Errorf("failed to query bookmark: %w", err)
	}

	return len(result.Items) > 0, nil
}
