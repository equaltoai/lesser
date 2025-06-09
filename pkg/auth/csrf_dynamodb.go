package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBAPI defines the subset of DynamoDB operations we use
type DynamoDBAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}

// DynamoDBCSRFStore implements distributed CSRF storage for serverless
type DynamoDBCSRFStore struct {
	db        DynamoDBAPI
	tableName string
}

// CSRFTokenRecord represents a CSRF token in DynamoDB
type CSRFTokenRecord struct {
	Token     string `dynamodbav:"token"` // Partition key
	UserID    string `dynamodbav:"user_id"`
	CreatedAt int64  `dynamodbav:"created_at"`
	ExpiresAt int64  `dynamodbav:"expires_at"` // TTL field
	Used      bool   `dynamodbav:"used"`
}

// NewDynamoDBCSRFStore creates a new DynamoDB-backed CSRF store
func NewDynamoDBCSRFStore(db DynamoDBAPI, tableName string) *DynamoDBCSRFStore {
	return &DynamoDBCSRFStore{
		db:        db,
		tableName: tableName,
	}
}

// Store saves a CSRF token with expiration
func (s *DynamoDBCSRFStore) Store(token string, csrf CSRFToken) error {
	ctx := context.Background()

	// Check token limit per user (prevent DoS)
	count, err := s.GetUserActiveTokenCount(csrf.UserID)
	if err == nil && count >= 10 {
		// Clean up old tokens before rejecting
		s.CleanupUserTokens(csrf.UserID)

		// Check again after cleanup
		count, err = s.GetUserActiveTokenCount(csrf.UserID)
		if err == nil && count >= 10 {
			return fmt.Errorf("too many active CSRF tokens for user")
		}
	}

	record := CSRFTokenRecord{
		Token:     token,
		UserID:    csrf.UserID,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: csrf.ExpiresAt.Unix(),
		Used:      false,
	}

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
		// Prevent duplicate tokens
		ConditionExpression: aws.String("attribute_not_exists(#token)"),
		ExpressionAttributeNames: map[string]string{
			"#token": "token",
		},
	}

	_, err = s.db.PutItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return fmt.Errorf("token already exists")
		}
		return fmt.Errorf("failed to store token: %w", err)
	}

	return nil
}

// Get retrieves a CSRF token
func (s *DynamoDBCSRFStore) Get(token string) (*CSRFToken, error) {
	ctx := context.Background()

	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"token": &types.AttributeValueMemberS{Value: token},
		},
	}

	result, err := s.db.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	if result.Item == nil {
		return nil, ErrInvalidCSRF
	}

	var record CSRFTokenRecord
	err = attributevalue.UnmarshalMap(result.Item, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Check if expired
	if time.Now().Unix() > record.ExpiresAt {
		return nil, ErrExpiredCSRF
	}

	// Check if already used
	if record.Used {
		return nil, ErrInvalidCSRF
	}

	return &CSRFToken{
		Token:     record.Token,
		UserID:    record.UserID,
		ExpiresAt: time.Unix(record.ExpiresAt, 0),
	}, nil
}

// Delete removes a CSRF token
func (s *DynamoDBCSRFStore) Delete(token string) error {
	ctx := context.Background()

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"token": &types.AttributeValueMemberS{Value: token},
		},
	}

	_, err := s.db.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	return nil
}

