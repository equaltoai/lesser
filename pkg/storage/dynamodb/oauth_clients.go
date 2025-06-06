package dynamodb

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// CreateOAuthClient creates a new OAuth client in DynamoDB
func (s *dynamoDBStorage) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	if client.Name == "" || len(client.RedirectURIs) == 0 {
		return errors.New("client name and redirect_uris are required")
	}

	// Generate client ID if not provided
	if client.ClientID == "" {
		clientID, err := generateClientID()
		if err != nil {
			return fmt.Errorf("failed to generate client ID: %w", err)
		}
		client.ClientID = clientID
	}

	// Generate client secret if not provided
	if client.ClientSecret == "" {
		clientSecret, err := generateClientSecret()
		if err != nil {
			return fmt.Errorf("failed to generate client secret: %w", err)
		}
		client.ClientSecret = clientSecret
	}

	// Set timestamp
	client.CreatedAt = time.Now()

	// Marshal client data
	av, err := s.MarshalItem(client)
	if err != nil {
		return fmt.Errorf("failed to marshal OAuth client: %w", err)
	}

	// Add keys
	av["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("CLIENT#%s", client.ClientID)}
	av["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}

	// Put item with condition that it doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			return fmt.Errorf("client with ID %s already exists", client.ClientID)
		}
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	return nil
}

// GetOAuthClient retrieves an OAuth client by ID
func (s *dynamoDBStorage) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CLIENT#%s", clientID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth client: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("OAuth client not found: %s", clientID)
	}

	var client storage.OAuthClient
	if err := s.UnmarshalItem(result.Item, &client); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OAuth client: %w", err)
	}

	return &client, nil
}

// UpdateOAuthClient updates an existing OAuth client
func (s *dynamoDBStorage) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return errors.New("no updates provided")
	}

	// Build update expression
	updateExpr := "SET updated_at = :updated_at"
	exprAttrValues := map[string]types.AttributeValue{
		":updated_at": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	for key, value := range updates {
		switch key {
		case "name", "website", "redirect_uris", "scopes":
			updateExpr += fmt.Sprintf(", %s = :%s", key, key)
			av, err := attributevalue.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal %s: %w", key, err)
			}
			exprAttrValues[":"+key] = av
		}
	}

	// Update item
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CLIENT#%s", clientID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprAttrValues,
		ConditionExpression:       aws.String("attribute_exists(PK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			return fmt.Errorf("OAuth client not found: %s", clientID)
		}
		return fmt.Errorf("failed to update OAuth client: %w", err)
	}

	return nil
}

// DeleteOAuthClient deletes an OAuth client
func (s *dynamoDBStorage) DeleteOAuthClient(ctx context.Context, clientID string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CLIENT#%s", clientID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete OAuth client: %w", err)
	}

	return nil
}

// ListOAuthClients retrieves a paginated list of OAuth clients
func (s *dynamoDBStorage) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Use Scan since we don't have a GSI for clients
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "CLIENT#"},
		},
		Limit: aws.Int32(limit),
	}

	// Add cursor if provided
	if cursor != "" {
		// Decode cursor (base64 encoded primary key)
		decodedCursor, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil {
			input.ExclusiveStartKey = map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: string(decodedCursor)},
				"SK": &types.AttributeValueMemberS{Value: "METADATA"},
			}
		}
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list OAuth clients: %w", err)
	}

	clients := make([]*storage.OAuthClient, 0, len(result.Items))
	for _, item := range result.Items {
		var client storage.OAuthClient
		if err := s.UnmarshalItem(item, &client); err != nil {
			continue
		}
		clients = append(clients, &client)
	}

	// Extract next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if pk, ok := result.LastEvaluatedKey["PK"]; ok {
			if s, ok := pk.(*types.AttributeValueMemberS); ok {
				nextCursor = base64.URLEncoding.EncodeToString([]byte(s.Value))
			}
		}
	}

	return clients, nextCursor, nil
}

// generateClientID generates a unique client ID
func generateClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// generateClientSecret generates a secure client secret
func generateClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ValidateRedirectURI checks if a redirect URI is valid for the client
func ValidateRedirectURI(clientURIs []string, requestedURI string) bool {
	for _, uri := range clientURIs {
		// Exact match
		if uri == requestedURI {
			return true
		}
		// Check if it's a prefix match for native apps (e.g., com.example.app://callback)
		if strings.HasPrefix(requestedURI, uri) && strings.Contains(uri, "://") {
			return true
		}
	}
	return false
}
