package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// StatusPinRecord represents a status pin in DynamoDB
type StatusPinRecord struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
	storage.StatusPin
}

// CreateStatusPin creates a new status pin
func (s *dynamoDBStorage) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	s.logger().Info("creating status pin",
		zap.String("username", pin.Username),
		zap.String("status_id", pin.StatusID))

	// Check if user already has too many pinned statuses (Mastodon limit is typically 5)
	count, err := s.CountUserPinnedStatuses(ctx, pin.Username)
	if err != nil {
		return fmt.Errorf("failed to count pinned statuses: %w", err)
	}
	if count >= 5 {
		return fmt.Errorf("too many pinned statuses (maximum 5)")
	}

	// Set timestamp if not provided
	if pin.CreatedAt.IsZero() {
		pin.CreatedAt = time.Now()
	}

	record := StatusPinRecord{
		PK:        fmt.Sprintf("USER#%s#PINS", pin.Username),
		SK:        fmt.Sprintf("STATUS#%s", pin.StatusID),
		StatusPin: *pin,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal status pin: %w", err)
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
			return fmt.Errorf("status already pinned")
		}
		return fmt.Errorf("failed to create status pin: %w", err)
	}

	return nil
}

// DeleteStatusPin removes a status pin
func (s *dynamoDBStorage) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#PINS", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete status pin: %w", err)
	}

	return nil
}

// GetStatusPins retrieves all pinned statuses for a user
func (s *dynamoDBStorage) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#PINS", username)},
		},
	}

	var pins []*storage.StatusPin
	paginator := dynamodb.NewQueryPaginator(s.client, input)

	for paginator.HasMorePages() {
		result, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query status pins: %w", err)
		}

		for _, item := range result.Items {
			var record StatusPinRecord
			err := attributevalue.UnmarshalMap(item, &record)
			if err != nil {
				s.logger().Error("failed to unmarshal status pin", zap.Error(err))
				continue
			}
			pins = append(pins, &record.StatusPin)
		}
	}

	return pins, nil
}

// IsStatusPinned checks if a status is pinned by a user
func (s *dynamoDBStorage) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#PINS", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check status pin: %w", err)
	}

	return result.Item != nil, nil
}

// CountUserPinnedStatuses counts how many statuses a user has pinned
func (s *dynamoDBStorage) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#PINS", username)},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to count pinned statuses: %w", err)
	}

	return int(result.Count), nil
}
