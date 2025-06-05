package dynamodb

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// UpdateFollowerCount updates an actor's follower count and GSI4 bucket placement
func (s *dynamoDBStorage) UpdateFollowerCount(ctx context.Context, username string, delta int) error {
	log := s.logger()

	// Build the key
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// First, get current follower count
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ProjectionExpression: aws.String("FollowerCount"),
	})

	if err != nil {
		log.Error("failed to get current follower count",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to get current follower count: %w", err)
	}

	if result.Item == nil {
		return fmt.Errorf("actor not found: %s", username)
	}

	// Extract current count
	currentCount := 0
	if fc, ok := result.Item["FollowerCount"]; ok {
		if fcNum, ok := fc.(*types.AttributeValueMemberN); ok {
			currentCount, _ = strconv.Atoi(fcNum.Value)
		}
	}

	// Calculate new count
	newCount := currentCount + delta
	if newCount < 0 {
		newCount = 0
	}

	// Determine old and new buckets
	oldBucket := GetFollowerCountBucket(currentCount)
	newBucket := GetFollowerCountBucket(newCount)

	// Build update expression
	updateExpr := "SET FollowerCount = :count"
	exprValues := map[string]types.AttributeValue{
		":count": &types.AttributeValueMemberN{Value: strconv.Itoa(newCount)},
	}

	// If bucket changed, update GSI4
	if oldBucket != newBucket {
		updateExpr += ", GSI4PK = :gsi4pk, GSI4SK = :gsi4sk"
		exprValues[":gsi4pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR_RANK#%s", newBucket)}
		exprValues[":gsi4sk"] = &types.AttributeValueMemberS{Value: FormatFollowerCountForGSI(newCount, username)}
	} else {
		// Just update the sort key for new count
		updateExpr += ", GSI4SK = :gsi4sk"
		exprValues[":gsi4sk"] = &types.AttributeValueMemberS{Value: FormatFollowerCountForGSI(newCount, username)}
	}

	// Update the item
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprValues,
		ConditionExpression:       aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		log.Error("failed to update follower count",
			zap.String("username", username),
			zap.Int("old_count", currentCount),
			zap.Int("new_count", newCount),
			zap.Error(err))
		return fmt.Errorf("failed to update follower count: %w", err)
	}

	log.Info("follower count updated",
		zap.String("username", username),
		zap.Int("old_count", currentCount),
		zap.Int("new_count", newCount),
		zap.String("old_bucket", oldBucket),
		zap.String("new_bucket", newBucket))

	return nil
}

// UpdateFollowingCount updates an actor's following count
func (s *dynamoDBStorage) UpdateFollowingCount(ctx context.Context, username string, delta int) error {
	log := s.logger()

	// Build the key
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// Update the count
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("ADD FollowingCount :delta"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":delta": &types.AttributeValueMemberN{Value: strconv.Itoa(delta)},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		log.Error("failed to update following count",
			zap.String("username", username),
			zap.Int("delta", delta),
			zap.Error(err))
		return fmt.Errorf("failed to update following count: %w", err)
	}

	return nil
}

// UpdateStatusCount updates an actor's status count
func (s *dynamoDBStorage) UpdateStatusCount(ctx context.Context, username string, delta int) error {
	log := s.logger()

	// Build the key
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// Update the count
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("ADD StatusCount :delta"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":delta": &types.AttributeValueMemberN{Value: strconv.Itoa(delta)},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		log.Error("failed to update status count",
			zap.String("username", username),
			zap.Int("delta", delta),
			zap.Error(err))
		return fmt.Errorf("failed to update status count: %w", err)
	}

	return nil
}

// logger returns the logger instance
func (s *dynamoDBStorage) logger() *zap.Logger {
	// Use the logger from common package or create a new one
	if logger := zap.L(); logger != nil {
		return logger
	}
	return zap.NewNop()
}
