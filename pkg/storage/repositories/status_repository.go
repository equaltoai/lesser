package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
	"sort"
	"strings"
	"time"
)

// StatusRepository implements status operations using DynamORM with EnhancedBaseRepository
type StatusRepository struct {
	*EnhancedBaseRepository[*models.Status]
	relationshipRepo interface{} // Temporarily use interface to avoid circular dependency
	bookmarkRepo     *BookmarkRepository
}

// NewStatusRepository creates a new status repository with enhanced functionality
func NewStatusRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *StatusRepository {
	// Create enhanced repository with cost tracking and full service integration
	enhancedRepo := NewEnhancedBaseRepository[*models.Status](db, tableName, logger, costService, "StatusRepository", "status")

	// Set up enhanced services for status operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &StatusRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// SetRelationshipRepository sets the relationship repository dependency for cross-repository operations
func (r *StatusRepository) SetRelationshipRepository(relationshipRepo interface{}) {
	r.relationshipRepo = relationshipRepo
}

// SetBookmarkRepository wires the bookmark repository dependency.
func (r *StatusRepository) SetBookmarkRepository(bookmarkRepo *BookmarkRepository) {
	r.bookmarkRepo = bookmarkRepo
}

func (r *StatusRepository) getBookmarkRepository() *BookmarkRepository {
	if r.bookmarkRepo == nil {
		r.bookmarkRepo = NewBookmarkRepository(r.db, r.tableName, r.logger)
	}
	return r.bookmarkRepo
}

const (
	defaultHomeTimelinePageLimit    = 20
	maxHomeTimelinePageLimit        = 40
	homeTimelineStatusesPerActor    = 20
	homeTimelineFollowingSampleSize = 1000
)

// CreateStatus creates a new status using enhanced validation and event emission
func (r *StatusRepository) CreateStatus(ctx context.Context, status *models.Status) error {
	// Use enhanced validation and creation with automatic event emission
	if err := r.ValidateAndCreate(ctx, status); err != nil {
		return err
	}

	if err := r.canonicalizeStatusIndexes(ctx, status); err != nil {
		r.logger.Warn("failed to canonicalize status index attributes",
			zap.String("status_id", status.StatusID),
			zap.Error(err))
	}

	if err := r.createHashtagTimelineIndexes(ctx, status); err != nil {
		r.logger.Warn("failed to create supplemental hashtag index records",
			zap.String("status_id", status.StatusID),
			zap.Strings("hashtags", status.Hashtags),
			zap.Error(err))
	}

	return nil
}

// CreateBoostStatus persists a boost wrapper as a first-class status.
func (r *StatusRepository) CreateBoostStatus(ctx context.Context, status *models.Status) error {
	if status == nil {
		return fmt.Errorf("boost status payload is required")
	}

	if status.BoostOfStatusID == "" && status.ReblogOfID == "" {
		return fmt.Errorf("boost status missing target reference")
	}

	if status.AuthorID == "" {
		return fmt.Errorf("boost status missing author id")
	}

	return r.CreateStatus(ctx, status)
}

// createHashtagTimelineIndexes persists supplemental hashtag index records so hashtag timelines can query efficiently.
func (r *StatusRepository) createHashtagTimelineIndexes(ctx context.Context, status *models.Status) error {
	if status == nil || len(status.Hashtags) == 0 {
		return nil
	}

	now := time.Now()
	records := make([]*models.HashtagStatusIndex, 0, len(status.Hashtags))

	for _, tag := range status.Hashtags {
		normalized := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(tag), "#"))
		if normalized == "" {
			continue
		}

		record := &models.HashtagStatusIndex{
			StatusID:     status.StatusID,
			AuthorID:     status.AuthorID,
			AuthorHandle: status.AuthorUsername,
			StatusURL:    "",
			Content:      status.Content,
			MediaCount:   status.MediaCount,
			Language:     status.Language,
			Visibility:   status.Visibility,
			Published:    status.PublishedAt,
			HashtagName:  normalized,
			TTL:          now.Add(90 * 24 * time.Hour).Unix(),
			CreatedAt:    now,
		}

		if status.Note != nil && status.Note.Get() != nil && status.Note.Get().ID != "" {
			record.StatusURL = status.Note.Get().ID
		}

		if record.Published.IsZero() {
			record.Published = now
		}

		if err := record.UpdateKeys(); err != nil {
			r.logger.Warn("failed to update hashtag timeline keys",
				zap.String("status_id", status.StatusID),
				zap.String("hashtag", normalized),
				zap.Error(err))
			continue
		}

		records = append(records, record)
	}

	if len(records) == 0 {
		return nil
	}

	for _, record := range records {
		err := r.db.WithContext(ctx).Model(record).Create()
		if err != nil {
			if errors.IsConditionFailed(err) {
				r.logger.Debug("hashtag timeline index already exists",
					zap.String("status_id", record.StatusID),
					zap.String("hashtag", record.HashtagName))
				continue
			}
			return err
		}
	}

	return nil
}

