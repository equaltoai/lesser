package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// Remove duplicate constants - they are defined in constants.go

// SearchRepositoryDeps interface for dependencies - implemented by the storage adapter
type SearchRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error)
	IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error)
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

// SearchRepository implements search functionality using DynamORM
type SearchRepository struct {
	db     core.DB
	logger *zap.Logger
	deps   SearchRepositoryDeps
}

// NewSearchRepository creates a new search repository
func NewSearchRepository(db core.DB, logger *zap.Logger) *SearchRepository {
	return &SearchRepository{
		db:     db,
		logger: logger,
	}
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *SearchRepository) SetDependencies(deps SearchRepositoryDeps) {
	r.deps = deps
}

// SearchAccounts searches for accounts matching the given query
func (r *SearchRepository) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	// For now, redirect to advanced search with default parameters
	return r.SearchAccountsAdvanced(ctx, query, false, limit, offset, followingOnly, "")
}

// SearchAccountsWithPrivacy searches for accounts with privacy enforcement
func (r *SearchRepository) SearchAccountsWithPrivacy(ctx context.Context, query string, limit int, followingOnly bool, offset int, searcherActorID string) ([]*activitypub.Actor, error) {
	// First get basic results
	results, err := r.SearchAccountsAdvanced(ctx, query, false, limit*2, offset, followingOnly, searcherActorID) // Get more to account for filtering
	if err != nil {
		return nil, err
	}

	// Apply privacy filters
	if searcherActorID != "" && r.deps != nil {
		results = r.filterAccountsByPrivacy(ctx, results, searcherActorID)
	}

	// Apply limit after filtering
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SearchAccountsAdvanced searches for accounts with advanced filtering
func (r *SearchRepository) SearchAccountsAdvanced(ctx context.Context, query string, _ bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	normalizedQuery := r.normalizeSearchQuery(query)
	if len(normalizedQuery) < 2 {
		return []*activitypub.Actor{}, nil
	}

	// Collect results from different search strategies
	results, _ := r.executeSearchStrategies(ctx, normalizedQuery, limit, offset)

	// Apply following filter if requested
	if following && accountID != "" {
		results = r.applyFollowingFilter(ctx, results, accountID)
	}

	// Sort and paginate results
	r.sortAccountResults(results, normalizedQuery)
	return r.paginateResults(results, offset, limit), nil
}

// normalizeSearchQuery normalizes a search query for account searching
func (r *SearchRepository) normalizeSearchQuery(query string) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	return strings.TrimPrefix(normalized, "@")
}

// executeSearchStrategies runs all search strategies and collects unique results
func (r *SearchRepository) executeSearchStrategies(ctx context.Context, query string, limit, offset int) ([]*activitypub.Actor, map[string]bool) {
	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	// Strategy 1: Exact username match
	r.searchExactUsername(ctx, query, &results, seen)

	// Strategy 2: Username prefix search
	r.searchUsernamePrefix(ctx, query, limit, offset, &results, seen)

	// Strategy 3: Display name search
	r.searchDisplayName(ctx, query, limit, &results, seen)

	return results, seen
}

// searchExactUsername searches for exact username matches
func (r *SearchRepository) searchExactUsername(ctx context.Context, query string, results *[]*activitypub.Actor, seen map[string]bool) {
	if len(query) < 3 {
		return
	}

	var exactMatch models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s", query)).
		Where("SK", "=", "PROFILE").
		First(&exactMatch)

	if err == nil && exactMatch.Actor != nil && !seen[exactMatch.Actor.ID] {
		*results = append(*results, exactMatch.Actor)
		seen[exactMatch.Actor.ID] = true
	}
}

// searchUsernamePrefix searches for usernames starting with the query
func (r *SearchRepository) searchUsernamePrefix(ctx context.Context, query string, limit, offset int, results *[]*activitypub.Actor, seen map[string]bool) {
	prefixKey := query[:2]
	var prefixMatches []models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("username-search-index").
		Where("GSI1PK", "=", fmt.Sprintf("USERNAME_SEARCH#%s", prefixKey)).
		Filter("GSI1SK", "BEGINS_WITH", query).
		Limit(limit + offset).
		All(&prefixMatches)

	if err != nil {
		r.logger.Warn("username prefix search failed", zap.Error(err))
		return
	}

	for _, match := range prefixMatches {
		if match.Actor != nil && !seen[match.Actor.ID] {
			*results = append(*results, match.Actor)
			seen[match.Actor.ID] = true
		}
	}
}

// searchDisplayName searches for display names containing the query
func (r *SearchRepository) searchDisplayName(ctx context.Context, query string, limit int, results *[]*activitypub.Actor, seen map[string]bool) {
	if len(query) < 2 {
		return
	}

	displayNameKey := query[:2]
	var displayNameMatches []models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("display-name-index").
		Where("GSI2PK", "=", fmt.Sprintf("NAME_SEARCH#%s", displayNameKey)).
		Filter("GSI2SK", "BEGINS_WITH", query).
		Limit(limit).
		All(&displayNameMatches)

	if err != nil {
		r.logger.Warn("display name search failed", zap.Error(err))
		return
	}

	for _, match := range displayNameMatches {
		if match.Actor != nil && !seen[match.Actor.ID] {
			*results = append(*results, match.Actor)
			seen[match.Actor.ID] = true
		}
	}
}

// applyFollowingFilter filters results to only include followed accounts
func (r *SearchRepository) applyFollowingFilter(ctx context.Context, results []*activitypub.Actor, accountID string) []*activitypub.Actor {
	if r.deps == nil {
		r.logger.Debug("following filter requested but dependencies not set",
			zap.String("account_id", accountID))
		return results
	}

	followingList, _, err := r.deps.GetFollowing(ctx, accountID, 1000, "")
	if err != nil {
		r.logger.Warn("failed to get following list for filter",
			zap.String("account_id", accountID),
			zap.Error(err))
		return results
	}

	// Create a set for fast lookups
	followingSet := make(map[string]bool)
	for _, username := range followingList {
		followingSet[username] = true
	}

	// Filter results to only include followed accounts
	filteredResults := make([]*activitypub.Actor, 0)
	for _, actor := range results {
		if followingSet[actor.PreferredUsername] {
			filteredResults = append(filteredResults, actor)
		}
	}
	return filteredResults
}

// filterAccountsByPrivacy filters account results based on privacy rules
func (r *SearchRepository) filterAccountsByPrivacy(ctx context.Context, results []*activitypub.Actor, searcherActorID string) []*activitypub.Actor {
	filteredResults := make([]*activitypub.Actor, 0)

	for _, actor := range results {
		if actor == nil {
			continue
		}

		// Don't filter the searcher themselves
		if actor.ID == searcherActorID {
			filteredResults = append(filteredResults, actor)
			continue
		}

		// Check if there's a bidirectional block
		isBlocked, err := r.deps.IsBlockedBidirectional(ctx, searcherActorID, actor.ID)
		if err != nil {
			r.logger.Debug("failed to check block status in search",
				zap.String("searcher", searcherActorID),
				zap.String("target", actor.ID),
				zap.Error(err))
			// On error, include the result (fail open for search)
			filteredResults = append(filteredResults, actor)
			continue
		}

		// If blocked in either direction, exclude from results
		if !isBlocked {
			filteredResults = append(filteredResults, actor)
		}
	}

	r.logger.Debug("filtered search results by privacy",
		zap.Int("original_count", len(results)),
		zap.Int("filtered_count", len(filteredResults)),
		zap.String("searcher", searcherActorID))

	return filteredResults
}

// sortAccountResults sorts search results by relevance
func (r *SearchRepository) sortAccountResults(results []*activitypub.Actor, query string) {
	sort.Slice(results, func(i, j int) bool {
		return r.compareRelevance(results[i], results[j], query)
	})
}

// compareRelevance compares two actors for search relevance
func (r *SearchRepository) compareRelevance(a, b *activitypub.Actor, query string) bool {
	// Exact matches first
	aExact := strings.ToLower(a.PreferredUsername) == query
	bExact := strings.ToLower(b.PreferredUsername) == query
	if aExact != bExact {
		return aExact
	}

	// Then prefix matches
	aPrefix := strings.HasPrefix(strings.ToLower(a.PreferredUsername), query)
	bPrefix := strings.HasPrefix(strings.ToLower(b.PreferredUsername), query)
	if aPrefix != bPrefix {
		return aPrefix
	}

	// Then by username length (shorter is better)
	return len(a.PreferredUsername) < len(b.PreferredUsername)
}

