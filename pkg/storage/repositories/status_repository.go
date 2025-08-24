package repositories

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// StatusRepository implements status operations using DynamORM with EnhancedBaseRepository
type StatusRepository struct {
	*EnhancedBaseRepository[*models.Status]
	relationshipRepo interface{} // Temporarily use interface to avoid circular dependency
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

// CreateStatus creates a new status using enhanced validation and event emission
func (r *StatusRepository) CreateStatus(ctx context.Context, status *models.Status) error {
	// Use enhanced validation and creation with automatic event emission
	return r.ValidateAndCreate(ctx, status)
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
	// Use enhanced validation and update with automatic event emission and cache invalidation
	return r.ValidateAndUpdate(ctx, status)
}

// DeleteStatus marks a status as deleted using BaseRepository
func (r *StatusRepository) DeleteStatus(ctx context.Context, statusID string) error {
	// Get the status first
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return err
	}

	// Mark as deleted instead of hard delete
	now := time.Now()
	status.Deleted = true
	status.DeletedAt = &now

	// Update using BaseRepository
	return r.Update(ctx, status)
}

// CountStatusesByAuthor counts the total number of statuses by an author
func (r *StatusRepository) CountStatusesByAuthor(ctx context.Context, authorID string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("author-timeline-index").
		Where("GSI1PK", "=", fmt.Sprintf("AUTHOR#%s", authorID)).
		Count()
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, EntityStatus, "count by author")
	}

	return int(count), nil
}

// CountReplies counts the number of replies to a status
func (r *StatusRepository) CountReplies(ctx context.Context, statusID string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("replies-index").
		Where("GSI4PK", "=", fmt.Sprintf("REPLIES#%s", statusID)).
		Count()
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "reply", "count")
	}

	return int(count), nil
}

// UpdateEngagementMetrics updates the cached engagement metrics for a status
func (r *StatusRepository) UpdateEngagementMetrics(ctx context.Context, statusID string, likes, reblogs, replies, quotes int) error {
	var status models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", fmt.Sprintf("status#%s", statusID)).
		Where("SK", "=", fmt.Sprintf("status#%s", statusID)).
		First(&status)
	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
		}
		return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	// Update metrics
	status.LikeCount = likes
	status.ReblogCount = reblogs
	status.ReplyCount = replies
	status.QuoteCount = quotes

	err = r.db.WithContext(ctx).Model(&status).Update()
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

// StatusFilter represents filtering criteria for admin status listing
type StatusFilter struct {
	Local      *bool      // Filter by local vs remote statuses
	Remote     *bool      // Filter by remote statuses only
	ByDomain   string     // Filter by specific domain
	Visibility string     // Filter by visibility (public, unlisted, private, direct)
	Flagged    *bool      // Filter by flagged status
	Reported   *bool      // Filter by reported status
	WithMedia  *bool      // Filter by presence of media attachments
	Sensitive  *bool      // Filter by sensitive flag
	MinDate    *time.Time // Filter by minimum creation date
	MaxDate    *time.Time // Filter by maximum creation date
}

