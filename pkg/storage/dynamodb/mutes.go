package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// MuteRecord represents a mute relationship in DynamoDB
type MuteRecord struct {
	PK     string `dynamodbav:"PK"`
	SK     string `dynamodbav:"SK"`
	GSI1PK string `dynamodbav:"GSI1PK"`
	GSI1SK string `dynamodbav:"GSI1SK"`
	storage.Mute
}

// CreateMute creates a new mute relationship
func (s *dynamoDBStorage) CreateMute(ctx context.Context, mute *storage.Mute) error {
	s.logger().Info("creating mute", zap.String("actor", mute.Actor), zap.String("muted", mute.Object))

	// Set timestamps if not provided
	if mute.CreatedAt.IsZero() {
		mute.CreatedAt = time.Now()
	}
	if mute.Published.IsZero() {
		mute.Published = time.Now()
	}

	record := MuteRecord{
		PK:     fmt.Sprintf("MUTE#%s", mute.Actor),
		SK:     fmt.Sprintf("MUTED#%s", mute.Object),
		GSI1PK: fmt.Sprintf("MUTED#%s", mute.Object),
		GSI1SK: fmt.Sprintf("MUTER#%s", mute.Actor),
		Mute:   *mute,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal mute: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return fmt.Errorf("mute already exists")
		}
		return fmt.Errorf("failed to create mute: %w", err)
	}

	return nil
}

// GetMute retrieves a mute relationship
func (s *dynamoDBStorage) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTE#%s", actor)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTED#%s", mutedActor)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get mute: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var record MuteRecord
	err = s.UnmarshalItem(result.Item, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal mute: %w", err)
	}

	return &record.Mute, nil
}

// DeleteMute removes a mute relationship
func (s *dynamoDBStorage) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTE#%s", actor)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTED#%s", mutedActor)},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete mute: %w", err)
	}

	return nil
}

// GetMutedActors retrieves all actors muted by the given actor
func (s *dynamoDBStorage) GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	if limit == 0 {
		limit = 20
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTE#%s", actor)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	if cursor != "" {
		lastEvaluatedKey, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		input.ExclusiveStartKey = lastEvaluatedKey
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query muted actors: %w", err)
	}

	mutes := make([]*storage.Mute, 0, len(result.Items))
	for _, item := range result.Items {
		var record MuteRecord
		err = s.UnmarshalItem(item, &record)
		if err != nil {
			s.logger().Error("failed to unmarshal mute record", zap.Error(err))
			continue
		}
		mutes = append(mutes, &record.Mute)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(result.LastEvaluatedKey)
	}

	return mutes, nextCursor, nil
}

// IsMuted checks if targetActor is muted by actor
func (s *dynamoDBStorage) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	mute, err := s.GetMute(ctx, actor, targetActor)
	if err != nil {
		return false, err
	}
	return mute != nil, nil
}
