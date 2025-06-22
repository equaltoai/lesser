package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// GetNotificationsAdvanced retrieves notifications with advanced filtering
func (s *dynamoDBStorage) GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*storage.Notification, error) {
	// Build query for user notifications
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTIFICATIONS#%s", userID)},
		},
		ScanIndexForward: aws.Bool(false),             // Recent first
		Limit:            aws.Int32(int32(limit * 2)), // Get more for filtering
	}

	// Add range conditions for pagination
	if maxID != nil || minID != nil {
		var keyCondition string
		keyCondition = "PK = :pk"

		if maxID != nil {
			keyCondition += " AND SK < :maxSK"
			input.ExpressionAttributeValues[":maxSK"] = &types.AttributeValueMemberS{Value: *maxID}
		}

		if minID != nil {
			keyCondition += " AND SK > :minSK"
			input.ExpressionAttributeValues[":minSK"] = &types.AttributeValueMemberS{Value: *minID}
		}

		input.KeyConditionExpression = aws.String(keyCondition)
	}

	// Add filter expression for excluded types
	if len(excludeTypes) > 0 {
		filterExpr := "NOT #type IN ("
		for i, excludeType := range excludeTypes {
			key := fmt.Sprintf(":exclude%d", i)
			filterExpr += key
			if i < len(excludeTypes)-1 {
				filterExpr += ", "
			}
			input.ExpressionAttributeValues[key] = &types.AttributeValueMemberS{Value: excludeType}
		}
		filterExpr += ")"

		input.FilterExpression = aws.String(filterExpr)
		if input.ExpressionAttributeNames == nil {
			input.ExpressionAttributeNames = make(map[string]string)
		}
		input.ExpressionAttributeNames["#type"] = "Type"
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}

	// Convert to notification objects
	notifications := make([]*storage.Notification, 0)
	for _, item := range result.Items {
		var notificationRecord NotificationRecord
		if err := s.UnmarshalItem(item, &notificationRecord); err != nil {
			s.logger().Warn("failed to unmarshal notification", zap.Error(err))
			continue
		}

		notification := &storage.Notification{
			ID:        notificationRecord.ID,
			Type:      notificationRecord.Type,
			Username:  notificationRecord.Username,
			AccountID: notificationRecord.AccountID,
			StatusID:  notificationRecord.StatusID,
			Read:      notificationRecord.Read,
			CreatedAt: time.Unix(notificationRecord.CreatedAt, 0),
		}

		notifications = append(notifications, notification)

		if len(notifications) >= limit {
			break
		}
	}

	return notifications, nil
}

// GetNotificationsByAccount retrieves notifications from a specific account
func (s *dynamoDBStorage) GetNotificationsByAccount(ctx context.Context, userID, accountID string, limit int) ([]*storage.Notification, error) {
	// Query with filter on account ID
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		FilterExpression:       aws.String("AccountID = :accountID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTIFICATIONS#%s", userID)},
			":accountID": &types.AttributeValueMemberS{Value: accountID},
		},
		ScanIndexForward: aws.Bool(false),             // Recent first
		Limit:            aws.Int32(int32(limit * 3)), // Get more since we're filtering
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications by account: %w", err)
	}

	notifications := make([]*storage.Notification, 0)
	for _, item := range result.Items {
		var notificationRecord NotificationRecord
		if err := s.UnmarshalItem(item, &notificationRecord); err != nil {
			s.logger().Warn("failed to unmarshal notification", zap.Error(err))
			continue
		}

		notification := &storage.Notification{
			ID:        notificationRecord.ID,
			Type:      notificationRecord.Type,
			Username:  notificationRecord.Username,
			AccountID: notificationRecord.AccountID,
			StatusID:  notificationRecord.StatusID,
			Read:      notificationRecord.Read,
			CreatedAt: time.Unix(notificationRecord.CreatedAt, 0),
		}

		notifications = append(notifications, notification)

		if len(notifications) >= limit {
			break
		}
	}

	return notifications, nil
}

// GetUnreadNotificationCount returns the count of unread notifications
func (s *dynamoDBStorage) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	// Query for unread notifications
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		FilterExpression:       aws.String("#read = :false"),
		ExpressionAttributeNames: map[string]string{
			"#read": "Read",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTIFICATIONS#%s", userID)},
			":false": &types.AttributeValueMemberBOOL{Value: false},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return int64(result.Count), nil
}

// MarkNotificationAsReadAdvanced marks a specific notification as read (advanced version)
func (s *dynamoDBStorage) MarkNotificationAsReadAdvanced(ctx context.Context, id string) error {
	// First, find the notification to get the user ID
	notification, err := s.GetNotification(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get notification: %w", err)
	}
	if notification == nil {
		return fmt.Errorf("notification not found")
	}

	// Update the notification
	timestamp := notification.CreatedAt.Unix()

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTIFICATIONS#%s", notification.Username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", timestamp, id)},
		},
		UpdateExpression: aws.String("SET #read = :true"),
		ExpressionAttributeNames: map[string]string{
			"#read": "Read",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true": &types.AttributeValueMemberBOOL{Value: true},
		},
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	return nil
}

// MarkAllNotificationsAsReadAdvanced marks all notifications as read for a user (advanced version)
func (s *dynamoDBStorage) MarkAllNotificationsAsReadAdvanced(ctx context.Context, username string) error {
	// Query for all unread notifications
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		FilterExpression:       aws.String("#read = :false"),
		ExpressionAttributeNames: map[string]string{
			"#read": "Read",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTIFICATIONS#%s", username)},
			":false": &types.AttributeValueMemberBOOL{Value: false},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to query unread notifications: %w", err)
	}

	// Update each notification in batches
	const batchSize = 25 // DynamoDB batch limit
	for i := 0; i < len(result.Items); i += batchSize {
		end := i + batchSize
		if end > len(result.Items) {
			end = len(result.Items)
		}

		batch := result.Items[i:end]
		if err := s.batchMarkNotificationsRead(ctx, batch); err != nil {
			s.logger().Warn("failed to batch mark notifications as read", zap.Error(err))
		}
	}

	return nil
}

// batchMarkNotificationsRead marks a batch of notifications as read
func (s *dynamoDBStorage) batchMarkNotificationsRead(ctx context.Context, items []map[string]types.AttributeValue) error {
	writeRequests := make([]types.WriteRequest, 0, len(items))

	for _, item := range items {
		// Create update request
		updateRequest := types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: make(map[string]types.AttributeValue),
			},
		}

		// Copy all existing attributes
		for k, v := range item {
			updateRequest.PutRequest.Item[k] = v
		}

		// Set Read to true
		updateRequest.PutRequest.Item["Read"] = &types.AttributeValueMemberBOOL{Value: true}

		writeRequests = append(writeRequests, updateRequest)
	}

	// Execute batch write
	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			s.tableName: writeRequests,
		},
	}

	_, err := s.client.BatchWriteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to batch write notifications: %w", err)
	}

	return nil
}
