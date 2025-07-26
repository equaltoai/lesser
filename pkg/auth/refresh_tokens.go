package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

var (
	// ErrTokenReuse indicates a refresh token was reused (security breach)
	ErrTokenReuse = errors.New("refresh token reuse detected")
	// ErrInvalidRefreshToken indicates the refresh token is invalid
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrExpiredRefreshToken indicates the refresh token has expired
	ErrExpiredRefreshToken = errors.New("refresh token expired")
)

// RefreshToken represents a refresh token with rotation support
type RefreshToken struct {
	Token         string `dynamodbav:"token"`      // Partition key
	UserID        string `dynamodbav:"user_id"`    // GSI partition key
	Family        string `dynamodbav:"family"`     // Token family for rotation
	Generation    int    `dynamodbav:"generation"` // Rotation generation
	CreatedAt     int64  `dynamodbav:"created_at"`
	ExpiresAt     int64  `dynamodbav:"expires_at"`
	LastUsedAt    int64  `dynamodbav:"last_used_at"`
	Revoked       bool   `dynamodbav:"revoked"`
	RevokedReason string `dynamodbav:"revoked_reason"`
	DeviceName    string `dynamodbav:"device_name"` // Optional device identifier
	IPAddress     string `dynamodbav:"ip_address"`  // IP address for security monitoring
}

// DynamoDBRefreshAPI defines the subset of DynamoDB operations we use for refresh tokens
type DynamoDBRefreshAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// RefreshTokenStore manages refresh tokens in DynamoDB
type RefreshTokenStore struct {
	db        DynamoDBRefreshAPI
	tableName string
	logger    *zap.Logger
}

// NewRefreshTokenStore creates a new refresh token store
func NewRefreshTokenStore(db DynamoDBRefreshAPI, tableName string) *RefreshTokenStore {
	return &RefreshTokenStore{
		db:        db,
		tableName: tableName,
		logger:    common.Logger(),
	}
}

// CreateRefreshToken generates a new refresh token
func (s *RefreshTokenStore) CreateRefreshToken(ctx context.Context, userID string, deviceName string, ipAddress string) (*RefreshToken, error) {
	tokenBytes := make([]byte, 32)
	familyBytes := make([]byte, 16)

	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	if _, err := rand.Read(familyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate family: %w", err)
	}

	token := &RefreshToken{
		Token:      base64.URLEncoding.EncodeToString(tokenBytes),
		UserID:     userID,
		Family:     base64.URLEncoding.EncodeToString(familyBytes),
		Generation: 1,
		CreatedAt:  time.Now().Unix(),
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days
		Revoked:    false,
		DeviceName: deviceName,
		IPAddress:  ipAddress,
	}

	// Store in DynamoDB
	item, err := attributevalue.MarshalMap(token)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store token: %w", err)
	}

	s.logger.Info("Created new refresh token",
		zap.String("user_id", userID),
		zap.String("family", token.Family),
		zap.String("device", deviceName),
		zap.String("ip", ipAddress))

	return token, nil
}

// GetRefreshToken retrieves a refresh token
func (s *RefreshTokenStore) GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error) {
	result, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"token": &types.AttributeValueMemberS{Value: token},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	if result.Item == nil {
		return nil, ErrInvalidRefreshToken
	}

	var refreshToken RefreshToken
	if err := attributevalue.UnmarshalMap(result.Item, &refreshToken); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Check expiration
	if time.Now().Unix() > refreshToken.ExpiresAt {
		return nil, ErrExpiredRefreshToken
	}

	return &refreshToken, nil
}

// RotateRefreshToken implements secure rotation with reuse detection
func (s *RefreshTokenStore) RotateRefreshToken(ctx context.Context, oldToken string, ipAddress string) (*RefreshToken, error) {
	// Get the old token
	oldRefresh, err := s.GetRefreshToken(ctx, oldToken)
	if err != nil {
		return nil, err
	}

	// Check if token was already used (reuse detection)
	if oldRefresh.Revoked {
		// SECURITY ALERT: Token reuse detected!
		// Revoke entire family
		if err := s.RevokeTokenFamily(ctx, oldRefresh.Family, "Token reuse detected"); err != nil {
			s.logger.Error("Failed to revoke token family after reuse detection",
				zap.String("family", oldRefresh.Family),
				zap.Error(err))
		}

		// Log security event
		common.LogSecurityEvent(common.EventTokenReuse,
			zap.String("user_id", oldRefresh.UserID),
			zap.String("family", oldRefresh.Family),
			zap.String("token", oldToken),
			zap.String("ip", ipAddress),
			zap.Int("generation", oldRefresh.Generation))

		return nil, ErrTokenReuse
	}

	// Create new token in same family
	newTokenBytes := make([]byte, 32)
	if _, err := rand.Read(newTokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate new token: %w", err)
	}

	newToken := &RefreshToken{
		Token:      base64.URLEncoding.EncodeToString(newTokenBytes),
		UserID:     oldRefresh.UserID,
		Family:     oldRefresh.Family,
		Generation: oldRefresh.Generation + 1,
		CreatedAt:  time.Now().Unix(),
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour).Unix(),
		Revoked:    false,
		DeviceName: oldRefresh.DeviceName,
		IPAddress:  ipAddress,
	}

	// Revoke old token
	oldRefresh.Revoked = true
	oldRefresh.RevokedReason = "Rotated"
	oldRefresh.LastUsedAt = time.Now().Unix()

	// Store both updates
	newItem, _ := attributevalue.MarshalMap(newToken)

	// Use transaction for atomicity
	_, err = s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Update: &types.Update{
					TableName: aws.String(s.tableName),
					Key: map[string]types.AttributeValue{
						"token": &types.AttributeValueMemberS{Value: oldToken},
					},
					UpdateExpression: aws.String("SET revoked = :true, revoked_reason = :reason, last_used_at = :now"),
					ExpressionAttributeValues: map[string]types.AttributeValue{
						":true":   &types.AttributeValueMemberBOOL{Value: true},
						":reason": &types.AttributeValueMemberS{Value: "Rotated"},
						":now":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
					},
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(s.tableName),
					Item:      newItem,
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to rotate token: %w", err)
	}

	s.logger.Info("Rotated refresh token",
		zap.String("user_id", newToken.UserID),
		zap.String("family", newToken.Family),
		zap.Int("generation", newToken.Generation),
		zap.String("ip", ipAddress))

	return newToken, nil
}

