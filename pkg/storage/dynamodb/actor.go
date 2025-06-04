package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// CreateActor creates a new actor in DynamoDB with encrypted private key
func (s *dynamoDBStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	log := common.WithContext(ctx)

	// Extract username from PreferredUsername
	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Build the actor record
	now := time.Now()
	record := storage.ActorRecord{
		PK:         storage.ActorPKPrefix + username,
		SK:         storage.ActorSK,
		Actor:      actor,
		PrivateKey: privateKey, // TODO: Encrypt this with AWS KMS before storing
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Marshal the record to DynamoDB attributes
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		log.Error("failed to marshal actor record",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to marshal actor record: %w", err)
	}

	// Put the item with condition that it doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})

	if err != nil {
		// Check if it's a conditional check failure (actor already exists)
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			log.Warn("actor already exists",
				zap.String("username", username))
			return common.ConflictError{
				Resource: "actor",
				Message:  fmt.Sprintf("actor %s already exists", username),
			}
		}

		log.Error("failed to create actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to create actor: %w", err)
	}

	log.Info("actor created successfully",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	return nil
}

// GetActor retrieves an actor by username from DynamoDB
func (s *dynamoDBStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Get the item
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})

	if err != nil {
		log.Error("failed to get actor",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	if result.Item == nil {
		return nil, common.ActorNotFoundError{Username: username}
	}

	// Unmarshal the actor record
	var record storage.ActorRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		log.Error("failed to unmarshal actor",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal actor: %w", err)
	}

	return record.Actor, nil
}

// GetActorPrivateKey retrieves an actor's private key from DynamoDB
func (s *dynamoDBStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Get only the private key attribute
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ProjectionExpression: aws.String("PrivateKey"),
	})

	if err != nil {
		log.Error("failed to get actor private key",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("failed to get actor private key: %w", err)
	}

	if result.Item == nil {
		return "", common.ActorNotFoundError{Username: username}
	}

	// Extract the private key
	privateKeyAttr, ok := result.Item["PrivateKey"]
	if !ok {
		return "", fmt.Errorf("private key not found for actor %s", username)
	}

	privateKeyStr, ok := privateKeyAttr.(*types.AttributeValueMemberS)
	if !ok || privateKeyStr.Value == "" {
		return "", fmt.Errorf("invalid private key format for actor %s", username)
	}

	// TODO: Decrypt the private key using AWS KMS

	return privateKeyStr.Value, nil
}

// UpdateActor updates an existing actor in DynamoDB
func (s *dynamoDBStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Marshal the actor to attribute value
	actorAttr, err := attributevalue.Marshal(actor)
	if err != nil {
		log.Error("failed to marshal actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to marshal actor: %w", err)
	}

	// Update the item
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("SET Actor = :actor, UpdatedAt = :updated"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":actor":   actorAttr,
			":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		// Check if actor doesn't exist
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return common.ActorNotFoundError{Username: username}
		}

		log.Error("failed to update actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to update actor: %w", err)
	}

	log.Info("actor updated successfully",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	return nil
}

// DeleteActor deletes an actor from DynamoDB
func (s *dynamoDBStorage) DeleteActor(ctx context.Context, username string) error {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Delete the item
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		// Check if actor doesn't exist
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return common.ActorNotFoundError{Username: username}
		}

		log.Error("failed to delete actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to delete actor: %w", err)
	}

	log.Info("actor deleted successfully",
		zap.String("username", username))

	// TODO: Also delete related data (activities, follows, etc.) or use DynamoDB transactions

	return nil
}