// ListStatusesForAdmin retrieves statuses with comprehensive admin filtering
func (r *StatusRepository) ListStatusesForAdmin(ctx context.Context, filter *StatusFilter, limit int, cursor string) ([]*models.Status, string, error) {
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
func (r *StatusRepository) applyDomainFiltering(ctx context.Context, query core.Query, filter *StatusFilter, limit int) ([]models.Status, error) {
	if filter.Local != nil && *filter.Local {
		return r.applyLocalFiltering(query, filter, limit)
	}

	if filter.Remote != nil && *filter.Remote {
		return r.applyRemoteFiltering(ctx, query, filter, limit)
	}

	return r.applyStandardFiltering(query, filter, limit)
}

// applyLocalFiltering filters for local statuses only
func (r *StatusRepository) applyLocalFiltering(query core.Query, filter *StatusFilter, limit int) ([]models.Status, error) {
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
func (r *StatusRepository) applyRemoteFiltering(_ context.Context, query core.Query, _ *StatusFilter, limit int) ([]models.Status, error) {
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
		Index("url-index").
		Where("GSI7PK", "=", "URL#"+normalizedURL).
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
func (r *StatusRepository) applyStandardFiltering(query core.Query, filter *StatusFilter, limit int) ([]models.Status, error) {
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
func (r *StatusRepository) applyContentFilters(query core.Query, filter *StatusFilter) core.Query {
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
func (r *StatusRepository) applyDateFilters(query core.Query, filter *StatusFilter) core.Query {
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
func (r *StatusRepository) CountStatusesForAdmin(ctx context.Context, filter *StatusFilter) (int64, error) {
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
		if cfg.Domain == "" || cfg.Domain == "localhost" {
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
		Index("url-index").
		Where("GSI7PK", "=", "URL#"+normalizedURL).
		Scan(&statuses)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "query by URL")
	}

	// Find exact match by checking the Note.ID field
	for _, status := range statuses {
		if status.Note != nil && status.Note.ID == url {
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

	// TODO: Implement relationship repository integration
	// Get list of users that this user follows
	var followingUsernames []string // Temporarily empty for compilation
	if r.relationshipRepo == nil {
		// Fallback to public timeline when relationship repo not available
		return r.GetPublicTimeline(ctx, opts)
	}

	// If user follows no one, return empty timeline (not public timeline)
	if err := common.ValidateSliceNotEmpty("following_usernames", followingUsernames); err != nil {
		r.logger.Debug("user follows no accounts, returning empty home timeline",
			zap.String("user_id", userID))
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	// Get statuses from all followed users
	// Note: This is a simplified implementation. In production, you might want to:
	// 1. Use a pre-computed home timeline cache
	// 2. Implement pagination across multiple author queries
	// 3. Use a timeline ranking algorithm
	var allStatuses []models.Status

	// Query statuses for each followed user using the author-timeline-index
	for _, username := range followingUsernames {
		var userStatuses []models.Status
		err := r.db.WithContext(ctx).Model(&models.Status{}).
			Index("author-timeline-index").
			Where("GSI1PK", "=", fmt.Sprintf("AUTHOR#%s", username)).
			OrderBy("GSI1SK", "DESC").
			Limit(20). // Limit per user to avoid overwhelming queries
			All(&userStatuses)

		if err != nil {
			r.logger.Error("failed to get statuses for followed user",
				zap.String("user_id", userID),
				zap.String("followed_user", username),
				zap.Error(err))
			continue // Skip this user on error
		}

		allStatuses = append(allStatuses, userStatuses...)
	}

	// Sort all statuses by published time (most recent first)
	sort.Slice(allStatuses, func(i, j int) bool {
		return allStatuses[i].PublishedAt.After(allStatuses[j].PublishedAt)
	})

	// Apply pagination limits
	limit := opts.Limit
	// Validate limit using centralized validation
	if err := common.ValidateQueryLimit(limit, 40, "timeline"); err != nil {
		limit = 20
	}

	// Take only the number needed for this page
	var pageStatuses []models.Status
	if len(allStatuses) > limit {
		pageStatuses = allStatuses[:limit]
	} else {
		pageStatuses = allStatuses
	}

	// Convert to pointer slice
	statusPtrs := make([]*models.Status, len(pageStatuses))
	for i := range pageStatuses {
		statusPtrs[i] = &pageStatuses[i]
	}

	// Generate next cursor if there are more items
	nextCursor := ""
	hasMore := len(allStatuses) > limit
	if hasMore && len(statusPtrs) > 0 {
		lastStatus := statusPtrs[len(statusPtrs)-1]
		nextCursor = lastStatus.StatusID
	}

	r.logger.Debug("successfully built home timeline",
		zap.String("user_id", userID),
		zap.Int("following_count", len(followingUsernames)),
		zap.Int("total_statuses_found", len(allStatuses)),
		zap.Int("page_statuses", len(statusPtrs)))

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      statusPtrs,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      int64(len(allStatuses)),
	}, nil
}

// queryStatusesByGSI is a consolidated helper for GSI-based status queries
func (r *StatusRepository) queryStatusesByGSI(ctx context.Context, indexName, gsiPKField, gsiPKValue, gsiSKField, orderDirection string, opts interfaces.PaginationOptions, errorMsg string) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status
	query := r.db.WithContext(ctx).Model(&models.Status{}).
		Index(indexName).
		Where(gsiPKField, "=", gsiPKValue).
		OrderBy(gsiSKField, orderDirection).
		Limit(opts.Limit)

	// Add cursor-based pagination if provided
	if opts.Cursor != "" {
		operator := "<"
		if orderDirection == "ASC" {
			operator = ">"
		}
		query = query.Where(gsiSKField, operator, opts.Cursor)
	}

	err := query.All(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, errorMsg)
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

// GetUserTimeline retrieves user's own statuses
func (r *StatusRepository) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	return r.queryStatusesByGSI(ctx, "author-timeline-index", "GSI1PK", fmt.Sprintf("AUTHOR#%s", userID), "GSI1SK", "DESC", opts, "failed to get user timeline")
}

// GetConversationThread retrieves all statuses in a conversation thread
func (r *StatusRepository) GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	return r.queryStatusesByGSI(ctx, "conversation-index", "GSI3PK", fmt.Sprintf("CONVERSATION#%s", conversationID), "GSI3SK", "ASC", opts, "failed to get conversation thread")
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
func (r *StatusRepository) GetStatusesByHashtag(ctx context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("hashtag-index").
		Where("GSI5PK", "=", fmt.Sprintf("HASHTAG#%s", strings.ToLower(hashtag))).
		OrderBy("GSI5SK", "DESC").
		Limit(opts.Limit).
		All(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "get by hashtag")
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

// GetTrendingStatuses retrieves trending statuses
func (r *StatusRepository) GetTrendingStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// Simple implementation: get recent public statuses sorted by engagement
	// In production, you'd want a more sophisticated trending algorithm
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("public-timeline-index").
		Where("GSI2PK", "=", "PUBLIC_TIMELINE").
		Filter("LikeCount", ">", 0). // Only statuses with likes
		OrderBy("GSI2SK", "DESC").
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
	// Create a like record using the existing StatusEngagement model
	now := time.Now()
	like := &models.StatusEngagement{
		PK:             fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID),
		SK:             fmt.Sprintf("like#%d#%s", now.Unix(), userID),
		StatusID:       statusID,
		EngagementType: "like",
		UserID:         userID,
		EngagedAt:      now,
		TTL:            now.AddDate(0, 0, 7).Unix(), // 7 day TTL
	}

	err := r.db.WithContext(ctx).Model(like).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "like", statusID)
	}

	// Update status like count
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	status.LikeCount++
	err = r.UpdateStatus(ctx, status)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	return nil
}

// removeEngagement removes an engagement (like or reblog) for a user and updates the status count
func (r *StatusRepository) removeEngagement(ctx context.Context, userID, statusID, engagementType, actionName string, updateCount func(*models.Status)) error {
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

	// Update status count
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	updateCount(status)
	err = r.UpdateStatus(ctx, status)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	return nil
}

// UnlikeStatus unlikes a status for a user
func (r *StatusRepository) UnlikeStatus(ctx context.Context, userID, statusID string) error {
	return r.removeEngagement(ctx, userID, statusID, "like", "like", func(status *models.Status) {
		if status.LikeCount > 0 {
			status.LikeCount--
		}
	})
}

// ReblogStatus reblogs a status for a user
func (r *StatusRepository) ReblogStatus(ctx context.Context, userID, statusID, _ string) error {
	// Create a reblog record using the existing StatusEngagement model
	now := time.Now()
	reblog := &models.StatusEngagement{
		PK:             fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID),
		SK:             fmt.Sprintf("boost#%d#%s", now.Unix(), userID),
		StatusID:       statusID,
		EngagementType: "boost",
		UserID:         userID,
		EngagedAt:      now,
		TTL:            now.AddDate(0, 0, 7).Unix(), // 7 day TTL
	}

	err := r.db.WithContext(ctx).Model(reblog).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "reblog", statusID)
	}

	// Update status reblog count
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	status.ReblogCount++
	err = r.UpdateStatus(ctx, status)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	return nil
}

