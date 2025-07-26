package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

const (
	// OAuth state key prefixes
	OAuthStatePKPrefix = "OAUTH_STATE#"
	OAuthStateSK       = "STATE"
)

// StoreOAuthState stores OAuth state for CSRF protection
func (s *dynamoDBStorage) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
	log := common.WithContext(ctx)

	// Set expiration time if not set
	if data.ExpiresAt.IsZero() {
		data.ExpiresAt = time.Now().Add(10 * time.Minute)
	}

	// Create DynamoDB item
	item := map[string]types.AttributeValue{
		"PK":          &types.AttributeValueMemberS{Value: OAuthStatePKPrefix + state},
		"SK":          &types.AttributeValueMemberS{Value: OAuthStateSK},
		"State":       &types.AttributeValueMemberS{Value: data.State},
		"Provider":    &types.AttributeValueMemberS{Value: data.Provider},
		"RedirectURI": &types.AttributeValueMemberS{Value: data.RedirectURI},
		"CreatedAt":   &types.AttributeValueMemberS{Value: data.CreatedAt.Format(time.RFC3339)},
		"ExpiresAt":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", data.ExpiresAt.Unix())}, // For TTL
		"TTL":         &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", data.ExpiresAt.Unix())}, // DynamoDB TTL
	}

	// Add optional fields
	if data.Username != "" {
		item["Username"] = &types.AttributeValueMemberS{Value: data.Username}
	}
	if data.ClientID != "" {
		item["ClientID"] = &types.AttributeValueMemberS{Value: data.ClientID}
	}
	if len(data.Scopes) > 0 {
		scopeList := make([]types.AttributeValue, len(data.Scopes))
		for i, scope := range data.Scopes {
			scopeList[i] = &types.AttributeValueMemberS{Value: scope}
		}
		item["Scopes"] = &types.AttributeValueMemberL{Value: scopeList}
	}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	_, err := s.client.PutItem(ctx, input)
	if err != nil {
		log.Error("failed to store OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return fmt.Errorf("failed to store OAuth state: %w", err)
	}

	log.Debug("stored OAuth state",
		zap.String("state", state),
		zap.String("provider", data.Provider),
		zap.Time("expires_at", data.ExpiresAt))

	return nil
}

// GetOAuthState retrieves OAuth state
func (s *dynamoDBStorage) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	log := common.WithContext(ctx)

	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: OAuthStatePKPrefix + state},
			"SK": &types.AttributeValueMemberS{Value: OAuthStateSK},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		log.Error("failed to get OAuth state", zap.Error(err))
		return nil, fmt.Errorf("failed to get OAuth state: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("OAuth state not found: %s", state)
	}

	// Parse the item
	oauthState := &storage.OAuthState{
		State: state,
	}

	// Parse required fields
	if val, ok := result.Item["Provider"].(*types.AttributeValueMemberS); ok {
		oauthState.Provider = val.Value
	}
	if val, ok := result.Item["RedirectURI"].(*types.AttributeValueMemberS); ok {
		oauthState.RedirectURI = val.Value
	}
	if val, ok := result.Item["CreatedAt"].(*types.AttributeValueMemberS); ok {
		oauthState.CreatedAt, _ = time.Parse(time.RFC3339, val.Value)
	}
	if val, ok := result.Item["ExpiresAt"].(*types.AttributeValueMemberN); ok {
		var expiresUnix int64
		if _, err := fmt.Sscanf(val.Value, "%d", &expiresUnix); err != nil {
			log.Warn("failed to parse ExpiresAt for OAuth state",
				zap.String("state", state),
				zap.String("value", val.Value),
				zap.Error(err))
		}
		oauthState.ExpiresAt = time.Unix(expiresUnix, 0)
	}

	// Parse optional fields
	if val, ok := result.Item["Username"].(*types.AttributeValueMemberS); ok {
		oauthState.Username = val.Value
	}
	if val, ok := result.Item["ClientID"].(*types.AttributeValueMemberS); ok {
		oauthState.ClientID = val.Value
	}
	if val, ok := result.Item["Scopes"].(*types.AttributeValueMemberL); ok {
		oauthState.Scopes = make([]string, len(val.Value))
		for i, scopeVal := range val.Value {
			if s, ok := scopeVal.(*types.AttributeValueMemberS); ok {
				oauthState.Scopes[i] = s.Value
			}
		}
	}

	// Check if expired
	if time.Now().After(oauthState.ExpiresAt) {
		// Clean up expired state
		_ = s.DeleteOAuthState(ctx, state)
		return nil, fmt.Errorf("OAuth state expired: %s", state)
	}

	log.Debug("retrieved OAuth state",
		zap.String("state", state),
		zap.String("provider", oauthState.Provider))

	return oauthState, nil
}

// DeleteOAuthState deletes OAuth state
func (s *dynamoDBStorage) DeleteOAuthState(ctx context.Context, state string) error {
	log := common.WithContext(ctx)

	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: OAuthStatePKPrefix + state},
			"SK": &types.AttributeValueMemberS{Value: OAuthStateSK},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		log.Error("failed to delete OAuth state", zap.Error(err))
		return fmt.Errorf("failed to delete OAuth state: %w", err)
	}

	log.Debug("deleted OAuth state", zap.String("state", state))
	return nil
}
