package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
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

	// Calculate tag statistics
	statusesCount, lastStatusAt := s.calculateTagStatistics(ctx, userID, tagName)

	// Create new featured tag
	id := uuid.New().String()
	featuredTag := &storage.FeaturedTag{
		ID:            id,
		Username:      userID,
		Name:          tagName,
		URL:           fmt.Sprintf("https://%s/tags/%s", config.Get().Domain, tagName),
		StatusesCount: statusesCount,
		LastStatusAt:  lastStatusAt,
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
	// Get already featured tags to exclude them
	featuredTags, err := s.GetFeaturedTags(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get featured tags: %w", err)
	}

	featuredMap := make(map[string]bool)
	for _, tag := range featuredTags {
		featuredMap[strings.ToLower(tag.Name)] = true
	}

	// Query user's recent statuses
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_STATUS#%s", userID)},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(100),  // Analyze last 100 statuses
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query user statuses: %w", err)
	}

	// Count tag usage
	tagCount := make(map[string]int)
	hashtagRegex := regexp.MustCompile(`#[a-zA-Z0-9_]+`)

	for _, item := range result.Items {
		if contentAttr, ok := item["Content"]; ok {
			if content, ok := contentAttr.(*types.AttributeValueMemberS); ok {
				// Extract all hashtags from content
				matches := hashtagRegex.FindAllString(content.Value, -1)
				for _, match := range matches {
					tag := strings.ToLower(strings.TrimPrefix(match, "#"))
					// Skip if already featured
					if !featuredMap[tag] {
						tagCount[tag]++
					}
				}
			}
		}
	}

	// Sort tags by usage count
	type tagFreq struct {
		tag   string
		count int
	}

	tagFreqs := make([]tagFreq, 0, len(tagCount))
	for tag, count := range tagCount {
		tagFreqs = append(tagFreqs, tagFreq{tag: tag, count: count})
	}

	sort.Slice(tagFreqs, func(i, j int) bool {
		return tagFreqs[i].count > tagFreqs[j].count
	})

	// Return top suggestions
	suggestions := make([]string, 0, limit)
	for i := 0; i < len(tagFreqs) && i < limit; i++ {
		suggestions = append(suggestions, tagFreqs[i].tag)
	}

	return suggestions, nil
}

// calculateTagStatistics calculates the count and last usage time for a tag
func (s *dynamoDBStorage) calculateTagStatistics(ctx context.Context, userID string, tagName string) (int, string) {
	// Query user's statuses to find those with the tag
	// Using GSI3 to get user's statuses
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_STATUS#%s", userID)},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		s.logger().Warn("failed to query user statuses for tag statistics",
			zap.String("user_id", userID),
			zap.String("tag", tagName),
			zap.Error(err))
		return 0, ""
	}

	count := 0
	var lastStatusAt string
	tagPattern := fmt.Sprintf("#%s", tagName)

	for _, item := range result.Items {
		// Check if status contains the tag
		if contentAttr, ok := item["Content"]; ok {
			if content, ok := contentAttr.(*types.AttributeValueMemberS); ok {
				// Simple case-insensitive check for the hashtag
				if strings.Contains(strings.ToLower(content.Value), strings.ToLower(tagPattern)) {
					count++

					// Get the timestamp of the first (most recent) match
					if lastStatusAt == "" {
						if publishedAttr, ok := item["Published"]; ok {
							if published, ok := publishedAttr.(*types.AttributeValueMemberS); ok {
								lastStatusAt = published.Value
							}
						}
					}
				}
			}
		}
	}

	return count, lastStatusAt
}