func (r *StatusRepository) canonicalizeStatusIndexes(ctx context.Context, status *models.Status) error {
	if status == nil {
		return nil
	}

	pk := fmt.Sprintf("status#%s", status.StatusID)
	sk := pk

	builder := r.db.WithContext(ctx).
		Model(&models.Status{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder()

	if status.Visibility == models.VisibilityPublic {
		timestamp := status.PublishedAt.Unix()
		if timestamp == 0 {
			timestamp = time.Now().Unix()
		}
		timestampStr := fmt.Sprintf("%d", timestamp)
		partitionKey := "PUBLIC_TIMELINE"
		sortKey := fmt.Sprintf("%s#%s", timestampStr, status.StatusID)

		builder.Set("gsi2PK", partitionKey)
		builder.Set("gsi2SK", sortKey)
	} else {
		builder.Remove("gsi2PK")
		builder.Remove("gsi2SK")
	}

	return builder.Execute()
}

func (r *StatusRepository) queryPublicTimelineDirect(ctx context.Context, opts interfaces.PaginationOptions) ([]*models.Status, error) {
	if opts.Limit <= 0 {
		opts.Limit = defaultHomeTimelinePageLimit
	}

	var statusModels []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi2").
		Where("gsi2PK", "=", "PUBLIC_TIMELINE").
		OrderBy("gsi2SK", "DESC").
		Limit(opts.Limit).
		All(&statusModels)
	if err != nil {
		return nil, fmt.Errorf("query public timeline: %w", err)
	}

	statuses := make([]*models.Status, len(statusModels))
	for i := range statusModels {
		statuses[i] = &statusModels[i]
	}

	r.logger.Info("public timeline query completed",
		zap.Int("items_returned", len(statuses)),
		zap.Int("limit", opts.Limit))

	return statuses, nil
}

// GetStatus retrieves a status by ID using BaseRepository
func (r *StatusRepository) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	var status models.Status
	pk := fmt.Sprintf("status#%s", statusID)
	sk := fmt.Sprintf("status#%s", statusID)

	err := r.Get(ctx, pk, sk, &status)
	if err != nil {
		return nil, err // BaseRepository handles error formatting
	}

	return &status, nil
}

// UpdateStatus updates an existing status using enhanced validation and event emission
func (r *StatusRepository) UpdateStatus(ctx context.Context, status *models.Status) error {
	// Use UpdateBuilder with explicit fields to prevent Note field corruption
	pk := status.PK
	if pk == "" {
		pk = fmt.Sprintf("status#%s", status.StatusID)
	}
	sk := status.SK
	if sk == "" {
		sk = fmt.Sprintf("status#%s", status.StatusID)
	}

	updateBuilder := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder()

	// Set only the fields that should be updated - explicitly exclude Note to prevent corruption
	// Update basic fields
	if status.Content != "" {
		updateBuilder.Set("Content", status.Content)
	}
	updateBuilder.Set("Sensitive", status.Sensitive)
	updateBuilder.Set("Language", status.Language)
	updateBuilder.Set("Visibility", status.Visibility)
	updateBuilder.Set("UpdatedAt", status.UpdatedAt)

	// Update engagement counts (if changed)
	if status.LikeCount >= 0 {
		updateBuilder.Set("LikeCount", status.LikeCount)
	}
	if status.ReblogCount >= 0 {
		updateBuilder.Set("ReblogCount", status.ReblogCount)
	}
	if status.ReplyCount >= 0 {
		updateBuilder.Set("ReplyCount", status.ReplyCount)
	}
	if status.QuoteCount >= 0 {
		updateBuilder.Set("QuoteCount", status.QuoteCount)
	}

	// Update flags
	updateBuilder.Set("Deleted", status.Deleted)
	if status.DeletedAt != nil {
		updateBuilder.Set("DeletedAt", status.DeletedAt)
	}
	updateBuilder.Set("Flagged", status.Flagged)

	// Update addressing fields
	if status.ToRecipients != nil {
		updateBuilder.Set("ToRecipients", status.ToRecipients)
	}
	if status.CcRecipients != nil {
		updateBuilder.Set("CcRecipients", status.CcRecipients)
	}

	// Update quote reference metadata so quote posts resolve correctly
	updateBuilder.Set("QuoteTargetStatusID", status.QuoteTargetStatusID)
	updateBuilder.Set("QuoteTargetAuthorID", status.QuoteTargetAuthorID)

	// Only update Note field if it's explicitly provided and valid
	// This prevents corruption from nil or partially-loaded Note fields
	if status.Note != nil && status.Note.Get() != nil {
		// Validate Note has required fields before updating
		if status.Note.Get().ID != "" && status.Note.Get().Type != "" {
			updateBuilder.Set("Note", status.Note)
		}
	}

	if err := updateBuilder.Execute(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, status.StatusID)
	}

	return nil
}

// DeleteStatus marks a status as deleted using BaseRepository
func (r *StatusRepository) DeleteStatus(ctx context.Context, statusID string) error {
	// Get the status first
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return err
	}

	// Mark as deleted using UpdateBuilder to avoid corrupting Note field
	now := time.Now()
	err = r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", status.PK).
		Where("SK", "=", status.SK).
		UpdateBuilder().
		Set("Deleted", true).
		Set("DeletedAt", now).
		Execute()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	return nil
}

// DeleteBoostStatus removes the boost wrapper for a given booster/target pair.
func (r *StatusRepository) DeleteBoostStatus(ctx context.Context, boosterID, targetStatusID string) (*models.Status, error) {
	if err := common.ValidateRequiredParam("booster_id", boosterID); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("target_status_id", targetStatusID); err != nil {
		return nil, err
	}

	boostStatus, err := r.findBoostStatus(ctx, boosterID, targetStatusID)
	if err != nil {
		if errors.IsNotFound(err) || stdErrors.Is(err, storage.ErrNotFound) {
			r.logger.Debug("no boost status found to delete",
				zap.String("booster_id", boosterID),
				zap.String("target_status_id", targetStatusID))
			return nil, nil
		}
		return nil, err
	}

	if err := r.DeleteStatus(ctx, boostStatus.StatusID); err != nil {
		return nil, err
	}

	return boostStatus, nil
}

func (r *StatusRepository) findBoostStatus(ctx context.Context, boosterID, targetStatusID string) (*models.Status, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("AUTHOR#%s", boosterID)).
		Filter("BoostOfStatusID", "=", targetStatusID).
		Limit(1).
		All(&statuses)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	if len(statuses) == 0 {
		return nil, storage.ErrNotFound
	}

	return &statuses[0], nil
}