// UnreblogStatus unreblogs a status for a user
func (r *StatusRepository) UnreblogStatus(ctx context.Context, userID, statusID string) error {
	return r.removeEngagement(ctx, userID, statusID, "boost", "reblog", func(status *models.Status) {
		if status.ReblogCount > 0 {
			status.ReblogCount--
		}
	})
}

// BookmarkStatus bookmarks a status for a user
func (r *StatusRepository) BookmarkStatus(ctx context.Context, userID, statusID string) error {
	// Create a bookmark record using the existing Bookmark model
	now := time.Now()
	bookmark := &models.Bookmark{
		Username:  userID,
		ObjectID:  statusID,
		CreatedAt: now,
		TTL:       0, // No TTL for bookmarks
	}
	_ = bookmark.UpdateKeys() // Ignore error as this is internal model operation

	err := r.db.WithContext(ctx).Model(bookmark).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityBookmark, statusID)
	}

	return nil
}

// UnbookmarkStatus unbookmarks a status for a user
func (r *StatusRepository) UnbookmarkStatus(ctx context.Context, userID, statusID string) error {
	// Find and delete the bookmark record
	var bookmarks []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", fmt.Sprintf("BOOKMARK#%s", userID)).
		Filter("ObjectID", "=", statusID).
		All(&bookmarks)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityBookmark, "find for removal")
	}

	// Delete the first matching record (there should only be one)
	if err := common.ValidateSliceNotEmpty("bookmarks", bookmarks); err == nil {
		err = r.db.WithContext(ctx).Model(&bookmarks[0]).Delete()
		if err != nil {
			return ErrorHandler.HandleDeleteError(err, EntityBookmark, statusID)
		}
	}

	return nil
}

