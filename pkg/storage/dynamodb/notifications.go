package dynamodb

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NotificationRecord represents a notification stored in DynamoDB
type NotificationRecord struct {
	PK        string `dynamodbav:"PK"` // NOTIFICATIONS#username
	SK        string `dynamodbav:"SK"` // timestamp#notificationID
	ID        string `dynamodbav:"ID"`
	Type      string `dynamodbav:"Type"`
	Username  string `dynamodbav:"Username"`
	AccountID string `dynamodbav:"AccountID"`
	StatusID  string `dynamodbav:"StatusID,omitempty"`
	Read      bool   `dynamodbav:"Read"`
	CreatedAt int64  `dynamodbav:"CreatedAt"` // Unix timestamp for sorting
	TTL       int64  `dynamodbav:"TTL"`       // 30 days auto-deletion
}

// CreateNotification creates a new notification
func (s *dynamoDBStorage) CreateNotification(ctx context.Context, notification *storage.Notification) error {
	// Generate ID if not provided
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}

	// Set creation time if not provided
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}

	// Create timestamp-based SK for chronological ordering
	timestamp := notification.CreatedAt.Unix()

	record := NotificationRecord{
		PK:        fmt.Sprintf("NOTIFICATIONS#%s", notification.Username),
		SK:        fmt.Sprintf("%d#%s", timestamp, notification.ID),
		ID:        notification.ID,
		Type:      notification.Type,
		Username:  notification.Username,
		AccountID: notification.AccountID,
		StatusID:  notification.StatusID,
		Read:      notification.Read,
		CreatedAt: timestamp,
		TTL:       time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		common.Logger().Error("failed to create notification",
			zap.String("notification_id", notification.ID),
			zap.String("username", notification.Username),
			zap.Error(err))
		return fmt.Errorf("failed to create notification: %w", err)
	}

	return nil
}

// GetNotification retrieves a notification by ID
func (s *dynamoDBStorage) GetNotification(ctx context.Context, id string) (*storage.Notification, error) {
	// We need to scan for the notification since we don't know the username
	// In a production system, you might want to maintain a GSI for notification ID lookups

	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("ID = :id AND begins_with(PK, :pk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id":        &types.AttributeValueMemberS{Value: id},
			":pk_prefix": &types.AttributeValueMemberS{Value: "NOTIFICATIONS#"},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("notification not found")
	}

	var record NotificationRecord
	if err := s.UnmarshalItem(result.Items[0], &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	return &storage.Notification{
		ID:        record.ID,
		Type:      record.Type,
		Username:  record.Username,
		AccountID: record.AccountID,
		StatusID:  record.StatusID,
		Read:      record.Read,
		CreatedAt: time.Unix(record.CreatedAt, 0),
	}, nil
}

// GetNotifications retrieves notifications for a user with pagination
func (s *dynamoDBStorage) GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*storage.Notification, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	pk := fmt.Sprintf("NOTIFICATIONS#%s", username)

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	// Handle pagination cursor
	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query notifications: %w", err)
	}

	notifications := make([]*storage.Notification, 0, len(result.Items))
	for _, item := range result.Items {
		var record NotificationRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			common.Logger().Warn("failed to unmarshal notification record", zap.Error(err))
			continue
		}

		notifications = append(notifications, &storage.Notification{
			ID:        record.ID,
			Type:      record.Type,
			Username:  record.Username,
			AccountID: record.AccountID,
			StatusID:  record.StatusID,
			Read:      record.Read,
			CreatedAt: time.Unix(record.CreatedAt, 0),
		})
	}

	// Build next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"].(*types.AttributeValueMemberS); ok {
			nextCursor = sk.Value
		}
	}

	return notifications, nextCursor, nil
}

