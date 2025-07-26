package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
)

// StatusRepository implements status operations using DynamORM
type StatusRepository struct {
	db core.DB
}

// NewStatusRepository creates a new status repository
func NewStatusRepository(db core.DB) *StatusRepository {
	return &StatusRepository{
		db: db,
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
func (r *StatusRepository) GetStatusesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Status, string, error) {
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
func (r *StatusRepository) GetPublicTimeline(ctx context.Context, limit int, cursor string) ([]*models.Status, string, error) {
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
func (r *StatusRepository) GetReplies(ctx context.Context, statusID string, limit int, cursor string) ([]*models.Status, string, error) {
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
func (r *StatusRepository) GetHashtagTimeline(ctx context.Context, hashtag string, limit int, cursor string) ([]*models.Status, string, error) {
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

// GetFlaggedStatuses retrieves flagged statuses for moderation
func (r *StatusRepository) GetFlaggedStatuses(ctx context.Context, limit int, cursor string) ([]*models.Status, string, error) {
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
