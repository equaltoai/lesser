package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// QuoteSettings represents quote permissions and settings for a status
type QuoteSettings struct {
	PK                  string                    `dynamodbav:"PK"` // STATUS#statusID
	SK                  string                    `dynamodbav:"SK"` // QUOTE_SETTINGS
	StatusID            string                    `dynamodbav:"StatusID"`
	QuotePermissions    *storage.QuotePermissions `dynamodbav:"QuotePermissions"`
	WithdrawnFromQuotes bool                      `dynamodbav:"WithdrawnFromQuotes"`
	QuoteType           string                    `dynamodbav:"QuoteType"` // "public", "unlisted", "followers_only", "mentioned_only"
	CreatedAt           time.Time                 `dynamodbav:"CreatedAt"`
	UpdatedAt           time.Time                 `dynamodbav:"UpdatedAt"`
}

// WithdrawStatusFromQuotes withdraws a status from quote functionality
func (s *dynamoDBStorage) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	now := time.Now()

	// Update or create quote settings
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "QUOTE_SETTINGS"},
		},
		UpdateExpression: aws.String("SET WithdrawnFromQuotes = :true, UpdatedAt = :now, StatusID = :statusID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true":     &types.AttributeValueMemberBOOL{Value: true},
			":now":      &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":statusID": &types.AttributeValueMemberS{Value: statusID},
		},
	}

	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to withdraw status from quotes: %w", err)
	}

	s.logger().Info("status withdrawn from quotes", zap.String("status_id", statusID))
	return nil
}

// UpdateQuotePermissions updates quote permissions for a status
func (s *dynamoDBStorage) UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error {
	if permissions == nil {
		return fmt.Errorf("permissions cannot be nil")
	}

	now := time.Now()

	// Marshal permissions
	permissionsAV, err := s.MarshalItem(permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal quote permissions: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "QUOTE_SETTINGS"},
		},
		UpdateExpression: aws.String("SET QuotePermissions = :permissions, UpdatedAt = :now, StatusID = :statusID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":permissions": &types.AttributeValueMemberM{Value: permissionsAV},
			":now":         &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":statusID":    &types.AttributeValueMemberS{Value: statusID},
		},
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update quote permissions: %w", err)
	}

	return nil
}

// IsQuoteAllowed checks if a user is allowed to quote a status
func (s *dynamoDBStorage) IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error) {
	// First check if status is withdrawn from quotes
	withdrawn, err := s.IsWithdrawnFromQuotes(ctx, statusID)
	if err != nil {
		return false, fmt.Errorf("failed to check withdrawal status: %w", err)
	}
	if withdrawn {
		return false, nil
	}

	// Get quote settings
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "QUOTE_SETTINGS"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to get quote settings: %w", err)
	}

	// If no settings exist, default to allowing quotes
	if result.Item == nil {
		return true, nil
	}

	var settings QuoteSettings
	if err := s.UnmarshalItem(result.Item, &settings); err != nil {
		return false, fmt.Errorf("failed to unmarshal quote settings: %w", err)
	}

	// Check permissions
	if settings.QuotePermissions == nil {
		return true, nil // Default allow
	}

	permissions := settings.QuotePermissions

	// Check block list first
	for _, blockedID := range permissions.BlockList {
		if blockedID == quoterID {
			return false, nil
		}
	}

	// If public quotes are allowed, allow
	if permissions.AllowPublic {
		return true, nil
	}

	// Check if quoter is a follower (AllowFollowers)
	if permissions.AllowFollowers {
		// Get the status to find its author
		statusObj, err := s.GetObject(ctx, statusID)
		if err != nil {
			s.logger().Warn("failed to get status for follower check",
				zap.String("status_id", statusID),
				zap.Error(err))
		} else {
			var authorID string
			switch obj := statusObj.(type) {
			case *activitypub.Note:
				authorID = obj.AttributedTo
			case map[string]interface{}:
				if attr, ok := obj["attributedTo"].(string); ok {
					authorID = attr
				}
			}
			
			if authorID != "" {
				// Check if quoter follows the status author
				isFollowing, err := s.IsFollowing(ctx, quoterID, authorID)
				if err == nil && isFollowing {
					// Quoter follows the author, allow quote
					return true, nil
				}
			}
		}
	}
	
	// Check if quoter is mentioned in the status (AllowMentioned)
	if permissions.AllowMentioned {
		// Get the status to check for mentions
		statusObj, err := s.GetObject(ctx, statusID)
		if err != nil {
			s.logger().Warn("failed to get status for mention check",
				zap.String("status_id", statusID),
				zap.Error(err))
		} else {
			// Check if quoter is mentioned in the status
			isMentioned, err := s.isUserMentionedInStatus(ctx, quoterID, statusObj)
			if err != nil {
				s.logger().Warn("failed to check mentions",
					zap.String("status_id", statusID),
					zap.String("quoter_id", quoterID),
					zap.Error(err))
			} else if isMentioned {
				// Quoter is mentioned, allow quote
				return true, nil
			}
		}
	}

	// If none of the permission conditions are met, deny
	return false, nil
}

