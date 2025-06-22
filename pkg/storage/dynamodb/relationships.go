package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// CreateFollow creates a new follow relationship
func (s *dynamoDBStorage) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	log := common.WithContext(ctx)

	// Create the relationship record
	now := time.Now()
	record := &storage.RelationshipRecord{
		PK:         fmt.Sprintf("%s%s", storage.FollowPKPrefix, followerUsername),
		SK:         fmt.Sprintf("%s%s", storage.FollowingSKPrefix, followedUsername),
		GSI1PK:     fmt.Sprintf("%s%s", storage.FollowPKPrefix, followedUsername),
		GSI1SK:     fmt.Sprintf("%s%s", storage.FollowerSKPrefix, followerUsername),
		ActivityID: followActivityID,
		State:      storage.RelationshipPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Marshal the record
	av, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal relationship record", zap.Error(err))
		return fmt.Errorf("failed to marshal relationship: %w", err)
	}

	// Put the item with conditional check to prevent duplicates
	input := &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			// Relationship already exists
			log.Debug("follow relationship already exists",
				zap.String("follower", followerUsername),
				zap.String("followed", followedUsername))
			return nil
		}
		log.Error("failed to create follow relationship", zap.Error(err))
		return fmt.Errorf("failed to create follow: %w", err)
	}

	log.Info("created follow relationship",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername),
		zap.String("activity_id", followActivityID))

	return nil
}

// AcceptFollow updates a follow relationship to accepted state
func (s *dynamoDBStorage) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	log := common.WithContext(ctx)

	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, followerUsername)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowingSKPrefix, followedUsername)},
		},
		UpdateExpression:    aws.String("SET #state = :state, UpdatedAt = :updated"),
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
		ExpressionAttributeNames: map[string]string{
			"#state": "State",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":state":   &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
			":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}

	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return fmt.Errorf("follow relationship not found")
		}
		log.Error("failed to accept follow", zap.Error(err))
		return fmt.Errorf("failed to accept follow: %w", err)
	}

	log.Info("accepted follow relationship",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	// Update follower counts
	// Increment followed user's follower count
	if err := s.UpdateFollowerCount(ctx, followedUsername, 1); err != nil {
		log.Error("failed to update follower count",
			zap.String("username", followedUsername),
			zap.Error(err))
		// Don't fail the operation, just log the error
	}

	// Increment follower's following count
	if err := s.UpdateFollowingCount(ctx, followerUsername, 1); err != nil {
		log.Error("failed to update following count",
			zap.String("username", followerUsername),
			zap.Error(err))
		// Don't fail the operation, just log the error
	}

	return nil
}

// RejectFollow updates a follow relationship to rejected state
func (s *dynamoDBStorage) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	log := common.WithContext(ctx)

	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, followerUsername)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowingSKPrefix, followedUsername)},
		},
		UpdateExpression:    aws.String("SET #state = :state, UpdatedAt = :updated"),
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
		ExpressionAttributeNames: map[string]string{
			"#state": "State",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":state":   &types.AttributeValueMemberS{Value: storage.RelationshipRejected},
			":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}

	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		log.Error("failed to reject follow", zap.Error(err))
		return fmt.Errorf("failed to reject follow: %w", err)
	}

	return nil
}

// RemoveFollow deletes a follow relationship
func (s *dynamoDBStorage) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	log := common.WithContext(ctx)

	// First check if the relationship was accepted (to know if we need to update counts)
	wasAccepted := false
	getInput := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, followerUsername)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowingSKPrefix, followedUsername)},
		},
		ProjectionExpression: aws.String("#state"),
		ExpressionAttributeNames: map[string]string{
			"#state": "State",
		},
	}

	getOutput, err := s.client.GetItem(ctx, getInput)
	if err == nil && getOutput.Item != nil {
		if state, ok := getOutput.Item["State"].(*types.AttributeValueMemberS); ok {
			wasAccepted = state.Value == storage.RelationshipAccepted
		}
	}

	// Delete the relationship
	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, followerUsername)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowingSKPrefix, followedUsername)},
		},
	}

	_, err = s.client.DeleteItem(ctx, input)
	if err != nil {
		log.Error("failed to remove follow", zap.Error(err))
		return fmt.Errorf("failed to remove follow: %w", err)
	}

	// Update follower counts if the relationship was accepted
	if wasAccepted {
		// Decrement followed user's follower count
		if err := s.UpdateFollowerCount(ctx, followedUsername, -1); err != nil {
			log.Error("failed to update follower count",
				zap.String("username", followedUsername),
				zap.Error(err))
			// Don't fail the operation, just log the error
		}

		// Decrement follower's following count
		if err := s.UpdateFollowingCount(ctx, followerUsername, -1); err != nil {
			log.Error("failed to update following count",
				zap.String("username", followerUsername),
				zap.Error(err))
			// Don't fail the operation, just log the error
		}
	}

	return nil
}

