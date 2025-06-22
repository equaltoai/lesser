package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// GetReplies retrieves all replies to a given object
func (s *dynamoDBStorage) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]interface{}, string, error) {
	log := common.WithContext(ctx)
	
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	
	// Ensure we have the full object URL for GSI6 key
	parentID := objectID
	if !strings.HasPrefix(objectID, "http") {
		parentID = fmt.Sprintf("%s/objects/%s", s.getDomainURL(), objectID)
	}
	
	// Use GSI6 to efficiently query replies
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI6"),
		KeyConditionExpression: aws.String("GSI6PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPLIES#%s", parentID)},
		},
		Limit:            aws.Int32(int32(limit)),
		ScanIndexForward: aws.Bool(true), // Oldest replies first
	}
	
	// Add pagination if cursor provided
	if cursor != "" {
		// Cursor is in format "timestamp#objectID"
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI6PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPLIES#%s", parentID)},
			"GSI6SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}
	
	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query for replies",
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query for replies: %w", err)
	}
	
	// Cost tracking is handled automatically by the wrapped DynamoDB client
	
	replies := make([]interface{}, 0, len(result.Items))
	for _, item := range result.Items {
		var record ObjectRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal reply",
				zap.Error(err))
			continue
		}
		
		// Convert Object to interface{} format expected by converter
		if record.Object != nil {
			objMap := s.objectToMap(record.Object)
			replies = append(replies, objMap)
		}
	}
	
	// No need to sort - GSI6 already returns in timestamp order
	
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		// Extract GSI6SK for cursor (format: timestamp#objectID)
		if sk, ok := result.LastEvaluatedKey["GSI6SK"].(*types.AttributeValueMemberS); ok {
			nextCursor = sk.Value
		}
	}
	
	log.Debug("found replies",
		zap.String("object_id", objectID),
		zap.Int("count", len(replies)),
		zap.String("next_cursor", nextCursor))
	
	return replies, nextCursor, nil
}

// CountReplies counts the number of replies to an object
func (s *dynamoDBStorage) CountReplies(ctx context.Context, objectID string) (int, error) {
	log := common.WithContext(ctx)
	
	// Construct full object key for stats lookup
	objectKey := objectID
	if !strings.HasPrefix(objectID, "http") {
		objectKey = fmt.Sprintf("%s/objects/%s", s.getDomainURL(), objectID)
	}
	
	// First check if we have a cached count
	countKey := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectKey)},
		"SK": &types.AttributeValueMemberS{Value: "STATS"},
	}
	
	getResult, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key:       countKey,
	})
	
	if err == nil && getResult.Item != nil {
		// Extract reply_count if it exists
		if countAttr, ok := getResult.Item["reply_count"]; ok {
			if n, ok := countAttr.(*types.AttributeValueMemberN); ok {
				var count int
				fmt.Sscanf(n.Value, "%d", &count)
				return count, nil
			}
		}
	}
	
	// Use GSI6 to efficiently count replies
	var count int
	var lastEvaluatedKey map[string]types.AttributeValue
	
	// Ensure we have the full object URL for GSI6 key
	parentID := objectID
	if !strings.HasPrefix(objectID, "http") {
		parentID = fmt.Sprintf("%s/objects/%s", s.getDomainURL(), objectID)
	}
	
	for {
		input := &dynamodb.QueryInput{
			TableName:              s.getTableName(),
			IndexName:              aws.String("GSI6"),
			KeyConditionExpression: aws.String("GSI6PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPLIES#%s", parentID)},
			},
			Select: types.SelectCount,
		}
		
		if lastEvaluatedKey != nil {
			input.ExclusiveStartKey = lastEvaluatedKey
		}
		
		result, err := s.client.Query(ctx, input)
		if err != nil {
			log.Error("failed to count replies",
				zap.String("object_id", objectID),
				zap.Error(err))
			return 0, fmt.Errorf("failed to count replies: %w", err)
		}
		
		count += int(result.Count)
		
		if result.LastEvaluatedKey == nil {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}
	
	// Cache the count for future use
	s.updateReplyCount(ctx, objectID, count)
	
	return count, nil
}

