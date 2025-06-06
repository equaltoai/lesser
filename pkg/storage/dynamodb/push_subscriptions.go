package dynamodb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// hashString creates a SHA256 hash of a string
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// PushSubscriptionRecord represents a push subscription stored in DynamoDB
type PushSubscriptionRecord struct {
	PK     string                   `dynamodbav:"PK"`     // PUSH#username
	SK     string                   `dynamodbav:"SK"`     // SUB#subscriptionID
	GSI1PK string                   `dynamodbav:"GSI1PK"` // PUSH_ENDPOINT#endpoint_hash
	GSI1SK string                   `dynamodbav:"GSI1SK"` // username
	Data   storage.PushSubscription `dynamodbav:"Data"`
}

// CreatePushSubscription creates a new push subscription
func (s *dynamoDBStorage) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	// Generate ID if not provided
	if subscription.ID == "" {
		subscription.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	subscription.Username = username
	subscription.CreatedAt = now
	subscription.UpdatedAt = now

	// Create endpoint hash for GSI lookup (to prevent duplicate endpoints)
	endpointHash := hashString(subscription.Endpoint)

	record := PushSubscriptionRecord{
		PK:     fmt.Sprintf("PUSH#%s", username),
		SK:     fmt.Sprintf("SUB#%s", subscription.ID),
		GSI1PK: fmt.Sprintf("PUSH_ENDPOINT#%s", endpointHash),
		GSI1SK: username,
		Data:   *subscription,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal push subscription: %w", err)
	}

	// Use conditional put to prevent duplicate endpoints
	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		common.Logger().Error("failed to create push subscription",
			zap.String("subscription_id", subscription.ID),
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to create push subscription: %w", err)
	}

	return nil
}

// GetPushSubscription retrieves a push subscription by ID
func (s *dynamoDBStorage) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PUSH#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SUB#%s", subscriptionID)},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get push subscription: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("push subscription not found")
	}

	var record PushSubscriptionRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal push subscription: %w", err)
	}

	return &record.Data, nil
}

// GetUserPushSubscriptions retrieves all push subscriptions for a user
func (s *dynamoDBStorage) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("PUSH#%s", username)},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query push subscriptions: %w", err)
	}

	subscriptions := make([]*storage.PushSubscription, 0, len(result.Items))
	for _, item := range result.Items {
		var record PushSubscriptionRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			common.Logger().Error("failed to unmarshal push subscription",
				zap.String("username", username),
				zap.Error(err))
			continue
		}
		subscriptions = append(subscriptions, &record.Data)
	}

	return subscriptions, nil
}

// UpdatePushSubscription updates the alerts for a push subscription
func (s *dynamoDBStorage) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	// First get the existing subscription
	subscription, err := s.GetPushSubscription(ctx, username, subscriptionID)
	if err != nil {
		return err
	}

	// Update alerts and timestamp
	subscription.Alerts = alerts
	subscription.UpdatedAt = time.Now()

	// Save the updated subscription
	record := PushSubscriptionRecord{
		PK:     fmt.Sprintf("PUSH#%s", username),
		SK:     fmt.Sprintf("SUB#%s", subscriptionID),
		GSI1PK: fmt.Sprintf("PUSH_ENDPOINT#%s", hashString(subscription.Endpoint)),
		GSI1SK: username,
		Data:   *subscription,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal push subscription: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update push subscription: %w", err)
	}

	return nil
}

// DeletePushSubscription deletes a push subscription
func (s *dynamoDBStorage) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PUSH#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SUB#%s", subscriptionID)},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete push subscription: %w", err)
	}

	return nil
}

// DeleteAllPushSubscriptions deletes all push subscriptions for a user
func (s *dynamoDBStorage) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	// First, get all subscriptions
	subscriptions, err := s.GetUserPushSubscriptions(ctx, username)
	if err != nil {
		return err
	}

	// Delete each subscription
	for _, sub := range subscriptions {
		if err := s.DeletePushSubscription(ctx, username, sub.ID); err != nil {
			common.Logger().Error("failed to delete push subscription",
				zap.String("subscription_id", sub.ID),
				zap.String("username", username),
				zap.Error(err))
			// Continue with other subscriptions
		}
	}

	return nil
}

// GetVAPIDKeys retrieves the VAPID keys for the instance
func (s *dynamoDBStorage) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "INSTANCE#CONFIG"},
			"SK": &types.AttributeValueMemberS{Value: "VAPID_KEYS"},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get VAPID keys: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("VAPID keys not found")
	}

	// Extract the VAPIDKeys from the result
	keysData, ok := result.Item["Data"]
	if !ok {
		return nil, fmt.Errorf("VAPID keys data not found in record")
	}

	var keys storage.VAPIDKeys
	if err := attributevalue.Unmarshal(keysData, &keys); err != nil {
		return nil, fmt.Errorf("failed to unmarshal VAPID keys: %w", err)
	}

	return &keys, nil
}

// SetVAPIDKeys stores the VAPID keys for the instance
func (s *dynamoDBStorage) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	// Set creation timestamp
	if keys.CreatedAt.IsZero() {
		keys.CreatedAt = time.Now()
	}

	// Create a record structure similar to instance data
	record := struct {
		PK        string            `dynamodbav:"PK"`
		SK        string            `dynamodbav:"SK"`
		Data      storage.VAPIDKeys `dynamodbav:"Data"`
		UpdatedAt time.Time         `dynamodbav:"UpdatedAt"`
	}{
		PK:        "INSTANCE#CONFIG",
		SK:        "VAPID_KEYS",
		Data:      *keys,
		UpdatedAt: time.Now(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal VAPID keys: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to save VAPID keys: %w", err)
	}

	return nil
}