// paginateResults applies offset and limit to results
func (r *SearchRepository) paginateResults(results []*activitypub.Actor, offset, limit int) []*activitypub.Actor {
	if offset >= len(results) {
		return []*activitypub.Actor{}
	}

	results = results[offset:]
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// SearchStatuses searches for statuses matching the given query
func (r *SearchRepository) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	// Use default options for simple search
	options := storage.StatusSearchOptions{
		Limit: limit,
	}
	return r.SearchStatusesWithOptions(ctx, query, options)
}

// SearchStatusesWithOptions searches for statuses with advanced options
func (r *SearchRepository) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	// Use new paginated method
	results, _, err := r.SearchStatusesWithOptionsPaginated(ctx, query, options)
	return results, err
}

// SearchStatusesWithOptionsPaginated searches for statuses with advanced options and pagination support
func (r *SearchRepository) SearchStatusesWithOptionsPaginated(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, *PaginationResult, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return []*storage.StatusSearchResult{}, CreatePaginationResult(false, "", 0), nil
	}

	// Convert storage options to pagination options
	paginationOpts := &PaginationOptions{
		Cursor: options.Cursor,
		Limit:  options.Limit,
	}

	// Map sort order
	switch options.SortOrder {
	case "time_asc":
		paginationOpts.SortOrder = SearchSortTimeAsc
	case "time_desc":
		paginationOpts.SortOrder = SearchSortTimeDesc
	case "relevance", "":
		paginationOpts.SortOrder = SearchSortRelevance
	default:
		paginationOpts.SortOrder = SearchSortRelevance
	}

	// Apply default options and validate
	if err := paginationOpts.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid pagination options: %w", err)
	}

	// Execute search strategies concurrently
	resultMap := r.executeStatusSearchStrategies(ctx, normalizedQuery)

	// Convert to slice and apply filters
	results := r.processSearchResults(resultMap, options)

	r.logger.Debug("status search results before sorting",
		zap.Int("result_count", len(results)),
		zap.String("sort_order", string(paginationOpts.SortOrder)))

	// Sort results based on specified sort order
	SortResults(results, paginationOpts.SortOrder,
		func(s *storage.StatusSearchResult) float64 { return s.Score },
		func(s *storage.StatusSearchResult) time.Time { return s.Published },
		func(s *storage.StatusSearchResult) string { return s.StatusID })

	// Apply pagination limits and determine if there are more results
	finalResults, hasNextPage := ApplyPaginationLimits(results, paginationOpts.Limit)

	// Create next cursor if there are more results
	var nextCursor string
	if hasNextPage && len(finalResults) > 0 {
		lastResult := finalResults[len(finalResults)-1]
		nextCursor = CreateNextCursor(
			nil, // No DynamoDB LastEvaluatedKey for in-memory sorting
			lastResult.Score,
			lastResult.Published,
			lastResult.StatusID,
			paginationOpts.SortOrder,
		)
	}

	paginationResultFinal := CreatePaginationResult(hasNextPage, nextCursor, len(results))

	r.logger.Info("status search completed",
		zap.Int("results", len(finalResults)),
		zap.Bool("has_next_page", hasNextPage),
		zap.Int("total_processed", len(results)))

	return finalResults, paginationResultFinal, nil
}

// SearchStatusesWithPrivacy searches for statuses with privacy enforcement
func (r *SearchRepository) SearchStatusesWithPrivacy(ctx context.Context, query string, options storage.StatusSearchOptions, searcherActorID string) ([]*storage.StatusSearchResult, error) {
	// Get results without privacy filtering first
	results, err := r.SearchStatusesWithOptions(ctx, query, options)
	if err != nil {
		return nil, err
	}

	// Apply privacy filters
	if searcherActorID != "" {
		results = r.filterStatusesByPrivacy(ctx, results, searcherActorID)
	}

	return results, nil
}

// SearchStatusesWithPrivacyPaginated searches for statuses with privacy enforcement and pagination
func (r *SearchRepository) SearchStatusesWithPrivacyPaginated(ctx context.Context, query string, options storage.StatusSearchOptions, searcherActorID string) ([]*storage.StatusSearchResult, *PaginationResult, error) {
	// Get results without privacy filtering first (get more to account for filtering)
	originalLimit := options.Limit
	options.Limit = originalLimit * 3 // Get more results to account for privacy filtering
	
	results, _, err := r.SearchStatusesWithOptionsPaginated(ctx, query, options)
	if err != nil {
		return nil, nil, err
	}

	// Apply privacy filters
	if searcherActorID != "" {
		results = r.filterStatusesByPrivacy(ctx, results, searcherActorID)
	}

	// Apply original limit after filtering
	finalResults := results
	hasNextPage := len(results) > originalLimit
	if hasNextPage {
		finalResults = results[:originalLimit]
	}

	// Update pagination to reflect filtering
	var nextCursor string
	if hasNextPage && len(finalResults) > 0 {
		lastResult := finalResults[len(finalResults)-1]
		nextCursor = CreateNextCursor(
			nil,
			lastResult.Score,
			lastResult.Published,
			lastResult.StatusID,
			SearchSortRelevance,
		)
	}

	finalPagination := CreatePaginationResult(hasNextPage, nextCursor, len(results))

	r.logger.Debug("search results after privacy filtering",
		zap.Int("original_count", len(results)+len(results)-len(finalResults)),
		zap.Int("filtered_count", len(finalResults)),
		zap.String("searcher", searcherActorID))

	return finalResults, finalPagination, nil
}

// filterStatusesByPrivacy filters status results based on privacy and visibility rules
func (r *SearchRepository) filterStatusesByPrivacy(ctx context.Context, results []*storage.StatusSearchResult, searcherActorID string) []*storage.StatusSearchResult {
	filteredResults := make([]*storage.StatusSearchResult, 0)

	for _, result := range results {
		if result == nil {
			continue
		}

		// Check if searcher is blocked by the author or has blocked the author
		if r.deps != nil {
			isBlocked, err := r.deps.IsBlockedBidirectional(ctx, searcherActorID, result.AuthorID)
			if err != nil {
				r.logger.Debug("failed to check block status for status search",
					zap.String("searcher", searcherActorID),
					zap.String("author", result.AuthorID),
					zap.Error(err))
				// On error, include the result (fail open)
				filteredResults = append(filteredResults, result)
				continue
			}

			// If blocked in either direction, exclude from results
			if isBlocked {
				r.logger.Debug("excluding status from search due to block",
					zap.String("status_id", result.StatusID),
					zap.String("author", result.AuthorID),
					zap.String("searcher", searcherActorID))
				continue
			}
		}

		// Check visibility - for now, only allow public and unlisted content in search
		// Private and direct messages should not appear in search results
		if r.isStatusPrivate(result) {
			r.logger.Debug("excluding private/direct status from search",
				zap.String("status_id", result.StatusID),
				zap.String("author", result.AuthorID))
			continue
		}

		// Status passed all privacy checks
		filteredResults = append(filteredResults, result)
	}

	r.logger.Debug("filtered status search results by privacy",
		zap.Int("original_count", len(results)),
		zap.Int("filtered_count", len(filteredResults)),
		zap.String("searcher", searcherActorID))

	return filteredResults
}

// isStatusPrivate checks if a status should be excluded from search due to privacy
func (r *SearchRepository) isStatusPrivate(_ *storage.StatusSearchResult) bool {
	// For now, we need to query the full status to check visibility
	// In a real implementation, you might want to include visibility in the search result
	// or index only public/unlisted content
	
	// We can make reasonable inferences from the content and context
	// Direct messages typically have very few recipients and specific patterns
	// Private posts are typically from followers-only accounts or have limited addressing
	
	// For safety, we'll be conservative and allow content that's likely public
	// This is a placeholder - ideally we'd have visibility info in the search index
	
	return false // Allow all content for now - this should be enhanced with proper visibility checking
}

// statusSearchResult wraps search result with sync protection
type statusSearchResult struct {
	mu        sync.Mutex
	resultMap map[string]*storage.StatusSearchResult
}

