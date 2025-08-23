package repositories

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// FilterRepository handles user content filtering operations using DynamORM
type FilterRepository struct {
	*BaseRepository[*models.Filter]
	logger *zap.Logger
	db     core.DB
}

// NewFilterRepository creates a new FilterRepository
func NewFilterRepository(db core.DB, logger *zap.Logger, costService *cost.TrackingService) *FilterRepository {
	return &FilterRepository{
		BaseRepository: NewBaseRepositoryWithCostTracking[*models.Filter](db, models.MainTableName, logger, costService, "filter"),
		logger:         logger,
		db:             db,
	}
}

// CreateFilter creates a new filter
func (r *FilterRepository) CreateFilter(ctx context.Context, filter *models.Filter) error {
	// Generate ID if not provided
	if filter.ID == "" {
		filter.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	filter.CreatedAt = now
	filter.UpdatedAt = now

	// Update keys
	if err := filter.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityFilter, "keys")
	}

	// Create the filter using BaseRepository
	if err := r.BaseRepository.Create(ctx, filter); err != nil {
		r.logger.Error("Failed to create filter",
			zap.Error(err),
			zap.String("filter_id", filter.ID),
			zap.String("username", filter.Username))
		return ErrorHandler.HandleCreateError(err, EntityFilter, filter.ID)
	}

	r.logger.Debug("Created filter",
		zap.String("filter_id", filter.ID),
		zap.String("username", filter.Username),
		zap.String("title", filter.Title))

	return nil
}

// GetFilter retrieves a filter by ID
func (r *FilterRepository) GetFilter(ctx context.Context, filterID string) (*models.Filter, error) {
	// We need to scan for the filter since we don't know the username
	var filters []*models.Filter

	err := r.db.WithContext(ctx).Model(&models.Filter{}).
		Where("SK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Limit(10). // Reasonable limit
		All(&filters)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFilter, "query")
	}

	// Find the matching filter
	for _, filter := range filters {
		if filter.ID == filterID {
			return filter, nil
		}
	}

	return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityFilter, filterID)
}

// UpdateFilter updates a filter
func (r *FilterRepository) UpdateFilter(ctx context.Context, filter *models.Filter) error {
	// Set updated timestamp
	filter.UpdatedAt = time.Now()

	// Update keys
	if err := filter.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityFilter, "keys")
	}

	if err := r.BaseRepository.Update(ctx, filter); err != nil {
		r.logger.Error("Failed to update filter",
			zap.Error(err),
			zap.String("filter_id", filter.ID))
		return ErrorHandler.HandleUpdateError(err, EntityFilter, filter.ID)
	}

	r.logger.Debug("Updated filter",
		zap.String("filter_id", filter.ID),
		zap.String("username", filter.Username))

	return nil
}

// DeleteFilter deletes a filter and all its associated keywords and statuses
func (r *FilterRepository) DeleteFilter(ctx context.Context, filterID string) error {
	// First get the filter to find the username
	filter, err := r.GetFilter(ctx, filterID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityFilter, filterID)
	}

	// Delete all keywords first
	keywords, err := r.GetFilterKeywords(ctx, filterID)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityFilterKeyword, "deletion")
	}
	for _, keyword := range keywords {
		if err := r.RemoveFilterKeyword(ctx, keyword.ID); err != nil {
			r.logger.Error("Failed to delete filter keyword during filter deletion",
				zap.Error(err),
				zap.String("keyword_id", keyword.ID))
			// Continue with other deletions
		}
	}

	// Delete all statuses
	statuses, err := r.GetFilterStatuses(ctx, filterID)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityFilterStatus, "deletion")
	}
	for _, status := range statuses {
		if err := r.RemoveFilterStatus(ctx, status.ID); err != nil {
			r.logger.Error("Failed to delete filter status during filter deletion",
				zap.Error(err),
				zap.String("status_id", status.ID))
			// Continue with other deletions
		}
	}

	// Delete the filter itself
	if err := r.BaseRepository.Delete(ctx, filter.PK, filter.SK); err != nil {
		r.logger.Error("Failed to delete filter",
			zap.Error(err),
			zap.String("filter_id", filterID),
			zap.String("username", filter.Username))
		return ErrorHandler.HandleDeleteError(err, EntityFilter, filterID)
	}

	r.logger.Debug("Deleted filter",
		zap.String("filter_id", filterID),
		zap.String("username", filter.Username))

	return nil
}

// GetUserFilters retrieves all filters for a user
func (r *FilterRepository) GetUserFilters(ctx context.Context, username string) ([]*models.Filter, error) {
	var filters []*models.Filter

	err := r.db.WithContext(ctx).Model(&models.Filter{}).
		Where("PK", "=", fmt.Sprintf(models.KeyPatternUser, username)).
		Where("SK", ">=", "FILTER#").
		Where("SK", "<", "FILTER~"). // Use ~ as upper bound since it's after # in ASCII
		All(&filters)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFilter, "user filters")
	}

	return filters, nil
}

