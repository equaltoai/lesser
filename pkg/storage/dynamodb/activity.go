package dynamodb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// CreateActivity creates a new activity in DynamoDB
func (s *dynamoDBStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	if activity.ID == "" {
		return common.ValidationError{Field: "ID", Message: "activity ID is required"}
	}

	// Extract username from actor ID (e.g., "https://example.com/users/alice" -> "alice")
	username := extractUsernameFromActorID(activity.Actor)
	if username == "" {
		return common.ValidationError{Field: "Actor", Message: "invalid actor ID format"}
	}

	// Build the activity record
	now := time.Now()
	timestamp := now.Format(time.RFC3339Nano)

	record := storage.ActivityRecord{
		PK:        storage.ActorPKPrefix + username,
		SK:        storage.ActivitySKPrefix + timestamp + "#" + activity.ID,
		Activity:  activity,
		CreatedAt: now,
	}

	// If this is an inbox activity (someone else's activity delivered to us), set GSI keys
	if isInboxActivity(activity, username) {
		record.GSI1PK = storage.InboxGSI1PKPrefix + username
		record.GSI1SK = timestamp
	}

	// Marshal the record to DynamoDB attributes
	item, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal activity record",
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to marshal activity record: %w", err)
	}

	// Put the item
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})

	if err != nil {
		log.Error("failed to create activity",
			zap.String("activity_id", activity.ID),
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to create activity: %w", err)
	}

	log.Info("activity created successfully",
		zap.String("activity_id", activity.ID),
		zap.String("username", username),
		zap.String("type", activity.Type))

	return nil
}

// GetActivity retrieves an activity by ID
func (s *dynamoDBStorage) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	log := common.WithContext(ctx)

	// We need to scan for the activity since we don't know the username
	// In a production system, you might want to extract username from the ID
	// or maintain a separate GSI for activity lookups

	expr, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal activity ID: %w", err)
	}

	result, err := s.client.Scan(ctx, &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("Activity.#id = :id"),
		ExpressionAttributeNames: map[string]string{
			"#id": "ID",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": expr,
		},
		Limit: aws.Int32(1),
	})

	if err != nil {
		log.Error("failed to scan for activity",
			zap.String("activity_id", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get activity: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, common.ActivityNotFoundError{ID: id}
	}

	// Unmarshal the first item
	var record storage.ActivityRecord
	if err := s.UnmarshalItem(result.Items[0], &record); err != nil {
		log.Error("failed to unmarshal activity",
			zap.String("activity_id", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal activity: %w", err)
	}

	return record.Activity, nil
}

// GetOutboxActivities retrieves activities created by a user (outbox)
func (s *dynamoDBStorage) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	log := common.WithContext(ctx)

	// Set default limit if not specified
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Build the query input
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: storage.ActorPKPrefix + username},
			":sk_prefix": &types.AttributeValueMemberS{Value: storage.ActivitySKPrefix},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	// If cursor provided, decode and set as exclusive start key
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			log.Warn("invalid cursor provided",
				zap.String("cursor", cursor),
				zap.Error(err))
			// Continue without cursor
		} else {
			queryInput.ExclusiveStartKey = startKey
		}
	}

	// Execute the query
	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		log.Error("failed to query outbox activities",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query outbox activities: %w", err)
	}

	// Unmarshal the activities
	activities := make([]*activitypub.Activity, 0, len(result.Items))
	for _, item := range result.Items {
		var record storage.ActivityRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal activity record",
				zap.String("username", username),
				zap.Error(err))
			continue // Skip this item
		}
		activities = append(activities, record.Activity)
	}

	// Encode next cursor if there are more results
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(result.LastEvaluatedKey)
	}

	log.Debug("retrieved outbox activities",
		zap.String("username", username),
		zap.Int("count", len(activities)),
		zap.Bool("has_more", nextCursor != ""))

	return activities, nextCursor, nil
}

// GetInboxActivities retrieves activities delivered to a user (inbox)
func (s *dynamoDBStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	log := common.WithContext(ctx)

	// Set default limit if not specified
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Query using GSI1 for inbox activities
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :gsi1pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gsi1pk": &types.AttributeValueMemberS{Value: storage.InboxGSI1PKPrefix + username},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	// If cursor provided, decode and set as exclusive start key
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			log.Warn("invalid cursor provided",
				zap.String("cursor", cursor),
				zap.Error(err))
			// Continue without cursor
		} else {
			queryInput.ExclusiveStartKey = startKey
		}
	}

	// Execute the query
	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		log.Error("failed to query inbox activities",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query inbox activities: %w", err)
	}

	// Unmarshal the activities
	activities := make([]*activitypub.Activity, 0, len(result.Items))
	for _, item := range result.Items {
		var record storage.ActivityRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal activity record",
				zap.String("username", username),
				zap.Error(err))
			continue // Skip this item
		}
		activities = append(activities, record.Activity)
	}

	// Encode next cursor if there are more results
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(result.LastEvaluatedKey)
	}

	log.Debug("retrieved inbox activities",
		zap.String("username", username),
		zap.Int("count", len(activities)),
		zap.Bool("has_more", nextCursor != ""))

	return activities, nextCursor, nil
}

// Helper functions

// extractUsernameFromActorID extracts username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsernameFromActorID(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// isInboxActivity determines if an activity should be stored in the inbox
// An activity is an inbox activity if the actor is different from the local user
func isInboxActivity(activity *activitypub.Activity, localUsername string) bool {
	actorUsername := extractUsernameFromActorID(activity.Actor)
	return actorUsername != localUsername
}

// encodeCursor encodes a DynamoDB LastEvaluatedKey to a string cursor
func encodeCursor(lastKey map[string]types.AttributeValue) string {
	data, err := json.Marshal(lastKey)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor decodes a string cursor to a DynamoDB ExclusiveStartKey
func decodeCursor(cursor string) (map[string]types.AttributeValue, error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor format: %w", err)
	}

	var key map[string]types.AttributeValue
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	return key, nil
}
