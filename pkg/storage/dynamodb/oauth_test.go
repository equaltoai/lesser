package dynamodb

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateAuthorizationCode(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	client := NewWithClient(mockDB, "test-table")

	ctx := context.Background()
	code := &storage.AuthorizationCode{
		Code:          "test-code-123",
		ClientID:      "test-client",
		Username:      "testuser",
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
		Scopes:        []string{"read", "write"},
	}

	t.Run("successful creation", func(t *testing.T) {
		mockDB.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
			return *input.TableName == "test-table" &&
				input.Item["PK"].(*types.AttributeValueMemberS).Value == "AUTHCODE#test-code-123" &&
				input.Item["SK"].(*types.AttributeValueMemberS).Value == "CODE"
		})).Return(&dynamodb.PutItemOutput{}, nil).Once()

		err := client.CreateAuthorizationCode(ctx, code)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	t.Run("code already exists", func(t *testing.T) {
		mockDB.On("PutItem", ctx, mock.Anything).Return(
			nil,
			&types.ConditionalCheckFailedException{
				Message: aws.String("The conditional request failed"),
			},
		).Once()

		err := client.CreateAuthorizationCode(ctx, code)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		mockDB.AssertExpectations(t)
	})
}

func TestGetAuthorizationCode(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	client := NewWithClient(mockDB, "test-table")

	ctx := context.Background()
	codeStr := "test-code-123"

	t.Run("successful retrieval", func(t *testing.T) {
		expiresAt := time.Now().Add(5 * time.Minute)

		mockDB.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return *input.TableName == "test-table" &&
				input.Key["PK"].(*types.AttributeValueMemberS).Value == "AUTHCODE#test-code-123" &&
				input.Key["SK"].(*types.AttributeValueMemberS).Value == "CODE"
		})).Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"Code": &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"Code":          &types.AttributeValueMemberS{Value: codeStr},
						"ClientID":      &types.AttributeValueMemberS{Value: "test-client"},
						"Username":      &types.AttributeValueMemberS{Value: "testuser"},
						"CodeChallenge": &types.AttributeValueMemberS{Value: "challenge"},
						"ExpiresAt":     &types.AttributeValueMemberS{Value: expiresAt.Format(time.RFC3339)},
						"Scopes": &types.AttributeValueMemberL{
							Value: []types.AttributeValue{
								&types.AttributeValueMemberS{Value: "read"},
								&types.AttributeValueMemberS{Value: "write"},
							},
						},
					},
				},
			},
		}, nil).Once()

		code, err := client.GetAuthorizationCode(ctx, codeStr)
		require.NoError(t, err)
		assert.Equal(t, codeStr, code.Code)
		assert.Equal(t, "test-client", code.ClientID)
		assert.Equal(t, "testuser", code.Username)
		assert.Equal(t, []string{"read", "write"}, code.Scopes)
		mockDB.AssertExpectations(t)
	})

	t.Run("code not found", func(t *testing.T) {
		mockDB.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil).Once()

		code, err := client.GetAuthorizationCode(ctx, codeStr)
		assert.Error(t, err)
		assert.Nil(t, code)
		assert.Contains(t, err.Error(), "not found")
		mockDB.AssertExpectations(t)
	})

	t.Run("expired code", func(t *testing.T) {
		expiresAt := time.Now().Add(-5 * time.Minute) // Already expired

		mockDB.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"Code": &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"Code":          &types.AttributeValueMemberS{Value: codeStr},
						"ClientID":      &types.AttributeValueMemberS{Value: "test-client"},
						"Username":      &types.AttributeValueMemberS{Value: "testuser"},
						"CodeChallenge": &types.AttributeValueMemberS{Value: "challenge"},
						"ExpiresAt":     &types.AttributeValueMemberS{Value: expiresAt.Format(time.RFC3339)},
						"Scopes":        &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
					},
				},
			},
		}, nil).Once()

		// Expect deletion of expired code
		mockDB.On("DeleteItem", ctx, mock.Anything).Return(&dynamodb.DeleteItemOutput{}, nil).Once()

		code, err := client.GetAuthorizationCode(ctx, codeStr)
		assert.Error(t, err)
		assert.Nil(t, code)
		assert.Contains(t, err.Error(), "expired")
		mockDB.AssertExpectations(t)
	})
}

