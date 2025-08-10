package repositories

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
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
func (r *StatusRepository) CreateStatus(ctx context.Context, note *activitypub.Note) (*models.Status, error) {
	status := &models.Status{
		StatusID: extractStatusIDFromURL(note.ID),
		Note:     note,
	}

	err := r.db.WithContext(ctx).Model(status).Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create status: %w", err)
	}

	return status, nil
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

// GetStatusesByAuthor retrieves statuses by author with pagination
func (r *StatusRepository) GetStatusesByAuthor(ctx context.Context, authorID string, limit int, _ string) ([]*models.Status, string, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("author-timeline-index").
		Where("GSI1PK", "=", fmt.Sprintf("AUTHOR#%s", authorID)).
		OrderBy("GSI1SK", "DESC").
		Limit(limit).
		All(&statuses)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get statuses by author: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	// For now, return empty cursor - pagination would need custom implementation
	return result, "", nil
}

// GetPublicTimeline retrieves public statuses with pagination
func (r *StatusRepository) GetPublicTimeline(ctx context.Context, limit int, _ string) ([]*models.Status, string, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("public-timeline-index").
		Where("GSI2PK", "=", "PUBLIC_TIMELINE").
		OrderBy("GSI2SK", "DESC").
		Limit(limit).
		All(&statuses)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get public timeline: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	// For now, return empty cursor - pagination would need custom implementation
	return result, "", nil
}

// GetConversationStatuses retrieves all statuses in a conversation
func (r *StatusRepository) GetConversationStatuses(ctx context.Context, conversationID string, limit int) ([]*models.Status, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("conversation-index").
		Where("GSI3PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
		OrderBy("GSI3SK", "ASC"). // Chronological order for conversations
		Limit(limit).
		All(&statuses)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation statuses: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	return result, nil
}

// GetReplies retrieves replies to a specific status
func (r *StatusRepository) GetReplies(ctx context.Context, statusID string, limit int, _ string) ([]*models.Status, string, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("replies-index").
		Where("GSI4PK", "=", fmt.Sprintf("REPLIES#%s", statusID)).
		OrderBy("GSI4SK", "ASC"). // Chronological order for replies
		Limit(limit).
		All(&statuses)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get replies: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	// For now, return empty cursor - pagination would need custom implementation
	return result, "", nil
}

// GetHashtagTimeline retrieves statuses for a specific hashtag
func (r *StatusRepository) GetHashtagTimeline(ctx context.Context, hashtag string, limit int, _ string) ([]*models.Status, string, error) {
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("hashtag-index").
		Where("GSI5PK", "=", fmt.Sprintf("HASHTAG#%s", hashtag)).
		OrderBy("GSI5SK", "DESC").
		Limit(limit).
		All(&statuses)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get hashtag timeline: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	// For now, return empty cursor - pagination would need custom implementation
	return result, "", nil
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

// FlagStatus flags a status for moderation
func (r *StatusRepository) FlagStatus(ctx context.Context, statusID string) error {
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

// GetFlaggedStatuses retrieves flagged statuses for moderation
func (r *StatusRepository) GetFlaggedStatuses(ctx context.Context, limit int, _ string) ([]*models.Status, string, error) {
	// This would require a GSI for flagged statuses in a real implementation
	// For now, we'll scan the table (not efficient for production)
	var statuses []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Filter("Flagged", "=", true).
		Limit(limit).
		Scan(&statuses)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get flagged statuses: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}

	return result, "", nil
}

// StatusFilter represents filtering criteria for admin status listing
type StatusFilter struct {
	Local      *bool     // Filter by local vs remote statuses
	Remote     *bool     // Filter by remote statuses only
	ByDomain   string    // Filter by specific domain
	Visibility string    // Filter by visibility (public, unlisted, private, direct)
	Flagged    *bool     // Filter by flagged status
	Reported   *bool     // Filter by reported status
	WithMedia  *bool     // Filter by presence of media attachments
	Sensitive  *bool     // Filter by sensitive flag
	MinDate    *time.Time // Filter by minimum creation date
	MaxDate    *time.Time // Filter by maximum creation date
}

// ListStatusesForAdmin retrieves statuses with comprehensive admin filtering
//
//nolint:gocognit // Complex filtering logic is necessary to support all admin moderation use cases
func (r *StatusRepository) ListStatusesForAdmin(ctx context.Context, filter *StatusFilter, limit int, cursor string) ([]*models.Status, string, error) {
	r.logger.Debug("listing statuses for admin with filter", 
		zap.Any("filter", filter),
		zap.Int("limit", limit),
		zap.String("cursor", cursor))

	var statuses []models.Status
	query := r.db.WithContext(ctx).Model(&models.Status{})
	
	// Base filter to exclude deleted statuses unless specifically requested
	query = query.Filter("Deleted", "=", false)
	
	// Apply domain filters
	if filter.Local != nil && *filter.Local {
		// Filter for local statuses by checking if AuthorID contains local domain
		// This is a simple check - in production you might want a more robust method
		domain := r.extractDomainFromEnv()
		if domain != "" {
			query = query.Filter("AuthorID", "CONTAINS", domain)
		}
	}
	
	if filter.Remote != nil && *filter.Remote {
		// Filter for remote statuses by checking if AuthorID does NOT contain local domain
		domain := r.extractDomainFromEnv()
		if domain != "" {
			// Note: DynamoDB doesn't have a direct "NOT CONTAINS" operation
			// We'll need to use a more complex approach or scan and filter
			// For now, we'll scan all statuses and then filter
			err := query.Scan(&statuses)
			if err != nil {
				return nil, "", fmt.Errorf("failed to scan statuses: %w", err)
			}
			
			// Post-process to filter remote only
			filteredStatuses := []models.Status{}
			for _, status := range statuses {
				if !strings.Contains(status.AuthorID, domain) {
					filteredStatuses = append(filteredStatuses, status)
				}
			}
			statuses = filteredStatuses
		}
	} else {
		// If not filtering by remote, apply specific domain filter
		if filter.ByDomain != "" {
			query = query.Filter("AuthorID", "CONTAINS", filter.ByDomain)
		}
		
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
		
		// Apply date filters
		if filter.MinDate != nil {
			query = query.Filter("PublishedAt", ">=", *filter.MinDate)
		}
		
		if filter.MaxDate != nil {
			query = query.Filter("PublishedAt", "<=", *filter.MaxDate)
		}
		
		// Execute query with limit
		query = query.Limit(limit)
		err := query.Scan(&statuses)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get filtered statuses: %w", err)
		}
	}
	
	// Convert to pointer slice
	result := make([]*models.Status, len(statuses))
	for i := range statuses {
		result[i] = &statuses[i]
	}
	
	// For now, return empty cursor - proper pagination would need custom implementation
	// In production, you'd implement proper cursor-based pagination
	nextCursor := ""
	if len(result) == limit {
		// Generate a simple cursor based on the last item
		if len(result) > 0 {
			lastStatus := result[len(result)-1]
			nextCursor = fmt.Sprintf("%d_%s", lastStatus.PublishedAt.Unix(), lastStatus.StatusID)
		}
	}
	
	r.logger.Debug("retrieved filtered statuses for admin", 
		zap.Int("count", len(result)),
		zap.String("nextCursor", nextCursor))
	
	return result, nextCursor, nil
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

// Helper function to extract status ID from URL
func extractStatusIDFromURL(url string) string {
	// Handle different status URL formats
	// e.g., "https://example.com/users/username/statuses/123" -> "123"
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