// GetActiveFilters retrieves active filters for a user in specific contexts
func (r *FilterRepository) GetActiveFilters(ctx context.Context, username string, context []string) ([]*models.Filter, error) {
	allFilters, err := r.GetUserFilters(ctx, username)
	if err != nil {
		return nil, err
	}

	var activeFilters []*models.Filter
	now := time.Now()

	for _, filter := range allFilters {
		// Check if filter is expired
		if filter.ExpiresAt != nil && filter.ExpiresAt.Before(now) {
			continue
		}

		// Check if filter applies to any of the requested contexts
		if len(context) > 0 && len(filter.Context) > 0 {
			hasMatchingContext := false
			for _, filterCtx := range filter.Context {
				for _, reqCtx := range context {
					if filterCtx == reqCtx {
						hasMatchingContext = true
						break
					}
				}
				if hasMatchingContext {
					break
				}
			}
			if !hasMatchingContext {
				continue
			}
		}

		activeFilters = append(activeFilters, filter)
	}

	return activeFilters, nil
}

// AddFilterKeyword adds a new keyword to a filter
func (r *FilterRepository) AddFilterKeyword(ctx context.Context, keyword *models.FilterKeyword) error {
	// Generate UUID if not provided
	if keyword.ID == "" {
		keyword.ID = uuid.New().String()
	}

	// Set CreatedAt
	keyword.CreatedAt = time.Now()

	// Update keys
	if err := keyword.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityFilterKeyword, "keys")
	}

	// Create the keyword using BaseRepository pattern
	if err := r.db.WithContext(ctx).Model(keyword).Create(); err != nil {
		r.logger.Error("Failed to add filter keyword",
			zap.Error(err),
			zap.String("filter_id", keyword.FilterID),
			zap.String("keyword", keyword.Keyword))
		return ErrorHandler.HandleCreateError(err, EntityFilterKeyword, keyword.ID)
	}

	r.logger.Debug("Added filter keyword",
		zap.String("filter_id", keyword.FilterID),
		zap.String("keyword_id", keyword.ID),
		zap.String("keyword", keyword.Keyword))

	return nil
}

// RemoveFilterKeyword removes a filter keyword
func (r *FilterRepository) RemoveFilterKeyword(ctx context.Context, keywordID string) error {
	// First find the keyword to get its FilterID
	var keywords []*models.FilterKeyword

	err := r.db.WithContext(ctx).Model(&models.FilterKeyword{}).
		Where("SK", "=", fmt.Sprintf("KEYWORD#%s", keywordID)).
		Limit(10).
		All(&keywords)

	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityFilterKeyword, "deletion")
	}

	var targetKeyword *models.FilterKeyword
	for _, keyword := range keywords {
		if keyword.ID == keywordID {
			targetKeyword = keyword
			break
		}
	}

	if targetKeyword == nil {
		return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityFilterKeyword, keywordID)
	}

	// Delete the keyword
	err = r.db.WithContext(ctx).Model(&models.FilterKeyword{}).
		Where("PK", "=", targetKeyword.PK).
		Where("SK", "=", targetKeyword.SK).
		Delete()

	if err != nil {
		r.logger.Error("Failed to delete filter keyword",
			zap.Error(err),
			zap.String("keyword_id", keywordID))
		return ErrorHandler.HandleDeleteError(err, EntityFilterKeyword, keywordID)
	}

	r.logger.Debug("Deleted filter keyword",
		zap.String("keyword_id", keywordID),
		zap.String("filter_id", targetKeyword.FilterID))

	return nil
}

// GetFilterKeywords retrieves all keywords for a filter
func (r *FilterRepository) GetFilterKeywords(ctx context.Context, filterID string) ([]*models.FilterKeyword, error) {
	var keywords []*models.FilterKeyword

	err := r.db.WithContext(ctx).Model(&models.FilterKeyword{}).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Where("SK", ">=", "KEYWORD#").
		Where("SK", "<", "KEYWORD~").
		All(&keywords)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFilterKeyword, "query")
	}

	return keywords, nil
}

// AddFilterStatus adds a new status to a filter
func (r *FilterRepository) AddFilterStatus(ctx context.Context, filterStatus *models.FilterStatus) error {
	// Generate UUID if not provided
	if filterStatus.ID == "" {
		filterStatus.ID = uuid.New().String()
	}

	// Set CreatedAt
	filterStatus.CreatedAt = time.Now()

	// Update keys
	if err := filterStatus.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityFilterStatus, "keys")
	}

	// Create the status using BaseRepository pattern
	if err := r.db.WithContext(ctx).Model(filterStatus).Create(); err != nil {
		r.logger.Error("Failed to add filter status",
			zap.Error(err),
			zap.String("filter_id", filterStatus.FilterID),
			zap.String("status_id", filterStatus.StatusID))
		return ErrorHandler.HandleCreateError(err, EntityFilterStatus, filterStatus.ID)
	}

	r.logger.Debug("Added filter status",
		zap.String("filter_id", filterStatus.FilterID),
		zap.String("filter_status_id", filterStatus.ID),
		zap.String("status_id", filterStatus.StatusID))

	return nil
}