// CountStatusesByAuthor counts the total number of statuses by an author
func (r *StatusRepository) CountStatusesByAuthor(ctx context.Context, authorID string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("AUTHOR#%s", authorID)).
		Count()
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, EntityStatus, "count by author")
	}

	return int(count), nil
}

// CountReplies counts the number of replies to a status
func (r *StatusRepository) CountReplies(ctx context.Context, statusID string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi4").
		Where("gsi4PK", "=", fmt.Sprintf("REPLIES#%s", statusID)).
		Count()
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "reply", "count")
	}

	return int(count), nil
}

// UpdateEngagementMetrics updates the cached engagement metrics for a status
func (r *StatusRepository) UpdateEngagementMetrics(ctx context.Context, statusID string, likes, reblogs, replies, quotes int) error {
	pk := fmt.Sprintf("status#%s", statusID)
	sk := fmt.Sprintf("status#%s", statusID)

	// Use UpdateBuilder to update only engagement metrics, avoiding Note field corruption
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder().
		Set("LikeCount", likes).
		Set("ReblogCount", reblogs).
		Set("ReplyCount", replies).
		Set("QuoteCount", quotes).
		Execute()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "engagement metrics", "status")
	}

	return nil
}

// GetTotalStatusCount returns the total number of statuses in the system
func (r *StatusRepository) GetTotalStatusCount(ctx context.Context) (int64, error) {
	r.logger.Debug("getting total status count")

	// For total status count, we need to scan the table with a filter for all statuses
	// Since statuses use PK = "status#{status_id}", we can filter by PK prefix
	// Note: This is less efficient than a GSI but necessary for total count across all statuses
	count, err := r.db.WithContext(ctx).Model(&models.Status{}).
		Filter("PK", "BEGINS_WITH", "status#").
		Count()

	if err != nil {
		r.logger.Error("failed to count total statuses", zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityStatus, "count total")
	}

	r.logger.Debug("retrieved total status count", zap.Int64("count", count))
	return count, nil
}

// ListStatusesForAdmin retrieves statuses with comprehensive admin filtering
func (r *StatusRepository) ListStatusesForAdmin(ctx context.Context, filter *interfaces.StatusFilter, limit int, cursor string) ([]*models.Status, string, error) {
	r.logger.Debug("listing statuses for admin with filter",
		zap.Any("filter", filter),
		zap.Int("limit", limit),
		zap.String("cursor", cursor))

	var statuses []models.Status
	query := r.db.WithContext(ctx).Model(&models.Status{})

	// Base filter to exclude deleted statuses unless specifically requested
	query = query.Filter("Deleted", "=", false)

	statuses, err := r.applyDomainFiltering(ctx, query, filter, limit)
	if err != nil {
		return nil, "", err
	}

	// Convert to pointer slice and generate pagination cursor
	result := r.convertToPointerSlice(statuses)
	nextCursor := r.generateNextCursor(result, limit)

	r.logger.Debug("retrieved filtered statuses for admin",
		zap.Int("count", len(result)),
		zap.String("nextCursor", nextCursor))

	return result, nextCursor, nil
}

// applyDomainFiltering applies domain-based filtering logic to admin status queries
func (r *StatusRepository) applyDomainFiltering(ctx context.Context, query core.Query, filter *interfaces.StatusFilter, limit int) ([]models.Status, error) {
	if filter.Local != nil && *filter.Local {
		return r.applyLocalFiltering(query, filter, limit)
	}

	if filter.Remote != nil && *filter.Remote {
		return r.applyRemoteFiltering(ctx, query, filter, limit)
	}

	return r.applyStandardFiltering(query, filter, limit)
}

// applyLocalFiltering filters for local statuses only
func (r *StatusRepository) applyLocalFiltering(query core.Query, filter *interfaces.StatusFilter, limit int) ([]models.Status, error) {
	var statuses []models.Status
	domain := r.extractDomainFromEnv()
	if domain != "" {
		query = query.Filter("AuthorID", "CONTAINS", domain)
	}

	query = r.applyContentFilters(query, filter)
	query = r.applyDateFilters(query, filter)
	query = query.Limit(limit)

	err := query.Scan(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "scan local")
	}

	return statuses, nil
}

// applyRemoteFiltering filters for remote statuses only (requires post-processing)
func (r *StatusRepository) applyRemoteFiltering(_ context.Context, query core.Query, _ *interfaces.StatusFilter, limit int) ([]models.Status, error) {
	var statuses []models.Status
	domain := r.extractDomainFromEnv()
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		query = query.Limit(limit)
		err := query.Scan(&statuses)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "scan paginated")
		}
		return statuses, nil
	}

	// Note: DynamoDB doesn't have a direct "NOT CONTAINS" operation
	// We'll need to use scan and post-process filtering
	err := query.Scan(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "scan remote filtering")
	}

	// Post-process to filter remote only
	filteredStatuses := []models.Status{}
	for _, status := range statuses {
		if !strings.Contains(status.AuthorID, domain) {
			filteredStatuses = append(filteredStatuses, status)
			if len(filteredStatuses) >= limit {
				break
			}
		}
	}

	return filteredStatuses, nil
}

