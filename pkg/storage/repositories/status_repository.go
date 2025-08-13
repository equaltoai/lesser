package repositories

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// StatusRepository implements status operations using DynamORM
type StatusRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewStatusRepository creates a new status repository
func NewStatusRepository(db core.DB, tableName string, logger *zap.Logger) *StatusRepository {
	return &StatusRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateStatus creates a new status
func (r *StatusRepository) CreateStatus(ctx context.Context, status *models.Status) error {
	err := r.db.WithContext(ctx).Model(status).Create()
	if err != nil {
		return fmt.Errorf("failed to create status: %w", err)
	}

	return nil
}

// GetStatus retrieves a status by ID
func (r *StatusRepository) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	var status models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", fmt.Sprintf("status#%s", statusID)).
		Where("SK", "=", fmt.Sprintf("status#%s", statusID)).
		First(&status)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("status not found: %s", statusID)
		}
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	return &status, nil
}

// UpdateStatus updates an existing status
func (r *StatusRepository) UpdateStatus(ctx context.Context, status *models.Status) error {
	err := r.db.WithContext(ctx).Model(status).Update()
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// DeleteStatus marks a status as deleted
func (r *StatusRepository) DeleteStatus(ctx context.Context, statusID string) error {
	var status models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", fmt.Sprintf("status#%s", statusID)).
		Where("SK", "=", fmt.Sprintf("status#%s", statusID)).
		First(&status)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("status not found: %s", statusID)
		}
		return fmt.Errorf("failed to find status for deletion: %w", err)
	}

	// Mark as deleted instead of hard delete
	now := time.Now()
	status.Deleted = true
	status.DeletedAt = &now

	err = r.db.WithContext(ctx).Model(&status).Update()
	if err != nil {
		return fmt.Errorf("failed to mark status as deleted: %w", err)
	}

	return nil
}

// CountStatusesByAuthor counts the total number of statuses by an author
func (r *StatusRepository) CountStatusesByAuthor(ctx context.Context, authorID string) (int, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("author-timeline-index").
		Where("GSI1PK", "=", fmt.Sprintf("AUTHOR#%s", authorID)).
		All(&statuses)
	if err != nil {
		return 0, fmt.Errorf("failed to count statuses by author: %w", err)
	}

	return len(statuses), nil
}

// CountReplies counts the number of replies to a status
func (r *StatusRepository) CountReplies(ctx context.Context, statusID string) (int, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("replies-index").
		Where("GSI4PK", "=", fmt.Sprintf("REPLIES#%s", statusID)).
		All(&statuses)
	if err != nil {
		return 0, fmt.Errorf("failed to count replies: %w", err)
	}

	return len(statuses), nil
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
			return fmt.Errorf("status not found: %s", statusID)
		}
		return fmt.Errorf("failed to find status for metrics update: %w", err)
	}

	// Update metrics
	status.LikeCount = likes
	status.ReblogCount = reblogs
	status.ReplyCount = replies
	status.QuoteCount = quotes

	err = r.db.WithContext(ctx).Model(&status).Update()
	if err != nil {
		return fmt.Errorf("failed to update engagement metrics: %w", err)
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
		return 0, fmt.Errorf("failed to count total statuses: %w", err)
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
		return nil, fmt.Errorf("failed to scan local statuses: %w", err)
	}

	return statuses, nil
}

// applyRemoteFiltering filters for remote statuses only (requires post-processing)
func (r *StatusRepository) applyRemoteFiltering(_ context.Context, query core.Query, _ *StatusFilter, limit int) ([]models.Status, error) {
	var statuses []models.Status
	domain := r.extractDomainFromEnv()
	if domain == "" {
		query = query.Limit(limit)
		err := query.Scan(&statuses)
		if err != nil {
			return nil, fmt.Errorf("failed to scan statuses: %w", err)
		}
		return statuses, nil
	}

	// Note: DynamoDB doesn't have a direct "NOT CONTAINS" operation
	// We'll need to use scan and post-process filtering
	err := query.Scan(&statuses)
	if err != nil {
		return nil, fmt.Errorf("failed to scan statuses: %w", err)
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
		return nil, fmt.Errorf("failed to get filtered statuses: %w", err)
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
				return 0, fmt.Errorf("failed to scan statuses for count: %w", err)
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
			return 0, fmt.Errorf("failed to count filtered statuses: %w", err)
		}

		r.logger.Debug("counted filtered statuses for admin", zap.Int64("count", count))
		return count, nil
	}

	return count, nil
}

