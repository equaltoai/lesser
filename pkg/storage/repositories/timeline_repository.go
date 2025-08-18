package repositories

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// TimelineRepository handles timeline operations using DynamORM
type TimelineRepository struct {
	db          core.DB
	tableName   string
	logger      *zap.Logger
	batchHelper *BatchOperationHelper
}

// NewTimelineRepository creates a new timeline repository
func NewTimelineRepository(db core.DB, tableName string, logger *zap.Logger) *TimelineRepository {
	return &TimelineRepository{
		db:          db,
		tableName:   tableName,
		logger:      logger,
		batchHelper: NewBatchOperationHelper(db, tableName, logger),
	}
}

// CreateTimelineEntry creates a new timeline entry
func (r *TimelineRepository) CreateTimelineEntry(_ context.Context, entry *models.Timeline) error {
	if err := entry.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare timeline entry for creation: %w", err)
	}

	err := r.db.Model(entry).Create()
	if err != nil {
		return fmt.Errorf("failed to create timeline entry: %w", err)
	}

	return nil
}

// CreateTimelineEntries creates multiple timeline entries in batch
func (r *TimelineRepository) CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error {
	// Convert to []interface{} for the batch helper
	items := make([]interface{}, len(entries))
	for i, entry := range entries {
		items[i] = entry
	}

	return r.batchHelper.BatchCreateItems(ctx, items, "timeline entries")
}

// GetHomeTimeline retrieves home timeline entries for a user
func (r *TimelineRepository) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntries(ctx, "HOME", username, limit, cursor)
}

// GetPublicTimeline retrieves public timeline entries
func (r *TimelineRepository) GetPublicTimeline(_ context.Context, local bool, limit int, cursor string) ([]*models.Timeline, string, error) {
	// Public timeline uses GSI1 in legacy code
	timelineID := "FEDERATED"
	if local {
		timelineID = "LOCAL"
	}

	gsi1pk := fmt.Sprintf("TIMELINE#PUBLIC#%s", timelineID)

	query := r.db.Model(&models.Timeline{}).
		Index("post-timeline-index"). // GSI1
		Where("GSI1PK", "=", gsi1pk).
		OrderBy("GSI1SK", "ASC") // ASC because we use reverse timestamp

	// Handle cursor-based pagination
	if cursor != "" {
		// With reverse timestamp, we use > for getting older entries
		query = query.Where("GSI1SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get public timeline entries: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("entries", entries, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].GSI1SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetListTimeline retrieves timeline entries for a specific list
func (r *TimelineRepository) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntries(ctx, "LIST", listID, limit, cursor)
}

// GetDirectTimeline retrieves direct message timeline entries for a user
func (r *TimelineRepository) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntries(ctx, "DIRECT", username, limit, cursor)
}

// GetHashtagTimeline retrieves timeline entries for a specific hashtag
func (r *TimelineRepository) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*models.Timeline, string, error) {
	timelineID := hashtag
	if local {
		timelineID = hashtag + "#LOCAL"
	}
	return r.getTimelineEntries(ctx, "HASHTAG", timelineID, limit, cursor)
}