// GetStatusesByURL searches for statuses that contain a specific URL in their URLs field
func (r *StatusRepository) GetStatusesByURL(ctx context.Context, targetURL string, limit int) ([]*models.Status, error) {
	// Validate limit using centralized validation
	if err := common.ValidateQueryLimit(limit, 100, "status search"); err != nil {
		limit = 20
	}

	// Use GSI7 (URL index) to efficiently find statuses containing the target URL
	normalizedURL := strings.ToLower(strings.TrimSpace(targetURL))
	var matchingStatuses []models.Status

	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi7").
		Where("gsi7PK", "=", "URL#"+normalizedURL).
		Limit(limit).
		All(&matchingStatuses)

	if err != nil {
		r.logger.Error("failed to query statuses by URL",
			zap.String("target_url", targetURL),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "query by URL")
	}

	// Convert to pointer slice and verify URL matches
	var results []*models.Status
	for i := range matchingStatuses {
		status := &matchingStatuses[i]
		// Double-check URL match since we only index the first URL
		for _, url := range status.URLs {
			if url == targetURL {
				results = append(results, status)
				break // Found match, no need to check other URLs for this status
			}
		}

		// Stop if we have enough matches
		if len(results) >= limit {
			break
		}
	}

	// Sort by published date (most recent first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].PublishedAt.After(results[j].PublishedAt)
	})

	return results, nil
}

// applyStandardFiltering applies standard non-domain specific filters
func (r *StatusRepository) applyStandardFiltering(query core.Query, filter *interfaces.StatusFilter, limit int) ([]models.Status, error) {
	var statuses []models.Status

	// Apply specific domain filter
	if filter.ByDomain != "" {
		query = query.Filter("AuthorID", "CONTAINS", filter.ByDomain)
	}

	query = r.applyContentFilters(query, filter)
	query = r.applyDateFilters(query, filter)
	query = query.Limit(limit)

	// Execute query with limit
	err := query.Scan(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "get filtered")
	}

	return statuses, nil
}

// applyContentFilters applies content-related filters (visibility, flagged, sensitive, media)
func (r *StatusRepository) applyContentFilters(query core.Query, filter *interfaces.StatusFilter) core.Query {
	// Apply visibility filter
	if filter.Visibility != "" {
		query = query.Filter("Visibility", "=", filter.Visibility)
	}

	// Apply flagged filter
	if filter.Flagged != nil {
		query = query.Filter("Flagged", "=", *filter.Flagged)
	}

	// Apply sensitive filter
	if filter.Sensitive != nil {
		query = query.Filter("Sensitive", "=", *filter.Sensitive)
	}

	// Apply media filter
	if filter.WithMedia != nil {
		if *filter.WithMedia {
			query = query.Filter("MediaCount", ">", 0)
		} else {
			query = query.Filter("MediaCount", "=", 0)
		}
	}

	return query
}

// applyDateFilters applies date-related filters (MinDate, MaxDate)
func (r *StatusRepository) applyDateFilters(query core.Query, filter *interfaces.StatusFilter) core.Query {
	// Apply date filters
	if filter.MinDate != nil {
		query = query.Filter("PublishedAt", ">=", *filter.MinDate)
	}

	if filter.MaxDate != nil {
		query = query.Filter("PublishedAt", "<=", *filter.MaxDate)
	}

	return query
}

// convertToPointerSlice converts slice of Status to slice of Status pointers
func (r *StatusRepository) convertToPointerSlice(statuses []models.Status) []*models.Status {
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}
	return result
}

// generateNextCursor generates pagination cursor for the next page
func (r *StatusRepository) generateNextCursor(result []*models.Status, limit int) string {
	nextCursor := ""
	if len(result) == limit && len(result) > 0 {
		lastStatus := result[len(result)-1]
		nextCursor = fmt.Sprintf("%d_%s", lastStatus.PublishedAt.Unix(), lastStatus.StatusID)
	}
	return nextCursor
}

// CountStatusesForAdmin counts statuses matching admin filter criteria
func (r *StatusRepository) CountStatusesForAdmin(ctx context.Context, filter *interfaces.StatusFilter) (int64, error) {
	r.logger.Debug("counting statuses for admin with filter", zap.Any("filter", filter))

	var count int64
	query := r.db.WithContext(ctx).Model(&models.Status{})

	// Base filter to exclude deleted statuses unless specifically requested
	query = query.Filter("Deleted", "=", false)

	// Apply domain filters
	if filter.Local != nil && *filter.Local {
		domain := r.extractDomainFromEnv()
		if domain != "" {
			query = query.Filter("AuthorID", "CONTAINS", domain)
		}
	}

	if filter.Remote != nil && *filter.Remote {
		// For remote filtering, we need to scan first then count (not efficient but necessary)
		var statuses []models.Status
		domain := r.extractDomainFromEnv()
		if domain != "" {
			err := query.Scan(&statuses)
			if err != nil {
				return 0, ErrorHandler.HandleQueryError(err, EntityStatus, "scan for count")
			}

			// Count only remote statuses
			remoteCount := 0
			for _, status := range statuses {
				if !strings.Contains(status.AuthorID, domain) {
					remoteCount++
				}
			}
			return int64(remoteCount), nil
		}
	} else {
		// Apply other filters for counting
		if filter.ByDomain != "" {
			query = query.Filter("AuthorID", "CONTAINS", filter.ByDomain)
		}

		if filter.Visibility != "" {
			query = query.Filter("Visibility", "=", filter.Visibility)
		}

		if filter.Flagged != nil {
			query = query.Filter("Flagged", "=", *filter.Flagged)
		}

		if filter.Sensitive != nil {
			query = query.Filter("Sensitive", "=", *filter.Sensitive)
		}

		if filter.WithMedia != nil {
			if *filter.WithMedia {
				query = query.Filter("MediaCount", ">", 0)
			} else {
				query = query.Filter("MediaCount", "=", 0)
			}
		}

		if filter.MinDate != nil {
			query = query.Filter("PublishedAt", ">=", *filter.MinDate)
		}

		if filter.MaxDate != nil {
			query = query.Filter("PublishedAt", "<=", *filter.MaxDate)
		}

		// Use Count method for efficient counting
		count, err := query.Count()
		if err != nil {
			return 0, ErrorHandler.HandleQueryError(err, EntityStatus, "count filtered")
		}

		r.logger.Debug("counted filtered statuses for admin", zap.Int64("count", count))
		return count, nil
	}

	return count, nil
}

