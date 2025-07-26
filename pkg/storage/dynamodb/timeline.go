package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	cfg "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// TimelineRecord represents a timeline entry in DynamoDB
type TimelineRecord struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	GSI1PK    string `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK    string `dynamodbav:"GSI1SK,omitempty"`
	Entry     *storage.TimelineEntry
	TTL       int64     `dynamodbav:"TTL,omitempty"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// WriteToTimeline writes a single timeline entry
func (s *dynamoDBStorage) WriteToTimeline(ctx context.Context, entry *storage.TimelineEntry) error {
	log := common.Logger().With(
		zap.String("timeline_type", entry.TimelineType),
		zap.String("timeline_id", entry.TimelineID),
		zap.String("post_id", entry.PostID),
	)

	// Create the record
	record := &TimelineRecord{
		PK:        s.timelinePK(entry.TimelineType, entry.TimelineID),
		SK:        s.timelineSK(entry.TimelineAt, entry.PostID),
		Entry:     entry,
		CreatedAt: time.Now(),
	}

	// Set TTL if specified
	if !entry.ExpiresAt.IsZero() {
		record.TTL = entry.ExpiresAt.Unix()
	}

	// Add GSI for public timeline
	if entry.TimelineType == "PUBLIC" {
		record.GSI1PK = fmt.Sprintf("TIMELINE#PUBLIC#%s", entry.TimelineID) // LOCAL or FEDERATED
		record.GSI1SK = record.SK
	}

	item, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal timeline entry", zap.Error(err))
		return fmt.Errorf("failed to marshal timeline entry: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		log.Error("failed to write timeline entry", zap.Error(err))
		return fmt.Errorf("failed to write timeline entry: %w", err)
	}

	log.Debug("timeline entry written successfully")
	return nil
}

// WriteToTimelines writes multiple timeline entries in batch
func (s *dynamoDBStorage) WriteToTimelines(ctx context.Context, entries []*storage.TimelineEntry) error {
	if len(entries) == 0 {
		return nil
	}

	log := common.Logger().With(zap.Int("entry_count", len(entries)))

	// DynamoDB batch write limit is 25 items
	const batchSize = 25

	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]
		writeRequests := make([]types.WriteRequest, 0, len(batch))

		for _, entry := range batch {
			record := &TimelineRecord{
				PK:        s.timelinePK(entry.TimelineType, entry.TimelineID),
				SK:        s.timelineSK(entry.TimelineAt, entry.PostID),
				Entry:     entry,
				CreatedAt: time.Now(),
			}

			// Set TTL if specified
			if !entry.ExpiresAt.IsZero() {
				record.TTL = entry.ExpiresAt.Unix()
			}

			// Add GSI for public timeline
			if entry.TimelineType == "PUBLIC" {
				record.GSI1PK = fmt.Sprintf("TIMELINE#PUBLIC#%s", entry.TimelineID)
				record.GSI1SK = record.SK
			}

			item, err := s.MarshalItem(record)
			if err != nil {
				log.Error("failed to marshal timeline entry", zap.Error(err))
				continue
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: item,
				},
			})
		}

		if len(writeRequests) == 0 {
			continue
		}

		input := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				s.tableName: writeRequests,
			},
		}

		_, err := s.client.BatchWriteItem(ctx, input)
		if err != nil {
			log.Error("failed to batch write timeline entries",
				zap.Error(err),
				zap.Int("batch_start", i),
				zap.Int("batch_size", len(writeRequests)))
			// Continue with next batch even if this one fails
		}
	}

	log.Debug("timeline entries batch written")
	return nil
}