// ValidateAndConsume validates a token and marks it as used atomically
func (s *DynamoDBCSRFStore) ValidateAndConsume(token string, userID string) error {
	ctx := context.Background()

	// Update with conditions in a single atomic operation
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"token": &types.AttributeValueMemberS{Value: token},
		},
		// Only update if: exists, belongs to user, not expired, not used
		ConditionExpression: aws.String(
			"attribute_exists(#token) AND " +
				"user_id = :user_id AND " +
				"expires_at > :now AND " +
				"used = :false"),
		UpdateExpression: aws.String("SET used = :true"),
		ExpressionAttributeNames: map[string]string{
			"#token": "token",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":user_id": &types.AttributeValueMemberS{Value: userID},
			":now":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
			":false":   &types.AttributeValueMemberBOOL{Value: false},
			":true":    &types.AttributeValueMemberBOOL{Value: true},
		},
	}

	_, err := s.db.UpdateItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			// Check if token exists to provide better error
			storedToken, getErr := s.Get(token)
			if getErr == ErrInvalidCSRF {
				return ErrInvalidCSRF
			}
			if getErr == ErrExpiredCSRF {
				return ErrExpiredCSRF
			}
			if storedToken != nil && storedToken.UserID != userID {
				return ErrInvalidCSRF
			}
			return ErrInvalidCSRF
		}
		return fmt.Errorf("failed to validate token: %w", err)
	}

	return nil
}

// CleanExpired removes expired tokens (called periodically)
func (s *DynamoDBCSRFStore) CleanExpired() error {
	// DynamoDB TTL handles this automatically
	// This method exists for interface compatibility
	return nil
}

// GetUserActiveTokenCount returns the number of active tokens for a user
func (s *DynamoDBCSRFStore) GetUserActiveTokenCount(userID string) (int, error) {
	ctx := context.Background()

	// Build a query expression
	expr, err := expression.NewBuilder().
		WithFilter(
			expression.And(
				expression.Name("user_id").Equal(expression.Value(userID)),
				expression.Name("expires_at").GreaterThan(expression.Value(time.Now().Unix())),
				expression.Name("used").Equal(expression.Value(false)),
			),
		).
		Build()

	if err != nil {
		return 0, fmt.Errorf("failed to build expression: %w", err)
	}

	// Scan with filter (not ideal for large tables, but CSRF tokens are short-lived)
	input := &dynamodb.ScanInput{
		TableName:                 aws.String(s.tableName),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Select:                    types.SelectCount,
	}

	result, err := s.db.Scan(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	return int(result.Count), nil
}

// CleanupUserTokens removes old/used tokens for a user
func (s *DynamoDBCSRFStore) CleanupUserTokens(userID string) error {
	ctx := context.Background()

	// Build scan expression for user's tokens
	expr, err := expression.NewBuilder().
		WithFilter(
			expression.And(
				expression.Name("user_id").Equal(expression.Value(userID)),
				expression.Or(
					expression.Name("expires_at").LessThanEqual(expression.Value(time.Now().Unix())),
					expression.Name("used").Equal(expression.Value(true)),
				),
			),
		).
		Build()

	if err != nil {
		return fmt.Errorf("failed to build expression: %w", err)
	}

	// Scan for tokens to delete
	scanInput := &dynamodb.ScanInput{
		TableName:                 aws.String(s.tableName),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ProjectionExpression:      aws.String("token"),
	}

	result, err := s.db.Scan(ctx, scanInput)
	if err != nil {
		return fmt.Errorf("failed to scan tokens: %w", err)
	}

	// Delete each token
	for _, item := range result.Items {
		if tokenAttr, ok := item["token"].(*types.AttributeValueMemberS); ok {
			s.Delete(tokenAttr.Value)
		}
	}

	return nil
}

// CreateCSRFTable creates the DynamoDB table for CSRF tokens
// Note: This requires a full DynamoDB client, not just the interface
func CreateCSRFTable(ctx context.Context, db *dynamodb.Client, tableName string) error {
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
		return fmt.Errorf("failed to create CSRF table: %w", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(db)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("failed waiting for table to be active: %w", err)
	}

	// Enable TTL on the table
	ttlInput := &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			Enabled:       aws.Bool(true),
			AttributeName: aws.String("expires_at"),
		},
	}

	_, err = db.UpdateTimeToLive(ctx, ttlInput)
	if err != nil {
		// TTL update might fail if it's already enabled, which is okay
		var resourceInUse *types.ResourceInUseException
		if !errors.As(err, &resourceInUse) {
			return fmt.Errorf("failed to enable TTL: %w", err)
		}
	}

	return nil
}