// getTimelineEntries is a helper method to retrieve timeline entries
func (r *TimelineRepository) getTimelineEntries(_ context.Context, timelineType, timelineID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	query := r.db.Model(&models.Timeline{}).
		Where("PK", "=", pk).
		OrderBy("SK", "ASC") // ASC because we use reverse timestamp

	// Handle cursor-based pagination
	if cursor != "" {
		// With reverse timestamp, we use > for getting older entries
		query = query.Where("SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get timeline entries: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("entries", entries, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// getTimelineEntriesByGSI is a consolidated function that handles timeline queries by different GSI patterns
func (r *TimelineRepository) getTimelineEntriesByGSI(ctx context.Context, indexName, pkField, skField, keyPrefix, value string, limit int, cursor, errorContext string) ([]*models.Timeline, string, error) {
	query := r.db.Model(&models.Timeline{}).
		Index(indexName).
		Where(pkField, "=", keyPrefix+value).
		OrderBy(skField, "ASC") // ASC because we use reverse timestamp

	if cursor != "" {
		query = query.Where(skField, ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get timeline entries by %s: %w", errorContext, err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("entries", entries, limit); err != nil {
		// We got more results than requested, so there are more pages
		// Use reflection to get the correct SK field value for cursor
		switch skField {
		case "GSI1SK":
			nextCursor = entries[limit-1].GSI1SK
		case "GSI2SK":
			nextCursor = entries[limit-1].GSI2SK
		case "GSI3SK":
			nextCursor = entries[limit-1].GSI3SK
		case "GSI4SK":
			nextCursor = entries[limit-1].GSI4SK
		}
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetTimelineEntriesByPost retrieves all timeline entries for a specific post
func (r *TimelineRepository) GetTimelineEntriesByPost(ctx context.Context, postID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "post-timeline-index", "GSI1PK", "GSI1SK", "POST#", postID, limit, cursor, "post")
}

// GetTimelineEntriesByActor retrieves all timeline entries by a specific actor
func (r *TimelineRepository) GetTimelineEntriesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "actor-timeline-index", "GSI2PK", "GSI2SK", "ACTOR#", actorID, limit, cursor, "actor")
}

// GetTimelineEntriesByVisibility retrieves timeline entries by visibility level
func (r *TimelineRepository) GetTimelineEntriesByVisibility(ctx context.Context, visibility string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "visibility-timeline-index", "GSI3PK", "GSI3SK", "VISIBILITY#", visibility, limit, cursor, "visibility")
}

// GetTimelineEntriesByLanguage retrieves timeline entries by language
func (r *TimelineRepository) GetTimelineEntriesByLanguage(ctx context.Context, language string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "language-timeline-index", "GSI4PK", "GSI4SK", "LANGUAGE#", language, limit, cursor, "language")
}

// GetTimelineEntry retrieves a specific timeline entry
func (r *TimelineRepository) GetTimelineEntry(_ context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) (*models.Timeline, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	// Use reverse timestamp like the model does
	reverseTimestamp := 9999999999 - timelineAt.Unix()
	sk := fmt.Sprintf("%010d#%s", reverseTimestamp, entryID)

	var entry models.Timeline
	err := r.db.Model(&models.Timeline{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&entry)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeline entry: %w", err)
	}

	return &entry, nil
}

// UpdateTimelineEntry updates an existing timeline entry
func (r *TimelineRepository) UpdateTimelineEntry(_ context.Context, entry *models.Timeline) error {
	if err := entry.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare timeline entry for update: %w", err)
	}

	err := r.db.Model(entry).Update()
	if err != nil {
		return fmt.Errorf("failed to update timeline entry: %w", err)
	}

	return nil
}

// DeleteTimelineEntry deletes a specific timeline entry
func (r *TimelineRepository) DeleteTimelineEntry(_ context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) error {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	// Use reverse timestamp like the model does
	reverseTimestamp := 9999999999 - timelineAt.Unix()
	sk := fmt.Sprintf("%010d#%s", reverseTimestamp, entryID)

	entry := &models.Timeline{
		PK: pk,
		SK: sk,
	}

	err := r.db.Model(entry).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete timeline entry: %w", err)
	}

	return nil
}

// DeleteTimelineEntriesByPost deletes all timeline entries for a specific post
func (r *TimelineRepository) DeleteTimelineEntriesByPost(ctx context.Context, postID string) error {
	// First, get all timeline entries for this post
	entries, _, err := r.GetTimelineEntriesByPost(ctx, postID, 1000, "")
	if err != nil {
		return fmt.Errorf("failed to get timeline entries for deletion: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("entries", entries); err != nil {
		return nil // Nothing to delete
	}

	// Use batch delete for efficient bulk deletion
	keys := make([]any, len(entries))
	for i, entry := range entries {
		// Create key structs with PK and SK for deletion
		keys[i] = &models.Timeline{
			PK: entry.PK,
			SK: entry.SK,
		}
	}

	// Use DynamORM's batch delete functionality
	err = r.db.Model(&models.Timeline{}).BatchDelete(keys)
	if err != nil {
		return fmt.Errorf("failed to batch delete timeline entries: %w", err)
	}

	r.logger.Info("batch deleted timeline entries for post",
		zap.String("post_id", postID),
		zap.Int("deleted_count", len(entries)),
	)

	return nil
}

// DeleteExpiredTimelineEntries deletes timeline entries that have expired
func (r *TimelineRepository) DeleteExpiredTimelineEntries(_ context.Context, before time.Time) error {
	// This method is deprecated - DynamoDB TTL automatically handles cleanup
	// Timeline entries now use TTL field for automatic expiration
	r.logger.Info("DeleteExpiredTimelineEntries called but TTL handles cleanup automatically",
		zap.Time("before", before))
	return nil
}

// CountTimelineEntries counts the number of entries in a timeline
func (r *TimelineRepository) CountTimelineEntries(_ context.Context, timelineType, timelineID string) (int, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)

	count, err := r.db.Model(&models.Timeline{}).
		Where("PK", "=", pk).
		Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count timeline entries: %w", err)
	}

	return int(count), nil
}

