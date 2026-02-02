package repositories

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// FilterItem defines the interface for filter-related items that can be created
type FilterItem interface {
	*models.FilterKeyword | *models.FilterStatus
}

// FilterItemCreatable defines the common interface for creatable filter items
type FilterItemCreatable interface {
	GetID() string
	SetID(string)
	SetCreatedAt(time.Time)
	UpdateKeys() error
}

// Ensure models implement the interface (compile-time check)
var (
	_ FilterItemCreatable = (*filterKeywordAdapter)(nil)
	_ FilterItemCreatable = (*filterStatusAdapter)(nil)
)

// Adapter structs to implement the common interface
type filterKeywordAdapter struct {
	*models.FilterKeyword
}

func (a *filterKeywordAdapter) GetID() string            { return a.ID }
func (a *filterKeywordAdapter) SetID(id string)          { a.ID = id }
func (a *filterKeywordAdapter) SetCreatedAt(t time.Time) { a.CreatedAt = t }

type filterStatusAdapter struct {
	*models.FilterStatus
}

func (a *filterStatusAdapter) GetID() string            { return a.ID }
func (a *filterStatusAdapter) SetID(id string)          { a.ID = id }
func (a *filterStatusAdapter) SetCreatedAt(t time.Time) { a.CreatedAt = t }

// FilterRepository handles user content filtering operations using enhanced DynamORM patterns
type FilterRepository struct {
	*EnhancedBaseRepository[*models.Filter]
	db core.DB
}

