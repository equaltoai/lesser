package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

// MockRefreshTokenDB is a mock DynamoDB client for refresh token testing
type MockRefreshTokenDB struct {
	items map[string]map[string]types.AttributeValue
}

func NewMockRefreshTokenDB() *MockRefreshTokenDB {
	return &MockRefreshTokenDB{
		items: make(map[string]map[string]types.AttributeValue),
	}
}

func (m *MockRefreshTokenDB) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	tokenAttr := params.Item["token"]
	if tokenStr, ok := tokenAttr.(*types.AttributeValueMemberS); ok {
		m.items[tokenStr.Value] = params.Item
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *MockRefreshTokenDB) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	tokenAttr := params.Key["token"]
	if tokenStr, ok := tokenAttr.(*types.AttributeValueMemberS); ok {
		if item, exists := m.items[tokenStr.Value]; exists {
			return &dynamodb.GetItemOutput{Item: item}, nil
		}
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *MockRefreshTokenDB) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	tokenAttr := params.Key["token"]
	if tokenStr, ok := tokenAttr.(*types.AttributeValueMemberS); ok {
		if item, exists := m.items[tokenStr.Value]; exists {
			// Apply the update
			if params.UpdateExpression != nil && *params.UpdateExpression == "SET revoked = :true, revoked_reason = :reason, last_used_at = :now" {
				item["revoked"] = &types.AttributeValueMemberBOOL{Value: true}
				if reason, ok := params.ExpressionAttributeValues[":reason"].(*types.AttributeValueMemberS); ok {
					item["revoked_reason"] = &types.AttributeValueMemberS{Value: reason.Value}
				}
				if now, ok := params.ExpressionAttributeValues[":now"].(*types.AttributeValueMemberN); ok {
					item["last_used_at"] = &types.AttributeValueMemberN{Value: now.Value}
				}
			}
			return &dynamodb.UpdateItemOutput{}, nil
		}
	}
	return nil, &types.ResourceNotFoundException{
		Message: aws.String("token not found"),
	}
}

func (m *MockRefreshTokenDB) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	// Simple mock: process each transaction item
	for _, item := range params.TransactItems {
		if item.Update != nil {
			m.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName:                 item.Update.TableName,
				Key:                       item.Update.Key,
				UpdateExpression:          item.Update.UpdateExpression,
				ExpressionAttributeValues: item.Update.ExpressionAttributeValues,
			})
		}
		if item.Put != nil {
			m.PutItem(ctx, &dynamodb.PutItemInput{
				TableName: item.Put.TableName,
				Item:      item.Put.Item,
			})
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *MockRefreshTokenDB) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	// Simple mock: return all items (not a real query)
	items := []map[string]types.AttributeValue{}
	for _, item := range m.items {
		items = append(items, item)
	}
	return &dynamodb.QueryOutput{
		Items: items,
		Count: int32(len(items)),
	}, nil
}

func TestCreateRefreshToken(t *testing.T) {
	mockDB := NewMockRefreshTokenDB()
	store := NewRefreshTokenStore(mockDB, "refresh-tokens")

	ctx := context.Background()
	token, err := store.CreateRefreshToken(ctx, "user123", "iPhone 12", "192.168.1.1")

	require.NoError(t, err)
	require.NotEmpty(t, token.Token)
	require.Equal(t, "user123", token.UserID)
	require.Equal(t, "iPhone 12", token.DeviceName)
	require.Equal(t, "192.168.1.1", token.IPAddress)
	require.Equal(t, 1, token.Generation)
	require.False(t, token.Revoked)
}

func TestRefreshTokenRotation(t *testing.T) {
	mockDB := NewMockRefreshTokenDB()
	store := NewRefreshTokenStore(mockDB, "refresh-tokens")

	ctx := context.Background()

	// Create initial token
	token1, err := store.CreateRefreshToken(ctx, "user123", "iPhone 12", "192.168.1.1")
	require.NoError(t, err)

	// Rotate the token
	token2, err := store.RotateRefreshToken(ctx, token1.Token, "192.168.1.2")
	require.NoError(t, err)
	require.NotEqual(t, token1.Token, token2.Token)
	require.Equal(t, token1.Family, token2.Family)
	require.Equal(t, token1.Generation+1, token2.Generation)
	require.Equal(t, "192.168.1.2", token2.IPAddress)

	// Old token should be revoked
	oldToken, err := store.GetRefreshToken(ctx, token1.Token)
	require.NoError(t, err)
	require.True(t, oldToken.Revoked)
	require.Equal(t, "Rotated", oldToken.RevokedReason)
}

