package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// HashtagFollow represents a user following a hashtag
type HashtagFollow struct {
	PK          string    `dynamodbav:"PK"` // USER#username
	SK          string    `dynamodbav:"SK"` // HASHTAG_FOLLOW#tagname
	Username    string    `dynamodbav:"Username"`
	Hashtag     string    `dynamodbav:"Hashtag"`
	CreatedAt   time.Time `dynamodbav:"CreatedAt"`
	NotifyLevel string    `dynamodbav:"NotifyLevel"` // "all", "mutuals", "none"

	// GSI attributes for follows-by-hashtag index
	GSI1PK string `dynamodbav:"GSI1PK"` // HASHTAG#tagname
	GSI1SK string `dynamodbav:"GSI1SK"` // USER#username
}

// HashtagStats tracks statistics for a hashtag
type HashtagStats struct {
	PK            string    `dynamodbav:"PK"` // HASHTAG#tagname
	SK            string    `dynamodbav:"SK"` // STATS#HASHTAG
	Hashtag       string    `dynamodbav:"Hashtag"`
	FollowerCount int       `dynamodbav:"FollowerCount"`
	PostCount     int       `dynamodbav:"PostCount"`
	LastActivity  time.Time `dynamodbav:"LastActivity"`
	TrendingScore float64   `dynamodbav:"TrendingScore"`
	UpdatedAt     time.Time `dynamodbav:"UpdatedAt"`
}

// HashtagNotificationPreference represents notification settings for hashtag follows
type HashtagNotificationPreference struct {
	UserID              string `dynamodbav:"UserID"`
	Hashtag             string `dynamodbav:"Hashtag"`
	NotifyFromFollowing bool   `dynamodbav:"NotifyFromFollowing"`
	NotifyFromMutuals   bool   `dynamodbav:"NotifyFromMutuals"`
	MinimumEngagement   int    `dynamodbav:"MinimumEngagement"`
}

// FollowHashtag creates a follow relationship between a user and a hashtag
func (s *dynamoDBStorage) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	return s.FollowHashtagWithNotifications(ctx, userID, hashtag, "all")
}

// FollowHashtagWithNotifications creates a follow relationship with notification preferences
func (s *dynamoDBStorage) FollowHashtagWithNotifications(ctx context.Context, userID string, hashtag string, notifyLevel string) error {
	now := time.Now()
	follow := HashtagFollow{
		PK:          s.userPK(userID),
		SK:          fmt.Sprintf("HASHTAG_FOLLOW#%s", hashtag),
		Username:    userID,
		Hashtag:     hashtag,
		CreatedAt:   now,
		NotifyLevel: notifyLevel,
		GSI1PK:      fmt.Sprintf("HASHTAG#%s", hashtag),
		GSI1SK:      s.userPK(userID),
	}

	av, err := s.MarshalItem(follow)
	if err != nil {
		return fmt.Errorf("failed to marshal hashtag follow: %w", err)
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
			// Already following
			return nil
		}
		return fmt.Errorf("failed to create hashtag follow: %w", err)
	}

	// Update hashtag follower count
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG#%s", hashtag)},
			"SK": &types.AttributeValueMemberS{Value: "STATS#HASHTAG"},
		},
		UpdateExpression: aws.String("ADD FollowerCount :inc SET UpdatedAt = :now, Hashtag = :hashtag"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc":     &types.AttributeValueMemberN{Value: "1"},
			":now":     &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":hashtag": &types.AttributeValueMemberS{Value: hashtag},
		},
	}

	_, err = s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		s.logger().Warn("failed to update hashtag follower count",
			zap.Error(err),
			zap.String("hashtag", hashtag))
	}

	return nil
}