// GetHomeTimeline retrieves a user's home timeline
func (s *dynamoDBStorage) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	log := common.Logger().With(
		zap.String("username", username),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	pk := s.timelinePK("HOME", username)

	// Build the query
	var keyCondition string
	var expressionAttributeValues map[string]types.AttributeValue

	if cursor == "" {
		// No cursor, get most recent
		keyCondition = "PK = :pk"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		}
	} else {
		// With cursor, get older entries
		keyCondition = "PK = :pk AND SK < :cursor"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: pk},
			":cursor": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeValues: expressionAttributeValues,
		Limit:                     safeInt32(limit + 1), // Get one extra to determine if there are more
		ScanIndexForward:          aws.Bool(false),      // Newest first
	}

	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query home timeline", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query home timeline: %w", err)
	}

	entries := make([]*storage.TimelineEntry, 0, len(output.Items))
	for _, item := range output.Items {
		var record TimelineRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal timeline record", zap.Error(err))
			continue
		}
		entries = append(entries, record.Entry)
	}

	// Determine next cursor
	var nextCursor string
	if len(entries) > limit {
		// We have more results
		entries = entries[:limit]
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			nextCursor = s.timelineSK(lastEntry.TimelineAt, lastEntry.PostID)
		}
	}

	log.Debug("retrieved home timeline",
		zap.Int("entry_count", len(entries)),
		zap.Bool("has_more", nextCursor != ""))

	return entries, nextCursor, nil
}

// GetPublicTimeline retrieves the public timeline
func (s *dynamoDBStorage) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	log := common.Logger().With(
		zap.Bool("local", local),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	// Determine which timeline to query
	timelineID := "FEDERATED"
	if local {
		timelineID = "LOCAL"
	}

	gsi1pk := fmt.Sprintf("TIMELINE#PUBLIC#%s", timelineID)

	// Build the query
	var keyCondition string
	var expressionAttributeValues map[string]types.AttributeValue

	if cursor == "" {
		// No cursor, get most recent
		keyCondition = "GSI1PK = :pk"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: gsi1pk},
		}
	} else {
		// With cursor, get older entries
		keyCondition = "GSI1PK = :pk AND GSI1SK < :cursor"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: gsi1pk},
			":cursor": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		IndexName:                 aws.String("GSI1"),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeValues: expressionAttributeValues,
		Limit:                     safeInt32(limit + 1),
		ScanIndexForward:          aws.Bool(false), // Newest first
	}

	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query public timeline", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query public timeline: %w", err)
	}

	entries := make([]*storage.TimelineEntry, 0, len(output.Items))
	for _, item := range output.Items {
		var record TimelineRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal timeline record", zap.Error(err))
			continue
		}
		entries = append(entries, record.Entry)
	}

	// Determine next cursor
	var nextCursor string
	if len(entries) > limit {
		// We have more results
		entries = entries[:limit]
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			nextCursor = s.timelineSK(lastEntry.TimelineAt, lastEntry.PostID)
		}
	}

	log.Debug("retrieved public timeline",
		zap.Int("entry_count", len(entries)),
		zap.Bool("has_more", nextCursor != ""))

	return entries, nextCursor, nil
}

// GetListTimeline retrieves a list timeline
func (s *dynamoDBStorage) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// Similar implementation to GetHomeTimeline but with LIST type
	log := common.Logger().With(
		zap.String("list_id", listID),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	pk := s.timelinePK("LIST", listID)

	var keyCondition string
	var expressionAttributeValues map[string]types.AttributeValue

	if cursor == "" {
		keyCondition = "PK = :pk"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		}
	} else {
		keyCondition = "PK = :pk AND SK < :cursor"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: pk},
			":cursor": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeValues: expressionAttributeValues,
		Limit:                     safeInt32(limit + 1),
		ScanIndexForward:          aws.Bool(false),
	}

	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query list timeline", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query list timeline: %w", err)
	}

	entries := make([]*storage.TimelineEntry, 0, len(output.Items))
	for _, item := range output.Items {
		var record TimelineRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal timeline record", zap.Error(err))
			continue
		}
		entries = append(entries, record.Entry)
	}

	var nextCursor string
	if len(entries) > limit {
		entries = entries[:limit]
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			nextCursor = s.timelineSK(lastEntry.TimelineAt, lastEntry.PostID)
		}
	}

	return entries, nextCursor, nil
}