// extractDomainFromEnv extracts the local domain from environment
func (r *StatusRepository) extractDomainFromEnv() string {
	// Get domain from environment variable
	domain := os.Getenv("DOMAIN_NAME")
	if domain == "" {
		domain = os.Getenv("INSTANCE_DOMAIN")
	}
	return domain
}

// GetStatusByURL retrieves a status by its URL
func (r *StatusRepository) GetStatusByURL(ctx context.Context, url string) (*models.Status, error) {
	// For now, we'll use a scan to find status by URL - in production you might want a GSI
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).Scan(&statuses)
	if err != nil {
		return nil, fmt.Errorf("failed to scan statuses: %w", err)
	}

	// Find status with matching URL in Note
	for _, status := range statuses {
		if status.Note != nil && status.Note.ID == url {
			return &status, nil
		}
	}

	return nil, fmt.Errorf("status not found with URL: %s", url)
}

// GetHomeTimeline retrieves home timeline for a user
func (r *StatusRepository) GetHomeTimeline(ctx context.Context, _ string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// This would typically require following relationships to build home timeline
	// For now, we'll return public timeline as a placeholder
	return r.GetPublicTimeline(ctx, opts)
}

// GetUserTimeline retrieves user's own statuses
func (r *StatusRepository) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("author-timeline-index").
		Where("GSI1PK", "=", fmt.Sprintf("AUTHOR#%s", userID)).
		OrderBy("GSI1SK", "DESC").
		Limit(opts.Limit).
		All(&statuses)
	if err != nil {
		return nil, fmt.Errorf("failed to get user timeline: %w", err)
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

// GetConversationThread retrieves all statuses in a conversation thread
func (r *StatusRepository) GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("conversation-index").
		Where("GSI3PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
		OrderBy("GSI3SK", "ASC").
		Limit(opts.Limit).
		All(&statuses)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation thread: %w", err)
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
		return nil, fmt.Errorf("failed to search statuses: %w", err)
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
		return nil, fmt.Errorf("failed to get statuses by hashtag: %w", err)
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
		return nil, fmt.Errorf("failed to get trending statuses: %w", err)
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
		return fmt.Errorf("failed to create like: %w", err)
	}

	// Update status like count
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get status for like update: %w", err)
	}

	status.LikeCount++
	err = r.UpdateStatus(ctx, status)
	if err != nil {
		return fmt.Errorf("failed to update status like count: %w", err)
	}

	return nil
}

// UnlikeStatus unlikes a status for a user
func (r *StatusRepository) UnlikeStatus(ctx context.Context, userID, statusID string) error {
	// Find and delete the like record - need to scan since we don't know the exact timestamp
	var engagements []models.StatusEngagement
	err := r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Where("PK", "=", fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)).
		Filter("EngagementType", "=", "like").
		Filter("UserID", "=", userID).
		All(&engagements)
	if err != nil {
		return fmt.Errorf("failed to find like record: %w", err)
	}

	// Delete the first matching record (there should only be one)
	if len(engagements) > 0 {
		err = r.db.WithContext(ctx).Model(&engagements[0]).Delete()
		if err != nil {
			return fmt.Errorf("failed to delete like: %w", err)
		}
	}

	// Update status like count
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get status for unlike update: %w", err)
	}

	if status.LikeCount > 0 {
		status.LikeCount--
	}
	err = r.UpdateStatus(ctx, status)
	if err != nil {
		return fmt.Errorf("failed to update status like count: %w", err)
	}

	return nil
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
		return fmt.Errorf("failed to create reblog: %w", err)
	}

	// Update status reblog count
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get status for reblog update: %w", err)
	}

	status.ReblogCount++
	err = r.UpdateStatus(ctx, status)
	if err != nil {
		return fmt.Errorf("failed to update status reblog count: %w", err)
	}

	return nil
}

