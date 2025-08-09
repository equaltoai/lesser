package repositories

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// TimelineRepository handles timeline operations using DynamORM
type TimelineRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewTimelineRepository creates a new timeline repository
func NewTimelineRepository(db core.DB, tableName string, logger *zap.Logger) *TimelineRepository {
	return &TimelineRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
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
	if len(entries) == 0 {
		return nil
	}

	// Prepare all entries
	for _, entry := range entries {
		if err := entry.BeforeCreate(); err != nil {
			return fmt.Errorf("failed to prepare timeline entry for creation: %w", err)
		}
	}

	// Convert to []any for batch operations
	items := make([]any, len(entries))
	for i, entry := range entries {
		items[i] = entry
	}

	// Use batch writer for efficient bulk creation
	batchWriter := batch.NewBatchWriter(r.db, batch.BatchWriterConfig{
		BatchSize: batch.DefaultBatchSize,
		Logger:    r.logger,
	})

	result, err := batchWriter.WriteItems(ctx, items)
	if err != nil {
		return fmt.Errorf("failed to batch create timeline entries: %w", err)
	}

	// Check if any items failed
	if result.FailedItems > 0 {
		r.logger.Warn("some timeline entries failed to create",
			zap.Int("failed_items", result.FailedItems),
			zap.Int("total_items", result.TotalItems),
		)
		// For timeline entries, we'll continue even with some failures
		// since they're not critical for app functionality
	}

	return nil
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
	if len(entries) > limit {
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
	pk := fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)
	query := r.db.Model(&models.Timeline{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC") // Most recent first

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
	if len(entries) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetTimelineEntriesByPost retrieves all timeline entries for a specific post
func (r *TimelineRepository) GetTimelineEntriesByPost(_ context.Context, postID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	query := r.db.Model(&models.Timeline{}).
		Index("post-timeline-index").
		Where("GSI1PK", "=", "POST#"+postID).
		OrderBy("GSI1SK", "ASC") // ASC because we use reverse timestamp

	if cursor != "" {
		query = query.Where("GSI1SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get timeline entries by post: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(entries) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].GSI1SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetTimelineEntriesByActor retrieves all timeline entries by a specific actor
func (r *TimelineRepository) GetTimelineEntriesByActor(_ context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	query := r.db.Model(&models.Timeline{}).
		Index("actor-timeline-index").
		Where("GSI2PK", "=", "ACTOR#"+actorID).
		OrderBy("GSI2SK", "ASC") // ASC because we use reverse timestamp

	if cursor != "" {
		query = query.Where("GSI2SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get timeline entries by actor: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(entries) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].GSI2SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetTimelineEntriesByVisibility retrieves timeline entries by visibility level
func (r *TimelineRepository) GetTimelineEntriesByVisibility(_ context.Context, visibility string, limit int, cursor string) ([]*models.Timeline, string, error) {
	query := r.db.Model(&models.Timeline{}).
		Index("visibility-timeline-index").
		Where("GSI3PK", "=", "VISIBILITY#"+visibility).
		OrderBy("GSI3SK", "ASC") // ASC because we use reverse timestamp

	if cursor != "" {
		query = query.Where("GSI3SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get timeline entries by visibility: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(entries) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].GSI3SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetTimelineEntriesByLanguage retrieves timeline entries by language
func (r *TimelineRepository) GetTimelineEntriesByLanguage(_ context.Context, language string, limit int, cursor string) ([]*models.Timeline, string, error) {
	query := r.db.Model(&models.Timeline{}).
		Index("language-timeline-index").
		Where("GSI4PK", "=", "LANGUAGE#"+language).
		OrderBy("GSI4SK", "ASC") // ASC because we use reverse timestamp

	if cursor != "" {
		query = query.Where("GSI4SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get timeline entries by language: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(entries) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].GSI4SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
}

// GetTimelineEntry retrieves a specific timeline entry
func (r *TimelineRepository) GetTimelineEntry(_ context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) (*models.Timeline, error) {
	pk := fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)
	sk := fmt.Sprintf("%d#%s", timelineAt.Unix(), entryID)

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
	pk := fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)
	sk := fmt.Sprintf("%d#%s", timelineAt.Unix(), entryID)

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

	if len(entries) == 0 {
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
	// This is a complex operation that would require scanning the table
	// In a real implementation, you might want to use DynamoDB TTL instead
	// For now, we'll implement a basic version that scans and deletes

	// Note: This is not the most efficient approach for large datasets
	// Consider using DynamoDB TTL for automatic expiration

	var expiredEntries []*models.Timeline

	// Scan for expired entries (this is expensive - consider using TTL instead)
	err := r.db.Model(&models.Timeline{}).
		Filter("ExpiresAt", "<", before).
		All(&expiredEntries)
	if err != nil {
		return fmt.Errorf("failed to scan for expired timeline entries: %w", err)
	}

	if len(expiredEntries) == 0 {
		return nil // Nothing to delete
	}

	// Use batch delete for efficient bulk deletion
	keys := make([]any, len(expiredEntries))
	for i, entry := range expiredEntries {
		// Create key structs with PK and SK for deletion
		keys[i] = &models.Timeline{
			PK: entry.PK,
			SK: entry.SK,
		}
	}

	// Use DynamORM's batch delete functionality
	err = r.db.Model(&models.Timeline{}).BatchDelete(keys)
	if err != nil {
		return fmt.Errorf("failed to batch delete expired timeline entries: %w", err)
	}

	r.logger.Info("batch deleted expired timeline entries",
		zap.Time("before", before),
		zap.Int("deleted_count", len(expiredEntries)),
	)

	return nil
}

// CountTimelineEntries counts the number of entries in a timeline
func (r *TimelineRepository) CountTimelineEntries(_ context.Context, timelineType, timelineID string) (int, error) {
	pk := fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)

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
	pk := fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)
	startSK := fmt.Sprintf("%d#", startTime.Unix())
	endSK := fmt.Sprintf("%d#", endTime.Unix())

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
		Where("PK", "=", fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)).
		OrderBy("SK", "DESC")

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
	if len(entries) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].SK
		entries = entries[:limit] // Trim to requested limit
	}

	return entries, nextCursor, nil
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