// extractDomainFromEnv extracts the local domain from environment
func (r *StatusRepository) extractDomainFromEnv() string {
	// Get domain from centralized config
	cfg := config.Get()
	domain := cfg.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		// Check if there's an alternative domain configured
		// Note: INSTANCE_DOMAIN is not in config yet, but Domain should be the primary source
		if cfg.Domain == "" || cfg.Domain == DefaultDomain {
			// Fallback logic can be added here if needed
			// Currently using the extracted domain as-is
			// Future enhancement: implement alternate domain resolution
			_ = cfg.Domain // acknowledge we're using the current domain
		}
	}
	return domain
}

// GetStatusByURL retrieves a status by its URL using GSI7 URL index
func (r *StatusRepository) GetStatusByURL(ctx context.Context, url string) (*models.Status, error) {
	// Normalize the URL for consistent indexing (same as in Status.setupGSIKeys)
	normalizedURL := strings.ToLower(strings.TrimSpace(url))

	// Query GSI7 for URL-indexed statuses
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi7").
		Where("gsi7PK", "=", "URL#"+normalizedURL).
		Scan(&statuses)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "query by URL")
	}

	// Find exact match by checking the Note.ID field
	for _, status := range statuses {
		if status.Note != nil && status.Note.Get() != nil && status.Note.Get().ID == url {
			return &status, nil
		}
	}

	// If no exact match found, also check the URLs array in case it's a link in content
	for _, status := range statuses {
		for _, statusURL := range status.URLs {
			if statusURL == url {
				return &status, nil
			}
		}
	}

	return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityStatus, url)
}

// GetHomeTimeline retrieves home timeline for a user (statuses from accounts they follow)
func (r *StatusRepository) GetHomeTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// Check if relationship repository dependency is available
	if r.relationshipRepo == nil {
		r.logger.Error("relationshipRepo dependency not set for GetHomeTimeline, falling back to public timeline")
		// Fallback to public timeline if relationship repo is not available
		return r.GetPublicTimeline(ctx, opts)
	}

	followingActorIDs, err := r.fetchFollowingActorIDs(ctx, userID)
	if err != nil {
		return r.GetPublicTimeline(ctx, opts)
	}

	// If user follows no one, return empty timeline (not public timeline)
	if err := common.ValidateSliceNotEmpty("following_actor_ids", followingActorIDs); err != nil {
		r.logger.Debug("user follows no accounts, returning empty home timeline",
			zap.String("user_id", userID))
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	allStatuses := r.collectStatusesForActors(ctx, userID, followingActorIDs)
	limit := sanitizeHomeTimelineLimit(opts.Limit)
	paginated := paginateHomeTimeline(allStatuses, limit)

	r.logger.Debug("successfully built home timeline",
		zap.String("user_id", userID),
		zap.Int("following_count", len(followingActorIDs)),
		zap.Int("total_statuses_found", len(allStatuses)),
		zap.Int("page_statuses", len(paginated.Items)))

	return paginated, nil
}

func (r *StatusRepository) fetchFollowingActorIDs(ctx context.Context, userID string) ([]string, error) {
	cfg := config.Get()

	switch repo := r.relationshipRepo.(type) {
	case interfaces.RelationshipRepository:
		followingResult, err := repo.GetFollowing(ctx, userID, interfaces.PaginationOptions{Limit: homeTimelineFollowingSampleSize})
		if err != nil {
			r.logger.Error("failed to get following list for home timeline",
				zap.String("user_id", userID),
				zap.Error(err))
			return nil, err
		}

		actorIDs := make([]string, 0, len(followingResult.Items))
		for _, account := range followingResult.Items {
			if account == nil {
				continue
			}
			if account.Actor != nil && account.Actor.ID != "" {
				actorIDs = append(actorIDs, account.Actor.ID)
				continue
			}
			if account.User != nil && account.User.Username != "" {
				actorIDs = append(actorIDs, cfg.ActorURL(account.User.Username))
			}
		}
		return actorIDs, nil
	case interface {
		GetFollowing(context.Context, string, int, string) ([]string, string, error)
	}:
		usernames, _, err := repo.GetFollowing(ctx, userID, homeTimelineFollowingSampleSize, "")
		if err != nil {
			r.logger.Error("failed to get following list for home timeline via username accessor",
				zap.String("user_id", userID),
				zap.Error(err))
			return nil, err
		}

		actorIDs := make([]string, 0, len(usernames))
		for _, username := range usernames {
			if username == "" {
				continue
			}
			actorIDs = append(actorIDs, cfg.ActorURL(username))
		}
		return actorIDs, nil
	default:
		r.logger.Error("relationshipRepo does not provide a compatible GetFollowing implementation",
			zap.String("repo_type", fmt.Sprintf("%T", r.relationshipRepo)))
		return nil, fmt.Errorf("relationshipRepo does not support GetFollowing")
	}
}

func (r *StatusRepository) collectStatusesForActors(ctx context.Context, userID string, actorIDs []string) []models.Status {
	// Gather statuses per actor and then sort globally so pagination is consistent
	collected := make([]models.Status, 0, len(actorIDs)*homeTimelineStatusesPerActor)
	for _, actorID := range actorIDs {
		userStatuses := r.fetchStatusesForActor(ctx, userID, actorID)
		if len(userStatuses) == 0 {
			continue
		}
		collected = append(collected, userStatuses...)
	}

	sort.Slice(collected, func(i, j int) bool {
		return collected[i].PublishedAt.After(collected[j].PublishedAt)
	})

	return collected
}

func (r *StatusRepository) fetchStatusesForActor(ctx context.Context, userID string, actorID string) []models.Status {
	var userStatuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("AUTHOR#%s", actorID)).
		OrderBy("gsi1SK", "DESC").
		Limit(homeTimelineStatusesPerActor).
		All(&userStatuses)

	if err != nil {
		r.logger.Error("failed to get statuses for followed user",
			zap.String("user_id", userID),
			zap.String("followed_actor", actorID),
			zap.Error(err))
		return nil
	}

	return userStatuses
}

