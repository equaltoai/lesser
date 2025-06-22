package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// Moderation pattern constants
const (
	moderationPatternPrefix = "MODERATION_PATTERN#"
	patternMatchPrefix      = "PATTERN_MATCH#"
)

// CreateModerationPattern creates a new moderation pattern
func (s *dynamoDBStorage) CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	if pattern.ID == "" {
		pattern.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	pattern.CreatedAt = now
	pattern.UpdatedAt = now

	// Marshal the pattern
	item, err := attributevalue.MarshalMap(pattern)
	if err != nil {
		return fmt.Errorf("failed to marshal moderation pattern: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: moderationPatternPrefix + pattern.ID}
	item["SK"] = &types.AttributeValueMemberS{Value: "PATTERN"}

	// Add GSI1 for active pattern queries
	if pattern.Active {
		item["GSI1PK"] = &types.AttributeValueMemberS{Value: "MODERATION_PATTERNS#ACTIVE"}
		item["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s#%s", pattern.Severity, pattern.Type, pattern.ID)}
	}

	// Add GSI2 for severity-based queries
	item["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("MODERATION_PATTERNS#%s", pattern.Severity)}
	item["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", pattern.UpdatedAt.Format(time.RFC3339), pattern.ID)}

	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to create moderation pattern: %w", err)
	}

	return nil
}

// GetModerationPattern retrieves a specific moderation pattern
func (s *dynamoDBStorage) GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: moderationPatternPrefix + patternID},
			"SK": &types.AttributeValueMemberS{Value: "PATTERN"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get moderation pattern: %w", err)
	}

	if result.Item == nil {
		return nil, storage.ErrNotFound
	}

	var pattern storage.ModerationPattern
	if err := attributevalue.UnmarshalMap(result.Item, &pattern); err != nil {
		return nil, fmt.Errorf("failed to unmarshal moderation pattern: %w", err)
	}

	return &pattern, nil
}

// GetModerationPatterns retrieves moderation patterns based on criteria
func (s *dynamoDBStorage) GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error) {
	var input *dynamodb.QueryInput

	if active && severity != "" {
		// Query by active status and severity
		input = &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			IndexName:              aws.String("GSI2"),
			KeyConditionExpression: aws.String("GSI2PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("MODERATION_PATTERNS#%s", severity)},
			},
			ScanIndexForward: aws.Bool(false), // Most recent first
			Limit:            safeInt32(limit),
		}
	} else if active {
		// Query by active status only
		input = &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			IndexName:              aws.String("GSI1"),
			KeyConditionExpression: aws.String("GSI1PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "MODERATION_PATTERNS#ACTIVE"},
			},
			Limit: safeInt32(limit),
		}
	} else {
		// Scan for all patterns (less efficient)
		return s.scanModerationPatterns(ctx, severity, limit)
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query moderation patterns: %w", err)
	}

	patterns := make([]*storage.ModerationPattern, 0, len(result.Items))
	for _, item := range result.Items {
		var pattern storage.ModerationPattern
		if err := attributevalue.UnmarshalMap(item, &pattern); err != nil {
			continue
		}
		patterns = append(patterns, &pattern)
	}

	return patterns, nil
}

// scanModerationPatterns scans for patterns when query is not efficient
func (s *dynamoDBStorage) scanModerationPatterns(ctx context.Context, severity string, limit int) ([]*storage.ModerationPattern, error) {
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :pk_prefix) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk_prefix": &types.AttributeValueMemberS{Value: moderationPatternPrefix},
			":sk":        &types.AttributeValueMemberS{Value: "PATTERN"},
		},
		Limit: safeInt32(limit),
	}

	if severity != "" {
		input.FilterExpression = aws.String("begins_with(PK, :pk_prefix) AND SK = :sk AND Severity = :severity")
		input.ExpressionAttributeValues[":severity"] = &types.AttributeValueMemberS{Value: severity}
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan moderation patterns: %w", err)
	}

	patterns := make([]*storage.ModerationPattern, 0, len(result.Items))
	for _, item := range result.Items {
		var pattern storage.ModerationPattern
		if err := attributevalue.UnmarshalMap(item, &pattern); err != nil {
			continue
		}
		patterns = append(patterns, &pattern)
	}

	return patterns, nil
}

