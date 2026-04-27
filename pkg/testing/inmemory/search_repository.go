// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// SearchRepository is a thread-safe in-memory implementation of interfaces.SearchRepository.
type SearchRepository struct {
	mu sync.RWMutex
	// actors stores actors for search
	actors map[string]*activitypub.Actor
	// statuses stores statuses for search
	statuses map[string]*storage.StatusSearchResult
	// deps stores dependencies for cross-repository operations
	deps interfaces.SearchRepositoryDeps
}

// NewSearchRepository creates a new in-memory search repository
func NewSearchRepository() *SearchRepository {
	return &SearchRepository{
		actors:   make(map[string]*activitypub.Actor),
		statuses: make(map[string]*storage.StatusSearchResult),
	}
}

// SearchAccounts searches for accounts matching the given query
func (r *SearchRepository) SearchAccounts(_ context.Context, query string, limit int, _ bool, offset int) ([]*activitypub.Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryLower := strings.ToLower(query)
	var results []*activitypub.Actor

	for _, actor := range r.actors {
		if strings.Contains(strings.ToLower(actor.PreferredUsername), queryLower) ||
			strings.Contains(strings.ToLower(actor.Name), queryLower) {
			results = append(results, actor)
		}
	}

	// Apply offset and limit
	if offset >= len(results) {
		return []*activitypub.Actor{}, nil
	}
	results = results[offset:]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// SearchAccountsWithPrivacy searches for accounts with privacy enforcement
func (r *SearchRepository) SearchAccountsWithPrivacy(ctx context.Context, query string, limit int, followingOnly bool, offset int, _ string) ([]*activitypub.Actor, error) {
	return r.SearchAccounts(ctx, query, limit, followingOnly, offset)
}

// SearchAccountsAdvanced searches for accounts with advanced filtering
func (r *SearchRepository) SearchAccountsAdvanced(ctx context.Context, query string, _ bool, limit int, offset int, following bool, _ string) ([]*activitypub.Actor, error) {
	return r.SearchAccounts(ctx, query, limit, following, offset)
}

// SearchStatuses searches for statuses matching the given query
func (r *SearchRepository) SearchStatuses(_ context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryLower := strings.ToLower(query)
	var results []*storage.StatusSearchResult

	for _, status := range r.statuses {
		if strings.Contains(strings.ToLower(status.Content), queryLower) {
			results = append(results, status)
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// SearchStatusesWithOptions searches for statuses with advanced options
func (r *SearchRepository) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	return r.SearchStatuses(ctx, query, options.Limit)
}

// SearchStatusesWithOptionsPaginated searches for statuses with pagination
func (r *SearchRepository) SearchStatusesWithOptionsPaginated(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, *interfaces.PaginationResult, error) {
	results, err := r.SearchStatuses(ctx, query, options.Limit)
	if err != nil {
		return nil, nil, err
	}
	pagination := &interfaces.PaginationResult{
		HasNextPage: false,
		TotalCount:  len(results),
	}
	return results, pagination, nil
}

// SearchStatusesWithPrivacy searches for statuses with privacy enforcement
func (r *SearchRepository) SearchStatusesWithPrivacy(ctx context.Context, query string, options storage.StatusSearchOptions, _ string) ([]*storage.StatusSearchResult, error) {
	results, err := r.SearchStatuses(ctx, query, options.Limit)
	if err != nil {
		return nil, err
	}
	return filterPublicStatusSearchResults(results), nil
}

// SearchStatusesWithPrivacyPaginated searches for statuses with privacy and pagination
func (r *SearchRepository) SearchStatusesWithPrivacyPaginated(ctx context.Context, query string, options storage.StatusSearchOptions, _ string) ([]*storage.StatusSearchResult, *interfaces.PaginationResult, error) {
	results, _, err := r.SearchStatusesWithOptionsPaginated(ctx, query, options)
	if err != nil {
		return nil, nil, err
	}
	results = filterPublicStatusSearchResults(results)
	return results, &interfaces.PaginationResult{
		HasNextPage: false,
		TotalCount:  len(results),
	}, nil
}

// SearchStatusesAdvanced searches for statuses with advanced filtering
func (r *SearchRepository) SearchStatusesAdvanced(ctx context.Context, query string, limit int, _, _ *string, _ string) ([]*storage.StatusSearchResult, error) {
	return r.SearchStatuses(ctx, query, limit)
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *SearchRepository) SetDependencies(deps interfaces.SearchRepositoryDeps) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deps = deps
}

// AddActor adds an actor for testing
func (r *SearchRepository) AddActor(actor *activitypub.Actor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actors[actor.ID] = actor
}

// AddStatus adds a status for testing
func (r *SearchRepository) AddStatus(status *storage.StatusSearchResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses[status.ID] = status
}

// Clear clears all data (test helper)
func (r *SearchRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actors = make(map[string]*activitypub.Actor)
	r.statuses = make(map[string]*storage.StatusSearchResult)
}

func filterPublicStatusSearchResults(results []*storage.StatusSearchResult) []*storage.StatusSearchResult {
	filtered := make([]*storage.StatusSearchResult, 0, len(results))
	for _, result := range results {
		if result == nil {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(result.Visibility)) {
		case models.VisibilityPublic, models.VisibilityUnlisted:
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// Ensure SearchRepository implements interfaces.SearchRepository
var _ interfaces.SearchRepository = (*SearchRepository)(nil)