func sanitizeHomeTimelineLimit(requested int) int {
	limit := requested
	if limit <= 0 {
		limit = defaultHomeTimelinePageLimit
	}

	if err := common.ValidateQueryLimit(limit, maxHomeTimelinePageLimit, "timeline"); err != nil {
		return defaultHomeTimelinePageLimit
	}

	return limit
}

func paginateHomeTimeline(allStatuses []models.Status, limit int) *interfaces.PaginatedResult[*models.Status] {
	if len(allStatuses) == 0 {
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}
	}

	pageStatuses := allStatuses
	if len(pageStatuses) > limit {
		pageStatuses = pageStatuses[:limit]
	}

	statusPtrs := make([]*models.Status, len(pageStatuses))
	for i := range pageStatuses {
		statusPtrs[i] = &pageStatuses[i]
	}

	hasMore := len(allStatuses) > limit
	nextCursor := ""
	if hasMore && len(statusPtrs) > 0 {
		nextCursor = statusPtrs[len(statusPtrs)-1].StatusID
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      statusPtrs,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      int64(len(allStatuses)),
	}
}

// queryStatusesByGSI is a consolidated helper for GSI-based status queries
func (r *StatusRepository) queryStatusesByGSI(ctx context.Context, indexName, gsiPKField, gsiPKValue, gsiSKField, orderDirection string, opts interfaces.PaginationOptions, errorMsg string) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status

	indexName = strings.ToLower(indexName)

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	query := r.db.WithContext(ctx).Model(&models.Status{}).
		Index(indexName).
		Where(gsiPKField, "=", gsiPKValue).
		OrderBy(gsiSKField, orderDirection)

	// Resume from the supplied cursor value when available
	if opts.Cursor != "" {
		operator := "<"
		if orderDirection == "ASC" {
			operator = ">"
		}
		query = query.Where(gsiSKField, operator, opts.Cursor)
	}

	err := query.Limit(limit + 1).All(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, errorMsg)
	}

	hasMore := len(statuses) > limit
	if hasMore {
		statuses = statuses[:limit]
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	var nextCursor string
	if hasMore && len(statuses) > 0 {
		last := statuses[len(statuses)-1]
		switch gsiSKField {
		case gsi1SKField:
			nextCursor = last.GSI1SK
		case gsi2SKField:
			nextCursor = last.GSI2SK
		case "gsi3SK":
			nextCursor = last.GSI3SK
		case "gsi4SK":
			nextCursor = last.GSI4SK
		case "gsi5SK":
			nextCursor = last.GSI5SK
		case "gsi6SK":
			nextCursor = last.GSI6SK
		case "gsi7SK":
			nextCursor = last.GSI7SK
		default:
			nextCursor = last.SK
		}
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      result,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// GetUserTimeline retrieves user's own statuses
func (r *StatusRepository) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	return r.queryStatusesByGSI(ctx, "gsi1", "gsi1PK", fmt.Sprintf("AUTHOR#%s", userID), "gsi1SK", "DESC", opts, "failed to get user timeline")
}

// GetConversationThread retrieves all statuses in a conversation thread
func (r *StatusRepository) GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	return r.queryStatusesByGSI(ctx, "gsi3", "gsi3PK", fmt.Sprintf("CONVERSATION#%s", conversationID), "gsi3SK", "ASC", opts, "failed to get conversation thread")
}

// SearchStatuses searches statuses by query string
func (r *StatusRepository) SearchStatuses(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// This is a basic implementation using scan with filter
	// In production, you'd want to use a search service like OpenSearch
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Filter("Content", "CONTAINS", query).
		Filter("Deleted", "=", false).
		Limit(opts.Limit).
		Scan(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "search by content")
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      result,
		NextCursor: "", // Simple implementation without cursor
		HasMore:    len(result) == opts.Limit,
		Total:      -1,
	}, nil
}

// GetStatusesByHashtag retrieves statuses containing a specific hashtag
// Hashtag must be in canonical format: lowercase, no # prefix (e.g., "test" not "#test" or "Test")
func (r *StatusRepository) GetStatusesByHashtag(ctx context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// Validate and set limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	// Enforce canonical format: lowercase, no # prefix
	// Reject invalid formats to prevent ambiguity
	if hashtag == "" {
		return nil, fmt.Errorf("hashtag cannot be empty")
	}
	if strings.HasPrefix(hashtag, "#") {
		return nil, fmt.Errorf("hashtag must not include # prefix (got: %q)", hashtag)
	}
	if hashtag != strings.ToLower(hashtag) {
		return nil, fmt.Errorf("hashtag must be lowercase (got: %q)", hashtag)
	}

	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi5").
		Where("gsi5PK", "=", fmt.Sprintf("HASHTAG#%s", hashtag)).
		OrderBy("gsi5SK", "DESC").
		Limit(limit).
		All(&statuses)
	if err != nil {
		// Check if it's a NotFound error (no results) - return empty instead of error
		if errors.IsNotFound(err) {
			r.logger.Debug("GetStatusesByHashtag: no results found",
				zap.String("hashtag", hashtag))
			return &interfaces.PaginatedResult[*models.Status]{
				Items:      []*models.Status{},
				NextCursor: "",
				HasMore:    false,
				Total:      0,
			}, nil
		}
		// Log the full error with context for debugging
		r.logger.Error("GetStatusesByHashtag query failed",
			zap.String("hashtag", hashtag),
			zap.String("gsi5pk", fmt.Sprintf("HASHTAG#%s", hashtag)),
			zap.Int("limit", limit),
			zap.Error(err))
		// Wrap with original error for better debugging
		return nil, fmt.Errorf("failed to query statuses by hashtag %q: %w", hashtag, err)
	}

	// Log if we got results
	r.logger.Debug("GetStatusesByHashtag query succeeded",
		zap.String("hashtag", hashtag),
		zap.Int("result_count", len(statuses)),
		zap.Int("limit", limit))

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      result,
		NextCursor: "", // Simple implementation without cursor
		HasMore:    len(result) == limit,
		Total:      -1,
	}, nil
}

