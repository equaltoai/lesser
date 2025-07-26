package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// Wallet challenge operations

// StoreWalletChallenge stores a temporary wallet authentication challenge
func (s *dynamoDBStorage) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	item, err := attributevalue.MarshalMap(challenge)
	if err != nil {
		return fmt.Errorf("failed to marshal challenge: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: "WALLET_CHALLENGE#" + challenge.ID}
	item["SK"] = &types.AttributeValueMemberS{Value: "CHALLENGE"}
	item["Type"] = &types.AttributeValueMemberS{Value: "WalletChallenge"}
	// TTL for automatic cleanup
	item["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", challenge.ExpiresAt.Unix())}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to store wallet challenge: %w", err)
	}

	return nil
}

// GetWalletChallenge retrieves a wallet challenge by ID
func (s *dynamoDBStorage) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "WALLET_CHALLENGE#" + challengeID},
			"SK": &types.AttributeValueMemberS{Value: "CHALLENGE"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet challenge: %w", err)
	}

	if len(result.Item) == 0 {
		return nil, fmt.Errorf("wallet challenge not found")
	}

	var challenge storage.WalletChallenge
	if err := s.UnmarshalItem(result.Item, &challenge); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wallet challenge: %w", err)
	}

	return &challenge, nil
}

// DeleteWalletChallenge deletes a wallet challenge
func (s *dynamoDBStorage) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "WALLET_CHALLENGE#" + challengeID},
			"SK": &types.AttributeValueMemberS{Value: "CHALLENGE"},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete wallet challenge: %w", err)
	}

	return nil
}

// Wallet credential operations

// StoreWalletCredential stores a wallet credential linked to a user
func (s *dynamoDBStorage) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	item, err := attributevalue.MarshalMap(credential)
	if err != nil {
		return fmt.Errorf("failed to marshal wallet credential: %w", err)
	}

	// Normalize address
	address := strings.ToLower(credential.Address)

	// Primary key for user's wallets
	item["PK"] = &types.AttributeValueMemberS{Value: s.userPK(credential.Username)}
	item["SK"] = &types.AttributeValueMemberS{Value: "WALLET#" + address}
	item["Type"] = &types.AttributeValueMemberS{Value: "WalletCredential"}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	if _, err = s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to store wallet credential: %w", err)
	}

	// Also create a reverse index for wallet->user lookup
	reverseItem := map[string]types.AttributeValue{
		"PK":       &types.AttributeValueMemberS{Value: fmt.Sprintf("WALLET#%s#%s", credential.Type, address)},
		"SK":       &types.AttributeValueMemberS{Value: "USER#" + credential.Username},
		"Type":     &types.AttributeValueMemberS{Value: "WalletIndex"},
		"Username": &types.AttributeValueMemberS{Value: credential.Username},
	}

	reverseInput := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      reverseItem,
	}

	if _, err = s.client.PutItem(ctx, reverseInput); err != nil {
		// Try to clean up the first item
		if _, cleanupErr := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: s.getTableName(),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: s.userPK(credential.Username)},
				"SK": &types.AttributeValueMemberS{Value: "WALLET#" + address},
			},
		}); cleanupErr != nil {
			s.log.Warn("failed to cleanup wallet credential after index failure",
				zap.String("username", credential.Username),
				zap.String("address", address),
				zap.Error(cleanupErr))
		}
		return fmt.Errorf("failed to store wallet index: %w", err)
	}

	return nil
}

// GetWalletCredential retrieves a wallet credential by wallet type and address
func (s *dynamoDBStorage) GetWalletCredential(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	// Normalize address
	address = strings.ToLower(address)

	// Query with begins_with since we don't know the exact username
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("WALLET#%s#%s", walletType, address)},
			":sk": &types.AttributeValueMemberS{Value: "USER#"},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to query wallet index: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil // Wallet not found
	}

	// Extract username
	var username string
	if v, ok := result.Items[0]["Username"]; ok {
		if s, ok := v.(*types.AttributeValueMemberS); ok {
			username = s.Value
		}
	}

	if username == "" {
		return nil, fmt.Errorf("username not found in wallet index")
	}

	// Now get the actual wallet credential
	credInput := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
			"SK": &types.AttributeValueMemberS{Value: "WALLET#" + address},
		},
	}

	credResult, err := s.client.GetItem(ctx, credInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet credential: %w", err)
	}

	if len(credResult.Item) == 0 {
		return nil, nil
	}

	var credential storage.WalletCredential
	if err := s.UnmarshalItem(credResult.Item, &credential); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wallet credential: %w", err)
	}

	return &credential, nil
}

// GetUserWalletCredentials retrieves all wallet credentials for a user
func (s *dynamoDBStorage) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: s.userPK(username)},
			":sk": &types.AttributeValueMemberS{Value: "WALLET#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query user wallet credentials: %w", err)
	}

	var credentials []*storage.WalletCredential
	for _, item := range result.Items {
		var credential storage.WalletCredential
		if err := s.UnmarshalItem(item, &credential); err != nil {
			common.Logger().Error("failed to unmarshal wallet credential", zap.Error(err))
			continue
		}
		credentials = append(credentials, &credential)
	}

	return credentials, nil
}

// DeleteWalletCredential deletes a wallet credential
func (s *dynamoDBStorage) DeleteWalletCredential(ctx context.Context, username, address string) error {
	// Normalize address
	address = strings.ToLower(address)

	// First get the wallet to determine type for index deletion
	credInput := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
			"SK": &types.AttributeValueMemberS{Value: "WALLET#" + address},
		},
	}

	credResult, err := s.client.GetItem(ctx, credInput)
	if err != nil {
		return fmt.Errorf("failed to get wallet credential: %w", err)
	}

	walletType := "ethereum" // default
	if len(credResult.Item) > 0 {
		var credential storage.WalletCredential
		if err := s.UnmarshalItem(credResult.Item, &credential); err == nil {
			walletType = credential.Type
		}
	}

	// Delete the wallet credential
	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
			"SK": &types.AttributeValueMemberS{Value: "WALLET#" + address},
		},
	}

	if _, err := s.client.DeleteItem(ctx, input); err != nil {
		return fmt.Errorf("failed to delete wallet credential: %w", err)
	}

	// Also delete the reverse index
	indexInput := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("WALLET#%s#%s", walletType, address)},
			"SK": &types.AttributeValueMemberS{Value: "USER#" + username},
		},
	}

	if _, err := s.client.DeleteItem(ctx, indexInput); err != nil {
		// Log but don't fail - index might already be gone
		common.Logger().Warn("failed to delete wallet index", zap.Error(err))
	}

	return nil
}

// UpdateWalletLastUsed updates the last used timestamp for a wallet
func (s *dynamoDBStorage) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	// Normalize address
	address = strings.ToLower(address)

	update := expression.Set(expression.Name("last_used"), expression.Value(time.Now().Format(time.RFC3339)))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
			"SK": &types.AttributeValueMemberS{Value: "WALLET#" + address},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update wallet last used: %w", err)
	}

	return nil
}