// executeStatusSearchStrategies runs all search strategies concurrently
func (r *SearchRepository) executeStatusSearchStrategies(ctx context.Context, query string) map[string]*storage.StatusSearchResult {
	result := &statusSearchResult{
		resultMap: make(map[string]*storage.StatusSearchResult),
	}

	var wg sync.WaitGroup

	// Execute different search strategies
	r.executeURLSearch(ctx, query, result, &wg)
	r.executeHashtagSearch(ctx, query, result, &wg)
	r.executeContentSearch(ctx, query, result, &wg)

	wg.Wait()
	return result.resultMap
}

// executeURLSearch searches for statuses by URL
func (r *SearchRepository) executeURLSearch(ctx context.Context, query string, result *statusSearchResult, wg *sync.WaitGroup) {
	if !r.isURL(query) {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.searchByURL(ctx, query, result)
	}()
}

// isURL checks if a query string is a URL
func (r *SearchRepository) isURL(query string) bool {
	return strings.HasPrefix(query, "http://") || strings.HasPrefix(query, "https://")
}

// searchByURL searches for a status by its URL
func (r *SearchRepository) searchByURL(ctx context.Context, url string, result *statusSearchResult) {
	var object models.Object
	err := r.db.WithContext(ctx).Model(&models.Object{}).
		Where("URL", "=", url).
		First(&object)

	if err == nil && object.Type == ActivityTypeNote {
		searchResult := r.objectToSearchResult(&object, 1.0, "url_match")
		result.mu.Lock()
		result.resultMap[object.ID] = searchResult
		result.mu.Unlock()
	}
}

// executeHashtagSearch searches for statuses by hashtags
func (r *SearchRepository) executeHashtagSearch(_ context.Context, query string, _ *statusSearchResult, wg *sync.WaitGroup) {
	hashtags := r.extractHashtags(query)
	if len(hashtags) == 0 {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.searchByHashtags(hashtags)
	}()
}

// searchByHashtags searches for statuses containing hashtags
func (r *SearchRepository) searchByHashtags(hashtags []string) {
	for _, hashtag := range hashtags {
		// Search for statuses with this hashtag
		// This would require a hashtag index or scanning status content
		// For now, we'll skip this strategy
		r.logger.Debug("hashtag search not fully implemented",
			zap.String("hashtag", hashtag))
	}
}

// executeContentSearch searches for statuses by content
func (r *SearchRepository) executeContentSearch(ctx context.Context, query string, result *statusSearchResult, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.searchByContent(ctx, query, result)
	}()
}

// searchByContent searches for statuses containing the query in their content
func (r *SearchRepository) searchByContent(ctx context.Context, query string, result *statusSearchResult) {
	recentStatuses := r.getRecentStatuses(ctx)

	for _, status := range recentStatuses {
		if r.statusMatchesQuery(status, query) {
			r.addStatusToResults(status, query, result)
		}
	}
}

// getRecentStatuses fetches recent status objects
func (r *SearchRepository) getRecentStatuses(ctx context.Context) []models.Object {
	var recentStatuses []models.Object
	err := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi2-index").
		Where("GSI2PK", "=", "object#type#Note").
		Limit(100). // Scan last 100 statuses
		All(&recentStatuses)

	if err != nil {
		r.logger.Warn("content search failed", zap.Error(err))
		return []models.Object{}
	}

	return recentStatuses
}

// statusMatchesQuery checks if a status matches the search query
func (r *SearchRepository) statusMatchesQuery(status models.Object, query string) bool {
	if status.Content == "" {
		return false
	}
	contentLower := strings.ToLower(status.Content)
	return strings.Contains(contentLower, query)
}

// addStatusToResults adds a status to the search results
func (r *SearchRepository) addStatusToResults(status models.Object, query string, result *statusSearchResult) {
	contentLower := strings.ToLower(status.Content)
	score := r.calculateContentScore(contentLower, query)
	searchResult := r.objectToSearchResult(&status, score, "content_match")

	result.mu.Lock()
	defer result.mu.Unlock()

	if existing, ok := result.resultMap[status.ID]; !ok || searchResult.Score > existing.Score {
		result.resultMap[status.ID] = searchResult
	}
}

// processSearchResults converts results map to slice and applies filters
func (r *SearchRepository) processSearchResults(resultMap map[string]*storage.StatusSearchResult, options storage.StatusSearchOptions) []*storage.StatusSearchResult {
	// Convert map to slice
	results := r.mapToSlice(resultMap)

	// Apply filters
	results = r.filterByAccount(results, options.AccountID)
	results = r.filterByLocality(results, options.LocalOnly)

	return results
}

// mapToSlice converts a map of results to a slice
func (r *SearchRepository) mapToSlice(resultMap map[string]*storage.StatusSearchResult) []*storage.StatusSearchResult {
	results := make([]*storage.StatusSearchResult, 0, len(resultMap))
	for _, result := range resultMap {
		results = append(results, result)
	}
	return results
}