// GetTrendingStatuses retrieves trending statuses
func (r *StatusRepository) GetTrendingStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// Simple implementation: get recent public statuses sorted by engagement
	// In production, you'd want a more sophisticated trending algorithm
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi2").
		Where("gsi2PK", "=", "PUBLIC_TIMELINE").
		Filter("LikeCount", ">", 0). // Only statuses with likes
		OrderBy("gsi2SK", "DESC").
		Limit(opts.Limit).
		All(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "get trending")
	}

	// Convert to pointer slice and sort by engagement score
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      result,
		NextCursor: "", // Simple implementation without cursor
		HasMore:    len(result) == opts.Limit,
		Total:      -1,
	}, nil
}

// LikeStatus likes a status for a user
func (r *StatusRepository) LikeStatus(ctx context.Context, userID, statusID string) error {
	return r.createEngagementAndIncrement(ctx, userID, statusID, "like", "LikeCount")
}

// ReblogStatus reblogs a status for a user
func (r *StatusRepository) ReblogStatus(ctx context.Context, userID, statusID, _ string) error {
	return r.createEngagementAndIncrement(ctx, userID, statusID, "boost", "ReblogCount")
}

// createEngagementAndIncrement creates an engagement record and atomically increments the count
func (r *StatusRepository) createEngagementAndIncrement(ctx context.Context, userID, statusID, engagementType, countField string) error {
	// Create an engagement record using the existing StatusEngagement model
	now := time.Now()
	engagement := &models.StatusEngagement{
		PK:             fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID),
		SK:             fmt.Sprintf("%s#%d#%s", engagementType, now.UnixNano(), userID),
		StatusID:       statusID,
		EngagementType: engagementType,
		UserID:         userID,
		EngagedAt:      now,
		TTL:            now.AddDate(0, 0, 7).Unix(), // 7 day TTL
	}

	err := r.db.WithContext(ctx).Model(engagement).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, engagementType, statusID)
	}

	// Atomically increment count using UpdateBuilder
	pk := fmt.Sprintf("status#%s", statusID)
	sk := fmt.Sprintf("status#%s", statusID)

	err = r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder().
		Add(countField, 1).
		Execute()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	return nil
}

// removeEngagement removes an engagement (like or reblog) for a user and updates the status count
func (r *StatusRepository) removeEngagement(ctx context.Context, userID, statusID, engagementType, actionName, counterField string) error {
	// Find and delete the engagement record - need to scan since we don't know the exact timestamp
	var engagements []models.StatusEngagement
	err := r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Where("PK", "=", fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)).
		Filter("EngagementType", "=", engagementType).
		Filter("UserID", "=", userID).
		All(&engagements)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, actionName, "find engagement")
	}

	// Delete the first matching record (there should only be one)
	if err := common.ValidateSliceNotEmpty("engagements", engagements); err == nil {
		err = r.db.WithContext(ctx).Model(&engagements[0]).Delete()
		if err != nil {
			return ErrorHandler.HandleDeleteError(err, actionName, statusID)
		}
	}

	// Atomically decrement the counter using UpdateBuilder
	// Only decrement if engagement was found and deleted
	if err := common.ValidateSliceNotEmpty("engagements", engagements); err == nil {
		pk := fmt.Sprintf("status#%s", statusID)
		sk := fmt.Sprintf("status#%s", statusID)

		err = r.db.WithContext(ctx).Model(&models.Status{}).
			Where("PK", "=", pk).
			Where("SK", "=", sk).
			UpdateBuilder().
			Add(counterField, -1).
			Condition(counterField, ">", 0). // Only decrement if count > 0
			Execute()
		if err != nil {
			return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
		}
	}

	return nil
}

// UnlikeStatus unlikes a status for a user
func (r *StatusRepository) UnlikeStatus(ctx context.Context, userID, statusID string) error {
	return r.removeEngagement(ctx, userID, statusID, "like", "like", "LikeCount")
}

// ReblogStatus reblogs a status for a user

// UnreblogStatus unreblogs a status for a user
func (r *StatusRepository) UnreblogStatus(ctx context.Context, userID, statusID string) error {
	return r.removeEngagement(ctx, userID, statusID, "boost", "reblog", "ReblogCount")
}

// BookmarkStatus bookmarks a status for a user
func (r *StatusRepository) BookmarkStatus(ctx context.Context, userID, statusID string) error {
	repo := r.getBookmarkRepository()
	if repo == nil {
		return ErrorHandler.HandleCreateError(fmt.Errorf("bookmark repository not configured"), EntityBookmark, statusID)
	}
	_, err := repo.CreateBookmark(ctx, userID, statusID)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityBookmark, statusID)
	}
	return nil
}

// UnbookmarkStatus unbookmarks a status for a user
func (r *StatusRepository) UnbookmarkStatus(ctx context.Context, userID, statusID string) error {
	repo := r.getBookmarkRepository()
	if repo == nil {
		return ErrorHandler.HandleDeleteError(fmt.Errorf("bookmark repository not configured"), EntityBookmark, statusID)
	}
	if err := repo.DeleteBookmark(ctx, userID, statusID); err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityBookmark, statusID)
	}
	return nil
}