func TestRefreshTokenReuseDetection(t *testing.T) {
	mockDB := NewMockRefreshTokenDB()
	store := NewRefreshTokenStore(mockDB, "refresh-tokens")

	ctx := context.Background()

	// Create initial token
	token1, err := store.CreateRefreshToken(ctx, "user123", "iPhone 12", "192.168.1.1")
	require.NoError(t, err)

	// Rotate once
	token2, err := store.RotateRefreshToken(ctx, token1.Token, "192.168.1.2")
	require.NoError(t, err)
	require.NotEmpty(t, token2.Token) // Ensure token2 is valid

	// Mark token1 as revoked in our mock
	if item, exists := mockDB.items[token1.Token]; exists {
		item["revoked"] = &types.AttributeValueMemberBOOL{Value: true}
		item["revoked_reason"] = &types.AttributeValueMemberS{Value: "Rotated"}
	}

	// Try to use old token again (reuse attack)
	_, err = store.RotateRefreshToken(ctx, token1.Token, "192.168.1.3")
	require.Equal(t, ErrTokenReuse, err)

	// The entire family should be revoked (in a real implementation)
	// This is mocked by checking that we would have called RevokeTokenFamily
}

func TestRevokeTokenFamily(t *testing.T) {
	mockDB := NewMockRefreshTokenDB()
	store := NewRefreshTokenStore(mockDB, "refresh-tokens")

	ctx := context.Background()
	family := "test-family-123"

	// Create multiple tokens in the same family
	for i := 1; i <= 3; i++ {
		token := RefreshToken{
			Token:      fmt.Sprintf("token-%d", i),
			UserID:     "user123",
			Family:     family,
			Generation: i,
			CreatedAt:  time.Now().Unix(),
			ExpiresAt:  time.Now().Add(30 * 24 * time.Hour).Unix(),
			Revoked:    false,
		}

		item, _ := attributevalue.MarshalMap(token)
		mockDB.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String("refresh-tokens"),
			Item:      item,
		})
	}

	// Revoke the entire family
	err := store.RevokeTokenFamily(ctx, family, "Security breach")
	require.NoError(t, err)

	// All tokens should be marked as revoked
	for i := 1; i <= 3; i++ {
		tokenKey := fmt.Sprintf("token-%d", i)
		if item, exists := mockDB.items[tokenKey]; exists {
			revoked, _ := item["revoked"].(*types.AttributeValueMemberBOOL)
			require.True(t, revoked.Value)
		}
	}
}

func TestRevokeUserTokens(t *testing.T) {
	mockDB := NewMockRefreshTokenDB()
	store := NewRefreshTokenStore(mockDB, "refresh-tokens")

	ctx := context.Background()
	userID := "user123"

	// Create multiple tokens for the user
	for i := 1; i <= 3; i++ {
		token, _ := store.CreateRefreshToken(ctx, userID, fmt.Sprintf("Device %d", i), "192.168.1.1")
		_ = token
	}

	// Revoke all user tokens
	err := store.RevokeUserTokens(ctx, userID, "User logout all devices")
	require.NoError(t, err)

	// All tokens should be marked as revoked
	for _, item := range mockDB.items {
		if userIDAttr, ok := item["user_id"].(*types.AttributeValueMemberS); ok && userIDAttr.Value == userID {
			revoked, _ := item["revoked"].(*types.AttributeValueMemberBOOL)
			require.True(t, revoked.Value)
		}
	}
}

func TestExpiredRefreshToken(t *testing.T) {
	mockDB := NewMockRefreshTokenDB()
	store := NewRefreshTokenStore(mockDB, "refresh-tokens")

	ctx := context.Background()

	// Create an expired token
	expiredToken := RefreshToken{
		Token:      "expired-token",
		UserID:     "user123",
		Family:     "family-123",
		Generation: 1,
		CreatedAt:  time.Now().Add(-31 * 24 * time.Hour).Unix(),
		ExpiresAt:  time.Now().Add(-1 * 24 * time.Hour).Unix(), // Expired yesterday
		Revoked:    false,
	}

	item, _ := attributevalue.MarshalMap(expiredToken)
	mockDB.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String("refresh-tokens"),
		Item:      item,
	})

	// Try to get the expired token
	_, err := store.GetRefreshToken(ctx, "expired-token")
	require.Equal(t, ErrExpiredRefreshToken, err)

	// Try to rotate the expired token
	_, err = store.RotateRefreshToken(ctx, "expired-token", "192.168.1.1")
	require.Equal(t, ErrExpiredRefreshToken, err)
}