// GetDirectTimeline retrieves direct messages timeline for a user
func (s *dynamoDBStorage) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	log := common.Logger().With(
		zap.String("username", username),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	pk := s.timelinePK("DIRECT", username)

	var keyCondition string
	var expressionAttributeValues map[string]types.AttributeValue

	if cursor == "" {
		keyCondition = "PK = :pk"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		}
	} else {
		keyCondition = "PK = :pk AND SK < :cursor"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: pk},
			":cursor": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeValues: expressionAttributeValues,
		Limit:                     safeInt32(limit + 1),
		ScanIndexForward:          aws.Bool(false),
	}

	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query direct timeline", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query direct timeline: %w", err)
	}

	entries := make([]*storage.TimelineEntry, 0, len(output.Items))
	for _, item := range output.Items {
		var record TimelineRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal timeline record", zap.Error(err))
			continue
		}
		// Only include entries with direct visibility
		if record.Entry.Visibility == "direct" {
			entries = append(entries, record.Entry)
		}
	}

	var nextCursor string
	if len(entries) > limit {
		entries = entries[:limit]
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			nextCursor = s.timelineSK(lastEntry.TimelineAt, lastEntry.PostID)
		}
	}

	return entries, nextCursor, nil
}

// GetHashtagTimeline retrieves posts with a specific hashtag
func (s *dynamoDBStorage) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	log := common.Logger().With(
		zap.String("hashtag", hashtag),
		zap.Bool("local", local),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	// Normalize hashtag (remove # if present)
	hashtag = strings.TrimPrefix(hashtag, "#")
	hashtag = strings.ToLower(hashtag)

	// Determine which timeline to query
	timelineID := hashtag
	if local {
		timelineID = fmt.Sprintf("%s#LOCAL", hashtag)
	}

	pk := s.timelinePK("HASHTAG", timelineID)

	var keyCondition string
	var expressionAttributeValues map[string]types.AttributeValue

	if cursor == "" {
		keyCondition = "PK = :pk"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		}
	} else {
		keyCondition = "PK = :pk AND SK < :cursor"
		expressionAttributeValues = map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: pk},
			":cursor": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeValues: expressionAttributeValues,
		Limit:                     safeInt32(limit + 1),
		ScanIndexForward:          aws.Bool(false), // Newest first
	}

	output, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query hashtag timeline", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query hashtag timeline: %w", err)
	}

	entries := make([]*storage.TimelineEntry, 0, len(output.Items))
	for _, item := range output.Items {
		var record TimelineRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal timeline record", zap.Error(err))
			continue
		}

		// Filter local-only if requested
		if local && !strings.HasPrefix(record.Entry.ActorID, cfg.Get().BaseURL()) {
			continue
		}

		entries = append(entries, record.Entry)
	}

	var nextCursor string
	if len(entries) > limit {
		entries = entries[:limit]
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			nextCursor = s.timelineSK(lastEntry.TimelineAt, lastEntry.PostID)
		}
	}

	log.Debug("retrieved hashtag timeline",
		zap.Int("entry_count", len(entries)),
		zap.Bool("has_more", nextCursor != ""))

	return entries, nextCursor, nil
}

// DeleteFromTimeline removes an entry from a timeline
func (s *dynamoDBStorage) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	log := common.Logger().With(
		zap.String("timeline_type", timelineType),
		zap.String("timeline_id", timelineID),
		zap.String("entry_id", entryID),
	)

	pk := s.timelinePK(timelineType, timelineID)

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: entryID},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		log.Error("failed to delete timeline entry", zap.Error(err))
		return fmt.Errorf("failed to delete timeline entry: %w", err)
	}

	return nil
}

// DeleteExpiredTimelineEntries removes expired entries (this would be handled by DynamoDB TTL in production)
func (s *dynamoDBStorage) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	// In production, DynamoDB TTL handles this automatically
	// This method is here for manual cleanup if needed
	log := common.Logger().With(zap.Time("before", before))
	log.Info("DeleteExpiredTimelineEntries called - in production, DynamoDB TTL handles this")
	return nil
}

// Helper methods

func (s *dynamoDBStorage) timelinePK(timelineType, timelineID string) string {
	return fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
}

func (s *dynamoDBStorage) timelineSK(timestamp time.Time, postID string) string {
	// Use reverse timestamp for newest-first ordering
	reverseTimestamp := 9999999999 - timestamp.Unix()
	return fmt.Sprintf("%010d#%s", reverseTimestamp, postID)
}
