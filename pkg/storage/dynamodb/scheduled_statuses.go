package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ScheduledStatusRecord represents a scheduled status in DynamoDB
type ScheduledStatusRecord struct {
	PK     string `dynamodbav:"PK"`
	SK     string `dynamodbav:"SK"`
	GSI1PK string `dynamodbav:"GSI1PK,omitempty"` // For querying by scheduled time
	GSI1SK string `dynamodbav:"GSI1SK,omitempty"`
	storage.ScheduledStatus
}

// CreateScheduledStatus creates a new scheduled status
func (s *dynamoDBStorage) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	s.logger().Info("creating scheduled status",
		zap.String("username", scheduled.Username),
		zap.Time("scheduled_at", scheduled.ScheduledAt))

	// Generate ID if not provided
	if scheduled.ID == "" {
		scheduled.ID = uuid.New().String()
	}

	// Set timestamps
	if scheduled.CreatedAt.IsZero() {
		scheduled.CreatedAt = time.Now()
	}
	if scheduled.UpdatedAt.IsZero() {
		scheduled.UpdatedAt = scheduled.CreatedAt
	}

	record := ScheduledStatusRecord{
		PK:              fmt.Sprintf("USER#%s#SCHEDULED", scheduled.Username),
		SK:              fmt.Sprintf("ID#%s", scheduled.ID),
		GSI1PK:          "SCHEDULED#DUE",
		GSI1SK:          fmt.Sprintf("TIME#%s#ID#%s", scheduled.ScheduledAt.Format(time.RFC3339Nano), scheduled.ID),
		ScheduledStatus: *scheduled,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduled status: %w", err)
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
			return fmt.Errorf("scheduled status already exists")
		}
		return fmt.Errorf("failed to create scheduled status: %w", err)
	}

	return nil
}

// GetScheduledStatus retrieves a scheduled status by ID
func (s *dynamoDBStorage) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	// We need to scan since we don't know the username
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("contains(SK, :sk) AND begins_with(PK, :pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ID#%s", id)},
			":pk": &types.AttributeValueMemberS{Value: "USER#"},
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled status: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("scheduled status not found")
	}

	var record ScheduledStatusRecord
	err = s.UnmarshalItem(result.Items[0], &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal scheduled status: %w", err)
	}

	return &record.ScheduledStatus, nil
}

// GetScheduledStatuses retrieves scheduled statuses for a user
func (s *dynamoDBStorage) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#SCHEDULED", username)},
		},
		Limit: safeInt32(limit),
	}

	// Add cursor if provided
	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#SCHEDULED", username)},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query scheduled statuses: %w", err)
	}

	statuses := make([]*storage.ScheduledStatus, 0, len(result.Items))
	for _, item := range result.Items {
		var record ScheduledStatusRecord
		err := s.UnmarshalItem(item, &record)
		if err != nil {
			s.logger().Error("failed to unmarshal scheduled status", zap.Error(err))
			continue
		}
		// Skip published statuses
		if !record.Published {
			statuses = append(statuses, &record.ScheduledStatus)
		}
	}

	// Determine next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			nextCursor = sk.(*types.AttributeValueMemberS).Value
		}
	}

	return statuses, nextCursor, nil
}

// UpdateScheduledStatus updates a scheduled status
func (s *dynamoDBStorage) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	// First get the existing status to find the username
	existing, err := s.GetScheduledStatus(ctx, scheduled.ID)
	if err != nil {
		return err
	}

	// Preserve username if not set
	if scheduled.Username == "" {
		scheduled.Username = existing.Username
	}

	// Update timestamp
	scheduled.UpdatedAt = time.Now()

	// Build update expression
	update := expression.Set(expression.Name("ScheduledStatus"), expression.Value(scheduled))

	// Update GSI keys if scheduled time changed
	if !scheduled.ScheduledAt.Equal(existing.ScheduledAt) {
		update = update.Set(expression.Name("GSI1SK"),
			expression.Value(fmt.Sprintf("TIME#%s#ID#%s", scheduled.ScheduledAt.Format(time.RFC3339Nano), scheduled.ID)))
	}

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#SCHEDULED", scheduled.Username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ID#%s", scheduled.ID)},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update scheduled status: %w", err)
	}

	return nil
}

// DeleteScheduledStatus deletes a scheduled status
func (s *dynamoDBStorage) DeleteScheduledStatus(ctx context.Context, id string) error {
	// First get the status to find the username
	status, err := s.GetScheduledStatus(ctx, id)
	if err != nil {
		return err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#SCHEDULED", status.Username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ID#%s", id)},
		},
	}

	_, err = s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete scheduled status: %w", err)
	}

	return nil
}

// GetDueScheduledStatuses retrieves scheduled statuses that are due to be published
func (s *dynamoDBStorage) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK < :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "SCHEDULED#DUE"},
			":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TIME#%s", before.Format(time.RFC3339Nano))},
		},
		Limit: safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query due scheduled statuses: %w", err)
	}

	statuses := make([]*storage.ScheduledStatus, 0, len(result.Items))
	for _, item := range result.Items {
		var record ScheduledStatusRecord
		err := s.UnmarshalItem(item, &record)
		if err != nil {
			s.logger().Error("failed to unmarshal scheduled status", zap.Error(err))
			continue
		}
		// Only include unpublished statuses
		if !record.Published {
			statuses = append(statuses, &record.ScheduledStatus)
		}
	}

	return statuses, nil
}

// MarkScheduledStatusPublished marks a scheduled status as published
func (s *dynamoDBStorage) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	// First get the status to find the username
	status, err := s.GetScheduledStatus(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	update := expression.Set(expression.Name("ScheduledStatus.Published"), expression.Value(true)).
		Set(expression.Name("ScheduledStatus.PublishedAt"), expression.Value(now))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#SCHEDULED", status.Username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ID#%s", id)},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to mark scheduled status as published: %w", err)
	}

	return nil
}
