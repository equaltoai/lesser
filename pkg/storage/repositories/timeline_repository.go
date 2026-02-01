package repositories

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// TimelineRepository handles timeline operations using enhanced DynamORM patterns
type TimelineRepository struct {
	*EnhancedBaseRepository[*models.Timeline]
}

// NewTimelineRepository creates a new timeline repository with enhanced functionality
func NewTimelineRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *TimelineRepository {
	// Create enhanced repository optimized for timeline operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Timeline](db, tableName, logger, costService, "TimelineRepository", "timeline")

	// Set up enhanced services for timeline operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Timeline entries cached for fast retrieval
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for timeline update events

	return &TimelineRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateTimelineEntry creates a new timeline entry using BaseRepository
func (r *TimelineRepository) CreateTimelineEntry(ctx context.Context, entry *models.Timeline) error {
	if err := entry.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityTimelineEntry, "prepare creation")
	}

	// Use enhanced validation and creation with automatic permission checking and event emission
	if err := r.ValidateAndCreate(ctx, entry); err != nil {
		r.logger.Error("failed to create timeline entry with enhanced validation",
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return err
	}

	return nil
}

// CreateTimelineEntries creates multiple timeline entries in batch using BaseRepository
func (r *TimelineRepository) CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error {
	// Prepare entries for creation
	for _, entry := range entries {
		if err := entry.BeforeCreate(); err != nil {
			return ErrorHandler.HandleCreateError(err, EntityTimelineEntry, "prepare creation")
		}
	}

	return r.BatchCreate(ctx, entries)
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
		Index("gsi1"). // GSI1
		Where("gsi1PK", "=", gsi1pk).
		OrderBy("gsi1SK", "ASC") // ASC because we use reverse timestamp

	// Resume from the supplied cursor value when available
	if cursor != "" {
		// With reverse timestamp, we use > for getting older entries
		query = query.Where("gsi1SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityTimelineEntry, "public timeline")
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

// getTimelineEntries is a helper method to retrieve timeline entries using BaseRepository pagination
func (r *TimelineRepository) getTimelineEntries(ctx context.Context, timelineType, timelineID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)

	opts := BasePaginationOptions{
		Limit:  limit,
		Cursor: cursor,
		Order:  "ASC", // ASC because we use reverse timestamp
	}

	result, err := r.FindWithPagination(ctx, pk, opts)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityTimelineEntry, "timeline entries")
	}

	return result.Items, result.NextCursor, nil
}

// getTimelineEntriesByGSI is a consolidated function that handles timeline queries by different GSI patterns
func (r *TimelineRepository) getTimelineEntriesByGSI(_ context.Context, indexName, pkField, skField, keyPrefix, value string, limit int, cursor, errorContext string) ([]*models.Timeline, string, error) {
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
		return nil, "", ErrorHandler.HandleQueryError(err, EntityTimelineEntry, errorContext)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("entries", entries, limit); err != nil {
		// We got more results than requested, so there are more pages
		// Use reflection to get the correct SK field value for cursor
		switch skField {
		case "gsi1SK":
			nextCursor = entries[limit-1].GSI1SK
		case "gsi2SK":
			nextCursor = entries[limit-1].GSI2SK
		case "gsi3SK":
			nextCursor = entries[limit-1].GSI3SK
		case "gsi4SK":
			nextCursor = entries[limit-1].GSI4SK
		}
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetTimelineEntriesByPost retrieves all timeline entries for a specific post
func (r *TimelineRepository) GetTimelineEntriesByPost(ctx context.Context, postID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "gsi1", "gsi1PK", "gsi1SK", "POST#", postID, limit, cursor, "post")
}

// GetTimelineEntriesByActor retrieves all timeline entries by a specific actor
func (r *TimelineRepository) GetTimelineEntriesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "gsi2", "gsi2PK", "gsi2SK", "ACTOR#", actorID, limit, cursor, "actor")
}

// GetTimelineEntriesByVisibility retrieves timeline entries by visibility level
func (r *TimelineRepository) GetTimelineEntriesByVisibility(ctx context.Context, visibility string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "gsi3", "gsi3PK", "gsi3SK", "VISIBILITY#", visibility, limit, cursor, "visibility")
}

// GetTimelineEntriesByLanguage retrieves timeline entries by language
func (r *TimelineRepository) GetTimelineEntriesByLanguage(ctx context.Context, language string, limit int, cursor string) ([]*models.Timeline, string, error) {
	return r.getTimelineEntriesByGSI(ctx, "gsi4", "gsi4PK", "gsi4SK", "LANGUAGE#", language, limit, cursor, "language")
}

// GetTimelineEntry retrieves a specific timeline entry using BaseRepository
func (r *TimelineRepository) GetTimelineEntry(ctx context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) (*models.Timeline, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	// Use reverse timestamp like the model does
	reverseTimestamp := 9999999999 - timelineAt.Unix()
	sk := fmt.Sprintf("%010d#%s", reverseTimestamp, entryID)

	var entry models.Timeline
	err := r.Get(ctx, pk, sk, &entry)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityTimelineEntry, entryID)
	}

	return &entry, nil
}