// GetFollowers returns a paginated list of followers for a given user
func (s *dynamoDBStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	log := common.WithContext(ctx)

	// Build the query input
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, username)},
		},
		Limit: safeInt32(limit),
	}

	// Add filter for accepted relationships only
	input.FilterExpression = aws.String("#state = :state")
	if input.ExpressionAttributeNames == nil {
		input.ExpressionAttributeNames = make(map[string]string)
	}
	input.ExpressionAttributeNames["#state"] = "State"
	input.ExpressionAttributeValues[":state"] = &types.AttributeValueMemberS{Value: storage.RelationshipAccepted}

	// Handle cursor for pagination
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			log.Error("failed to decode cursor", zap.Error(err))
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		input.ExclusiveStartKey = startKey
	}

	// Execute query
	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query followers", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query followers: %w", err)
	}

	// Extract follower usernames
	followers := make([]string, 0, len(output.Items))
	for _, item := range output.Items {
		var record storage.RelationshipRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal relationship record", zap.Error(err))
			continue
		}

		// Extract username from GSI1SK (format: FOLLOWER#{username})
		if len(record.GSI1SK) > len(storage.FollowerSKPrefix) {
			followerUsername := record.GSI1SK[len(storage.FollowerSKPrefix):]
			followers = append(followers, followerUsername)
		}
	}

	// Handle pagination cursor
	var nextCursor string
	if output.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(output.LastEvaluatedKey)
	}

	return followers, nextCursor, nil
}

// GetFollowing returns a paginated list of users that the given user follows
func (s *dynamoDBStorage) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	log := common.WithContext(ctx)

	// Build the query input
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, username)},
		},
		Limit: safeInt32(limit),
	}

	// Add filter for accepted relationships only
	input.FilterExpression = aws.String("#state = :state")
	input.ExpressionAttributeNames = map[string]string{
		"#state": "State",
	}
	input.ExpressionAttributeValues[":state"] = &types.AttributeValueMemberS{Value: storage.RelationshipAccepted}

	// Handle cursor for pagination
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			log.Error("failed to decode cursor", zap.Error(err))
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		input.ExclusiveStartKey = startKey
	}

	// Execute query
	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query following", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query following: %w", err)
	}

	// Extract followed usernames
	following := make([]string, 0, len(output.Items))
	for _, item := range output.Items {
		var record storage.RelationshipRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal relationship record", zap.Error(err))
			continue
		}

		// Extract username from SK (format: FOLLOWING#{username})
		if len(record.SK) > len(storage.FollowingSKPrefix) {
			followedUsername := record.SK[len(storage.FollowingSKPrefix):]
			following = append(following, followedUsername)
		}
	}

	// Handle pagination cursor
	var nextCursor string
	if output.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(output.LastEvaluatedKey)
	}

	return following, nextCursor, nil
}

// IsFollowing checks if followerUsername follows followedUsername
func (s *dynamoDBStorage) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	log := common.WithContext(ctx)

	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, followerUsername)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowingSKPrefix, followedUsername)},
		},
	}

	output, err := s.client.GetItem(ctx, input)
	if err != nil {
		log.Error("failed to check following status", zap.Error(err))
		return false, fmt.Errorf("failed to check following: %w", err)
	}

	// If no item found, not following
	if output.Item == nil {
		return false, nil
	}

	// Check if the relationship is accepted
	var record storage.RelationshipRecord
	if err := s.UnmarshalItem(output.Item, &record); err != nil {
		log.Error("failed to unmarshal relationship record", zap.Error(err))
		return false, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return record.State == storage.RelationshipAccepted, nil
}