// RevokeTokenFamily revokes all tokens in a family (security breach response)
func (s *RefreshTokenStore) RevokeTokenFamily(ctx context.Context, family string, reason string) error {
	// Build expression to find all tokens in family
	expr, err := expression.NewBuilder().
		WithKeyCondition(expression.Key("family").Equal(expression.Value(family))).
		WithFilter(expression.Name("revoked").Equal(expression.Value(false))).
		Build()
	if err != nil {
		return fmt.Errorf("failed to build expression: %w", err)
	}

	// Query all active tokens in family using GSI
	result, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		IndexName:                 aws.String("family-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return fmt.Errorf("failed to query family tokens: %w", err)
	}

	// Revoke each token
	for _, item := range result.Items {
		if tokenAttr, ok := item["token"].(*types.AttributeValueMemberS); ok {
			_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: aws.String(s.tableName),
				Key: map[string]types.AttributeValue{
					"token": &types.AttributeValueMemberS{Value: tokenAttr.Value},
				},
				UpdateExpression: aws.String("SET revoked = :true, revoked_reason = :reason"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":true":   &types.AttributeValueMemberBOOL{Value: true},
					":reason": &types.AttributeValueMemberS{Value: reason},
				},
			})
			if err != nil {
				s.logger.Error("Failed to revoke token in family",
					zap.String("token", tokenAttr.Value),
					zap.String("family", family),
					zap.Error(err))
			}
		}
	}

	s.logger.Warn("Revoked entire token family",
		zap.String("family", family),
		zap.String("reason", reason),
		zap.Int("count", len(result.Items)))

	return nil
}

// RevokeUserTokens revokes all tokens for a user (logout all devices)
func (s *RefreshTokenStore) RevokeUserTokens(ctx context.Context, userID string, reason string) error {
	// Build expression to find all user's tokens
	expr, err := expression.NewBuilder().
		WithKeyCondition(expression.Key("user_id").Equal(expression.Value(userID))).
		WithFilter(expression.Name("revoked").Equal(expression.Value(false))).
		Build()
	if err != nil {
		return fmt.Errorf("failed to build expression: %w", err)
	}

	// Query all active tokens for user using GSI
	result, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		IndexName:                 aws.String("user-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return fmt.Errorf("failed to query user tokens: %w", err)
	}

	// Revoke each token
	for _, item := range result.Items {
		if tokenAttr, ok := item["token"].(*types.AttributeValueMemberS); ok {
			_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: aws.String(s.tableName),
				Key: map[string]types.AttributeValue{
					"token": &types.AttributeValueMemberS{Value: tokenAttr.Value},
				},
				UpdateExpression: aws.String("SET revoked = :true, revoked_reason = :reason"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":true":   &types.AttributeValueMemberBOOL{Value: true},
					":reason": &types.AttributeValueMemberS{Value: reason},
				},
			})
			if err != nil {
				s.logger.Error("Failed to revoke user token",
					zap.String("token", tokenAttr.Value),
					zap.String("user_id", userID),
					zap.Error(err))
			}
		}
	}

	s.logger.Info("Revoked all user tokens",
		zap.String("user_id", userID),
		zap.String("reason", reason),
		zap.Int("count", len(result.Items)))

	return nil
}

// CreateRefreshTokenTable creates the DynamoDB table for refresh tokens
// Note: This requires a full DynamoDB client, not just the interface
func CreateRefreshTokenTable(ctx context.Context, db *dynamodb.Client, tableName string) error {
	input := &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("token"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("token"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("user_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("family"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("user-index"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("user_id"),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
			{
				IndexName: aws.String("family-index"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("family"),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	}

	_, err := db.CreateTable(ctx, input)
	if err != nil {
		var resourceInUse *types.ResourceInUseException
		if errors.As(err, &resourceInUse) {
			// Table already exists
			return nil
		}
		return fmt.Errorf("failed to create refresh token table: %w", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(db)
	return waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 5*time.Minute)
}