// RemoveFilterStatus removes a filter status by its ID
func (r *FilterRepository) RemoveFilterStatus(ctx context.Context, filterStatusID string) error {
	// First find the status to get its FilterID
	var statuses []*models.FilterStatus

	err := r.db.WithContext(ctx).Model(&models.FilterStatus{}).
		Where("SK", "=", fmt.Sprintf("STATUS#%s", filterStatusID)).
		Limit(10).
		All(&statuses)

	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityFilterStatus, "deletion")
	}

	var targetStatus *models.FilterStatus
	for _, status := range statuses {
		if status.StatusID == filterStatusID {
			targetStatus = status
			break
		}
	}

	if targetStatus == nil {
		return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityFilterStatus, filterStatusID)
	}

	// Delete the status
	err = r.db.WithContext(ctx).Model(&models.FilterStatus{}).
		Where("PK", "=", targetStatus.PK).
		Where("SK", "=", targetStatus.SK).
		Delete()

	if err != nil {
		r.logger.Error("Failed to delete filter status",
			zap.Error(err),
			zap.String("status_id", filterStatusID))
		return ErrorHandler.HandleDeleteError(err, EntityFilterStatus, filterStatusID)
	}

	r.logger.Debug("Deleted filter status",
		zap.String("status_id", filterStatusID),
		zap.String("filter_id", targetStatus.FilterID))

	return nil
}

// GetFilterStatuses retrieves all statuses for a filter
func (r *FilterRepository) GetFilterStatuses(ctx context.Context, filterID string) ([]*models.FilterStatus, error) {
	var statuses []*models.FilterStatus

	err := r.db.WithContext(ctx).Model(&models.FilterStatus{}).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Where("SK", ">=", "STATUS#").
		Where("SK", "<", "STATUS~").
		All(&statuses)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFilterStatus, "query")
	}

	return statuses, nil
}

// EvaluateFilters evaluates content against user filters
func (r *FilterRepository) EvaluateFilters(ctx context.Context, username string, content string, context []string) ([]*models.Filter, error) {
	// Get active filters for the user and context
	filters, err := r.GetActiveFilters(ctx, username, context)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFilter, "active filters")
	}

	var matchingFilters []*models.Filter

	// Evaluate each filter
	for _, filter := range filters {
		// Get keywords for this filter
		keywords, err := r.GetFilterKeywords(ctx, filter.ID)
		if err != nil {
			r.logger.Error("Failed to get keywords for filter evaluation",
				zap.Error(err),
				zap.String("filter_id", filter.ID))
			continue
		}

		// Check if any keyword matches the content
		for _, keyword := range keywords {
			if r.matchesKeyword(content, keyword, filter.CaseSensitive) {
				matchingFilters = append(matchingFilters, filter)
				break // Only need one keyword to match per filter
			}
		}
	}

	return matchingFilters, nil
}

// CheckContentFiltered checks if a specific status is filtered for a user
func (r *FilterRepository) CheckContentFiltered(ctx context.Context, username, statusID string, context []string) (bool, []*models.Filter, error) {
	// Get active filters for the user and context
	filters, err := r.GetActiveFilters(ctx, username, context)
	if err != nil {
		return false, nil, ErrorHandler.HandleQueryError(err, EntityFilter, "active filters")
	}

	var matchingFilters []*models.Filter

	// Check each filter
	for _, filter := range filters {
		// Check if this status is explicitly filtered
		statuses, err := r.GetFilterStatuses(ctx, filter.ID)
		if err != nil {
			r.logger.Error("Failed to get statuses for filter evaluation",
				zap.Error(err),
				zap.String("filter_id", filter.ID))
			continue
		}

		for _, status := range statuses {
			if status.StatusID == statusID {
				matchingFilters = append(matchingFilters, filter)
				break
			}
		}
	}

	return len(matchingFilters) > 0, matchingFilters, nil
}

// matchesKeyword checks if content matches a keyword based on filter settings
func (r *FilterRepository) matchesKeyword(content string, keyword *models.FilterKeyword, caseSensitive bool) bool {
	searchContent := content
	searchKeyword := keyword.Keyword

	if !caseSensitive {
		searchContent = strings.ToLower(content)
		searchKeyword = strings.ToLower(keyword.Keyword)
	}

	if keyword.IsRegex {
		// Handle regex matching
		regex, err := regexp.Compile(searchKeyword)
		if err != nil {
			r.logger.Error("Invalid regex in filter keyword",
				zap.Error(err),
				zap.String("keyword", keyword.Keyword))
			return false
		}
		return regex.MatchString(searchContent)
	}

	if keyword.WholeWord {
		// Match whole words only
		regex, err := regexp.Compile(`\b` + regexp.QuoteMeta(searchKeyword) + `\b`)
		if err != nil {
			return false
		}
		return regex.MatchString(searchContent)
	}

	// Simple substring match
	return strings.Contains(searchContent, searchKeyword)
}