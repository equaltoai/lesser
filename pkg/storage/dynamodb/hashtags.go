package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// Hashtag represents a hashtag with usage statistics
type Hashtag struct {
	Name       string    `dynamodbav:"Name" json:"name"`
	URL        string    `dynamodbav:"URL" json:"url"`
	UsageCount int64     `dynamodbav:"UsageCount" json:"usage_count"`
	FirstSeen  time.Time `dynamodbav:"FirstSeen" json:"first_seen"`
	LastUsed   time.Time `dynamodbav:"LastUsed" json:"last_used"`
}

// HashtagUsage represents a single use of a hashtag
type HashtagUsage struct {
	StatusID   string    `dynamodbav:"StatusID" json:"status_id"`
	AuthorID   string    `dynamodbav:"AuthorID" json:"author_id"`
	UsedAt     time.Time `dynamodbav:"UsedAt" json:"used_at"`
	Visibility string    `dynamodbav:"Visibility" json:"visibility"`
}

// IndexHashtag indexes a hashtag when used in a status
func (s *dynamoDBStorage) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	now := time.Now()
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// Update or create hashtag metadata
	metadataPK := fmt.Sprintf("HASHTAG#%s", tagLower)
	metadataSK := "METADATA"

	// First, try to get existing metadata
	getResult, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: metadataPK},
			"SK": &types.AttributeValueMemberS{Value: metadataSK},
		},
	})

	var existingCount int64 = 0
	var firstSeen time.Time = now

	if err == nil && getResult.Item != nil {
		// Extract existing values
		if count, ok := getResult.Item["UsageCount"]; ok {
			if n, ok := count.(*types.AttributeValueMemberN); ok {
				fmt.Sscanf(n.Value, "%d", &existingCount)
			}
		}
		if fs, ok := getResult.Item["FirstSeen"]; ok {
			if s, ok := fs.(*types.AttributeValueMemberS); ok {
				firstSeen, _ = time.Parse(time.RFC3339, s.Value)
			}
		}
	}

	// Update metadata
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item: map[string]types.AttributeValue{
			"PK":         &types.AttributeValueMemberS{Value: metadataPK},
			"SK":         &types.AttributeValueMemberS{Value: metadataSK},
			"Name":       &types.AttributeValueMemberS{Value: tagLower},
			"URL":        &types.AttributeValueMemberS{Value: fmt.Sprintf("%s/tags/%s", s.config.BaseURL(), tagLower)},
			"UsageCount": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", existingCount+1)},
			"FirstSeen":  &types.AttributeValueMemberS{Value: firstSeen.Format(time.RFC3339)},
			"LastUsed":   &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			"UpdatedAt":  &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			// GSI3 for hashtag search
			"GSI3PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG_SEARCH#%s", getHashtagPrefix(tagLower))},
			"GSI3SK": &types.AttributeValueMemberS{Value: tagLower},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to update hashtag metadata: %w", err)
	}

	// Record usage
	usagePK := fmt.Sprintf("HASHTAG#%s", tagLower)
	usageSK := fmt.Sprintf("USAGE#%d#%s", now.Unix(), statusID)

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item: map[string]types.AttributeValue{
			"PK":         &types.AttributeValueMemberS{Value: usagePK},
			"SK":         &types.AttributeValueMemberS{Value: usageSK},
			"StatusID":   &types.AttributeValueMemberS{Value: statusID},
			"AuthorID":   &types.AttributeValueMemberS{Value: authorID},
			"UsedAt":     &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			"Visibility": &types.AttributeValueMemberS{Value: visibility},
			"TTL":        &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", now.Add(30*24*time.Hour).Unix())}, // 30 days TTL
		},
	})

	if err != nil {
		return fmt.Errorf("failed to record hashtag usage: %w", err)
	}

	return nil
}