// UnflagStatus unflags a previously flagged status
func (r *StatusRepository) UnflagStatus(ctx context.Context, statusID string) error {
	var status models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", fmt.Sprintf("status#%s", statusID)).
		Where("SK", "=", fmt.Sprintf("status#%s", statusID)).
		First(&status)
	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
		}
		return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	status.Flagged = false

	err = r.db.WithContext(ctx).Model(&status).Update()
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

	// Check for bookmark
	var bookmarks []models.Bookmark
	err = r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", fmt.Sprintf("BOOKMARK#%s", userID)).
		Filter("ObjectID", "=", statusID).
		All(&bookmarks)
	bookmarked = (err == nil && len(bookmarks) > 0)

	return liked, reblogged, bookmarked, nil
}

// GetStatusesByIDs gets multiple statuses by their IDs
func (r *StatusRepository) GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error) {
	result := make([]*models.Status, 0, len(statusIDs))

	for _, statusID := range statusIDs {
		status, err := r.GetStatus(ctx, statusID)
		if err != nil {
			// Skip statuses that can't be found rather than failing entirely
			r.logger.Warn("failed to get status by ID", zap.String("statusID", statusID), zap.Error(err))
			continue
		}
		result = append(result, status)
	}

	return result, nil
}

// GetPublicTimeline retrieves the public timeline with pagination
func (r *StatusRepository) GetPublicTimeline(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("public-timeline-index").
		Where("GSI2PK", "=", "PUBLIC_TIMELINE").
		OrderBy("GSI2SK", "DESC").
		Limit(opts.Limit).
		All(&statuses)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "get public timeline")
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

// GetReplies retrieves replies to a status with pagination
func (r *StatusRepository) GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("replies-index").
		Where("GSI4PK", "=", fmt.Sprintf("REPLIES#%s", parentStatusID)).
		OrderBy("GSI4SK", "ASC"). // Chronological order for replies
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

// GetFlaggedStatuses retrieves flagged statuses with pagination using GSI6 (flagged-content-index)
func (r *StatusRepository) GetFlaggedStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// Use GSI6 (flagged-content-index) for efficient flagged content queries
	return r.queryStatusesByGSI(ctx, "flagged-content-index", "GSI6PK", "FLAGGED_CONTENT", "GSI6SK", "DESC", opts, "failed to get flagged statuses")
}

// FlagStatus marks a status as flagged for moderation
func (r *StatusRepository) FlagStatus(ctx context.Context, statusID, _ string, _ string) error {
	var status models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", fmt.Sprintf("status#%s", statusID)).
		Where("SK", "=", fmt.Sprintf("status#%s", statusID)).
		First(&status)
	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
		}
		return ErrorHandler.HandleGetError(err, EntityStatus, statusID)
	}

	status.Flagged = true

	err = r.db.WithContext(ctx).Model(&status).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
	}

	// In a full implementation, you'd also create a moderation report record
	// with the reason and reportedBy information

	return nil
}

// Helper function to extract status ID from URL