// GetQuoteType returns the quote type for a status
func (s *dynamoDBStorage) GetQuoteType(ctx context.Context, statusID string) (string, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "QUOTE_SETTINGS"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get quote settings: %w", err)
	}

	if result.Item == nil {
		return "public", nil // Default quote type
	}

	var settings QuoteSettings
	if err := s.UnmarshalItem(result.Item, &settings); err != nil {
		return "", fmt.Errorf("failed to unmarshal quote settings: %w", err)
	}

	if settings.QuoteType == "" {
		return "public", nil
	}

	return settings.QuoteType, nil
}

// IsWithdrawnFromQuotes checks if a status is withdrawn from quotes
func (s *dynamoDBStorage) IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "QUOTE_SETTINGS"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to get quote settings: %w", err)
	}

	if result.Item == nil {
		return false, nil // No settings = not withdrawn
	}

	var settings QuoteSettings
	if err := s.UnmarshalItem(result.Item, &settings); err != nil {
		return false, fmt.Errorf("failed to unmarshal quote settings: %w", err)
	}

	return settings.WithdrawnFromQuotes, nil
}

// GetQuotesOfStatus returns quotes of a specific status
func (s *dynamoDBStorage) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	// Query for quotes of this status
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("quotes-by-target"),
		KeyConditionExpression: aws.String("GSI1PK = :targetPK"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":targetPK": &types.AttributeValueMemberS{Value: fmt.Sprintf("QUOTE_TARGET#%s", statusID)},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            aws.Int32(int32(limit)),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If GSI doesn't exist, fall back to basic method
		s.logger().Warn("quotes-by-target GSI not available, using fallback",
			zap.String("status_id", statusID),
			zap.Error(err))
		return s.fallbackGetQuotesOfStatus(ctx, statusID, limit)
	}

	quotes := make([]*storage.StatusSearchResult, 0)
	for _, item := range result.Items {
		var quoteRecord QuoteRelationshipRecord
		if err := s.UnmarshalItem(item, &quoteRecord); err != nil {
			s.logger().Warn("failed to unmarshal quote relationship", zap.Error(err))
			continue
		}

		if quoteRecord.QuoteRelationship == nil {
			continue
		}

		// Create status search result for the quote
		quote := &storage.StatusSearchResult{
			StatusID:  quoteRecord.QuoteRelationship.QuoterNoteID,
			Content:   "", // Would be populated from full status fetch
			AuthorID:  quoteRecord.QuoteRelationship.QuoterID,
			Published: quoteRecord.QuoteRelationship.Timestamp,
			Score:     1.0,
		}

		quotes = append(quotes, quote)
	}

	return quotes, nil
}

// fallbackGetQuotesOfStatus provides a fallback when GSI is not available
func (s *dynamoDBStorage) fallbackGetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	// Use the existing GetQuotesForNote method
	quoteRelationships, _, err := s.GetQuotesForNote(ctx, statusID, limit, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get quotes using fallback: %w", err)
	}

	quotes := make([]*storage.StatusSearchResult, 0)
	for _, rel := range quoteRelationships {
		quote := &storage.StatusSearchResult{
			StatusID:  rel.QuoterNoteID,
			Content:   "",
			AuthorID:  rel.QuoterID,
			Published: rel.Timestamp,
			Score:     1.0,
		}
		quotes = append(quotes, quote)
	}

	return quotes, nil
}

// isUserMentionedInStatus checks if a user is mentioned in a status
func (s *dynamoDBStorage) isUserMentionedInStatus(ctx context.Context, userID string, statusObj interface{}) (bool, error) {
	// Get the user's actor to find their handle
	userActor, err := s.GetActor(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user actor: %w", err)
	}
	
	// Extract possible mention formats for this user
	possibleMentions := []string{
		userActor.PreferredUsername,                    // @username
		fmt.Sprintf("@%s", userActor.PreferredUsername), // @username
		userActor.ID,                                   // full actor ID
	}
	
	// Extract content and tags from the status
	var content string
	var tags []interface{}
	
	switch obj := statusObj.(type) {
	case *activitypub.Note:
		content = obj.Content
		if obj.Tag != nil {
			for _, tag := range obj.Tag {
				tags = append(tags, tag)
			}
		}
	case map[string]interface{}:
		if c, ok := obj["content"].(string); ok {
			content = c
		}
		if tagArray, ok := obj["tag"].([]interface{}); ok {
			tags = tagArray
		}
	}
	
	// Check tags for mentions (ActivityPub Mention type)
	for _, tag := range tags {
		if tagMap, ok := tag.(map[string]interface{}); ok {
			if tagType, ok := tagMap["type"].(string); ok && tagType == "Mention" {
				if href, ok := tagMap["href"].(string); ok {
					// Check if the href matches the user's actor ID
					if href == userActor.ID {
						return true, nil
					}
				}
			}
		}
	}
	
	// Check content for mention patterns
	for _, mention := range possibleMentions {
		if strings.Contains(content, mention) {
			return true, nil
		}
	}
	
	return false, nil
}