// filterByAccount filters results by account ID
func (r *SearchRepository) filterByAccount(results []*storage.StatusSearchResult, accountID string) []*storage.StatusSearchResult {
	if accountID == "" {
		return results
	}

	filtered := make([]*storage.StatusSearchResult, 0)
	for _, result := range results {
		if result.AuthorID == accountID {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// filterByLocality filters results to only local statuses
func (r *SearchRepository) filterByLocality(results []*storage.StatusSearchResult, localOnly bool) []*storage.StatusSearchResult {
	if !localOnly {
		return results
	}

	filtered := make([]*storage.StatusSearchResult, 0)
	for _, result := range results {
		if r.isLocalStatus(result.StatusID) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// SearchStatusesAdvanced searches for statuses with advanced filtering
func (r *SearchRepository) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	options := storage.StatusSearchOptions{
		Limit:     limit,
		AccountID: accountID,
	}

	// Apply maxID/minID pagination filters
	if maxID != nil || minID != nil {
		// Get the search results first, then apply pagination
		allResults, err := r.SearchStatusesWithOptions(ctx, query, options)
		if err != nil {
			return nil, err
		}

		// Apply pagination filtering
		filteredResults := make([]*storage.StatusSearchResult, 0)
		for _, result := range allResults {
			// maxID filters to results older than the specified ID
			if maxID != nil && result.StatusID >= *maxID {
				continue
			}
			// minID filters to results newer than the specified ID
			if minID != nil && result.StatusID <= *minID {
				continue
			}
			filteredResults = append(filteredResults, result)
		}

		// Apply limit after filtering
		if len(filteredResults) > limit {
			filteredResults = filteredResults[:limit]
		}

		return filteredResults, nil
	}

	return r.SearchStatusesWithOptions(ctx, query, options)
}

// SearchAll performs a comprehensive search across accounts, statuses, and hashtags
func (r *SearchRepository) SearchAll(ctx context.Context, query string, limit int, _ string) (*storage.SearchResults, error) {
	// Use new paginated method with default options
	options := &PaginationOptions{
		Limit:     limit,
		SortOrder: SearchSortRelevance,
	}

	result, _, err := r.SearchAllPaginated(ctx, query, options)
	return result, err
}

// SearchAllPaginated performs a comprehensive search across accounts, statuses, and hashtags with pagination
func (r *SearchRepository) SearchAllPaginated(ctx context.Context, query string, options *PaginationOptions) (*storage.SearchResults, *PaginationResult, error) {
	if options == nil {
		options = NewPaginationOptions()
	}
	if err := options.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid pagination options: %w", err)
	}

	results := &storage.SearchResults{
		Accounts: make([]storage.Account, 0),
		Statuses: make([]storage.StatusSearchResult, 0),
		Hashtags: make([]storage.HashtagSearchResult, 0),
	}

	// Divide limit across different search types
	// Give most results to statuses as they're typically most requested
	statusLimit := options.Limit / 2
	accountLimit := options.Limit / 4
	hashtagLimit := options.Limit / 4

	// Ensure minimum limits
	if statusLimit < 5 {
		statusLimit = 5
	}
	if accountLimit < 2 {
		accountLimit = 2
	}
	if hashtagLimit < 2 {
		hashtagLimit = 2
	}

	var totalScanned int

	// Search accounts
	actors, err := r.SearchAccountsAdvanced(ctx, query, false, accountLimit, 0, false, "")
	if err != nil {
		r.logger.Warn("account search failed in SearchAll", zap.Error(err))
	} else {
		// Convert actors to accounts
		for _, actor := range actors {
			if actor != nil {
				account := storage.Account{
					Actor: actor,
				}
				results.Accounts = append(results.Accounts, account)
			}
		}
		totalScanned += len(actors)
	}

	// Search statuses with pagination support
	searchOptions := storage.StatusSearchOptions{
		Limit:     statusLimit,
		Cursor:    options.Cursor,
		SortOrder: string(options.SortOrder),
	}

	statusResults, statusPagination, err := r.SearchStatusesWithOptionsPaginated(ctx, query, searchOptions)
	if err != nil {
		r.logger.Warn("status search failed in SearchAll", zap.Error(err))
	} else {
		// Convert pointer slice to value slice
		for _, status := range statusResults {
			if status != nil {
				results.Statuses = append(results.Statuses, *status)
			}
		}
		totalScanned += statusPagination.TotalScanned
	}

	// Search hashtags
	hashtagResults, err := r.SearchHashtagsAdvanced(ctx, query, hashtagLimit, "")
	if err != nil {
		r.logger.Warn("hashtag search failed in SearchAll", zap.Error(err))
	} else {
		// Convert to HashtagSearchResult format
		for _, ht := range hashtagResults {
			results.Hashtags = append(results.Hashtags, storage.HashtagSearchResult{
				Name: ht.Name,
				URL:  ht.URL,
			})
		}
		totalScanned += len(hashtagResults)
	}

	// Determine if there are more results based on status search (primary content type)
	var hasNextPage bool
	var nextCursor string
	if statusPagination != nil {
		hasNextPage = statusPagination.HasNextPage
		nextCursor = statusPagination.NextCursor
	}

	// Update results with pagination metadata
	results.NextCursor = nextCursor
	results.HasNextPage = hasNextPage
	results.TotalScanned = totalScanned

	paginationResult := CreatePaginationResult(hasNextPage, nextCursor, totalScanned)

	r.logger.Info("comprehensive search completed",
		zap.Int("accounts", len(results.Accounts)),
		zap.Int("statuses", len(results.Statuses)),
		zap.Int("hashtags", len(results.Hashtags)),
		zap.Bool("has_next_page", hasNextPage),
		zap.Int("total_scanned", totalScanned))

	return results, paginationResult, nil
}

// SearchHashtags searches for hashtags matching the given query
func (r *SearchRepository) SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	normalizedQuery = strings.TrimPrefix(normalizedQuery, "#")

	if normalizedQuery == "" {
		return []*storage.Hashtag{}, nil
	}

	// Search hashtags using GSI
	var hashtags []models.Hashtag
	err := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Index("name-index").
		Where("GSI1PK", "=", "HASHTAG").
		Filter("GSI1SK", "BEGINS_WITH", normalizedQuery).
		Limit(limit).
		All(&hashtags)

	if err != nil {
		return nil, fmt.Errorf("failed to search hashtags: %w", err)
	}

	// Convert to storage format
	results := make([]*storage.Hashtag, 0, len(hashtags))
	for _, ht := range hashtags {
		results = append(results, &storage.Hashtag{
			Name:       ht.Name,
			URL:        "/tags/" + ht.Name,
			UsageCount: int(ht.UsageCount),
			FirstSeen:  ht.FirstSeen,
			LastUsed:   ht.LastUsed,
		})
	}

	return results, nil
}

// SearchHashtagsAdvanced searches for hashtags with advanced filtering
func (r *SearchRepository) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, _ string) ([]*storage.HashtagSearchResult, error) {
	// Use new paginated method with default options
	options := &PaginationOptions{
		Limit:     limit,
		SortOrder: SearchSortRelevance,
	}

	results, _, err := r.SearchHashtagsAdvancedPaginated(ctx, query, options)
	return results, err
}

// SearchHashtagsAdvancedPaginated searches for hashtags with advanced filtering and pagination support
func (r *SearchRepository) SearchHashtagsAdvancedPaginated(ctx context.Context, query string, options *PaginationOptions) ([]*storage.HashtagSearchResult, *PaginationResult, error) {
	if options == nil {
		options = NewPaginationOptions()
	}
	if err := options.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid pagination options: %w", err)
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	normalizedQuery = strings.TrimPrefix(normalizedQuery, "#")

	if normalizedQuery == "" {
		return []*storage.HashtagSearchResult{}, CreatePaginationResult(false, "", 0), nil
	}

	// Parse cursor for continuation
	cursorData, err := DecodeCursor(options.Cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor: %w", err)
	}

	// Use efficient cursor-based pagination with the hashtag search index
	var hashtags []models.Hashtag

	// Build query with proper cursor handling
	hashtagQuery := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Index("hashtag-search-index").
		Where("GSI3PK", "=", fmt.Sprintf("HASHTAG_SEARCH#%s", normalizedQuery[:2])).
		Filter("GSI3SK", "BEGINS_WITH", normalizedQuery).
		OrderBy("GSI3SK", "ASC")

	// Apply cursor if provided
	if cursorData.LastID != "" {
		hashtagQuery = hashtagQuery.Filter("GSI3SK", ">", cursorData.LastID)
	}

	// Execute query with limit + 1 to check for more results
	hashtagQuery = hashtagQuery.Limit(options.Limit + 1)
	err = hashtagQuery.All(&hashtags)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search hashtags: %w", err)
	}

	r.logger.Debug("hashtag search results",
		zap.Int("result_count", len(hashtags)),
		zap.String("query", normalizedQuery),
		zap.Bool("has_cursor", options.Cursor != ""))

	// Determine if there are more results and apply pagination
	hasNextPage := len(hashtags) > options.Limit
	if hasNextPage {
		hashtags = hashtags[:options.Limit] // Remove the extra item
	}

	// Convert hashtags to search results
	results := make([]*storage.HashtagSearchResult, len(hashtags))
	for i, ht := range hashtags {
		results[i] = &storage.HashtagSearchResult{
			Name: ht.Name,
			URL:  "/tags/" + ht.Name,
			Uses: int(ht.UsageCount),
			// History would need to be populated from trending data
		}
	}

	// Sort results based on specified sort order
	SortResults(results, options.SortOrder,
		func(h *storage.HashtagSearchResult) float64 { return float64(h.Uses) }, // Use usage count as score
		func(_ *storage.HashtagSearchResult) time.Time { return time.Now() },    // No timestamp field, use current time
		func(h *storage.HashtagSearchResult) string { return h.Name })

	// Create next cursor if there are more results
	var nextCursor string
	if hasNextPage && len(results) > 0 {
		lastResult := results[len(results)-1]
		nextCursor = CreateNextCursor(
			nil, // No DynamoDB LastEvaluatedKey needed for this query type
			float64(lastResult.Uses),
			time.Now(), // No timestamp field available
			lastResult.Name,
			options.SortOrder,
		)
	}

	paginationResultFinal := CreatePaginationResult(hasNextPage, nextCursor, len(hashtags))

	r.logger.Info("hashtag search completed",
		zap.Int("results", len(results)),
		zap.Bool("has_next_page", hasNextPage),
		zap.Int("total_processed", len(hashtags)))

	return results, paginationResultFinal, nil
}

// Helper methods

func (r *SearchRepository) objectToSearchResult(obj *models.Object, score float64, strategy string) *storage.StatusSearchResult {
	if obj == nil || obj.Type != ActivityTypeNote {
		return nil
	}

	// Extract author username from AttributedTo
	authorUsername := r.extractUsername(obj.AttributedTo)

	return &storage.StatusSearchResult{
		StatusID:       obj.ID,
		Content:        obj.Content,
		URL:            obj.URL,
		AuthorID:       obj.AttributedTo,
		AuthorUsername: authorUsername,
		Published:      obj.Published,
		Score:          score,
		Highlights:     []string{strategy},
	}
}

func (r *SearchRepository) calculateContentScore(content, query string) float64 {
	// Simple scoring based on match position and frequency
	contentLower := strings.ToLower(content)
	queryLower := strings.ToLower(query)

	score := 0.0

	// Exact match bonus
	if contentLower == queryLower {
		return 1.0
	}

	// Count occurrences
	count := strings.Count(contentLower, queryLower)
	score += float64(count) * 0.1

	// Position bonus (earlier is better)
	pos := strings.Index(contentLower, queryLower)
	if pos >= 0 {
		positionScore := 1.0 - (float64(pos) / float64(len(content)))
		score += positionScore * 0.3
	}

	// Length ratio (prefer shorter content for same match)
	lengthRatio := float64(len(query)) / float64(len(content))
	score += lengthRatio * 0.2

	// Cap at 0.9 (reserve 1.0 for exact matches)
	return math.Min(score, 0.9)
}

