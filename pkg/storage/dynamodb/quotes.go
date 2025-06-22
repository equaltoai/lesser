package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// QuoteRelationshipRecord represents how quote relationships are stored in DynamoDB
type QuoteRelationshipRecord struct {
	PK                string                     `dynamodbav:"PK"` // QUOTE#targetNoteID
	SK                string                     `dynamodbav:"SK"` // QUOTE#quoteNoteID
	QuoteRelationship *storage.QuoteRelationship `dynamodbav:"QuoteRelationship"`

	// GSI attributes
	GSI1PK string `dynamodbav:"GSI1PK"` // QUOTE_TARGET#targetNoteID (for quotes-by-target GSI)
	GSI1SK string `dynamodbav:"GSI1SK"` // TIMESTAMP#timestamp
}

// QuoteStats tracks quote statistics for a note
type QuoteStats struct {
	PK         string    `dynamodbav:"PK"` // NOTE#noteID
	SK         string    `dynamodbav:"SK"` // STATS#QUOTES
	NoteID     string    `dynamodbav:"NoteID"`
	QuoteCount int       `dynamodbav:"QuoteCount"`
	UpdatedAt  time.Time `dynamodbav:"UpdatedAt"`
}

// CreateQuoteRelationship creates a new quote relationship and updates stats
func (s *dynamoDBStorage) CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error {
	// Validate input
	if quote.TargetNoteID == "" || quote.QuoterNoteID == "" {
		return fmt.Errorf("target note ID and quoter note ID are required")
	}

	// Set timestamps
	now := time.Now()
	quote.Timestamp = now

	// Create the record with proper keys
	record := QuoteRelationshipRecord{
		PK:                fmt.Sprintf("QUOTE#%s", quote.TargetNoteID),
		SK:                fmt.Sprintf("QUOTE#%s", quote.QuoterNoteID),
		QuoteRelationship: quote,
		GSI1PK:            fmt.Sprintf("QUOTE_TARGET#%s", quote.TargetNoteID),
		GSI1SK:            fmt.Sprintf("TIMESTAMP#%d", now.Unix()),
	}

	// Marshal the record
	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal quote relationship: %w", err)
	}

	// Put the quote relationship
	putInput := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, putInput)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return fmt.Errorf("quote relationship already exists")
		}
		return fmt.Errorf("failed to create quote relationship: %w", err)
	}

	// Update quote count
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", quote.TargetNoteID)},
			"SK": &types.AttributeValueMemberS{Value: "STATS#QUOTES"},
		},
		UpdateExpression: aws.String("ADD QuoteCount :inc SET UpdatedAt = :now, NoteID = :noteID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc":    &types.AttributeValueMemberN{Value: "1"},
			":now":    &types.AttributeValueMemberS{Value: quote.Timestamp.Format(time.RFC3339)},
			":noteID": &types.AttributeValueMemberS{Value: quote.TargetNoteID},
		},
	}

	_, err = s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		s.logger().Warn("failed to update quote count",
			zap.Error(err),
			zap.String("targetNoteID", quote.TargetNoteID))
	}

	// Cost tracking is handled automatically by the wrapped DynamoDB client

	return nil
}

// GetQuotesForNote retrieves quotes for a specific note with pagination
func (s *dynamoDBStorage) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	// Build query input using GSI
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("QUOTE_TARGET#%s", noteID)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	// Add cursor if provided
	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("QUOTE_TARGET#%s", noteID)},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	// Add filter for non-withdrawn quotes
	input.FilterExpression = aws.String("Withdrawn = :false OR attribute_not_exists(Withdrawn)")
	input.ExpressionAttributeValues[":false"] = &types.AttributeValueMemberBOOL{Value: false}

	// Execute query
	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query quotes: %w", err)
	}

	// Unmarshal results
	quotes := make([]*storage.QuoteRelationship, 0, len(result.Items))
	for _, item := range result.Items {
		var quote storage.QuoteRelationship
		if err := s.UnmarshalItem(item, &quote); err != nil {
			s.logger().Warn("failed to unmarshal quote", zap.Error(err))
			continue
		}
		quotes = append(quotes, &quote)
	}

	// Extract next cursor
	nextCursor := ""
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	// Cost tracking is handled automatically by the wrapped DynamoDB client

	return quotes, nextCursor, nil
}

