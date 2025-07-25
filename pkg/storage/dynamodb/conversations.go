package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// ConversationRecord represents a conversation in DynamoDB
type ConversationRecord struct {
	PK     string `dynamodbav:"PK"`
	SK     string `dynamodbav:"SK"`
	GSI1PK string `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK string `dynamodbav:"GSI1SK,omitempty"`
	*storage.Conversation
}

// ConversationStatusRecord represents a user's read status for a conversation
type ConversationStatusRecord struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
	*storage.ConversationStatus
}

// CreateConversation creates a new conversation
func (s *dynamoDBStorage) CreateConversation(ctx context.Context, conversation *storage.Conversation) error {
	log := common.Logger().With(zap.String("conversation_id", conversation.ID))

	// Generate ID if not provided
	if conversation.ID == "" {
		conversation.ID = generateRandomString(12)
	}

	// Set timestamps
	now := time.Now()
	conversation.CreatedAt = now
	conversation.UpdatedAt = now

	record := &ConversationRecord{
		PK:           fmt.Sprintf("CONVERSATION#%s", conversation.ID),
		SK:           "METADATA",
		Conversation: conversation,
	}

	// Create GSI entries for each participant
	records := []any{record}
	for _, participantID := range conversation.Participants {
		participantRecord := &ConversationRecord{
			PK:           fmt.Sprintf("USER_CONVERSATIONS#%s", participantID),
			SK:           fmt.Sprintf("%s#%s", conversation.UpdatedAt.Format(time.RFC3339), conversation.ID),
			GSI1PK:       fmt.Sprintf("CONVERSATION#%s", conversation.ID),
			GSI1SK:       fmt.Sprintf("PARTICIPANT#%s", participantID),
			Conversation: conversation,
		}
		records = append(records, participantRecord)
	}

	// Batch write all records
	for _, rec := range records {
		item, err := s.MarshalItem(rec)
		if err != nil {
			log.Error("failed to marshal conversation record", zap.Error(err))
			return fmt.Errorf("failed to marshal conversation: %w", err)
		}

		_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(s.tableName),
			Item:      item,
		})
		if err != nil {
			log.Error("failed to create conversation", zap.Error(err))
			return fmt.Errorf("failed to create conversation: %w", err)
		}
	}

	log.Debug("conversation created successfully")
	return nil
}

// GetConversation retrieves a conversation by ID
func (s *dynamoDBStorage) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	log := common.Logger().With(zap.String("conversation_id", id))

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONVERSATION#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		log.Error("failed to get conversation", zap.Error(err))
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("conversation not found")
	}

	var record ConversationRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		log.Error("failed to unmarshal conversation", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal conversation: %w", err)
	}

	return record.Conversation, nil
}

// GetConversationByParticipants finds a conversation with exact participants
func (s *dynamoDBStorage) GetConversationByParticipants(ctx context.Context, participants []string) (*storage.Conversation, error) {
	log := common.Logger().With(zap.Any("participants", participants))

	// Sort participants to create a consistent lookup key
	// This ensures conversation lookup works regardless of participant order
	sortedParticipants := make([]string, len(participants))
	copy(sortedParticipants, participants)
	// Simple sort for deterministic order
	for i := 0; i < len(sortedParticipants)-1; i++ {
		for j := i + 1; j < len(sortedParticipants); j++ {
			if sortedParticipants[i] > sortedParticipants[j] {
				sortedParticipants[i], sortedParticipants[j] = sortedParticipants[j], sortedParticipants[i]
			}
		}
	}

	// Create a consistent participant key
	participantKey := strings.Join(sortedParticipants, ",")

	// Query by participant key using GSI
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONVERSATION_PARTICIPANTS#%s", participantKey)},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query conversation by participants", zap.Error(err))
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("conversation not found")
	}

	// Parse the conversation from the result
	var record ConversationRecord
	err = attributevalue.UnmarshalMap(result.Items[0], &record)
	if err != nil {
		log.Error("failed to unmarshal conversation", zap.Error(err))
		return nil, fmt.Errorf("failed to parse conversation: %w", err)
	}

	return record.Conversation, nil
}

// UpdateConversationLastStatus updates the last status in a conversation
func (s *dynamoDBStorage) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	log := common.Logger().With(
		zap.String("conversation_id", id),
		zap.String("last_status_id", lastStatusID),
	)

	// Update the main conversation record
	update := expression.Set(expression.Name("LastStatusID"), expression.Value(lastStatusID)).
		Set(expression.Name("UpdatedAt"), expression.Value(time.Now()))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONVERSATION#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		log.Error("failed to update conversation", zap.Error(err))
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	// Get the conversation to get participants
	conv, err := s.GetConversation(ctx, id)
	if err != nil {
		log.Error("failed to get conversation for participant update", zap.Error(err))
		return fmt.Errorf("failed to get conversation: %w", err)
	}

	// Update participant records with new timestamp for sorting
	now := time.Now()
	for _, participantID := range conv.Participants {
		// Delete old participant record
		_, deleteErr := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_CONVERSATIONS#%s", participantID)},
				"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", conv.UpdatedAt.Format(time.RFC3339), id)},
			},
		})
		if deleteErr != nil {
			log.Warn("failed to delete old participant record",
				zap.String("participant_id", participantID),
				zap.Error(deleteErr))
		}

		// Create new participant record with updated timestamp
		participantRecord := &ConversationRecord{
			PK:           fmt.Sprintf("USER_CONVERSATIONS#%s", participantID),
			SK:           fmt.Sprintf("%s#%s", now.Format(time.RFC3339), id),
			GSI1PK:       fmt.Sprintf("CONVERSATION#%s", id),
			GSI1SK:       fmt.Sprintf("PARTICIPANT#%s", participantID),
			Conversation: conv,
		}
		participantRecord.UpdatedAt = now
		participantRecord.LastStatusID = lastStatusID

		item, marshalErr := s.MarshalItem(participantRecord)
		if marshalErr != nil {
			log.Error("failed to marshal participant record",
				zap.String("participant_id", participantID),
				zap.Error(marshalErr))
			continue
		}

		_, putErr := s.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(s.tableName),
			Item:      item,
		})
		if putErr != nil {
			log.Error("failed to update participant record",
				zap.String("participant_id", participantID),
				zap.Error(putErr))
		}
	}

	return nil
}

// MarkConversationRead marks a conversation as read for a user
func (s *dynamoDBStorage) MarkConversationRead(ctx context.Context, id, username string) error {
	log := common.Logger().With(
		zap.String("conversation_id", id),
		zap.String("username", username),
	)

	// Create/update read status record
	status := &ConversationStatusRecord{
		PK: fmt.Sprintf("CONVERSATION_STATUS#%s", id),
		SK: fmt.Sprintf("USER#%s", username),
		ConversationStatus: &storage.ConversationStatus{
			ConversationID: id,
			UserID:         username,
			Unread:         false,
			LastReadAt:     time.Now(),
		},
	}

	item, err := s.MarshalItem(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		log.Error("failed to mark conversation as read", zap.Error(err))
		return fmt.Errorf("failed to mark conversation as read: %w", err)
	}

	return nil
}

// DeleteConversation deletes a conversation
func (s *dynamoDBStorage) DeleteConversation(ctx context.Context, id string) error {
	log := common.Logger().With(zap.String("conversation_id", id))

	// Get the conversation first to get participant list
	conv, err := s.GetConversation(ctx, id)
	if err != nil {
		log.Warn("failed to get conversation for cleanup", zap.Error(err))
		// Continue with deletion even if we can't get the conversation
	}

	// Delete main record
	_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONVERSATION#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		log.Error("failed to delete conversation", zap.Error(err))
		return fmt.Errorf("failed to delete conversation: %w", err)
	}

	// Delete participant records
	if conv != nil {
		for _, participantID := range conv.Participants {
			// Delete participant record
			_, deleteErr := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(s.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_CONVERSATIONS#%s", participantID)},
					"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", conv.UpdatedAt.Format(time.RFC3339), id)},
				},
			})
			if deleteErr != nil {
				log.Warn("failed to delete participant record",
					zap.String("participant_id", participantID),
					zap.Error(deleteErr))
			}
		}
	}

	// Delete all status records for this conversation
	// Query for all status records
	statusQuery := expression.Key("PK").Equal(expression.Value(fmt.Sprintf("CONVERSATION_STATUS#%s", id)))
	expr, err := expression.NewBuilder().WithKeyCondition(statusQuery).Build()
	if err != nil {
		log.Warn("failed to build status query expression", zap.Error(err))
		return nil // Main record deleted, don't fail on cleanup
	}

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		log.Warn("failed to query conversation status records", zap.Error(err))
		return nil // Main record deleted, don't fail on cleanup
	}

	// Delete each status record
	for _, item := range result.Items {
		pk := item["PK"]
		sk := item["SK"]
		if pk != nil && sk != nil {
			_, deleteErr := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(s.tableName),
				Key: map[string]types.AttributeValue{
					"PK": pk,
					"SK": sk,
				},
			})
			if deleteErr != nil {
				log.Warn("failed to delete status record", zap.Error(deleteErr))
			}
		}
	}

	return nil
}

// GetUserConversations retrieves conversations for a user
func (s *dynamoDBStorage) GetUserConversations(ctx context.Context, userID string, limit int, cursor string) ([]*storage.Conversation, string, error) {
	log := common.Logger().With(
		zap.String("user_id", userID),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	// Query by user partition key
	keyCondition := expression.Key("PK").Equal(expression.Value(fmt.Sprintf("USER_CONVERSATIONS#%s", userID)))
	if cursor != "" {
		keyCondition = keyCondition.And(expression.Key("SK").LessThan(expression.Value(cursor)))
	}

	expr, err := expression.NewBuilder().WithKeyCondition(keyCondition).Build()
	if err != nil {
		return nil, "", fmt.Errorf("failed to build query expression: %w", err)
	}

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     safeInt32(limit + 1),
		ScanIndexForward:          aws.Bool(false), // Most recent first
	})
	if err != nil {
		log.Error("failed to query user conversations", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query user conversations: %w", err)
	}

	conversations := make([]*storage.Conversation, 0, len(result.Items))
	for _, item := range result.Items {
		var record ConversationRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal conversation record", zap.Error(err))
			continue
		}
		conversations = append(conversations, record.Conversation)
	}

	// Determine next cursor
	var nextCursor string
	if len(conversations) > limit {
		conversations = conversations[:limit]
		if len(conversations) > 0 {
			lastConv := conversations[len(conversations)-1]
			nextCursor = fmt.Sprintf("%s#%s", lastConv.UpdatedAt.Format(time.RFC3339), lastConv.ID)
		}
	}

	return conversations, nextCursor, nil
}

// AddParticipantToConversation adds a participant to a conversation
func (s *dynamoDBStorage) AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error {
	log := common.Logger().With(
		zap.String("conversation_id", conversationID),
		zap.String("participant_id", participantID),
	)

	// Get the conversation
	conv, err := s.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}

	// Check if already a participant
	for _, p := range conv.Participants {
		if p == participantID {
			log.Debug("participant already in conversation")
			return nil // Already a participant
		}
	}

	// Add participant
	conv.Participants = append(conv.Participants, participantID)
	conv.UpdatedAt = time.Now()

	// Update the conversation
	return s.CreateConversation(ctx, conv)
}

// RemoveParticipantFromConversation removes a participant from a conversation
func (s *dynamoDBStorage) RemoveParticipantFromConversation(ctx context.Context, conversationID, participantID string) error {
	log := common.Logger().With(
		zap.String("conversation_id", conversationID),
		zap.String("participant_id", participantID),
	)

	// Get the conversation
	conv, err := s.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}

	// Remove participant
	newParticipants := make([]string, 0, len(conv.Participants))
	for _, p := range conv.Participants {
		if p != participantID {
			newParticipants = append(newParticipants, p)
		}
	}

	if len(newParticipants) == len(conv.Participants) {
		log.Debug("participant not found in conversation")
		return nil // Participant not found
	}

	conv.Participants = newParticipants
	conv.UpdatedAt = time.Now()

	// Update the conversation
	return s.CreateConversation(ctx, conv)
}

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
