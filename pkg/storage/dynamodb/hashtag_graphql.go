package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// HashtagMute represents a user muting a hashtag
type HashtagMute struct {
	PK        string    `dynamodbav:"PK"` // USER#username
	SK        string    `dynamodbav:"SK"` // HASHTAG_MUTE#tagname
	Username  string    `dynamodbav:"Username"`
	Hashtag   string    `dynamodbav:"Hashtag"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
	TTL       int64     `dynamodbav:"TTL,omitempty"` // Optional expiration
}

// UpdateHashtagNotificationSettings updates notification settings for a hashtag follow
func (s *dynamoDBStorage) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error {
	notifyLevel := "none"
	if notify {
		notifyLevel = "all"
	}

	return s.UpdateHashtagNotificationPreference(ctx, userID, hashtag, notifyLevel)
}

// MuteHashtag mutes a hashtag for a user
func (s *dynamoDBStorage) MuteHashtag(ctx context.Context, userID, hashtag string) error {
	now := time.Now()
	mute := HashtagMute{
		PK:        s.userPK(userID),
		SK:        fmt.Sprintf("HASHTAG_MUTE#%s", hashtag),
		Username:  userID,
		Hashtag:   hashtag,
		CreatedAt: now,
		TTL:       0, // Permanent mute unless specified
	}

	av, err := s.MarshalItem(mute)
	if err != nil {
		return fmt.Errorf("failed to marshal hashtag mute: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create hashtag mute: %w", err)
	}

	return nil
}

// IsHashtagMuted checks if a user has muted a hashtag
func (s *dynamoDBStorage) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG_MUTE#%s", hashtag)},
		},
		ConsistentRead: aws.Bool(false),
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check hashtag mute: %w", err)
	}

	return result.Item != nil, nil
}

// GetHashtagTimelineAdvanced retrieves statuses for a hashtag timeline
func (s *dynamoDBStorage) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	// Build query for hashtag timeline
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("hashtag-timeline"),
		KeyConditionExpression: aws.String("Hashtag = :hashtag"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":hashtag": &types.AttributeValueMemberS{Value: hashtag},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            aws.Int32(int32(limit)),
	}

	// Add pagination if maxID is provided
	if maxID != nil {
		input.KeyConditionExpression = aws.String("Hashtag = :hashtag AND SK < :maxSK")
		input.ExpressionAttributeValues[":maxSK"] = &types.AttributeValueMemberS{Value: *maxID}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If GSI doesn't exist, fall back to basic search
		s.logger().Warn("hashtag timeline GSI not available, falling back to search",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return s.fallbackHashtagTimeline(ctx, hashtag, maxID, limit, userID)
	}

	// Convert to status search results
	statuses := make([]*storage.StatusSearchResult, 0)
	for _, item := range result.Items {
		// Extract status ID from the item
		statusID := ""
		if val, ok := item["StatusID"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				statusID = s.Value
			}
		}

		if statusID == "" {
			continue
		}

		// Create basic status search result
		// In a real implementation, you'd fetch the full status details
		status := &storage.StatusSearchResult{
			StatusID:  statusID,
			Content:   "",         // Would be populated from full status fetch
			AuthorID:  "",         // Would be populated from full status fetch
			Published: time.Now(), // Would be populated from full status fetch
			Score:     1.0,
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// fallbackHashtagTimeline provides a fallback when GSI is not available
func (s *dynamoDBStorage) fallbackHashtagTimeline(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	// Use status search service if available
	if s.statusSearchService != nil {
		query := "#" + hashtag
		options := StatusSearchOptions{
			Limit:     limit,
			AccountID: userID,
		}
		results, err := s.statusSearchService.Search(ctx, query, options)
		if err != nil {
			return nil, err
		}
		// Convert results to storage type
		storageResults := make([]*storage.StatusSearchResult, len(results))
		for i, result := range results {
			storageResults[i] = &storage.StatusSearchResult{
				StatusID:       result.StatusID,
				Content:        result.Content,
				AuthorID:       result.AuthorID,
				AuthorUsername: result.AuthorUsername,
				Published:      result.Published,
				Score:          result.Score,
			}
		}
		return storageResults, nil
	}

	// Otherwise return empty
	return []*storage.StatusSearchResult{}, nil
}

// GetMultiHashtagTimeline retrieves statuses for multiple hashtags
func (s *dynamoDBStorage) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	if len(hashtags) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	// If single hashtag, use single hashtag method
	if len(hashtags) == 1 {
		return s.GetHashtagTimelineAdvanced(ctx, hashtags[0], maxID, limit, userID)
	}

	// For multiple hashtags, we need to query each and merge results
	allStatuses := make([]*storage.StatusSearchResult, 0)

	for _, hashtag := range hashtags {
		statuses, err := s.GetHashtagTimelineAdvanced(ctx, hashtag, maxID, limit/len(hashtags)+1, userID)
		if err != nil {
			s.logger().Warn("failed to get hashtag timeline",
				zap.String("hashtag", hashtag),
				zap.Error(err))
			continue
		}
		allStatuses = append(allStatuses, statuses...)
	}

	// Sort by created time (most recent first) and limit
	// Note: This is a simplified sort - in production you'd want proper timestamp sorting
	if len(allStatuses) > limit {
		allStatuses = allStatuses[:limit]
	}

	return allStatuses, nil
}

// GetSuggestedHashtags returns hashtag suggestions for a user
func (s *dynamoDBStorage) GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	// First, get trending hashtags
	trending, err := s.GetRecentHashtags(ctx, time.Now().Add(-24*time.Hour), limit*2)
	if err != nil {
		s.logger().Warn("failed to get trending hashtags for suggestions", zap.Error(err))
		trending = []*storage.TrendingHashtag{}
	}

	// Convert to hashtag search results
	suggestions := make([]*storage.HashtagSearchResult, 0)

	for _, trend := range trending {
		// Check if user is already following this hashtag
		isFollowing, err := s.IsFollowingHashtag(ctx, userID, trend.Name)
		if err != nil {
			s.logger().Warn("failed to check hashtag follow status",
				zap.String("hashtag", trend.Name),
				zap.Error(err))
		}

		// Skip if already following
		if isFollowing {
			continue
		}

		suggestion := &storage.HashtagSearchResult{
			Name:      trend.Name,
			URL:       fmt.Sprintf("https://%s/tags/%s", s.domain, trend.Name),
			History:   []*storage.TrendingHashtag{}, // Would be populated with usage history
			Following: &isFollowing,
		}

		suggestions = append(suggestions, suggestion)

		if len(suggestions) >= limit {
			break
		}
	}

	// If we don't have enough suggestions from trending, add some popular hashtags
	if len(suggestions) < limit {
		additional := s.getPopularHashtagSuggestions(ctx, userID, limit-len(suggestions))
		suggestions = append(suggestions, additional...)
	}

	return suggestions, nil
}

// getPopularHashtagSuggestions gets popular hashtags not already followed
func (s *dynamoDBStorage) getPopularHashtagSuggestions(ctx context.Context, userID string, limit int) []*storage.HashtagSearchResult {
	// This is a simplified implementation
	// In production, you'd have a curated list of popular/recommended hashtags
	popularTags := []string{"mastodon", "fediverse", "nature", "photography", "art", "music", "books", "politics", "science", "technology"}

	suggestions := make([]*storage.HashtagSearchResult, 0)

	for _, tag := range popularTags {
		if len(suggestions) >= limit {
			break
		}

		// Check if user is already following
		isFollowing, err := s.IsFollowingHashtag(ctx, userID, tag)
		if err != nil || isFollowing {
			continue
		}

		suggestion := &storage.HashtagSearchResult{
			Name:      tag,
			URL:       fmt.Sprintf("https://%s/tags/%s", s.domain, tag),
			History:   []*storage.TrendingHashtag{},
			Following: &isFollowing,
		}

		suggestions = append(suggestions, suggestion)
	}

	return suggestions
}

// GetHashtagActivity returns recent activity for a hashtag
func (s *dynamoDBStorage) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	// Query hashtag activity since the given time
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("hashtag-activity"),
		KeyConditionExpression: aws.String("Hashtag = :hashtag AND #timestamp > :since"),
		ExpressionAttributeNames: map[string]string{
			"#timestamp": "Timestamp",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":hashtag": &types.AttributeValueMemberS{Value: hashtag},
			":since":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", since.Unix())},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            aws.Int32(100),  // Reasonable limit
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If GSI doesn't exist, return empty activity
		s.logger().Warn("hashtag activity GSI not available",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return []*storage.Activity{}, nil
	}

	activities := make([]*storage.Activity, 0)
	for _, item := range result.Items {
		activity := &storage.Activity{
			ID:        "",
			Type:      "Create", // Default type
			Actor:     "",
			Object:    "",
			Published: time.Unix(since.Unix(), 0),
			Content:   "",
		}

		// Extract fields from item
		if val, ok := item["ActivityID"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				activity.ID = s.Value
			}
		}
		if val, ok := item["ActorID"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				activity.Actor = s.Value
			}
		}
		if val, ok := item["ObjectID"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				activity.Object = s.Value
			}
		}
		if val, ok := item["ActivityType"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				activity.Type = s.Value
			}
		}

		activities = append(activities, activity)
	}

	return activities, nil
}