// GetTimelineEntriesInRange retrieves timeline entries within a time range
func (r *TimelineRepository) GetTimelineEntriesInRange(_ context.Context, timelineType, timelineID string, startTime, endTime time.Time, limit int) ([]*models.Timeline, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	// Use reverse timestamp like the model does
	startReverseTimestamp := 9999999999 - startTime.Unix()
	endReverseTimestamp := 9999999999 - endTime.Unix()
	// Note: with reverse timestamp, the range logic is inverted
	startSK := fmt.Sprintf("%010d#", endReverseTimestamp) // Earlier time becomes larger reverse timestamp
	endSK := fmt.Sprintf("%010d#", startReverseTimestamp) // Later time becomes smaller reverse timestamp

	var entries []*models.Timeline
	err := r.db.Model(&models.Timeline{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit).
		All(&entries)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeline entries in range: %w", err)
	}

	return entries, nil
}

// GetTimelineEntriesWithFilters retrieves timeline entries with various filters
func (r *TimelineRepository) GetTimelineEntriesWithFilters(_ context.Context, timelineType, timelineID string, filters TimelineFilters, limit int, cursor string) ([]*models.Timeline, string, error) {
	query := r.db.Model(&models.Timeline{}).
		Where("PK", "=", fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)).
		OrderBy("SK", "ASC") // ASC because we use reverse timestamp

	// Apply filters
	if filters.OnlyMedia {
		query = query.Filter("HasMedia", "=", true)
	}

	if filters.ExcludeReplies {
		query = query.Filter("IsReply", "=", false)
	}

	if filters.ExcludeBoosts {
		query = query.Filter("IsBoost", "=", false)
	}

	if filters.Language != "" {
		query = query.Filter("Language", "=", filters.Language)
	}

	if filters.MinID != "" {
		// Convert minID to timestamp for comparison
		if timestamp, err := strconv.ParseInt(filters.MinID, 10, 64); err == nil {
			query = query.Filter("TimelineAt", ">=", time.Unix(timestamp, 0))
		}
	}

	if filters.MaxID != "" {
		// Convert maxID to timestamp for comparison
		if timestamp, err := strconv.ParseInt(filters.MaxID, 10, 64); err == nil {
			query = query.Filter("TimelineAt", "<=", time.Unix(timestamp, 0))
		}
	}

	// Handle cursor-based pagination
	if cursor != "" {
		// With reverse timestamp, we use > for getting older entries
		query = query.Where("SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get filtered timeline entries: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("entries", entries, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetConversations retrieves conversations for a user (timeline interface compatibility)
// This bridges between timeline interface and conversation repository
func (r *TimelineRepository) GetConversations(ctx context.Context, username string, limit int, cursor string) ([]*models.Conversation, string, error) {
	// Query user's conversation participant records using the established pattern
	// PK = USER_CONVERSATIONS#username, SK = timestamp#conversationID
	pk := fmt.Sprintf("USER_CONVERSATIONS#%s", username)

	query := r.db.WithContext(ctx).Model(&models.ConversationParticipantRecord{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC") // Most recent first (timestamp-based sorting)

	// Handle cursor-based pagination
	if cursor != "" {
		query = query.Where("SK", "<", cursor) // < for getting older conversations
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var participantRecords []*models.ConversationParticipantRecord
	err := query.All(&participantRecords)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get conversation participant records: %w", err)
	}

	// Extract conversations from participant records
	conversations := make([]*models.Conversation, 0, len(participantRecords))
	for _, record := range participantRecords {
		if record.Conversation != nil {
			conversations = append(conversations, record.Conversation)
		}
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("conversations", conversations, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = participantRecords[limit-1].SK
		conversations = conversations[:limit] // Trim to requested limit
	}

	return conversations, nextCursor, nil
}

// RemoveFromTimelines removes timeline entries for a specific object across all timelines
func (r *TimelineRepository) RemoveFromTimelines(ctx context.Context, objectID string) error {
	// Use the existing DeleteTimelineEntriesByPost method which handles all timeline entries for an object
	return r.DeleteTimelineEntriesByPost(ctx, objectID)
}

// TimelineFilters represents filters for timeline queries
type TimelineFilters struct {
	OnlyMedia      bool   // Only show entries with media
	ExcludeReplies bool   // Exclude reply entries
	ExcludeBoosts  bool   // Exclude boost/announce entries
	Language       string // Filter by language
	MinID          string // Minimum entry ID (for pagination)
	MaxID          string // Maximum entry ID (for pagination)
}
