package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockDynamoDBClient is a mock implementation for testing
type MockDynamoDBClient struct {
	items map[string]map[string]types.AttributeValue
}

func NewMockDynamoDBClient() *MockDynamoDBClient {
	return &MockDynamoDBClient{
		items: make(map[string]map[string]types.AttributeValue),
	}
}

func (m *MockDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	// Extract token from item
	tokenAttr := params.Item["token"]
	if tokenStr, ok := tokenAttr.(*types.AttributeValueMemberS); ok {
		m.items[tokenStr.Value] = params.Item
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *MockDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	tokenAttr := params.Key["token"]
	if tokenStr, ok := tokenAttr.(*types.AttributeValueMemberS); ok {
		if item, exists := m.items[tokenStr.Value]; exists {
			return &dynamodb.GetItemOutput{Item: item}, nil
		}
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *MockDynamoDBClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	tokenAttr := params.Key["token"]
	if tokenStr, ok := tokenAttr.(*types.AttributeValueMemberS); ok {
		if item, exists := m.items[tokenStr.Value]; exists {
			// Simple mock: just mark as used
			item["used"] = &types.AttributeValueMemberBOOL{Value: true}
			m.items[tokenStr.Value] = item
			return &dynamodb.UpdateItemOutput{}, nil
		}
	}
	return nil, &types.ConditionalCheckFailedException{
		Message: aws.String("conditional check failed"),
	}
}

func (m *MockDynamoDBClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	tokenAttr := params.Key["token"]
	if tokenStr, ok := tokenAttr.(*types.AttributeValueMemberS); ok {
		delete(m.items, tokenStr.Value)
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func (m *MockDynamoDBClient) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	// Simple mock: return count of items
	count := int32(len(m.items))
	return &dynamodb.ScanOutput{
		Count: count,
		Items: []map[string]types.AttributeValue{},
	}, nil
}

func TestCSRFTokenAcrossLambdas(t *testing.T) {
	// This test simulates two different Lambda instances using the same DynamoDB table
	mockDB := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	// Simulate two different Lambda instances
	store1 := NewDynamoDBCSRFStore(mockDB, "csrf-tokens")
	store2 := NewDynamoDBCSRFStore(mockDB, "csrf-tokens")

	// Create token in "Lambda 1"
	token := "test-token-123"
	csrf := CSRFToken{
		Token:     token,
		UserID:    "user123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	// Mock the Scan call for checking user token count
	mockDB.On("Scan", ctx, mock.MatchedBy(func(input *dynamodb.ScanInput) bool {
		return *input.TableName == "csrf-tokens"
	})).Return(&dynamodb.ScanOutput{
		Count: 2, // Under the limit
	}, nil).Once()

	// Mock the PutItem call
	mockDB.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
		return *input.TableName == "csrf-tokens" &&
			input.Item["token"].(*types.AttributeValueMemberS).Value == token
	})).Return(&dynamodb.PutItemOutput{}, nil).Once()

	err := store1.Store(token, csrf)
	require.NoError(t, err)

	// Mock the UpdateItem call for validation
	mockDB.On("UpdateItem", ctx, mock.MatchedBy(func(input *dynamodb.UpdateItemInput) bool {
		return *input.TableName == "csrf-tokens" &&
			input.Key["token"].(*types.AttributeValueMemberS).Value == token
	})).Return(&dynamodb.UpdateItemOutput{}, nil).Once()

	// Validate in "Lambda 2" - should work
	err = store2.ValidateAndConsume(token, "user123")
	require.NoError(t, err)

	// Second validation should fail (single use)
	mockDB.On("UpdateItem", ctx, mock.MatchedBy(func(input *dynamodb.UpdateItemInput) bool {
		return *input.TableName == "csrf-tokens" &&
			input.Key["token"].(*types.AttributeValueMemberS).Value == token
	})).Return(nil, &types.ConditionalCheckFailedException{
		Message: aws.String("conditional check failed"),
	}).Once()

	// Mock GetItem for error checking
	mockDB.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
		return *input.TableName == "csrf-tokens" &&
			input.Key["token"].(*types.AttributeValueMemberS).Value == token
	})).Return(&dynamodb.GetItemOutput{
		Item: map[string]types.AttributeValue{
			"token":      &types.AttributeValueMemberS{Value: token},
			"user_id":    &types.AttributeValueMemberS{Value: "user123"},
			"expires_at": &types.AttributeValueMemberN{Value: "999999999999"},
			"used":       &types.AttributeValueMemberBOOL{Value: true},
		},
	}, nil).Once()

	err = store2.ValidateAndConsume(token, "user123")
	require.Error(t, err)

	mockDB.AssertExpectations(t)
}