// WithdrawQuote marks a quote as withdrawn (soft delete) by quote note ID
func (s *dynamoDBStorage) WithdrawQuote(ctx context.Context, quoteNoteID string) error {
	now := time.Now()

	// First, we need to find the quote relationship by quote note ID
	// We need to scan for the quote since we only have the quote note ID
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :pkPrefix) AND begins_with(SK, :skPrefix) AND QuoteRelationship.QuoterNoteID = :quoteNoteID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pkPrefix":    &types.AttributeValueMemberS{Value: "QUOTE#"},
			":skPrefix":    &types.AttributeValueMemberS{Value: "QUOTE#"},
			":quoteNoteID": &types.AttributeValueMemberS{Value: quoteNoteID},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Scan(ctx, scanInput)
	if err != nil {
		return fmt.Errorf("failed to find quote relationship: %w", err)
	}

	if len(result.Items) == 0 {
		return fmt.Errorf("quote relationship not found")
	}

	// Extract the quote record
	var record QuoteRelationshipRecord
	if err := s.UnmarshalItem(result.Items[0], &record); err != nil {
		return fmt.Errorf("failed to unmarshal quote relationship: %w", err)
	}

	quote := record.QuoteRelationship

	// Update the quote relationship to mark as withdrawn
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("QUOTE#%s", quote.TargetNoteID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("QUOTE#%s", quoteNoteID)},
		},
		UpdateExpression: aws.String("SET Withdrawn = :true, WithdrawnAt = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true": &types.AttributeValueMemberBOOL{Value: true},
			":now":  &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err = s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return fmt.Errorf("quote relationship not found")
		}
		return fmt.Errorf("failed to withdraw quote: %w", err)
	}

	// Decrement quote count
	updateStatsInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", quote.TargetNoteID)},
			"SK": &types.AttributeValueMemberS{Value: "STATS#QUOTES"},
		},
		UpdateExpression:    aws.String("ADD QuoteCount :dec SET UpdatedAt = :now"),
		ConditionExpression: aws.String("QuoteCount > :zero"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":dec":  &types.AttributeValueMemberN{Value: "-1"},
			":now":  &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":zero": &types.AttributeValueMemberN{Value: "0"},
		},
	}

	_, err = s.client.UpdateItem(ctx, updateStatsInput)
	if err != nil {
		s.logger().Warn("failed to update quote count on withdrawal",
			zap.Error(err),
			zap.String("targetNoteID", quote.TargetNoteID))
	}

	// Cost tracking is handled automatically by the wrapped DynamoDB client

	// Create audit trail entry
	s.logger().Info("quote withdrawn",
		zap.String("targetNoteID", quote.TargetNoteID),
		zap.String("quoteNoteID", quoteNoteID),
		zap.Time("withdrawnAt", now))

	return nil
}

// GetQuoteStats retrieves quote statistics for a note
func (s *dynamoDBStorage) GetQuoteStats(ctx context.Context, noteID string) (*QuoteStats, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			"SK": &types.AttributeValueMemberS{Value: "STATS#QUOTES"},
		},
		ConsistentRead: aws.Bool(false),
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get quote stats: %w", err)
	}

	if result.Item == nil {
		// No stats yet, return zero count
		return &QuoteStats{
			NoteID:     noteID,
			QuoteCount: 0,
			UpdatedAt:  time.Now(),
		}, nil
	}

	var stats QuoteStats
	if err := s.UnmarshalItem(result.Item, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal quote stats: %w", err)
	}

	// Cost tracking is handled automatically by the wrapped DynamoDB client

	return &stats, nil
}

// IsQuoteable checks if a note allows quotes
func (s *dynamoDBStorage) IsQuoteable(ctx context.Context, noteID string) (bool, error) {
	// Query the note to check quoteable flag
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
		},
		ProjectionExpression: aws.String("Quoteable"),
		ConsistentRead:       aws.Bool(false),
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check quoteable status: %w", err)
	}

	if result.Item == nil {
		return false, fmt.Errorf("note not found")
	}

	// Default to true if attribute doesn't exist
	if quoteable, ok := result.Item["Quoteable"]; ok {
		if boolVal, ok := quoteable.(*types.AttributeValueMemberBOOL); ok {
			return boolVal.Value, nil
		}
	}

	// Cost tracking is handled automatically by the wrapped DynamoDB client

	return true, nil // Default to quoteable
}

// CountQuotes returns the number of quotes for a given note
func (s *dynamoDBStorage) CountQuotes(ctx context.Context, noteID string) (int, error) {
	stats, err := s.GetQuoteStats(ctx, noteID)
	if err != nil {
		return 0, err
	}
	return stats.QuoteCount, nil
}

// IsQuoted checks if an actor has quoted a specific note
func (s *dynamoDBStorage) IsQuoted(ctx context.Context, actorID, noteID string) (bool, error) {
	// Query for quotes by this actor on this note
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		FilterExpression:       aws.String("QuoterID = :actor"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("QUOTE_TARGET#%s", noteID)},
			":actor": &types.AttributeValueMemberS{Value: actorID},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check if quoted: %w", err)
	}

	return len(result.Items) > 0, nil
}