func (r *SearchRepository) extractHashtags(text string) []string {
	hashtags := make([]string, 0)
	words := strings.Fields(text)

	for _, word := range words {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			hashtag := strings.TrimPrefix(word, "#")
			hashtag = strings.ToLower(hashtag)
			hashtags = append(hashtags, hashtag)
		}
	}

	return hashtags
}

func (r *SearchRepository) extractUsername(actorID string) string {
	// Extract username from actor ID
	// Format: https://domain.com/users/username -> username
	if strings.Contains(actorID, "/users/") {
		parts := strings.Split(actorID, "/users/")
		if len(parts) == 2 {
			return parts[1]
		}
	}

	// For simple usernames
	if !strings.Contains(actorID, "://") {
		return actorID
	}

	return ""
}

func (r *SearchRepository) isLocalStatus(statusID string) bool {
	// Check if status is from local instance
	// This is a simplified check - would need domain configuration
	return !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://")
}

// Search Suggestion Methods

// CreateSearchSuggestion creates a new search suggestion
func (r *SearchRepository) CreateSearchSuggestion(ctx context.Context, suggestion *models.SearchSuggestion) error {
	if suggestion == nil {
		return fmt.Errorf("suggestion cannot be nil")
	}

	// Set timestamps
	now := time.Now()
	suggestion.CreatedAt = now
	suggestion.UpdatedAt = now
	suggestion.LastUsed = now

	// Update keys
	suggestion.UpdateKeys()

	// Create the suggestion
	err := r.db.WithContext(ctx).Model(suggestion).Create()
	if err != nil {
		r.logger.Error("failed to create search suggestion",
			zap.String("type", suggestion.Type),
			zap.String("term", suggestion.Term),
			zap.Error(err))
		return fmt.Errorf("failed to create search suggestion: %w", err)
	}

	return nil
}

// UpdateSearchSuggestion updates an existing search suggestion
func (r *SearchRepository) UpdateSearchSuggestion(ctx context.Context, suggestionType, term string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	// Add updated timestamp
	updates["updated_at"] = time.Now()

	// Build and update the suggestion model
	suggestion := &models.SearchSuggestion{
		Type: suggestionType,
		Term: term,
	}
	suggestion.UpdateKeys()

	// Apply updates to the model
	if score, ok := updates["score"].(float64); ok {
		suggestion.Score = score
	}
	if lastUsed, ok := updates["last_used"].(time.Time); ok {
		suggestion.LastUsed = lastUsed
	}
	if useCount, ok := updates["use_count"].(int); ok {
		suggestion.UseCount = useCount
	}
	if updatedAt, ok := updates["updated_at"].(time.Time); ok {
		suggestion.UpdatedAt = updatedAt
	}

	// Execute update
	err := r.db.WithContext(ctx).Model(suggestion).Update()
	if err != nil {
		r.logger.Error("failed to update search suggestion",
			zap.String("type", suggestionType),
			zap.String("term", term),
			zap.Error(err))
		return fmt.Errorf("failed to update search suggestion: %w", err)
	}

	return nil
}

// GetSearchSuggestions retrieves search suggestions based on prefix
func (r *SearchRepository) GetSearchSuggestions(ctx context.Context, prefix string, limit int) ([]*models.SearchSuggestion, error) {
	if len(prefix) < 2 {
		return []*models.SearchSuggestion{}, nil
	}

	normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	suggestions := make([]*models.SearchSuggestion, 0)
	seen := make(map[string]bool)

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Search usernames in USERNAME_SEARCH index
	wg.Add(1)
	go func() {
		defer wg.Done()

		prefixKey := normalizedPrefix[:2]
		var usernameSuggestions []models.SearchSuggestion

		err := r.db.WithContext(ctx).Model(&models.SearchSuggestion{}).
			Index("username-search-index").
			Where("GSI1PK", "=", fmt.Sprintf("USERNAME_SEARCH#%s", prefixKey)).
			Filter("GSI1SK", "BEGINS_WITH", normalizedPrefix).
			Limit(limit).
			All(&usernameSuggestions)

		if err != nil {
			r.logger.Warn("username search suggestions failed", zap.Error(err))
			return
		}

		mu.Lock()
		for _, sugg := range usernameSuggestions {
			key := fmt.Sprintf("%s:%s", sugg.Type, sugg.Term)
			if !seen[key] {
				suggestions = append(suggestions, &sugg)
				seen[key] = true
			}
		}
		mu.Unlock()
	}()

	// Search display names in NAME_SEARCH index
	wg.Add(1)
	go func() {
		defer wg.Done()

		if len(normalizedPrefix) >= 2 {
			prefixKey := normalizedPrefix[:2]
			var nameSuggestions []models.SearchSuggestion

			err := r.db.WithContext(ctx).Model(&models.SearchSuggestion{}).
				Index("name-search-index").
				Where("GSI2PK", "=", fmt.Sprintf("NAME_SEARCH#%s", prefixKey)).
				Filter("GSI2SK", "BEGINS_WITH", normalizedPrefix).
				Limit(limit).
				All(&nameSuggestions)

			if err != nil {
				r.logger.Warn("name search suggestions failed", zap.Error(err))
				return
			}

			mu.Lock()
			for _, sugg := range nameSuggestions {
				key := fmt.Sprintf("%s:%s", sugg.Type, sugg.Term)
				if !seen[key] {
					suggestions = append(suggestions, &sugg)
					seen[key] = true
				}
			}
			mu.Unlock()
		}
	}()

	// Search hashtag suggestions
	wg.Add(1)
	go func() {
		defer wg.Done()

		var hashtagSuggestions []models.SearchSuggestion

		err := r.db.WithContext(ctx).Model(&models.SearchSuggestion{}).
			Where("PK", "=", "SEARCH_SUGGEST#hashtag").
			Filter("SK", "BEGINS_WITH", normalizedPrefix).
			Limit(limit).
			All(&hashtagSuggestions)

		if err != nil {
			r.logger.Warn("hashtag search suggestions failed", zap.Error(err))
			return
		}

		mu.Lock()
		for _, sugg := range hashtagSuggestions {
			key := fmt.Sprintf("%s:%s", sugg.Type, sugg.Term)
			if !seen[key] {
				suggestions = append(suggestions, &sugg)
				seen[key] = true
			}
		}
		mu.Unlock()
	}()

	wg.Wait()

	// Sort by score and last used
	sort.Slice(suggestions, func(i, j int) bool {
		// Higher score wins
		if suggestions[i].Score != suggestions[j].Score {
			return suggestions[i].Score > suggestions[j].Score
		}
		// More recently used wins
		return suggestions[i].LastUsed.After(suggestions[j].LastUsed)
	})

	// Apply limit
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	return suggestions, nil
}