func TestDeleteAuthorizationCode(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	client := NewWithClient(mockDB, "test-table")

	ctx := context.Background()
	codeStr := "test-code-123"

	mockDB.On("DeleteItem", ctx, mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
		return *input.TableName == "test-table" &&
			input.Key["PK"].(*types.AttributeValueMemberS).Value == "AUTHCODE#test-code-123" &&
			input.Key["SK"].(*types.AttributeValueMemberS).Value == "CODE"
	})).Return(&dynamodb.DeleteItemOutput{}, nil).Once()

	err := client.DeleteAuthorizationCode(ctx, codeStr)
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestCreateRefreshToken(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	client := NewWithClient(mockDB, "test-table")

	ctx := context.Background()
	token := &storage.RefreshToken{
		Token:     "refresh-token-123",
		ClientID:  "test-client",
		Username:  "testuser",
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Scopes:    []string{"read", "write"},
	}

	t.Run("successful creation", func(t *testing.T) {
		mockDB.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
			return *input.TableName == "test-table" &&
				input.Item["PK"].(*types.AttributeValueMemberS).Value == "REFRESHTOKEN#refresh-token-123" &&
				input.Item["SK"].(*types.AttributeValueMemberS).Value == "TOKEN"
		})).Return(&dynamodb.PutItemOutput{}, nil).Once()

		err := client.CreateRefreshToken(ctx, token)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	t.Run("token already exists", func(t *testing.T) {
		mockDB.On("PutItem", ctx, mock.Anything).Return(
			nil,
			&types.ConditionalCheckFailedException{
				Message: aws.String("The conditional request failed"),
			},
		).Once()

		err := client.CreateRefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		mockDB.AssertExpectations(t)
	})
}

func TestGetRefreshToken(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	client := NewWithClient(mockDB, "test-table")

	ctx := context.Background()
	tokenStr := "refresh-token-123"

	t.Run("successful retrieval", func(t *testing.T) {
		expiresAt := time.Now().Add(15 * 24 * time.Hour)

		mockDB.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return *input.TableName == "test-table" &&
				input.Key["PK"].(*types.AttributeValueMemberS).Value == "REFRESHTOKEN#refresh-token-123" &&
				input.Key["SK"].(*types.AttributeValueMemberS).Value == "TOKEN"
		})).Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"Token": &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"Token":     &types.AttributeValueMemberS{Value: tokenStr},
						"ClientID":  &types.AttributeValueMemberS{Value: "test-client"},
						"Username":  &types.AttributeValueMemberS{Value: "testuser"},
						"ExpiresAt": &types.AttributeValueMemberS{Value: expiresAt.Format(time.RFC3339)},
						"Scopes": &types.AttributeValueMemberL{
							Value: []types.AttributeValue{
								&types.AttributeValueMemberS{Value: "read"},
								&types.AttributeValueMemberS{Value: "write"},
							},
						},
					},
				},
			},
		}, nil).Once()

		token, err := client.GetRefreshToken(ctx, tokenStr)
		require.NoError(t, err)
		assert.Equal(t, tokenStr, token.Token)
		assert.Equal(t, "test-client", token.ClientID)
		assert.Equal(t, "testuser", token.Username)
		assert.Equal(t, []string{"read", "write"}, token.Scopes)
		mockDB.AssertExpectations(t)
	})

	t.Run("token not found", func(t *testing.T) {
		mockDB.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil).Once()

		token, err := client.GetRefreshToken(ctx, tokenStr)
		assert.Error(t, err)
		assert.Nil(t, token)
		assert.Contains(t, err.Error(), "not found")
		mockDB.AssertExpectations(t)
	})

	t.Run("expired token", func(t *testing.T) {
		expiresAt := time.Now().Add(-5 * time.Minute) // Already expired

		mockDB.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"Token": &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"Token":     &types.AttributeValueMemberS{Value: tokenStr},
						"ClientID":  &types.AttributeValueMemberS{Value: "test-client"},
						"Username":  &types.AttributeValueMemberS{Value: "testuser"},
						"ExpiresAt": &types.AttributeValueMemberS{Value: expiresAt.Format(time.RFC3339)},
						"Scopes":    &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
					},
				},
			},
		}, nil).Once()

		// Expect deletion of expired token
		mockDB.On("DeleteItem", ctx, mock.Anything).Return(&dynamodb.DeleteItemOutput{}, nil).Once()

		token, err := client.GetRefreshToken(ctx, tokenStr)
		assert.Error(t, err)
		assert.Nil(t, token)
		assert.Contains(t, err.Error(), "expired")
		mockDB.AssertExpectations(t)
	})
}

func TestDeleteRefreshToken(t *testing.T) {
	mockDB := new(mocks.MockDynamoDBClient)
	client := NewWithClient(mockDB, "test-table")

	ctx := context.Background()
	tokenStr := "refresh-token-123"

	mockDB.On("DeleteItem", ctx, mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
		return *input.TableName == "test-table" &&
			input.Key["PK"].(*types.AttributeValueMemberS).Value == "REFRESHTOKEN#refresh-token-123" &&
			input.Key["SK"].(*types.AttributeValueMemberS).Value == "TOKEN"
	})).Return(&dynamodb.DeleteItemOutput{}, nil).Once()

	err := client.DeleteRefreshToken(ctx, tokenStr)
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}
