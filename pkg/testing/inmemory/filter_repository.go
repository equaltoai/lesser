// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
)

// FilterRepository is a thread-safe in-memory implementation of interfaces.FilterRepository.
type FilterRepository struct {
	mu sync.RWMutex

	// Filters by ID: filterID -> Filter
	filtersByID map[string]*models.Filter

	// Filters by user: username -> []Filter
	filtersByUser map[string][]*models.Filter

	// Keywords by filter: filterID -> []FilterKeyword
	keywordsByFilter map[string][]*models.FilterKeyword

	// Keywords by ID: keywordID -> FilterKeyword
	keywordsByID map[string]*models.FilterKeyword

	// Statuses by filter: filterID -> []FilterStatus
	statusesByFilter map[string][]*models.FilterStatus

	// Statuses by ID: statusID -> FilterStatus
	statusesByID map[string]*models.FilterStatus
}

// NewFilterRepository creates a new in-memory filter repository
func NewFilterRepository() *FilterRepository {
	return &FilterRepository{
		filtersByID:      make(map[string]*models.Filter),
		filtersByUser:    make(map[string][]*models.Filter),
		keywordsByFilter: make(map[string][]*models.FilterKeyword),
		keywordsByID:     make(map[string]*models.FilterKeyword),
		statusesByFilter: make(map[string][]*models.FilterStatus),
		statusesByID:     make(map[string]*models.FilterStatus),
	}
}

// ===== Filter CRUD Operations =====

// CreateFilter creates a new filter
func (r *FilterRepository) CreateFilter(_ context.Context, filter *models.Filter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if filter.ID == "" {
		filter.ID = uuid.New().String()
	}

	now := time.Now()
	filter.CreatedAt = now
	filter.UpdatedAt = now

	r.filtersByID[filter.ID] = filter
	r.filtersByUser[filter.Username] = append(r.filtersByUser[filter.Username], filter)

	return nil
}

// GetFilter retrieves a filter by ID
func (r *FilterRepository) GetFilter(_ context.Context, filterID string) (*models.Filter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filter, exists := r.filtersByID[filterID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return filter, nil
}

// UpdateFilter updates a filter
func (r *FilterRepository) UpdateFilter(_ context.Context, filter *models.Filter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.filtersByID[filter.ID]; !exists {
		return storage.ErrNotFound
	}

	filter.UpdatedAt = time.Now()
	r.filtersByID[filter.ID] = filter

	// Update in user's list
	for i, f := range r.filtersByUser[filter.Username] {
		if f.ID == filter.ID {
			r.filtersByUser[filter.Username][i] = filter
			break
		}
	}

	return nil
}

// ===== Filter Query Operations =====

// GetUserFilters retrieves all filters for a user
func (r *FilterRepository) GetUserFilters(_ context.Context, username string) ([]*models.Filter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filters := r.filtersByUser[username]
	result := make([]*models.Filter, len(filters))
	copy(result, filters)
	return result, nil
}

// GetActiveFilters retrieves active filters for a user in specific contexts
func (r *FilterRepository) GetActiveFilters(_ context.Context, username string, contexts []string) ([]*models.Filter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var activeFilters []*models.Filter
	now := time.Now()

	for _, filter := range r.filtersByUser[username] {
		// Check if filter is expired
		if filter.ExpiresAt != nil && filter.ExpiresAt.Before(now) {
			continue
		}

		// Check if filter applies to any of the requested contexts
		if len(contexts) > 0 && len(filter.Context) > 0 {
			hasMatchingContext := false
			for _, filterCtx := range filter.Context {
				for _, reqCtx := range contexts {
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

// ===== Filter Keyword Operations =====

// AddFilterKeyword adds a new keyword to a filter
func (r *FilterRepository) AddFilterKeyword(_ context.Context, keyword *models.FilterKeyword) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if keyword.ID == "" {
		keyword.ID = uuid.New().String()
	}
	keyword.CreatedAt = time.Now()

	r.keywordsByID[keyword.ID] = keyword
	r.keywordsByFilter[keyword.FilterID] = append(r.keywordsByFilter[keyword.FilterID], keyword)

	return nil
}

// GetFilterKeywords retrieves all keywords for a filter
func (r *FilterRepository) GetFilterKeywords(_ context.Context, filterID string) ([]*models.FilterKeyword, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keywords := r.keywordsByFilter[filterID]
	result := make([]*models.FilterKeyword, len(keywords))
	copy(result, keywords)
	return result, nil
}

// ===== Filter Status Operations =====

// AddFilterStatus adds a new status to a filter
func (r *FilterRepository) AddFilterStatus(_ context.Context, filterStatus *models.FilterStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if filterStatus.ID == "" {
		filterStatus.ID = uuid.New().String()
	}
	filterStatus.CreatedAt = time.Now()

	r.statusesByID[filterStatus.ID] = filterStatus
	r.statusesByFilter[filterStatus.FilterID] = append(r.statusesByFilter[filterStatus.FilterID], filterStatus)

	return nil
}

// GetFilterStatuses retrieves all statuses for a filter
func (r *FilterRepository) GetFilterStatuses(_ context.Context, filterID string) ([]*models.FilterStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statuses := r.statusesByFilter[filterID]
	result := make([]*models.FilterStatus, len(statuses))
	copy(result, statuses)
	return result, nil
}

// ===== Filter Evaluation Operations =====

// EvaluateFilters evaluates content against user filters
func (r *FilterRepository) EvaluateFilters(ctx context.Context, username string, content string, contexts []string) ([]*models.Filter, error) {
	filters, err := r.GetActiveFilters(ctx, username, contexts)
	if err != nil {
		return nil, err
	}

	var matchingFilters []*models.Filter

	for _, filter := range filters {
		keywords, err := r.GetFilterKeywords(ctx, filter.ID)
		if err != nil {
			continue
		}

		for _, keyword := range keywords {
			if r.matchesKeyword(content, keyword, filter.CaseSensitive) {
				matchingFilters = append(matchingFilters, filter)
				break
			}
		}
	}

	return matchingFilters, nil
}

// CheckContentFiltered checks if a specific status is filtered for a user
func (r *FilterRepository) CheckContentFiltered(ctx context.Context, username, statusID string, contexts []string) (bool, []*models.Filter, error) {
	filters, err := r.GetActiveFilters(ctx, username, contexts)
	if err != nil {
		return false, nil, err
	}

	var matchingFilters []*models.Filter

	for _, filter := range filters {
		statuses, err := r.GetFilterStatuses(ctx, filter.ID)
		if err != nil {
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
		regex, err := regexp.Compile(searchKeyword)
		if err != nil {
			return false
		}
		return regex.MatchString(searchContent)
	}

	if keyword.WholeWord {
		regex, err := regexp.Compile(`\b` + regexp.QuoteMeta(searchKeyword) + `\b`)
		if err != nil {
			return false
		}
		return regex.MatchString(searchContent)
	}

	return strings.Contains(searchContent, searchKeyword)
}

// Clear clears all data (test helper)
func (r *FilterRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.filtersByID = make(map[string]*models.Filter)
	r.filtersByUser = make(map[string][]*models.Filter)
	r.keywordsByFilter = make(map[string][]*models.FilterKeyword)
	r.keywordsByID = make(map[string]*models.FilterKeyword)
	r.statusesByFilter = make(map[string][]*models.FilterStatus)
	r.statusesByID = make(map[string]*models.FilterStatus)
}

// Ensure FilterRepository implements interfaces.FilterRepository
var _ interfaces.FilterRepository = (*FilterRepository)(nil)
