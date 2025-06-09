package dynamodb

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// AnnounceRecord represents an Announce stored in DynamoDB
type AnnounceRecord struct {
	PK        string            `dynamodbav:"PK"`
	SK        string            `dynamodbav:"SK"`
	GSI4PK    string            `dynamodbav:"GSI4PK"` // For actor's announces index
	GSI4SK    string            `dynamodbav:"GSI4SK"`
	Announce  *storage.Announce `dynamodbav:"Announce"`
	CreatedAt time.Time         `dynamodbav:"CreatedAt"`
}

// CreateAnnounce creates a new Announce activity in DynamoDB
func (s *dynamoDBStorage) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	if announce.Actor == "" || announce.Object == "" {
		return errors.New("actor and object are required")
	}

	// Generate activity ID if not provided
	if announce.ID == "" {
		announce.ID = fmt.Sprintf("%s/activities/announce-%d-%s",
			announce.Actor,
			time.Now().Unix(),
			generateRandomID(8))
	}

	// Set timestamps
	if announce.Published.IsZero() {
		announce.Published = time.Now()
	}
	announce.CreatedAt = time.Now()

	// Create the DynamoDB record
	record := &AnnounceRecord{
		PK:        fmt.Sprintf("OBJECT#%s#ANNOUNCES", announce.Object),
		SK:        fmt.Sprintf("ACTOR#%s", announce.Actor),
		GSI4PK:    fmt.Sprintf("ACTOR#%s#ANNOUNCES", announce.Actor),
		GSI4SK:    fmt.Sprintf("PUBLISHED#%s#OBJECT#%s", announce.Published.Format(time.RFC3339), announce.Object),
		Announce:  announce,
		CreatedAt: announce.CreatedAt,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal announce: %w", err)
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
			return fmt.Errorf("actor %s already announced object %s", announce.Actor, announce.Object)
		}
		return fmt.Errorf("failed to create announce: %w", err)
	}

	return nil
}

// GetAnnounce retrieves a specific Announce by actor and object
func (s *dynamoDBStorage) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#ANNOUNCES", object)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actor)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get announce: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("announce not found for actor %s on object %s", actor, object)
	}

	var record AnnounceRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal announce: %w", err)
	}

	return record.Announce, nil
}

// DeleteAnnounce removes an Announce activity
func (s *dynamoDBStorage) DeleteAnnounce(ctx context.Context, actor, object string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#ANNOUNCES", object)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actor)},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete announce: %w", err)
	}

	return nil
}

// GetObjectAnnounces retrieves all announces for a specific object with pagination
func (s *dynamoDBStorage) GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#ANNOUNCES", objectID)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	// Add cursor if provided
	if cursor != "" {
		decodedCursor, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil && len(decodedCursor) > 0 {
			input.ExclusiveStartKey = map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#ANNOUNCES", objectID)},
				"SK": &types.AttributeValueMemberS{Value: string(decodedCursor)},
			}
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query object announces: %w", err)
	}

	announces := make([]*storage.Announce, 0, len(result.Items))
	for _, item := range result.Items {
		var record AnnounceRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			continue
		}
		announces = append(announces, record.Announce)
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

	return announces, nextCursor, nil
}

// GetActorAnnounces retrieves all objects announced by a specific actor with pagination
func (s *dynamoDBStorage) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI4"),
		KeyConditionExpression: aws.String("GSI4PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#ANNOUNCES", actorID)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	// Add cursor if provided
	if cursor != "" {
		decodedCursor, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil && len(decodedCursor) > 0 {
			input.ExclusiveStartKey = map[string]types.AttributeValue{
				"GSI4PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#ANNOUNCES", actorID)},
				"GSI4SK": &types.AttributeValueMemberS{Value: string(decodedCursor)},
			}
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query actor announces: %w", err)
	}

	announces := make([]*storage.Announce, 0, len(result.Items))
	for _, item := range result.Items {
		var record AnnounceRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			continue
		}
		announces = append(announces, record.Announce)
	}

	// Generate next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI4SK"]; ok {
			if s, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = base64.URLEncoding.EncodeToString([]byte(s.Value))
			}
		}
	}

	return announces, nextCursor, nil
}

// CountObjectAnnounces returns the total number of announces for an object
func (s *dynamoDBStorage) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#ANNOUNCES", objectID)},
		},
		Select: types.SelectCount,
	}

	var totalCount int
	for {
		result, err := s.client.Query(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("failed to count object announces: %w", err)
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