// RemoveFromFollowers removes a follower from the current user's followers list
// This is an alias for RemoveFollow with parameters in the order expected by the interface
func (s *dynamoDBStorage) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	// RemoveFollow expects (follower, followed), so we swap the parameters
	return s.RemoveFollow(ctx, followerUsername, username)
}

// GetPendingFollowRequests returns pending follow requests for a user with locked account
func (s *dynamoDBStorage) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	log := common.WithContext(ctx)

	// Build the query input to get all follow relationships where this user is followed
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, username)},
		},
		Limit: safeInt32(limit),
	}

	// Add filter for pending relationships only
	input.FilterExpression = aws.String("#state = :state")
	if input.ExpressionAttributeNames == nil {
		input.ExpressionAttributeNames = make(map[string]string)
	}
	input.ExpressionAttributeNames["#state"] = "State"
	input.ExpressionAttributeValues[":state"] = &types.AttributeValueMemberS{Value: storage.RelationshipPending}

	// Handle cursor for pagination
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			log.Error("failed to decode cursor", zap.Error(err))
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		input.ExclusiveStartKey = startKey
	}

	// Execute query
	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query pending follow requests", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query pending requests: %w", err)
	}

	// Extract follower usernames from pending requests
	followers := make([]string, 0, len(output.Items))
	for _, item := range output.Items {
		var record storage.RelationshipRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal relationship record", zap.Error(err))
			continue
		}

		// Extract username from GSI1SK (format: FOLLOWER#{username})
		if len(record.GSI1SK) > len(storage.FollowerSKPrefix) {
			followerUsername := record.GSI1SK[len(storage.FollowerSKPrefix):]
			followers = append(followers, followerUsername)
		}
	}

	// Handle pagination cursor
	var nextCursor string
	if output.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(output.LastEvaluatedKey)
	}

	return followers, nextCursor, nil
}

// GetFollowRequestState returns the state of a follow request between two users
func (s *dynamoDBStorage) GetFollowRequestState(ctx context.Context, followerUsername, followedUsername string) (string, error) {
	log := common.WithContext(ctx)

	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowPKPrefix, followerUsername)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", storage.FollowingSKPrefix, followedUsername)},
		},
		ProjectionExpression: aws.String("#state"),
		ExpressionAttributeNames: map[string]string{
			"#state": "State",
		},
	}

	output, err := s.client.GetItem(ctx, input)
	if err != nil {
		log.Error("failed to get follow request state", zap.Error(err))
		return "", fmt.Errorf("failed to get follow state: %w", err)
	}

	// If no item found, no relationship exists
	if output.Item == nil {
		return "", nil
	}

	// Extract the state
	if state, ok := output.Item["State"].(*types.AttributeValueMemberS); ok {
		return state.Value, nil
	}

	return "", fmt.Errorf("invalid state format in relationship record")
}

// GetFollowRequest gets a follow request by follower and target IDs
func (s *dynamoDBStorage) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	log := common.WithContext(ctx)

	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FOLLOW#%s", followerID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FOLLOWING#%s", targetID)},
		},
	}

	output, err := s.client.GetItem(ctx, input)
	if err != nil {
		log.Error("failed to get follow request", zap.Error(err))
		return nil, fmt.Errorf("failed to get follow request: %w", err)
	}

	if output.Item == nil {
		return nil, fmt.Errorf("follow request not found")
	}

	var record storage.RelationshipRecord
	err = s.UnmarshalItem(output.Item, &record)
	if err != nil {
		log.Error("failed to unmarshal follow request", zap.Error(err))
		return nil, fmt.Errorf("failed to parse follow request: %w", err)
	}

	return &record, nil
}

// AcceptFollowRequest accepts a follow request by follower and target IDs
func (s *dynamoDBStorage) AcceptFollowRequest(ctx context.Context, followerID, targetID string) error {
	// Use the existing AcceptFollow method
	return s.AcceptFollow(ctx, followerID, targetID)
}

// RejectFollowRequest rejects a follow request by follower and target IDs
func (s *dynamoDBStorage) RejectFollowRequest(ctx context.Context, followerID, targetID string) error {
	// Use the existing RejectFollow method
	return s.RejectFollow(ctx, followerID, targetID)
}