// NewFilterRepository creates a new FilterRepository with enhanced functionality and cost tracking
func NewFilterRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *FilterRepository {
	// Create enhanced repository for filter operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Filter](db, tableName, logger, costService, "FilterRepository", "filter")

	// Set up enhanced services for filter operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cache filters for performance
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &FilterRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
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

	// Create the filter using enhanced validation and creation
	if err := r.ValidateAndCreate(ctx, filter); err != nil {
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

	if err := r.Update(ctx, filter); err != nil {
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
	if err := r.Delete(ctx, filter.PK, filter.SK); err != nil {
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

// createFilterItem is a helper for creating filter items (keywords, statuses)
func (r *FilterRepository) createFilterItem(ctx context.Context, item interface{}, adapter FilterItemCreatable, entityType string, logFields []zap.Field) error {
	// Generate UUID if not provided
	if adapter.GetID() == "" {
		adapter.SetID(uuid.New().String())
	}

	// Set CreatedAt
	adapter.SetCreatedAt(time.Now())

	// Update keys
	if err := adapter.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, entityType, "keys")
	}

	// Create the item using DynamORM
	if err := r.db.WithContext(ctx).Model(item).Create(); err != nil {
		errorFields := append(logFields, zap.Error(err))
		r.logger.Error("Failed to add "+entityType, errorFields...)
		return ErrorHandler.HandleCreateError(err, entityType, adapter.GetID())
	}

	// Success logging
	r.logger.Debug("Added "+entityType, logFields...)

	return nil
}

// AddFilterKeyword adds a new keyword to a filter
func (r *FilterRepository) AddFilterKeyword(ctx context.Context, keyword *models.FilterKeyword) error {
	adapter := &filterKeywordAdapter{FilterKeyword: keyword}
	logFields := []zap.Field{
		zap.String("filter_id", keyword.FilterID),
		zap.String("keyword_id", keyword.ID),
		zap.String("keyword", keyword.Keyword),
	}
	return r.createFilterItem(ctx, keyword, adapter, EntityFilterKeyword, logFields)
}

// FilterItemDeletable defines the common interface for deletable filter items
type FilterItemDeletable interface {
	GetPK() string
	GetSK() string
	GetFilterID() string
	GetItemID() string
}

// filterKeywordDeletable adapter
type filterKeywordDeletable struct {
	*models.FilterKeyword
}

func (a *filterKeywordDeletable) GetPK() string       { return a.PK }
func (a *filterKeywordDeletable) GetSK() string       { return a.SK }
func (a *filterKeywordDeletable) GetFilterID() string { return a.FilterID }
func (a *filterKeywordDeletable) GetItemID() string   { return a.ID }

// filterStatusDeletable adapter
type filterStatusDeletable struct {
	*models.FilterStatus
}

func (a *filterStatusDeletable) GetPK() string       { return a.PK }
func (a *filterStatusDeletable) GetSK() string       { return a.SK }
func (a *filterStatusDeletable) GetFilterID() string { return a.FilterID }
func (a *filterStatusDeletable) GetItemID() string   { return a.ID }

// removeFilterItem is a helper for removing filter items (keywords, statuses)
func (r *FilterRepository) removeFilterItem(ctx context.Context, itemID, skPattern, entityType string, model interface{}, findItemFunc func(interface{}) (FilterItemDeletable, bool)) error {
	// First find the item
	var items interface{}
	switch model.(type) {
	case *models.FilterKeyword:
		var keywords []*models.FilterKeyword
		items = &keywords
	case *models.FilterStatus:
		var statuses []*models.FilterStatus
		items = &statuses
	}

	err := r.db.WithContext(ctx).Model(model).
		Where("SK", "=", fmt.Sprintf(skPattern, itemID)).
		Limit(10).
		All(items)

	if err != nil {
		return ErrorHandler.HandleQueryError(err, entityType, "deletion")
	}

	// Find the target item using the provided function
	targetItem, found := findItemFunc(items)
	if !found {
		return ErrorHandler.HandleGetError(storage.ErrNotFound, entityType, itemID)
	}

	// Delete the item
	err = r.db.WithContext(ctx).Model(model).
		Where("PK", "=", targetItem.GetPK()).
		Where("SK", "=", targetItem.GetSK()).
		Delete()

	if err != nil {
		r.logger.Error("Failed to delete "+entityType,
			zap.Error(err),
			zap.String("item_id", itemID))
		return ErrorHandler.HandleDeleteError(err, entityType, itemID)
	}

	r.logger.Debug("Deleted "+entityType,
		zap.String("item_id", itemID),
		zap.String("filter_id", targetItem.GetFilterID()))

	return nil
}

// RemoveFilterKeyword removes a filter keyword
func (r *FilterRepository) RemoveFilterKeyword(ctx context.Context, keywordID string) error {
	return r.removeFilterItem(ctx, keywordID, "KEYWORD#%s", EntityFilterKeyword, &models.FilterKeyword{},
		func(items interface{}) (FilterItemDeletable, bool) {
			keywords := items.(*[]*models.FilterKeyword)
			for _, keyword := range *keywords {
				if keyword.ID == keywordID {
					return &filterKeywordDeletable{FilterKeyword: keyword}, true
				}
			}
			return nil, false
		})
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
	adapter := &filterStatusAdapter{FilterStatus: filterStatus}
	logFields := []zap.Field{
		zap.String("filter_id", filterStatus.FilterID),
		zap.String("filter_status_id", filterStatus.ID),
		zap.String("status_id", filterStatus.StatusID),
	}
	return r.createFilterItem(ctx, filterStatus, adapter, EntityFilterStatus, logFields)
}

// RemoveFilterStatus removes a filter status by its ID
func (r *FilterRepository) RemoveFilterStatus(ctx context.Context, filterStatusID string) error {
	return r.removeFilterItem(ctx, filterStatusID, "STATUS#%s", EntityFilterStatus, &models.FilterStatus{},
		func(items interface{}) (FilterItemDeletable, bool) {
			statuses := items.(*[]*models.FilterStatus)
			for _, status := range *statuses {
				if status.StatusID == filterStatusID {
					return &filterStatusDeletable{FilterStatus: status}, true
				}
			}
			return nil, false
		})
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