// IncrementReplyCount increments the reply count for an object
func (s *dynamoDBStorage) IncrementReplyCount(ctx context.Context, objectID string) error {
	log := common.WithContext(ctx)
	
	// Construct full object key
	objectKey := objectID
	if !strings.HasPrefix(objectID, "http") {
		objectKey = fmt.Sprintf("%s/objects/%s", s.getDomainURL(), objectID)
	}
	
	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectKey)},
			"SK": &types.AttributeValueMemberS{Value: "STATS"},
		},
		UpdateExpression: aws.String("ADD reply_count :inc SET updated_at = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}
	
	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		log.Error("failed to increment reply count",
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to increment reply count: %w", err)
	}
	
	return nil
}

// IncrementReblogCount increments the reblog count for an object
func (s *dynamoDBStorage) IncrementReblogCount(ctx context.Context, objectID string) error {
	log := common.WithContext(ctx)
	
	// Construct full object key
	objectKey := objectID
	if !strings.HasPrefix(objectID, "http") {
		objectKey = fmt.Sprintf("%s/objects/%s", s.getDomainURL(), objectID)
	}
	
	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectKey)},
			"SK": &types.AttributeValueMemberS{Value: "STATS"},
		},
		UpdateExpression: aws.String("ADD reblog_count :inc SET updated_at = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}
	
	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		log.Error("failed to increment reblog count",
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to increment reblog count: %w", err)
	}
	
	return nil
}

// Helper to update reply count (for caching)
func (s *dynamoDBStorage) updateReplyCount(ctx context.Context, objectID string, count int) {
	// Construct full object key
	objectKey := objectID
	if !strings.HasPrefix(objectID, "http") {
		objectKey = fmt.Sprintf("%s/objects/%s", s.getDomainURL(), objectID)
	}
	
	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectKey)},
			"SK": &types.AttributeValueMemberS{Value: "STATS"},
		},
		UpdateExpression: aws.String("SET reply_count = :count, updated_at = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":count": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", count)},
			":now":   &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}
	
	s.client.UpdateItem(ctx, input)
}

// Helper to convert Object to map for converter compatibility
func (s *dynamoDBStorage) objectToMap(obj *Object) map[string]interface{} {
	objMap := map[string]interface{}{
		"id":           obj.ID,
		"type":         obj.Type,
		"attributedTo": obj.AttributedTo,
		"content":      obj.Content,
		"name":         obj.Name,
		"summary":      obj.Summary,
		"url":          obj.URL,
		"published":    obj.Published.Format(time.RFC3339),
		"to":           obj.To,
		"cc":           obj.CC,
		"sensitive":    obj.Sensitive,
	}
	
	if obj.InReplyTo != nil {
		objMap["inReplyTo"] = *obj.InReplyTo
	}
	
	if !obj.Updated.IsZero() {
		objMap["updated"] = obj.Updated.Format(time.RFC3339)
	}
	
	if len(obj.Attachment) > 0 {
		attachments := make([]interface{}, len(obj.Attachment))
		for i, att := range obj.Attachment {
			attachments[i] = map[string]interface{}{
				"type":      att.Type,
				"url":       att.URL,
				"mediaType": att.MediaType,
				"name":      att.Name,
				"width":     att.Width,
				"height":    att.Height,
			}
		}
		objMap["attachment"] = attachments
	}
	
	if len(obj.Tag) > 0 {
		tags := make([]interface{}, len(obj.Tag))
		for i, tag := range obj.Tag {
			tags[i] = map[string]interface{}{
				"type": tag.Type,
				"href": tag.Href,
				"name": tag.Name,
			}
		}
		objMap["tag"] = tags
	}
	
	return objMap
}