// UnflagStatus unflags a previously flagged status
func (r *StatusRepository) UnflagStatus(ctx context.Context, statusID string) error {
	pk := fmt.Sprintf("status#%s", statusID)
	sk := fmt.Sprintf("status#%s", statusID)

	// Use UpdateBuilder to update only Flagged field, avoiding Note field corruption
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder().
		Set("Flagged", false).
		Execute()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	return nil
}

// GetStatusCounts gets engagement counts for a status
func (r *StatusRepository) GetStatusCounts(ctx context.Context, statusID string) (likes, reblogs, replies int, err error) {
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return 0, 0, 0, ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	return status.LikeCount, status.ReblogCount, status.ReplyCount, nil
}

// GetStatusContext gets ancestors and descendants of a status
func (r *StatusRepository) GetStatusContext(ctx context.Context, statusID string) (ancestors, descendants []*models.Status, err error) {
	// Get the status first to find its in_reply_to
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return nil, nil, ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	// Get ancestors by following the reply chain
	ancestors = []*models.Status{}
	currentStatus := status
	for currentStatus.InReplyToID != "" {
		parent, err := r.GetStatus(ctx, currentStatus.InReplyToID)
		if err != nil {
			break // Stop if parent not found
		}
		ancestors = append([]*models.Status{parent}, ancestors...) // Prepend to maintain order
		currentStatus = parent
	}

	// Get descendants (replies)
	replies, err := r.GetReplies(ctx, statusID, interfaces.PaginationOptions{Limit: 100})
	if err != nil {
		return ancestors, nil, ErrorHandler.HandleQueryError(err, "replies", "get for context")
	}

	return ancestors, replies.Items, nil
}

// GetStatusEngagement gets user's engagement state with a status
func (r *StatusRepository) GetStatusEngagement(ctx context.Context, statusID, userID string) (liked, reblogged, bookmarked bool, err error) {
	// Check for like
	var likeEngagements []models.StatusEngagement
	err = r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Where("PK", "=", fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)).
		Filter("EngagementType", "=", "like").
		Filter("UserID", "=", userID).
		All(&likeEngagements)
	liked = (err == nil && len(likeEngagements) > 0)

	// Check for reblog/boost
	var boostEngagements []models.StatusEngagement
	err = r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Where("PK", "=", fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)).
		Filter("EngagementType", "=", "boost").
		Filter("UserID", "=", userID).
		All(&boostEngagements)
	reblogged = (err == nil && len(boostEngagements) > 0)

	// Check for bookmark via bookmark repository
	if repo := r.getBookmarkRepository(); repo != nil {
		bookmarked, err = repo.IsBookmarked(ctx, userID, statusID)
	} else {
		err = fmt.Errorf("bookmark repository not configured")
	}

	return liked, reblogged, bookmarked, err
}

// GetStatusesByIDs gets multiple statuses by their IDs using batched BatchGetItem calls to minimize Dynamo round-trips.
func (r *StatusRepository) GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error) {
	if len(statusIDs) == 0 {
		return []*models.Status{}, nil
	}

	keySet := make(map[string]struct{}, len(statusIDs))
	keys := make([]struct {
		PK string
		SK string
	}, 0, len(statusIDs))

	for _, id := range statusIDs {
		if id == "" {
			continue
		}
		if _, exists := keySet[id]; exists {
			continue
		}
		keySet[id] = struct{}{}

		pk := fmt.Sprintf("status#%s", id)
		keys = append(keys, struct {
			PK string
			SK string
		}{PK: pk, SK: pk})
	}

	records, err := r.BatchGet(ctx, keys)
	if err != nil {
		return nil, err
	}

	statusMap := make(map[string]*models.Status, len(records))
	for _, status := range records {
		if status == nil {
			continue
		}
		statusMap[status.StatusID] = status
	}

	ordered := make([]*models.Status, 0, len(statusMap))
	for _, id := range statusIDs {
		if status, ok := statusMap[id]; ok {
			ordered = append(ordered, status)
		}
	}

	return ordered, nil
}

// GetPublicTimeline retrieves the public timeline with pagination
func (r *StatusRepository) GetPublicTimeline(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	statuses, err := r.queryPublicTimelineDirect(ctx, opts)
	if err != nil {
		r.logger.Error("failed to query public timeline",
			zap.Error(err),
			zap.Int("limit", opts.Limit))
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "get public timeline")
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      statuses,
		NextCursor: "",
		HasMore:    len(statuses) == opts.Limit,
		Total:      -1,
	}, nil
}

// GetReplies retrieves replies to a status with pagination
func (r *StatusRepository) GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi4").
		Where("gsi4PK", "=", fmt.Sprintf("REPLIES#%s", parentStatusID)).
		OrderBy("gsi4SK", "ASC"). // Chronological order for replies
		Limit(opts.Limit).
		All(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "replies", "get for status")
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      result,
		NextCursor: "", // Simple implementation without cursor
		HasMore:    len(result) == opts.Limit,
		Total:      -1,
	}, nil
}

// GetFlaggedStatuses retrieves flagged statuses with pagination using GSI6
func (r *StatusRepository) GetFlaggedStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// Use GSI6 for efficient flagged content queries
	return r.queryStatusesByGSI(ctx, "gsi6", "gsi6PK", "FLAGGED_CONTENT", "gsi6SK", "DESC", opts, "failed to get flagged statuses")
}

// FlagStatus marks a status as flagged for moderation
func (r *StatusRepository) FlagStatus(ctx context.Context, statusID, _ string, _ string) error {
	pk := fmt.Sprintf("status#%s", statusID)
	sk := fmt.Sprintf("status#%s", statusID)

	// Use UpdateBuilder to update only Flagged field, avoiding Note field corruption
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder().
		Set("Flagged", true).
		Execute()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	// In a full implementation, you'd also create a moderation report record
	// with the reason and reportedBy information

	return nil
}

// Helper function to extract status ID from URL