// IncrementSuggestionUse increments the use count for a suggestion
func (r *SearchRepository) IncrementSuggestionUse(ctx context.Context, suggestionType, term string) error {
	// First, get the current suggestion
	var suggestion models.SearchSuggestion
	err := r.db.WithContext(ctx).Model(&models.SearchSuggestion{}).
		Where("PK", "=", fmt.Sprintf("SEARCH_SUGGEST#%s", suggestionType)).
		Where("SK", "=", term).
		First(&suggestion)

	if err != nil {
		// If not found, create a new suggestion
		if errors.IsNotFound(err) {
			suggestion = models.SearchSuggestion{
				Type:      suggestionType,
				Term:      term,
				UseCount:  1,
				Score:     0.1,
				LastUsed:  time.Now(),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			suggestion.UpdateKeys()
			return r.db.WithContext(ctx).Model(&suggestion).Create()
		}
		return fmt.Errorf("failed to get suggestion: %w", err)
	}

	// Update the suggestion
	suggestion.UseCount++
	suggestion.Score += 0.1
	suggestion.LastUsed = time.Now()
	suggestion.UpdatedAt = time.Now()

	return r.db.WithContext(ctx).Model(&suggestion).Update()
}

// PruneOldSuggestions removes suggestions older than the specified time
func (r *SearchRepository) PruneOldSuggestions(ctx context.Context, olderThan time.Time) error {
	// Query old suggestions
	var oldSuggestions []models.SearchSuggestion

	err := r.db.WithContext(ctx).Model(&models.SearchSuggestion{}).
		Filter("last_used", "<", olderThan).
		All(&oldSuggestions)

	if err != nil {
		return fmt.Errorf("failed to query old suggestions: %w", err)
	}

	// Delete in batches
	for _, suggestion := range oldSuggestions {
		err = r.db.WithContext(ctx).Model(&models.SearchSuggestion{}).
			Where("PK", "=", suggestion.PK).
			Where("SK", "=", suggestion.SK).
			Delete()

		if err != nil {
			r.logger.Error("failed to delete old suggestion",
				zap.String("pk", suggestion.PK),
				zap.String("sk", suggestion.SK),
				zap.Error(err))
		}
	}

	r.logger.Info("pruned old search suggestions",
		zap.Time("older_than", olderThan),
		zap.Int("count", len(oldSuggestions)))

	return nil
}

// Status Search Methods

// IndexStatus indexes a status for search
func (r *SearchRepository) IndexStatus(_ context.Context, status *models.Object) error {
	if status == nil || status.Type != ActivityTypeNote {
		return nil
	}

	// Extract hashtags from content
	hashtags := r.extractHashtags(status.Content)

	// Index by hashtag
	for _, hashtag := range hashtags {
		// Create a search index entry for hashtag
		// This would typically be handled by a separate indexing service
		r.logger.Debug("would index status by hashtag",
			zap.String("status_id", status.ID),
			zap.String("hashtag", hashtag))
	}

	// Index by author - this is handled by GSI2 on Object model

	return nil
}

// UnindexStatus removes a status from search indexes
func (r *SearchRepository) UnindexStatus(ctx context.Context, statusID string) error {
	r.logger.Debug("removing status from search indexes",
		zap.String("status_id", statusID))

	// Remove any search embeddings for this status
	var embeddings []models.SearchEmbedding
	err := r.db.WithContext(ctx).Model(&models.SearchEmbedding{}).
		Where("PK", "=", fmt.Sprintf("EMBEDDING#STATUS#%s", statusID)).
		All(&embeddings)

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to query embeddings for deletion: %w", err)
	}

	// Delete embeddings in batch if they exist
	if len(embeddings) > 0 {
		keys := make([]any, len(embeddings))
		for i, embedding := range embeddings {
			keys[i] = &models.SearchEmbedding{
				PK: embedding.PK,
				SK: embedding.SK,
			}
		}

		err = r.db.WithContext(ctx).Model(&models.SearchEmbedding{}).BatchDelete(keys)
		if err != nil {
			return fmt.Errorf("failed to batch delete embeddings: %w", err)
		}

		r.logger.Debug("removed embeddings for status",
			zap.String("status_id", statusID),
			zap.Int("embedding_count", len(embeddings)))
	}

	// Remove from trending if present
	trendingRecord := &models.TrendingStatus{
		PK: fmt.Sprintf("TRENDING#STATUS#%s", statusID),
		SK: "TRENDING",
	}

	err = r.db.WithContext(ctx).Model(trendingRecord).Delete()
	if err != nil && !errors.IsNotFound(err) {
		// Log but don't fail - trending removal is not critical
		r.logger.Warn("failed to remove status from trending",
			zap.String("status_id", statusID),
			zap.Error(err))
	}

	// Remove from search cache entries that reference this status
	var cacheEntries []models.SearchCache
	err = r.db.WithContext(ctx).Model(&models.SearchCache{}).
		Filter("StatusIDs", "contains", statusID).
		Limit(50). // Limit cache cleanup
		All(&cacheEntries)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to query search cache for cleanup",
			zap.String("status_id", statusID),
			zap.Error(err))
	} else if len(cacheEntries) > 0 {
		// Update cache entries to remove this status ID
		for _, cacheEntry := range cacheEntries {
			// Remove statusID from the Results array
			if statusIDs, ok := cacheEntry.Results["status_ids"].([]string); ok {
				updatedIDs := make([]string, 0)
				for _, id := range statusIDs {
					if id != statusID {
						updatedIDs = append(updatedIDs, id)
					}
				}
				cacheEntry.Results["status_ids"] = updatedIDs
				cacheEntry.Results["updated_at"] = time.Now()
			}

			// Update the cache entry
			err = r.db.WithContext(ctx).Model(&cacheEntry).Update()
			if err != nil {
				r.logger.Warn("failed to update search cache entry",
					zap.String("cache_key", cacheEntry.PK),
					zap.Error(err))
			}
		}

		r.logger.Debug("updated search cache entries",
			zap.String("status_id", statusID),
			zap.Int("cache_entries", len(cacheEntries)))
	}

	r.logger.Info("successfully removed status from search indexes",
		zap.String("status_id", statusID))

	return nil
}

// SearchStatusesByHashtag searches for statuses containing a specific hashtag using efficient indexing
func (r *SearchRepository) SearchStatusesByHashtag(ctx context.Context, hashtag string, limit int) ([]*storage.StatusSearchResult, error) {
	normalizedHashtag := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	results := make([]*storage.StatusSearchResult, 0)

	// Use the new HashtagStatusIndex for efficient hashtag timeline queries
	var hashtagIndex []models.HashtagStatusIndex

	// Query the hashtag timeline index directly - this is much more efficient than scanning all statuses
	err := r.db.WithContext(ctx).Model(&models.HashtagStatusIndex{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG_TIMELINE#%s", normalizedHashtag)).
		Where("SK", "BEGINS_WITH", "STATUS#").
		OrderBy("SK", "DESC"). // Newest first (due to timestamp desc encoding)
		Limit(limit).
		All(&hashtagIndex)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.StatusSearchResult{}, nil
		}
		return nil, fmt.Errorf("failed to search statuses by hashtag: %w", err)
	}

	// Convert hashtag index entries to search results
	for _, indexEntry := range hashtagIndex {
		result := &storage.StatusSearchResult{
			StatusID:       indexEntry.StatusID,
			Content:        indexEntry.Content,
			URL:            indexEntry.StatusURL,
			AuthorID:       indexEntry.AuthorID,
			AuthorUsername: indexEntry.AuthorHandle,
			Published:      indexEntry.Published,
			Score:          1.0, // Exact hashtag match
			Highlights:     []string{"hashtag_match"},
		}
		results = append(results, result)
	}

	r.logger.Debug("hashtag search completed",
		zap.String("hashtag", normalizedHashtag),
		zap.Int("results", len(results)),
		zap.Int("limit", limit))

	return results, nil
}

// SearchStatusesByAuthor searches for statuses by a specific author
func (r *SearchRepository) SearchStatusesByAuthor(ctx context.Context, authorID string, limit int) ([]*storage.StatusSearchResult, error) {
	results := make([]*storage.StatusSearchResult, 0)

	// Query statuses by author using GSI4
	var statuses []models.Object

	err := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi4-index").
		Where("GSI4PK", "=", fmt.Sprintf("author#%s", authorID)).
		Filter("Type", "=", ActivityTypeNote).
		Limit(limit).
		All(&statuses)

	if err != nil {
		return nil, fmt.Errorf("failed to search statuses by author: %w", err)
	}

	// Convert to search results
	for _, status := range statuses {
		result := r.objectToSearchResult(&status, 1.0, "author_match")
		if result != nil {
			results = append(results, result)
		}
	}

	return results, nil
}

// Analytics Methods