// SearchHashtags searches for hashtags matching the query
func (s *dynamoDBStorage) SearchHashtags(ctx context.Context, query string, limit int) ([]*Hashtag, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 40 {
		limit = 40
	}

	searchTerm := strings.ToLower(strings.TrimPrefix(query, "#"))

	// Need at least 1 character for search
	if len(searchTerm) < 1 {
		return []*Hashtag{}, nil
	}

	// Try exact match first
	exactPK := fmt.Sprintf("HASHTAG#%s", searchTerm)
	exactSK := "METADATA"

	exactResult, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: exactPK},
			"SK": &types.AttributeValueMemberS{Value: exactSK},
		},
	})

	results := make([]*Hashtag, 0)

	// Add exact match if found
	if err == nil && exactResult.Item != nil {
		var hashtag Hashtag
		if err := attributevalue.UnmarshalMap(exactResult.Item, &hashtag); err == nil {
			results = append(results, &hashtag)
		}
	}

	// If we need more results, search with prefix
	if len(results) < limit && len(searchTerm) >= 2 {
		// Use GSI3 for prefix search
		gsi3pk := fmt.Sprintf("HASHTAG_SEARCH#%s", getHashtagPrefix(searchTerm))

		expr, err := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("GSI3PK").Equal(expression.Value(gsi3pk)).
					And(expression.Key("GSI3SK").BeginsWith(searchTerm)),
			).
			Build()

		if err != nil {
			return results, fmt.Errorf("failed to build expression: %w", err)
		}

		queryInput := &dynamodb.QueryInput{
			TableName:                 aws.String(s.tableName),
			IndexName:                 aws.String("GSI3"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     aws.Int32(int32(limit - len(results))),
		}

		queryResult, err := s.client.Query(ctx, queryInput)
		if err != nil {
			s.logger.Warn("hashtag prefix search failed", zap.Error(err))
		} else {
			for _, item := range queryResult.Items {
				var hashtag Hashtag
				if err := attributevalue.UnmarshalMap(item, &hashtag); err == nil {
					// Skip if it's the exact match we already added
					if hashtag.Name != searchTerm {
						results = append(results, &hashtag)
					}
				}
			}
		}
	}

	return results, nil
}

// GetHashtagInfo retrieves information about a specific hashtag
func (s *dynamoDBStorage) GetHashtagInfo(ctx context.Context, hashtag string) (*Hashtag, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	pk := fmt.Sprintf("HASHTAG#%s", tagLower)
	sk := "METADATA"

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get hashtag info: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var hashtagInfo Hashtag
	if err := attributevalue.UnmarshalMap(result.Item, &hashtagInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hashtag: %w", err)
	}

	return &hashtagInfo, nil
}

// GetHashtagUsageHistory retrieves recent usage history for a hashtag
func (s *dynamoDBStorage) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// Initialize result array
	history := make([]int64, days)

	// Get usage for each day
	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i)
		dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		dayEnd := dayStart.Add(24 * time.Hour)

		// Query usage for this day
		pk := fmt.Sprintf("HASHTAG#%s", tagLower)
		skStart := fmt.Sprintf("USAGE#%d", dayStart.Unix())
		skEnd := fmt.Sprintf("USAGE#%d", dayEnd.Unix())

		expr, err := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("PK").Equal(expression.Value(pk)).
					And(expression.Key("SK").Between(expression.Value(skStart), expression.Value(skEnd))),
			).
			Build()

		if err != nil {
			continue
		}

		queryInput := &dynamodb.QueryInput{
			TableName:                 aws.String(s.tableName),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Select:                    types.SelectCount,
		}

		result, err := s.client.Query(ctx, queryInput)
		if err == nil {
			history[i] = int64(result.Count)
		}
	}

	return history, nil
}

// getHashtagPrefix returns the first 2 characters of a hashtag for GSI partitioning
func getHashtagPrefix(hashtag string) string {
	tag := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	if len(tag) >= 2 {
		return tag[:2]
	}
	return tag
}

// ExtractHashtags extracts hashtags from text content
func ExtractHashtags(content string) []string {
	// Simple regex-like extraction
	// In production, you'd want a proper regex or parser
	words := strings.Fields(content)
	hashtags := make([]string, 0)
	seen := make(map[string]bool)

	for _, word := range words {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			// Clean the hashtag
			tag := strings.TrimPrefix(word, "#")
			// Remove trailing punctuation
			tag = strings.TrimRight(tag, ".,!?;:")

			if tag != "" && !seen[strings.ToLower(tag)] {
				hashtags = append(hashtags, tag)
				seen[strings.ToLower(tag)] = true
			}
		}
	}

	return hashtags
}