// UpdateTimelineEntry updates an existing timeline entry using BaseRepository
func (r *TimelineRepository) UpdateTimelineEntry(ctx context.Context, entry *models.Timeline) error {
	if err := entry.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityTimelineEntry, "prepare update")
	}

	return r.Update(ctx, entry)
}

// DeleteTimelineEntry deletes a specific timeline entry using BaseRepository
func (r *TimelineRepository) DeleteTimelineEntry(ctx context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) error {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	// Use reverse timestamp like the model does
	reverseTimestamp := 9999999999 - timelineAt.Unix()
	sk := fmt.Sprintf("%010d#%s", reverseTimestamp, entryID)

	return r.Delete(ctx, pk, sk)
}

// DeleteTimelineEntriesByPost deletes all timeline entries for a specific post using BaseRepository
func (r *TimelineRepository) DeleteTimelineEntriesByPost(ctx context.Context, postID string) error {
	// First, get all timeline entries for this post
	entries, _, err := r.GetTimelineEntriesByPost(ctx, postID, 1000, "")
	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityTimelineEntry, "deletion query")
	}

	if err := common.ValidateSliceNotEmpty("entries", entries); err != nil {
		return nil // Nothing to delete
	}

	// Convert to keys for BaseRepository.BatchDelete
	keys := make([]struct{ PK, SK string }, len(entries))
	for i, entry := range entries {
		keys[i] = struct{ PK, SK string }{
			PK: entry.PK,
			SK: entry.SK,
		}
	}

	// Use BaseRepository's batch delete functionality
	err = r.BatchDelete(ctx, keys)
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityTimelineEntry, "batch delete")
	}

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

// CountTimelineEntries counts the number of entries in a timeline using BaseRepository
func (r *TimelineRepository) CountTimelineEntries(ctx context.Context, timelineType, timelineID string) (int, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	return r.Count(ctx, pk)
}

// GetTimelineEntriesInRange retrieves timeline entries within a time range using BaseRepository
func (r *TimelineRepository) GetTimelineEntriesInRange(ctx context.Context, timelineType, timelineID string, startTime, endTime time.Time, limit int) ([]*models.Timeline, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)
	// Use reverse timestamp like the model does
	startReverseTimestamp := 9999999999 - startTime.Unix()
	endReverseTimestamp := 9999999999 - endTime.Unix()
	// Note: with reverse timestamp, the range logic is inverted
	startSK := fmt.Sprintf("%010d#", endReverseTimestamp) // Earlier time becomes larger reverse timestamp
	endSK := fmt.Sprintf("%010d#", startReverseTimestamp) // Later time becomes smaller reverse timestamp

	entries, err := r.QueryBetween(ctx, pk, startSK, endSK, limit)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityTimelineEntry, "range query")
	}

	return entries, nil
}

// GetTimelineEntriesWithFilters retrieves timeline entries with various filters using BaseRepository
func (r *TimelineRepository) GetTimelineEntriesWithFilters(ctx context.Context, timelineType, timelineID string, filters interfaces.TimelineFilters, limit int, _ string) ([]*models.Timeline, string, error) {
	pk := fmt.Sprintf("TIMELINE#%s#%s", timelineType, timelineID)

	// Build filter map for BaseRepository's QueryWithFilter
	filterMap := make(map[string]interface{})

	if filters.OnlyMedia {
		filterMap["HasMedia"] = true
	}

	if filters.ExcludeReplies {
		filterMap["IsReply"] = false
	}

	if filters.ExcludeBoosts {
		filterMap["IsBoost"] = false
	}

	if filters.Language != "" {
		filterMap["Language"] = filters.Language
	}

	if filters.MinID != "" {
		// Convert minID to timestamp for comparison
		if timestamp, err := strconv.ParseInt(filters.MinID, 10, 64); err == nil {
			filterMap["TimelineAt"] = map[string]interface{}{
				"op":    ">=",
				"value": time.Unix(timestamp, 0),
			}
		}
	}

	if filters.MaxID != "" {
		// Convert maxID to timestamp for comparison
		if timestamp, err := strconv.ParseInt(filters.MaxID, 10, 64); err == nil {
			filterMap["TimelineAt"] = map[string]interface{}{
				"op":    "<=",
				"value": time.Unix(timestamp, 0),
			}
		}
	}

	// Base repository handles retrieving limit+1 items to detect more pages
	entries, err := r.QueryWithFilter(ctx, pk, filterMap, limit)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityTimelineEntry, "filtered")
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

	// Resume from the supplied cursor value when available
	if cursor != "" {
		query = query.Where("SK", "<", cursor) // < for getting older conversations
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var participantRecords []*models.ConversationParticipantRecord
	err := query.All(&participantRecords)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityConversation, "participants")
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