// UnfollowHashtag removes a follow relationship between a user and a hashtag
func (s *dynamoDBStorage) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG_FOLLOW#%s", hashtag)},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete hashtag follow: %w", err)
	}

	// Update hashtag follower count
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG#%s", hashtag)},
			"SK": &types.AttributeValueMemberS{Value: "STATS#HASHTAG"},
		},
		UpdateExpression:    aws.String("ADD FollowerCount :dec SET UpdatedAt = :now"),
		ConditionExpression: aws.String("FollowerCount > :zero"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":dec":  &types.AttributeValueMemberN{Value: "-1"},
			":now":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
			":zero": &types.AttributeValueMemberN{Value: "0"},
		},
	}

	_, err = s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		s.logger().Warn("failed to update hashtag follower count on unfollow",
			zap.Error(err),
			zap.String("hashtag", hashtag))
	}

	return nil
}

// IsFollowingHashtag checks if a user is following a hashtag
func (s *dynamoDBStorage) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG_FOLLOW#%s", hashtag)},
		},
		ConsistentRead: aws.Bool(false),
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check hashtag follow: %w", err)
	}

	return result.Item != nil, nil
}

// GetFollowedHashtags returns hashtags followed by a user
func (s *dynamoDBStorage) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	var exclusiveStartKey map[string]types.AttributeValue
	if cursor != "" {
		exclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: s.userPK(userID)},
			":prefix": &types.AttributeValueMemberS{Value: "HASHTAG_FOLLOW#"},
		},
		Limit:             safeInt32(limit),
		ExclusiveStartKey: exclusiveStartKey,
		ConsistentRead:    aws.Bool(false),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query followed hashtags: %w", err)
	}

	hashtags := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		var follow HashtagFollow
		if err := s.UnmarshalItem(item, &follow); err != nil {
			s.logger().Warn("failed to unmarshal hashtag follow", zap.Error(err))
			continue
		}
		hashtags = append(hashtags, follow.Hashtag)
	}

	// Prepare next cursor
	nextCursor := ""
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return hashtags, nextCursor, nil
}

// GetHashtagFollowers returns users following a hashtag
func (s *dynamoDBStorage) GetHashtagFollowers(ctx context.Context, hashtag string, limit int, cursor string) ([]string, string, error) {
	var exclusiveStartKey map[string]types.AttributeValue
	if cursor != "" {
		exclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG#%s", hashtag)},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("follows-by-hashtag"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG#%s", hashtag)},
		},
		Limit:             safeInt32(limit),
		ExclusiveStartKey: exclusiveStartKey,
		ConsistentRead:    aws.Bool(false),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query hashtag followers: %w", err)
	}

	users := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		var follow HashtagFollow
		if err := s.UnmarshalItem(item, &follow); err != nil {
			s.logger().Warn("failed to unmarshal hashtag follow", zap.Error(err))
			continue
		}
		users = append(users, follow.Username)
	}

	// Prepare next cursor
	nextCursor := ""
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return users, nextCursor, nil
}

// GetHashtagStats retrieves statistics for a hashtag
func (s *dynamoDBStorage) GetHashtagStats(ctx context.Context, hashtag string) (interface{}, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG#%s", hashtag)},
			"SK": &types.AttributeValueMemberS{Value: "STATS#HASHTAG"},
		},
		ConsistentRead: aws.Bool(false),
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get hashtag stats: %w", err)
	}

	if result.Item == nil {
		// No stats yet, return zero values
		return &HashtagStats{
			Hashtag:       hashtag,
			FollowerCount: 0,
			PostCount:     0,
			UpdatedAt:     time.Now(),
		}, nil
	}

	var stats HashtagStats
	if err := s.UnmarshalItem(result.Item, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hashtag stats: %w", err)
	}

	return &stats, nil
}

// UpdateHashtagNotificationPreference updates notification preferences for a hashtag follow
func (s *dynamoDBStorage) UpdateHashtagNotificationPreference(ctx context.Context, userID string, hashtag string, notifyLevel string) error {
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG_FOLLOW#%s", hashtag)},
		},
		UpdateExpression: aws.String("SET NotifyLevel = :level"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":level": &types.AttributeValueMemberS{Value: notifyLevel},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err := s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return fmt.Errorf("hashtag follow not found")
		}
		return fmt.Errorf("failed to update notification preference: %w", err)
	}

	return nil
}
