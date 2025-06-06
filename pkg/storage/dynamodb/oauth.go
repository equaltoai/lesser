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

// CreateAuthorizationCode stores a new authorization code
func (s *dynamoDBStorage) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	log := common.WithContext(ctx)

	record := &storage.AuthorizationCodeRecord{
		PK:        storage.AuthCodePKPrefix + code.Code,
		SK:        storage.AuthCodeSK,
		Code:      code,
		CreatedAt: time.Now(),
	}

	item, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal authorization code", zap.Error(err))
		return fmt.Errorf("failed to marshal authorization code: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
		// Only create if it doesn't exist
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return common.ConflictError{
				Resource: "authorization_code",
				Message:  fmt.Sprintf("authorization code already exists: %s", code.Code),
			}
		}
		log.Error("failed to create authorization code", zap.Error(err))
		return fmt.Errorf("failed to create authorization code: %w", err)
	}

	return nil
}

// GetAuthorizationCode retrieves an authorization code
func (s *dynamoDBStorage) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	log := common.WithContext(ctx)

	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: storage.AuthCodePKPrefix + code},
			"SK": &types.AttributeValueMemberS{Value: storage.AuthCodeSK},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		log.Error("failed to get authorization code", zap.Error(err))
		return nil, fmt.Errorf("failed to get authorization code: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("authorization code not found: %s", code)
	}

	var record storage.AuthorizationCodeRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		log.Error("failed to unmarshal authorization code", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal authorization code: %w", err)
	}

	// Check if code has expired
	if time.Now().After(record.Code.ExpiresAt) {
		// Clean up expired code
		_ = s.DeleteAuthorizationCode(ctx, code)
		return nil, fmt.Errorf("authorization code expired: %s", code)
	}

	return record.Code, nil
}

// DeleteAuthorizationCode removes an authorization code
func (s *dynamoDBStorage) DeleteAuthorizationCode(ctx context.Context, code string) error {
	log := common.WithContext(ctx)

	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: storage.AuthCodePKPrefix + code},
			"SK": &types.AttributeValueMemberS{Value: storage.AuthCodeSK},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		log.Error("failed to delete authorization code", zap.Error(err))
		return fmt.Errorf("failed to delete authorization code: %w", err)
	}

	return nil
}

// CreateRefreshToken stores a new refresh token
func (s *dynamoDBStorage) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	log := common.WithContext(ctx)

	record := &storage.RefreshTokenRecord{
		PK:        storage.RefreshTokenPKPrefix + token.Token,
		SK:        storage.RefreshTokenSK,
		Token:     token,
		CreatedAt: time.Now(),
	}

	item, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal refresh token", zap.Error(err))
		return fmt.Errorf("failed to marshal refresh token: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
		// Only create if it doesn't exist
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return common.ConflictError{
				Resource: "refresh_token",
				Message:  "refresh token already exists",
			}
		}
		log.Error("failed to create refresh token", zap.Error(err))
		return fmt.Errorf("failed to create refresh token: %w", err)
	}

	return nil
}

// GetRefreshToken retrieves a refresh token
func (s *dynamoDBStorage) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	log := common.WithContext(ctx)

	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: storage.RefreshTokenPKPrefix + token},
			"SK": &types.AttributeValueMemberS{Value: storage.RefreshTokenSK},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		log.Error("failed to get refresh token", zap.Error(err))
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("refresh token not found")
	}

	var record storage.RefreshTokenRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		log.Error("failed to unmarshal refresh token", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal refresh token: %w", err)
	}

	// Check if token has expired
	if time.Now().After(record.Token.ExpiresAt) {
		// Clean up expired token
		_ = s.DeleteRefreshToken(ctx, token)
		return nil, fmt.Errorf("refresh token expired")
	}

	return record.Token, nil
}

// DeleteRefreshToken removes a refresh token
func (s *dynamoDBStorage) DeleteRefreshToken(ctx context.Context, token string) error {
	log := common.WithContext(ctx)

	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: storage.RefreshTokenPKPrefix + token},
			"SK": &types.AttributeValueMemberS{Value: storage.RefreshTokenSK},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		log.Error("failed to delete refresh token", zap.Error(err))
		return fmt.Errorf("failed to delete refresh token: %w", err)
	}

	return nil
}