// UnreblogStatus unreblogs a status for a user
func (r *StatusRepository) UnreblogStatus(ctx context.Context, userID, statusID string) error {
	// Find and delete the reblog record - need to scan since we don't know the exact timestamp
	var engagements []models.StatusEngagement
	err := r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Where("PK", "=", fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)).
		Filter("EngagementType", "=", "boost").
		Filter("UserID", "=", userID).
		All(&engagements)
	if err != nil {
		return fmt.Errorf("failed to find reblog record: %w", err)
	}

	// Delete the first matching record (there should only be one)
	if len(engagements) > 0 {
		err = r.db.WithContext(ctx).Model(&engagements[0]).Delete()
		if err != nil {
			return fmt.Errorf("failed to delete reblog: %w", err)
		}
	}

	// Update status reblog count
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get status for unreblog update: %w", err)
	}

	if status.ReblogCount > 0 {
		status.ReblogCount--
	}
	err = r.UpdateStatus(ctx, status)
	if err != nil {
		return fmt.Errorf("failed to update status reblog count: %w", err)
	}

	return nil
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
	bookmark.UpdateKeys()

	err := r.db.WithContext(ctx).Model(bookmark).Create()
	if err != nil {
		return fmt.Errorf("failed to create bookmark: %w", err)
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
		return fmt.Errorf("failed to find bookmark: %w", err)
	}

	// Delete the first matching record (there should only be one)
	if len(bookmarks) > 0 {
		err = r.db.WithContext(ctx).Model(&bookmarks[0]).Delete()
		if err != nil {
			return fmt.Errorf("failed to delete bookmark: %w", err)
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
			return fmt.Errorf("status not found: %s", statusID)
		}
		return fmt.Errorf("failed to find status for unflagging: %w", err)
	}

	status.Flagged = false

	err = r.db.WithContext(ctx).Model(&status).Update()
	if err != nil {
		return fmt.Errorf("failed to unflag status: %w", err)
	}

	return nil
}

// GetStatusCounts gets engagement counts for a status
func (r *StatusRepository) GetStatusCounts(ctx context.Context, statusID string) (likes, reblogs, replies int, err error) {
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get status: %w", err)
	}

	return status.LikeCount, status.ReblogCount, status.ReplyCount, nil
}

// GetStatusContext gets ancestors and descendants of a status
func (r *StatusRepository) GetStatusContext(ctx context.Context, statusID string) (ancestors, descendants []*models.Status, err error) {
	// Get the status first to find its in_reply_to
	status, err := r.GetStatus(ctx, statusID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get status: %w", err)
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
		return ancestors, nil, fmt.Errorf("failed to get replies: %w", err)
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
		return nil, fmt.Errorf("failed to get public timeline: %w", err)
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
		return nil, fmt.Errorf("failed to get replies: %w", err)
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

// GetFlaggedStatuses retrieves flagged statuses with pagination
func (r *StatusRepository) GetFlaggedStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// This would require a GSI for flagged statuses in a real implementation
	// For now, we'll scan the table (not efficient for production)
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Filter("Flagged", "=", true).
		Limit(opts.Limit).
		Scan(&statuses)
	if err != nil {
		return nil, fmt.Errorf("failed to get flagged statuses: %w", err)
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

// FlagStatus marks a status as flagged for moderation
func (r *StatusRepository) FlagStatus(ctx context.Context, statusID, _ string, _ string) error {
	var status models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", fmt.Sprintf("status#%s", statusID)).
		Where("SK", "=", fmt.Sprintf("status#%s", statusID)).
		First(&status)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("status not found: %s", statusID)
		}
		return fmt.Errorf("failed to find status for flagging: %w", err)
	}

	status.Flagged = true

	err = r.db.WithContext(ctx).Model(&status).Update()
	if err != nil {
		return fmt.Errorf("failed to flag status: %w", err)
	}

	// In a full implementation, you'd also create a moderation report record
	// with the reason and reportedBy information

	return nil
}

// Helper function to extract status ID from URL