// GetNotificationsFiltered retrieves notifications with filtering options
func (s *dynamoDBStorage) GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error) {
	// For now, get all notifications and filter in memory
	// In production, you might want to use GSIs for better filtering
	allNotifications, _, err := s.GetNotifications(ctx, username, 100, "")
	if err != nil {
		return nil, "", err
	}

	filtered := make([]*storage.Notification, 0)

	for _, notif := range allNotifications {
		// Apply type filters
		if len(filter.Types) > 0 {
			found := false
			for _, t := range filter.Types {
				if notif.Type == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Apply exclude type filters
		if len(filter.ExcludeTypes) > 0 {
			excluded := false
			for _, t := range filter.ExcludeTypes {
				if notif.Type == t {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		// Apply account filter
		if filter.AccountID != "" && notif.AccountID != filter.AccountID {
			continue
		}

		// Apply ID filters
		if filter.MinID != "" && notif.ID <= filter.MinID {
			continue
		}
		if filter.MaxID != "" && notif.ID >= filter.MaxID {
			continue
		}
		if filter.SinceID != "" && notif.ID <= filter.SinceID {
			continue
		}

		filtered = append(filtered, notif)
	}

	// Sort by creation time (newest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	// Apply limit
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > len(filtered) {
		limit = len(filtered)
	}

	return filtered[:limit], "", nil
}

// MarkNotificationAsRead marks a notification as read
func (s *dynamoDBStorage) MarkNotificationAsRead(ctx context.Context, id string) error {
	// First get the notification to find its PK/SK
	notif, err := s.GetNotification(ctx, id)
	if err != nil {
		return err
	}

	pk := fmt.Sprintf("NOTIFICATIONS#%s", notif.Username)
	sk := fmt.Sprintf("%d#%s", notif.CreatedAt.Unix(), notif.ID)

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
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
	return err
}

// MarkAllNotificationsAsRead marks all notifications as read for a user
func (s *dynamoDBStorage) MarkAllNotificationsAsRead(ctx context.Context, username string) error {
	// Get all unread notifications
	notifications, _, err := s.GetNotifications(ctx, username, 100, "")
	if err != nil {
		return err
	}

	// Update each unread notification
	for _, notif := range notifications {
		if !notif.Read {
			if err := s.MarkNotificationAsRead(ctx, notif.ID); err != nil {
				common.Logger().Warn("failed to mark notification as read",
					zap.String("notification_id", notif.ID),
					zap.Error(err))
			}
		}
	}

	return nil
}

// DeleteNotification deletes a notification
func (s *dynamoDBStorage) DeleteNotification(ctx context.Context, id string) error {
	// First get the notification to find its PK/SK
	notif, err := s.GetNotification(ctx, id)
	if err != nil {
		return err
	}

	pk := fmt.Sprintf("NOTIFICATIONS#%s", notif.Username)
	sk := fmt.Sprintf("%d#%s", notif.CreatedAt.Unix(), notif.ID)

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	}

	_, err = s.client.DeleteItem(ctx, input)
	return err
}

// ClearNotifications deletes all notifications for a user
func (s *dynamoDBStorage) ClearNotifications(ctx context.Context, username string) error {
	// Get all notifications
	notifications, _, err := s.GetNotifications(ctx, username, 100, "")
	if err != nil {
		return err
	}

	// Delete each notification
	for _, notif := range notifications {
		if err := s.DeleteNotification(ctx, notif.ID); err != nil {
			common.Logger().Warn("failed to delete notification",
				zap.String("notification_id", notif.ID),
				zap.Error(err))
		}
	}

	return nil
}

// CountUnreadNotifications counts unread notifications for a user
func (s *dynamoDBStorage) CountUnreadNotifications(ctx context.Context, username string) (int, error) {
	// Get all notifications and count unread ones
	notifications, _, err := s.GetNotifications(ctx, username, 100, "")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, notif := range notifications {
		if !notif.Read {
			count++
		}
	}

	return count, nil
}

// GetNotificationPreferences retrieves notification preferences for a user
func (s *dynamoDBStorage) GetNotificationPreferences(ctx context.Context, username string) (*storage.NotificationPreferences, error) {
	// For now, return default preferences
	// In a real implementation, this would fetch from DynamoDB
	return &storage.NotificationPreferences{
		Username:        username,
		EmailEnabled:    true,
		PushEnabled:     true,
		FollowEnabled:   true,
		MentionEnabled:  true,
		ReblogEnabled:   true,
		FavoriteEnabled: true,
		PollEnabled:     true,
		UpdatedAt:       time.Now(),
	}, nil
}

// UpdateNotificationPreferences updates notification preferences for a user
func (s *dynamoDBStorage) UpdateNotificationPreferences(ctx context.Context, username string, prefs *storage.NotificationPreferences) error {
	// For now, this is a no-op
	// In a real implementation, this would save to DynamoDB
	prefs.Username = username
	prefs.UpdatedAt = time.Now()
	return nil
}

// BatchMarkNotificationsAsRead marks multiple notifications as read
func (s *dynamoDBStorage) BatchMarkNotificationsAsRead(ctx context.Context, username string, notificationIDs []string) error {
	// Mark each notification as read
	for _, id := range notificationIDs {
		if err := s.MarkNotificationAsRead(ctx, id); err != nil {
			common.Logger().Warn("failed to mark notification as read",
				zap.String("notification_id", id),
				zap.Error(err))
			// Continue with other notifications
		}
	}
	return nil
}
