package dynamodb

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// LikeRecord represents a Like stored in DynamoDB
type LikeRecord struct {
	PK        string        `dynamodbav:"PK"`
	SK        string        `dynamodbav:"SK"`
	GSI3PK    string        `dynamodbav:"GSI3PK"` // For actor's likes index
	GSI3SK    string        `dynamodbav:"GSI3SK"`
	Like      *storage.Like `dynamodbav:"Like"`
	CreatedAt time.Time     `dynamodbav:"CreatedAt"`
}

// CreateLike creates a new Like activity in DynamoDB
func (s *dynamoDBStorage) CreateLike(ctx context.Context, like *storage.Like) error {
	if like.Actor == "" || like.Object == "" {
		return errors.New("actor and object are required")
	}

	// Generate activity ID if not provided
	if like.ID == "" {
		like.ID = fmt.Sprintf("%s/activities/like-%d-%s",
			like.Actor,
			time.Now().Unix(),
			generateRandomID(8))
	}

	// Set timestamps
	if like.Published.IsZero() {
		like.Published = time.Now()
	}
	like.CreatedAt = time.Now()

	// Create the DynamoDB record
	record := &LikeRecord{
		PK:        fmt.Sprintf("OBJECT#%s#LIKES", like.Object),
		SK:        fmt.Sprintf("ACTOR#%s", like.Actor),
		GSI3PK:    fmt.Sprintf("ACTOR#%s#LIKES", like.Actor),
		GSI3SK:    fmt.Sprintf("PUBLISHED#%s#OBJECT#%s", like.Published.Format(time.RFC3339), like.Object),
		Like:      like,
		CreatedAt: like.CreatedAt,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal like: %w", err)
	}

	// Put with condition that the item doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			return fmt.Errorf("actor %s already liked object %s", like.Actor, like.Object)
		}
		return fmt.Errorf("failed to create like: %w", err)
	}

	return nil
}

// GetLike retrieves a specific Like by actor and object
func (s *dynamoDBStorage) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#LIKES", object)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actor)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get like: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("like not found for actor %s on object %s", actor, object)
	}

	var record LikeRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal like: %w", err)
	}

	return record.Like, nil
}

// DeleteLike removes a Like activity
func (s *dynamoDBStorage) DeleteLike(ctx context.Context, actor, object string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#LIKES", object)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actor)},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete like: %w", err)
	}

	return nil
}

// GetObjectLikes retrieves all likes for a specific object with pagination
func (s *dynamoDBStorage) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Like, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#LIKES", objectID)},
		},
		Limit:            aws.Int32(int32(limit)),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	// Add cursor if provided
	if cursor != "" {
		decodedCursor, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil && len(decodedCursor) > 0 {
			input.ExclusiveStartKey = map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#LIKES", objectID)},
				"SK": &types.AttributeValueMemberS{Value: string(decodedCursor)},
			}
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query object likes: %w", err)
	}

	likes := make([]*storage.Like, 0, len(result.Items))
	for _, item := range result.Items {
		var record LikeRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			continue
		}
		likes = append(likes, record.Like)
	}

	// Generate next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			if s, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = base64.URLEncoding.EncodeToString([]byte(s.Value))
			}
		}
	}

	return likes, nextCursor, nil
}

// GetActorLikes retrieves all objects liked by a specific actor with pagination
func (s *dynamoDBStorage) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Like, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#LIKES", actorID)},
		},
		Limit:            aws.Int32(int32(limit)),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	// Add cursor if provided
	if cursor != "" {
		decodedCursor, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil && len(decodedCursor) > 0 {
			input.ExclusiveStartKey = map[string]types.AttributeValue{
				"GSI3PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#LIKES", actorID)},
				"GSI3SK": &types.AttributeValueMemberS{Value: string(decodedCursor)},
			}
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query actor likes: %w", err)
	}

	likes := make([]*storage.Like, 0, len(result.Items))
	for _, item := range result.Items {
		var record LikeRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			continue
		}
		likes = append(likes, record.Like)
	}

	// Generate next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI3SK"]; ok {
			if s, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = base64.URLEncoding.EncodeToString([]byte(s.Value))
			}
		}
	}

	return likes, nextCursor, nil
}

// CountObjectLikes returns the total number of likes for an object
func (s *dynamoDBStorage) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#LIKES", objectID)},
		},
		Select: types.SelectCount,
	}

	var totalCount int
	for {
		result, err := s.client.Query(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("failed to count object likes: %w", err)
		}

		totalCount += int(result.Count)

		// Check if there are more results
		if result.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}

	return totalCount, nil
}

// generateRandomID generates a random string of specified length
func generateRandomID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