// RecordSearch records a search event for analytics
func (r *SearchRepository) RecordSearch(ctx context.Context, event *models.SearchAnalytics) error {
	if event == nil {
		return fmt.Errorf("search event cannot be nil")
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Privacy-preserve the analytics event
	r.privacyFilterAnalyticsEvent(event)

	// Update keys
	event.UpdateKeys()

	// Create the analytics record
	err := r.db.WithContext(ctx).Model(event).Create()
	if err != nil {
		r.logger.Error("failed to record search analytics",
			zap.String("query_hash", r.hashQuery(event.Query)),
			zap.Error(err))
		return fmt.Errorf("failed to record search analytics: %w", err)
	}

	return nil
}

// RecordSearchWithPrivacy records a privacy-safe search event
func (r *SearchRepository) RecordSearchWithPrivacy(ctx context.Context, query, searchType string, resultCount int, searchTimeMs int64, userID *string) error {
	event := &models.SearchAnalytics{
		Query:       query,
		ResultCount: resultCount,
		SearchTime:  searchTimeMs,
		UserID:      userID,
		Timestamp:   time.Now(),
		SearchType:  searchType,
	}

	return r.RecordSearch(ctx, event)
}

// privacyFilterAnalyticsEvent applies privacy filters to analytics events
func (r *SearchRepository) privacyFilterAnalyticsEvent(event *models.SearchAnalytics) {
	// Hash or truncate sensitive queries
	if r.isSensitiveQuery(event.Query) {
		event.Query = r.hashQuery(event.Query)
	}

	// Limit query length for privacy
	if len(event.Query) > 100 {
		event.Query = event.Query[:97] + "..."
	}

	// Remove user ID for certain query types that might be personal
	if r.isPersonalQuery(event.Query) {
		event.UserID = nil
	}
}

// isSensitiveQuery checks if a query contains sensitive information
func (r *SearchRepository) isSensitiveQuery(query string) bool {
	query = strings.ToLower(query)
	
	// Check for email patterns
	if strings.Contains(query, "@") && strings.Contains(query, ".") {
		return true
	}

	// Check for potential personal identifiers
	sensitivePatterns := []string{
		"password", "token", "key", "secret", "private",
		"phone", "email", "address", "ssn", "credit",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(query, pattern) {
			return true
		}
	}

	return false
}

// isPersonalQuery checks if a query might be personal/private
func (r *SearchRepository) isPersonalQuery(query string) bool {
	query = strings.ToLower(query)
	
	// Queries that might be personal searches
	personalIndicators := []string{
		"direct", "private", "dm", "message",
		"block", "mute", "report",
	}

	for _, indicator := range personalIndicators {
		if strings.Contains(query, indicator) {
			return true
		}
	}

	return false
}

// hashQuery creates a privacy-safe hash of the query
func (r *SearchRepository) hashQuery(query string) string {
	// Simple hash for analytics while preserving some structure
	if len(query) == 0 {
		return "[empty]"
	}
	
	if len(query) <= 3 {
		return "[short]"
	}
	
	// Return first 2 chars + hash of the rest for trending analysis
	return fmt.Sprintf("%s_hash_%d", query[:2], len(query))
}

// GetSearchAnalytics retrieves search analytics for a date range
func (r *SearchRepository) GetSearchAnalytics(ctx context.Context, startDate, endDate time.Time) ([]*models.SearchAnalytics, error) {
	analytics := make([]*models.SearchAnalytics, 0)

	// Query each day in the range
	current := startDate
	for !current.After(endDate) {
		dateStr := current.Format(common.DateFormat)
		var dayAnalytics []models.SearchAnalytics

		err := r.db.WithContext(ctx).Model(&models.SearchAnalytics{}).
			Where("PK", "=", fmt.Sprintf("SEARCH_LOG#%s", dateStr)).
			All(&dayAnalytics)

		if err != nil {
			r.logger.Warn("failed to get analytics for date",
				zap.String("date", dateStr),
				zap.Error(err))
		} else {
			for i := range dayAnalytics {
				analytics = append(analytics, &dayAnalytics[i])
			}
		}

		current = current.AddDate(0, 0, 1)
	}

	// Sort by timestamp
	sort.Slice(analytics, func(i, j int) bool {
		return analytics[i].Timestamp.Before(analytics[j].Timestamp)
	})

	return analytics, nil
}

// GetPopularSearches retrieves popular search queries (privacy-safe)
func (r *SearchRepository) GetPopularSearches(ctx context.Context, limit int, timeWindow time.Duration) ([]*models.SearchQueryStats, error) {
	// Get analytics for time window
	endDate := time.Now()
	startDate := endDate.Add(-timeWindow)

	analytics, err := r.GetSearchAnalytics(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Aggregate by query, filtering out sensitive/personal queries
	queryCount := make(map[string]int64)
	lastQueried := make(map[string]time.Time)

	for _, event := range analytics {
		// Skip sensitive queries from trending
		if r.isSensitiveQuery(event.Query) || r.isPersonalQuery(event.Query) {
			continue
		}

		// Only include queries with sufficient activity to preserve privacy
		queryCount[event.Query]++
		if event.Timestamp.After(lastQueried[event.Query]) {
			lastQueried[event.Query] = event.Timestamp
		}
	}

	// Filter out queries with low counts to preserve privacy
	minCount := int64(5) // Minimum query count to appear in trending
	filtered := make(map[string]int64)
	for query, count := range queryCount {
		if count >= minCount {
			filtered[query] = count
		}
	}

	// Convert to SearchQueryStats
	stats := make([]*models.SearchQueryStats, 0, len(filtered))
	for query, count := range filtered {
		stat := &models.SearchQueryStats{
			QueryHash:   r.hashQuery(query), // Store hash for privacy
			QueryCount:  count,
			LastQueried: lastQueried[query],
			QueryType:   "search_suggestion",
		}
		stat.UpdateKeys()
		stats = append(stats, stat)
	}

	// Sort by count
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].QueryCount > stats[j].QueryCount
	})

	// Apply limit
	if len(stats) > limit {
		stats = stats[:limit]
	}

	r.logger.Info("generated popular searches",
		zap.Int("total_queries", len(queryCount)),
		zap.Int("filtered_queries", len(filtered)),
		zap.Int("result_count", len(stats)))

	return stats, nil
}