// UpdateModerationPattern updates an existing moderation pattern
func (s *dynamoDBStorage) UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	pattern.UpdatedAt = time.Now()

	// Marshal the pattern
	item, err := attributevalue.MarshalMap(pattern)
	if err != nil {
		return fmt.Errorf("failed to marshal moderation pattern: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: moderationPatternPrefix + pattern.ID}
	item["SK"] = &types.AttributeValueMemberS{Value: "PATTERN"}

	// Update GSI1 for active pattern queries
	if pattern.Active {
		item["GSI1PK"] = &types.AttributeValueMemberS{Value: "MODERATION_PATTERNS#ACTIVE"}
		item["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s#%s", pattern.Severity, pattern.Type, pattern.ID)}
	} else {
		// Remove from active index
		item["GSI1PK"] = &types.AttributeValueMemberNULL{Value: true}
		item["GSI1SK"] = &types.AttributeValueMemberNULL{Value: true}
	}

	// Update GSI2 for severity-based queries
	item["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("MODERATION_PATTERNS#%s", pattern.Severity)}
	item["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", pattern.UpdatedAt.Format(time.RFC3339), pattern.ID)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to update moderation pattern: %w", err)
	}

	return nil
}

// DeleteModerationPattern deletes a moderation pattern
func (s *dynamoDBStorage) DeleteModerationPattern(ctx context.Context, patternID string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: moderationPatternPrefix + patternID},
			"SK": &types.AttributeValueMemberS{Value: "PATTERN"},
		},
	}

	if _, err := s.client.DeleteItem(ctx, input); err != nil {
		return fmt.Errorf("failed to delete moderation pattern: %w", err)
	}

	return nil
}

// RecordPatternMatch records a pattern match event
func (s *dynamoDBStorage) RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error {
	// Create a match record
	matchRecord := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: moderationPatternPrefix + patternID},
		"SK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("MATCH#%s", timestamp.Format(time.RFC3339))},
		"PatternID": &types.AttributeValueMemberS{Value: patternID},
		"Matched":   &types.AttributeValueMemberBOOL{Value: matched},
		"Timestamp": &types.AttributeValueMemberS{Value: timestamp.Format(time.RFC3339)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", timestamp.Add(90*24*time.Hour).Unix())}, // Keep for 90 days
	}

	// Add GSI for time-based queries
	matchRecord["GSI3PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN_MATCHES#%s", patternID)}
	matchRecord["GSI3SK"] = &types.AttributeValueMemberS{Value: timestamp.Format(time.RFC3339)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      matchRecord,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to record pattern match: %w", err)
	}

	// Also update the pattern's last match time if it was a successful match
	if matched {
		updateInput := &dynamodb.UpdateItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: moderationPatternPrefix + patternID},
				"SK": &types.AttributeValueMemberS{Value: "PATTERN"},
			},
			UpdateExpression: aws.String("SET LastMatch = :timestamp"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":timestamp": &types.AttributeValueMemberS{Value: timestamp.Format(time.RFC3339)},
			},
		}

		if _, err := s.client.UpdateItem(ctx, updateInput); err != nil {
			// Log error but don't fail the record operation
			fmt.Printf("Failed to update pattern last match time: %v\n", err)
		}
	}

	return nil
}

// GetPatternMatches retrieves recent matches for a pattern
func (s *dynamoDBStorage) GetPatternMatches(ctx context.Context, patternID string, limit int) ([]*PatternMatch, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN_MATCHES#%s", patternID)},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get pattern matches: %w", err)
	}

	matches := make([]*PatternMatch, 0, len(result.Items))
	for _, item := range result.Items {
		var match PatternMatch
		if err := attributevalue.UnmarshalMap(item, &match); err != nil {
			continue
		}
		matches = append(matches, &match)
	}

	return matches, nil
}

// PatternMatch represents a recorded pattern match
type PatternMatch struct {
	PatternID string    `json:"pattern_id" dynamodbav:"PatternID"`
	Matched   bool      `json:"matched" dynamodbav:"Matched"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"Timestamp"`
}