func TestTokenLimitPerUser(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	store := NewDynamoDBCSRFStore(mockDB, "csrf-tokens")
	ctx := context.Background()

	userID := "user123"

	// Mock the Scan call for token count
	mockDB.On("Scan", ctx, mock.MatchedBy(func(input *dynamodb.ScanInput) bool {
		return *input.TableName == "csrf-tokens"
	})).Return(&dynamodb.ScanOutput{
		Count: 5, // Less than limit
	}, nil).Times(10)

	// Create multiple tokens for the same user
	for i := 0; i < 10; i++ {
		token := fmt.Sprintf("token-%d", i)
		csrf := CSRFToken{
			Token:     token,
			UserID:    userID,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}

		mockDB.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
			return *input.TableName == "csrf-tokens"
		})).Return(&dynamodb.PutItemOutput{}, nil).Once()

		err := store.Store(token, csrf)
		require.NoError(t, err)
	}

	mockDB.AssertExpectations(t)
}

func TestExpiredTokenValidation(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	store := NewDynamoDBCSRFStore(mockDB, "csrf-tokens")
	ctx := context.Background()

	// Create an expired token
	token := "expired-token"
	csrf := CSRFToken{
		Token:     token,
		UserID:    "user123",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
	}

	// Store should succeed
	mockDB.On("Scan", ctx, mock.Anything).Return(&dynamodb.ScanOutput{Count: 0}, nil).Once()
	mockDB.On("PutItem", ctx, mock.Anything).Return(&dynamodb.PutItemOutput{}, nil).Once()

	err := store.Store(token, csrf)
	require.NoError(t, err)

	// Get should return expired error
	expiredTime := time.Now().Add(-1 * time.Hour).Unix()
	mockDB.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{
		Item: map[string]types.AttributeValue{
			"token":      &types.AttributeValueMemberS{Value: token},
			"user_id":    &types.AttributeValueMemberS{Value: "user123"},
			"expires_at": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expiredTime)},
			"used":       &types.AttributeValueMemberBOOL{Value: false},
		},
	}, nil).Once()

	_, err = store.Get(token)
	require.Equal(t, ErrExpiredCSRF, err)

	// Validate should fail
	mockDB.On("UpdateItem", ctx, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{
		Message: aws.String("conditional check failed"),
	}).Once()

	mockDB.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{
		Item: map[string]types.AttributeValue{
			"token":      &types.AttributeValueMemberS{Value: token},
			"user_id":    &types.AttributeValueMemberS{Value: "user123"},
			"expires_at": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expiredTime)},
			"used":       &types.AttributeValueMemberBOOL{Value: false},
		},
	}, nil).Once()

	err = store.ValidateAndConsume(token, "user123")
	require.Error(t, err)

	mockDB.AssertExpectations(t)
}

func TestWrongUserValidation(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	store := NewDynamoDBCSRFStore(mockDB, "csrf-tokens")
	ctx := context.Background()

	// Create token for user1
	token := "user-token"
	csrf := CSRFToken{
		Token:     token,
		UserID:    "user1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	mockDB.On("Scan", ctx, mock.Anything).Return(&dynamodb.ScanOutput{Count: 0}, nil).Once()
	mockDB.On("PutItem", ctx, mock.Anything).Return(&dynamodb.PutItemOutput{}, nil).Once()

	err := store.Store(token, csrf)
	require.NoError(t, err)

	// Try to validate with different user
	mockDB.On("UpdateItem", ctx, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{
		Message: aws.String("conditional check failed"),
	}).Once()

	validTime := time.Now().Add(1 * time.Hour).Unix()
	mockDB.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{
		Item: map[string]types.AttributeValue{
			"token":      &types.AttributeValueMemberS{Value: token},
			"user_id":    &types.AttributeValueMemberS{Value: "user1"},
			"expires_at": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", validTime)},
			"used":       &types.AttributeValueMemberBOOL{Value: false},
		},
	}, nil).Once()

	err = store.ValidateAndConsume(token, "user2")
	require.Equal(t, ErrInvalidCSRF, err)

	// Token should still be valid for correct user
	mockDB.On("UpdateItem", ctx, mock.Anything).Return(&dynamodb.UpdateItemOutput{}, nil).Once()

	err = store.ValidateAndConsume(token, "user1")
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestAtomicValidateAndConsume(t *testing.T) {
	mockDB := NewMockDynamoDBClient()
	store := NewDynamoDBCSRFStore(mockDB, "csrf-tokens")

	// Create token
	token := "atomic-token"
	csrf := CSRFToken{
		Token:     token,
		UserID:    "user123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	err := store.Store(token, csrf)
	require.NoError(t, err)

	// First validation should succeed
	err = store.ValidateAndConsume(token, "user123")
	require.NoError(t, err)

	// Second validation should fail (already consumed)
	err = store.ValidateAndConsume(token, "user123")
	require.Equal(t, ErrInvalidCSRF, err)

	// Getting the token should show it as used
	retrievedToken, err := store.Get(token)
	require.Error(t, err) // Should fail because token is used
	require.Nil(t, retrievedToken)
}
