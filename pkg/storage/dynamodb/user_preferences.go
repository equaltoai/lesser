package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// GetUserLanguagePreference retrieves a user's preferred language
func (s *dynamoDBStorage) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	prefs, err := s.GetUserPreferences(ctx, username)
	if err != nil {
		return "", err
	}

	if prefs != nil && prefs.Language != "" {
		return prefs.Language, nil
	}

	// Default to English if no preference set
	return "en", nil
}

// SetUserLanguagePreference updates a user's preferred language
func (s *dynamoDBStorage) SetUserLanguagePreference(ctx context.Context, username string, language string) error {
	// Get existing preferences or create new ones
	prefs, err := s.GetUserPreferences(ctx, username)
	if err != nil {
		// If preferences don't exist, create new ones
		prefs = &storage.UserPreferences{
			Language: language,
		}
	} else {
		prefs.Language = language
	}

	return s.UpdateUserPreferences(ctx, username, prefs)
}

// GetUserPreferences retrieves all user preferences
func (s *dynamoDBStorage) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + username},
			"SK": &types.AttributeValueMemberS{Value: "PREFERENCES"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get user preferences: %w", err)
	}

	if result.Item == nil {
		// Return default preferences if none exist
		return &storage.UserPreferences{
			Language:                  "en",
			DefaultPostingVisibility:  "public",
			DefaultMediaSensitive:     false,
			ExpandSpoilers:            false,
			ShowFollowCounts:          true,
			PreferredTimelineOrder:    "newest",
			SearchSuggestionsEnabled:  true,
			PersonalizedSearchEnabled: true,
		}, nil
	}

	var prefs storage.UserPreferences
	if err := attributevalue.UnmarshalMap(result.Item, &prefs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal preferences: %w", err)
	}

	return &prefs, nil
}

// UpdateUserPreferences updates user preferences
func (s *dynamoDBStorage) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	// Marshal preferences
	av, err := attributevalue.MarshalMap(preferences)
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	// Add keys and metadata
	av["PK"] = &types.AttributeValueMemberS{Value: "USER#" + username}
	av["SK"] = &types.AttributeValueMemberS{Value: "PREFERENCES"}
	av["UpdatedAt"] = &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to update preferences: %w", err)
	}

	s.logger().Debug("updated user preferences",
		zap.String("username", username),
		zap.String("language", preferences.Language))

	return nil
}
