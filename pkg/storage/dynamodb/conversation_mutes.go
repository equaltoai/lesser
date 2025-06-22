package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// ConversationMuteRecord represents a conversation mute in DynamoDB
type ConversationMuteRecord struct {
	PK  string `dynamodbav:"PK"`
	SK  string `dynamodbav:"SK"`
	TTL int64  `dynamodbav:"TTL,omitempty"` // For auto-expiration
	storage.ConversationMute
}

// CreateConversationMute creates a new conversation mute
func (s *dynamoDBStorage) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	s.logger().Info("creating conversation mute",
		zap.String("username", mute.Username),
		zap.String("conversation_id", mute.ConversationID))

	// Set timestamp if not provided
	if mute.CreatedAt.IsZero() {
		mute.CreatedAt = time.Now()
	}

	record := ConversationMuteRecord{
		PK:               fmt.Sprintf("USER#%s#CONV_MUTES", mute.Username),
		SK:               fmt.Sprintf("CONV#%s", mute.ConversationID),
		ConversationMute: *mute,
	}

	// Set TTL if expiration is specified
	if !mute.ExpiresAt.IsZero() {
		record.TTL = mute.ExpiresAt.Unix()
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation mute: %w", err)
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
			return fmt.Errorf("conversation already muted")
		}
		return fmt.Errorf("failed to create conversation mute: %w", err)
	}

	return nil
}

// DeleteConversationMute removes a conversation mute
func (s *dynamoDBStorage) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#CONV_MUTES", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONV#%s", conversationID)},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete conversation mute: %w", err)
	}

	return nil
}

// IsConversationMuted checks if a conversation is muted by a user
func (s *dynamoDBStorage) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#CONV_MUTES", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONV#%s", conversationID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check conversation mute: %w", err)
	}

	if result.Item == nil {
		return false, nil
	}

	// Check if the mute has expired
	var record ConversationMuteRecord
	err = s.UnmarshalItem(result.Item, &record)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal conversation mute: %w", err)
	}

	// If it has an expiration and it's past, consider it not muted
	if !record.ConversationMute.ExpiresAt.IsZero() && record.ConversationMute.ExpiresAt.Before(time.Now()) {
		return false, nil
	}

	return true, nil
}

// GetMutedConversations retrieves all muted conversations for a user
func (s *dynamoDBStorage) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#CONV_MUTES", username)},
		},
	}

	var conversationIDs []string
	paginator := dynamodb.NewQueryPaginator(s.client, input)

	for paginator.HasMorePages() {
		result, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query muted conversations: %w", err)
		}

		for _, item := range result.Items {
			var record ConversationMuteRecord
			err := s.UnmarshalItem(item, &record)
			if err != nil {
				s.logger().Error("failed to unmarshal conversation mute", zap.Error(err))
				continue
			}

			// Skip expired mutes
			if !record.ConversationMute.ExpiresAt.IsZero() && record.ConversationMute.ExpiresAt.Before(time.Now()) {
				continue
			}

			conversationIDs = append(conversationIDs, record.ConversationMute.ConversationID)
		}
	}

	return conversationIDs, nil
}