// GetSearchTrends retrieves search trends over days
func (r *SearchRepository) GetSearchTrends(ctx context.Context, days int) (map[string]int, error) {
	trends := make(map[string]int)

	// Get analytics for the period
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	analytics, err := r.GetSearchAnalytics(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Count searches by day
	for _, event := range analytics {
		day := event.Timestamp.Format(common.DateFormat)
		trends[day]++
	}

	return trends, nil
}

// Semantic Search Methods

// IndexContentEmbedding indexes content with its embedding vector
func (r *SearchRepository) IndexContentEmbedding(ctx context.Context, embedding *models.SearchEmbedding) error {
	if embedding == nil {
		return fmt.Errorf("embedding cannot be nil")
	}

	// Set created time
	if embedding.CreatedAt.IsZero() {
		embedding.CreatedAt = time.Now()
	}

	// Update keys
	embedding.UpdateKeys()

	// Create the embedding
	err := r.db.WithContext(ctx).Model(embedding).Create()
	if err != nil {
		r.logger.Error("failed to index content embedding",
			zap.String("content_id", embedding.ContentID),
			zap.Error(err))
		return fmt.Errorf("failed to index content embedding: %w", err)
	}

	return nil
}

// SearchByEmbedding searches for similar content using vector similarity
func (r *SearchRepository) SearchByEmbedding(ctx context.Context, queryEmbedding []float32, limit int, threshold float64) ([]*models.SearchEmbedding, error) {
	// Use new paginated method with default options
	options := NewPaginationOptions()
	options.Limit = limit

	result, _, err := r.SearchByEmbeddingPaginated(ctx, queryEmbedding, threshold, options)
	return result, err
}

// SearchByEmbeddingPaginated searches for similar content using vector similarity with pagination support
func (r *SearchRepository) SearchByEmbeddingPaginated(ctx context.Context, queryEmbedding []float32, threshold float64, options *PaginationOptions) ([]*models.SearchEmbedding, *PaginationResult, error) {
	if err := r.validateSearchParams(queryEmbedding, &threshold, &options); err != nil {
		return nil, nil, err
	}

	r.logSearchStart(queryEmbedding, threshold, options)

	cursorData, err := DecodeCursor(options.Cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor: %w", err)
	}

	baseQuery := r.buildBaseQuery(ctx, cursorData)
	allResults, processed := r.searchInBatches(ctx, baseQuery, queryEmbedding, threshold, options)

	r.logger.Debug("completed embedding search scan",
		zap.Int("processed", processed),
		zap.Int("candidates", len(allResults)))

	finalResults, paginationResult := r.finalizePaginatedResults(allResults, options, processed)

	r.logSearchCompletion(finalResults, paginationResult, processed)

	return finalResults, paginationResult, nil
}

// validateSearchParams validates and normalizes search parameters
func (r *SearchRepository) validateSearchParams(queryEmbedding []float32, threshold *float64, options **PaginationOptions) error {
	if len(queryEmbedding) == 0 {
		return fmt.Errorf("query embedding cannot be empty")
	}

	if *threshold < 0 || *threshold > 1 {
		*threshold = 0.7 // Default threshold for semantic similarity
	}

	if *options == nil {
		*options = NewPaginationOptions()
	}
	if err := (*options).Validate(); err != nil {
		return fmt.Errorf("invalid pagination options: %w", err)
	}

	return nil
}

// logSearchStart logs the beginning of the search operation
func (r *SearchRepository) logSearchStart(queryEmbedding []float32, threshold float64, options *PaginationOptions) {
	r.logger.Debug("searching by embedding with pagination",
		zap.Int("embedding_dim", len(queryEmbedding)),
		zap.Float64("threshold", threshold),
		zap.Int("limit", options.Limit),
		zap.String("cursor", options.Cursor),
		zap.String("sort_order", string(options.SortOrder)))
}

// buildBaseQuery constructs the base database query with cursor constraints
func (r *SearchRepository) buildBaseQuery(ctx context.Context, cursorData *CursorData) interface{} {
	baseQuery := r.db.WithContext(ctx).Model(&models.SearchEmbedding{}).
		OrderBy("CreatedAt", "DESC")

	if cursorData.LastID != "" {
		baseQuery = baseQuery.Filter("ContentID", ">", cursorData.LastID)
	}
	if !cursorData.LastTimestamp.IsZero() {
		baseQuery = baseQuery.Filter("CreatedAt", "<", cursorData.LastTimestamp.Format(time.RFC3339))
	}

	return baseQuery
}

// searchInBatches processes embeddings in batches to find similar results
func (r *SearchRepository) searchInBatches(_ context.Context, baseQuery interface{}, queryEmbedding []float32, threshold float64, options *PaginationOptions) ([]*models.SearchEmbedding, int) {
	var allResults []*models.SearchEmbedding
	maxScan := 500
	batchSize := 100
	currentBatch := 0
	processed := 0

	for len(allResults) < options.Limit && currentBatch < maxScan/batchSize {
		embeddings, shouldBreak, err := r.processBatch(baseQuery, currentBatch, batchSize)
		if err != nil {
			r.logger.Error("failed to process batch", zap.Error(err))
			break
		}
		if shouldBreak {
			break
		}

		batchResults := r.evaluateBatchSimilarity(embeddings, queryEmbedding, threshold, &processed)
		allResults = append(allResults, batchResults...)

		if r.shouldEarlyTerminate(allResults, options) {
			break
		}

		currentBatch++
		if len(embeddings) < batchSize {
			break
		}
	}

	return allResults, processed
}

// processBatch handles fetching a single batch of embeddings
func (r *SearchRepository) processBatch(baseQuery interface{}, currentBatch, batchSize int) ([]models.SearchEmbedding, bool, error) {
	var embeddings []models.SearchEmbedding

	// Type assert to get the actual query
	query := baseQuery.(interface {
		Limit(int) interface{}
		Offset(int) interface{}
		All(interface{}) error
	})
	batchQuery := query.Limit(batchSize)
	if currentBatch > 0 {
		batchQuery = batchQuery.(interface{ Offset(int) interface{} }).Offset(currentBatch * batchSize)
	}

	err := batchQuery.(interface{ All(interface{}) error }).All(&embeddings)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("failed to search embeddings batch %d: %w", currentBatch, err)
	}

	if len(embeddings) == 0 {
		return nil, true, nil
	}

	return embeddings, false, nil
}

// evaluateBatchSimilarity calculates similarity for embeddings in a batch
func (r *SearchRepository) evaluateBatchSimilarity(embeddings []models.SearchEmbedding, queryEmbedding []float32, threshold float64, processed *int) []*models.SearchEmbedding {
	var batchResults []*models.SearchEmbedding

	for _, embedding := range embeddings {
		*processed++

		if !r.isValidEmbedding(embedding, queryEmbedding) {
			continue
		}

		similarity := r.cosineSimilarity(queryEmbedding, embedding.Embedding)
		if similarity >= threshold {
			embedding.Score = similarity
			batchResults = append(batchResults, &embedding)
		}
	}

	return batchResults
}

// isValidEmbedding checks if an embedding has matching dimensions
func (r *SearchRepository) isValidEmbedding(embedding models.SearchEmbedding, queryEmbedding []float32) bool {
	if len(embedding.Embedding) != len(queryEmbedding) {
		r.logger.Debug("embedding dimension mismatch",
			zap.String("content_id", embedding.ContentID),
			zap.Int("expected", len(queryEmbedding)),
			zap.Int("actual", len(embedding.Embedding)))
		return false
	}
	return true
}

// shouldEarlyTerminate determines if search should stop early
func (r *SearchRepository) shouldEarlyTerminate(allResults []*models.SearchEmbedding, options *PaginationOptions) bool {
	return len(allResults) >= options.Limit*2
}

// finalizePaginatedResults sorts and applies pagination to final results
func (r *SearchRepository) finalizePaginatedResults(allResults []*models.SearchEmbedding, options *PaginationOptions, processed int) ([]*models.SearchEmbedding, *PaginationResult) {
	// Sort results based on specified sort order
	SortResults(allResults, options.SortOrder,
		func(e *models.SearchEmbedding) float64 { return e.Score },
		func(e *models.SearchEmbedding) time.Time { return e.CreatedAt },
		func(e *models.SearchEmbedding) string { return e.ContentID })

	// Apply pagination limits and determine if there are more results
	finalResults, hasNextPage := ApplyPaginationLimits(allResults, options.Limit)

	// Create next cursor if there are more results
	var nextCursor string
	if hasNextPage && len(finalResults) > 0 {
		lastResult := finalResults[len(finalResults)-1]
		nextCursor = CreateNextCursor(
			nil, // No DynamoDB LastEvaluatedKey for in-memory pagination
			lastResult.Score,
			lastResult.CreatedAt,
			lastResult.ContentID,
			options.SortOrder,
		)
	}

	return finalResults, CreatePaginationResult(hasNextPage, nextCursor, processed)
}

// logSearchCompletion logs the final results of the search operation
func (r *SearchRepository) logSearchCompletion(finalResults []*models.SearchEmbedding, paginationResult *PaginationResult, processed int) {
	topScore := 0.0
	if len(finalResults) > 0 {
		topScore = finalResults[0].Score
	}

	r.logger.Info("embedding search completed",
		zap.Int("results", len(finalResults)),
		zap.Bool("has_next_page", paginationResult.HasNextPage),
		zap.Int("total_scanned", processed),
		zap.Float64("top_score", topScore))
}

// UpdateEmbedding updates an existing embedding
func (r *SearchRepository) UpdateEmbedding(ctx context.Context, contentID string, embedding []float32) error {
	// Get the existing embedding
	var searchEmbedding models.SearchEmbedding
	err := r.db.WithContext(ctx).Model(&models.SearchEmbedding{}).
		Where("PK", "=", fmt.Sprintf("EMBEDDING#%s", contentID)).
		Where("SK", "=", "VECTOR").
		First(&searchEmbedding)

	if err != nil {
		return fmt.Errorf("failed to get embedding: %w", err)
	}

	// Update the embedding
	searchEmbedding.Embedding = embedding
	searchEmbedding.CreatedAt = time.Now()

	err = r.db.WithContext(ctx).Model(&searchEmbedding).Update()
	if err != nil {
		r.logger.Error("failed to update embedding",
			zap.String("content_id", contentID),
			zap.Error(err))
		return fmt.Errorf("failed to update embedding: %w", err)
	}

	return nil
}

// DeleteEmbedding removes an embedding
func (r *SearchRepository) DeleteEmbedding(ctx context.Context, contentID string) error {
	err := r.db.WithContext(ctx).Model(&models.SearchEmbedding{}).
		Where("PK", "=", fmt.Sprintf("EMBEDDING#%s", contentID)).
		Where("SK", "=", "VECTOR").
		Delete()

	if err != nil {
		r.logger.Error("failed to delete embedding",
			zap.String("content_id", contentID),
			zap.Error(err))
		return fmt.Errorf("failed to delete embedding: %w", err)
	}

	return nil
}

// Helper method for cosine similarity
func (r *SearchRepository) cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
