package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// HashtagFollow represents a user following a hashtag
type HashtagFollow struct {
	PK        string    `dynamodbav:"PK"` // USER#username
	SK        string    `dynamodbav:"SK"` // HASHTAG_FOLLOW#tagname
	Username  string    `dynamodbav:"Username"`
	Hashtag   string    `dynamodbav:"Hashtag"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// FollowHashtag creates a follow relationship between a user and a hashtag
func (s *dynamoDBStorage) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	follow := HashtagFollow{
		PK:        s.userPK(userID),
		SK:        fmt.Sprintf("HASHTAG_FOLLOW#%s", hashtag),
		Username:  userID,
		Hashtag:   hashtag,
		CreatedAt: time.Now(),
	}

	av, err := attributevalue.MarshalMap(follow)
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
		Limit:             aws.Int32(int32(limit)),
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
		if err := attributevalue.UnmarshalMap(item, &follow); err != nil {
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
