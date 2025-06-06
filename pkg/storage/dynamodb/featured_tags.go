package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
)

// FeaturedTagRecord represents a featured tag in DynamoDB
type FeaturedTagRecord struct {
	PK            string    `dynamodbav:"PK"` // USER#username
	SK            string    `dynamodbav:"SK"` // FEATURED_TAG#id
	ID            string    `dynamodbav:"ID"`
	Username      string    `dynamodbav:"Username"`
	Name          string    `dynamodbav:"Name"` // Tag name without #
	URL           string    `dynamodbav:"URL"`
	StatusesCount int       `dynamodbav:"StatusesCount"`
	LastStatusAt  string    `dynamodbav:"LastStatusAt"`
	CreatedAt     time.Time `dynamodbav:"CreatedAt"`
}

// CreateFeaturedTag creates a new featured tag for a user
func (s *dynamoDBStorage) CreateFeaturedTag(ctx context.Context, userID string, tagName string) (*storage.FeaturedTag, error) {
	// Normalize tag name (remove # if present)
	tagName = strings.TrimPrefix(tagName, "#")
	tagName = strings.ToLower(tagName)

	// Check if already featured
	existing, err := s.GetFeaturedTags(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing featured tags: %w", err)
	}

	for _, tag := range existing {
		if tag.Name == tagName {
			return nil, storage.ErrAlreadyExists
		}
	}

	// Create new featured tag
	id := uuid.New().String()
	featuredTag := &storage.FeaturedTag{
		ID:            id,
		Username:      userID,
		Name:          tagName,
		URL:           fmt.Sprintf("https://%s/tags/%s", config.Get().Domain, tagName),
		StatusesCount: 0,  // TODO: Calculate actual count
		LastStatusAt:  "", // TODO: Find last status with this tag
		CreatedAt:     time.Now(),
	}

	record := FeaturedTagRecord{
		PK:            s.userPK(userID),
		SK:            fmt.Sprintf("FEATURED_TAG#%s", id),
		ID:            featuredTag.ID,
		Username:      featuredTag.Username,
		Name:          featuredTag.Name,
		URL:           featuredTag.URL,
		StatusesCount: featuredTag.StatusesCount,
		LastStatusAt:  featuredTag.LastStatusAt,
		CreatedAt:     featuredTag.CreatedAt,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal featured tag: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create featured tag: %w", err)
	}

	return featuredTag, nil
}

// DeleteFeaturedTag removes a featured tag
func (s *dynamoDBStorage) DeleteFeaturedTag(ctx context.Context, userID string, featuredTagID string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FEATURED_TAG#%s", featuredTagID)},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to delete featured tag: %w", err)
	}

	return nil
}

// GetFeaturedTags returns all featured tags for a user
func (s *dynamoDBStorage) GetFeaturedTags(ctx context.Context, userID string) ([]*storage.FeaturedTag, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: s.userPK(userID)},
			":prefix": &types.AttributeValueMemberS{Value: "FEATURED_TAG#"},
		},
		ConsistentRead: aws.Bool(false),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query featured tags: %w", err)
	}

	tags := make([]*storage.FeaturedTag, 0, len(result.Items))
	for _, item := range result.Items {
		var record FeaturedTagRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			s.logger().Warn("failed to unmarshal featured tag", zap.Error(err))
			continue
		}

		tags = append(tags, &storage.FeaturedTag{
			ID:            record.ID,
			Username:      record.Username,
			Name:          record.Name,
			URL:           record.URL,
			StatusesCount: record.StatusesCount,
			LastStatusAt:  record.LastStatusAt,
			CreatedAt:     record.CreatedAt,
		})
	}

	return tags, nil
}

// GetTagSuggestions returns suggested tags based on user's usage
func (s *dynamoDBStorage) GetTagSuggestions(ctx context.Context, userID string, limit int) ([]string, error) {
	// TODO: Implement actual tag usage tracking and suggestions
	// For now, return empty list
	// In a real implementation, this would:
	// 1. Query the user's posts
	// 2. Extract hashtags from posts
	// 3. Count usage frequency
	// 4. Exclude already featured tags
	// 5. Return most frequently used tags

	return []string{}, nil
}
