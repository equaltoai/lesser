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
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// SearchRepositoryDeps interface for dependencies - implemented by the storage adapter
type SearchRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

// SearchRepository implements search functionality using DynamORM
type SearchRepository struct {
	db     dynamorm.DB
	logger *zap.Logger
	deps   SearchRepositoryDeps
}

// NewSearchRepository creates a new search repository
func NewSearchRepository(db dynamorm.DB, logger *zap.Logger) *SearchRepository {
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

// SearchAccountsAdvanced searches for accounts with advanced filtering
func (r *SearchRepository) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	normalizedQuery = strings.TrimPrefix(normalizedQuery, "@")
	
	if len(normalizedQuery) < 2 {
		return []*activitypub.Actor{}, nil
	}

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	// Strategy 1: Exact username match
	if len(normalizedQuery) >= 3 {
		var exactMatch models.Actor
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Where("PK", "=", fmt.Sprintf("ACTOR#%s", normalizedQuery)).
			Where("SK", "=", "PROFILE").
			First(&exactMatch)
		
		if err == nil && exactMatch.Actor != nil {
			results = append(results, exactMatch.Actor)
			seen[exactMatch.Actor.ID] = true
		}
	}

	// Strategy 2: Username prefix search using GSI1
	prefixKey := normalizedQuery[:2]
	var prefixMatches []models.Actor
	
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("username-search-index").
		Where("GSI1PK", "=", fmt.Sprintf("USERNAME_SEARCH#%s", prefixKey)).
		Filter("GSI1SK", "BEGINS_WITH", normalizedQuery).
		Limit(limit + offset).
		All(&prefixMatches)
	
	if err != nil {
		r.logger.Warn("username prefix search failed", zap.Error(err))
	} else {
		for _, match := range prefixMatches {
			if match.Actor != nil && !seen[match.Actor.ID] {
				results = append(results, match.Actor)
				seen[match.Actor.ID] = true
			}
		}
	}

	// Strategy 3: Display name search using GSI2
	if len(normalizedQuery) >= 2 {
		displayNameKey := normalizedQuery[:2]
		var displayNameMatches []models.Actor
		
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("display-name-index").
			Where("GSI2PK", "=", fmt.Sprintf("NAME_SEARCH#%s", displayNameKey)).
			Filter("GSI2SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&displayNameMatches)
		
		if err != nil {
			r.logger.Warn("display name search failed", zap.Error(err))
		} else {
			for _, match := range displayNameMatches {
				if match.Actor != nil && !seen[match.Actor.ID] {
					results = append(results, match.Actor)
					seen[match.Actor.ID] = true
				}
			}
		}
	}

	// Apply following filter if requested
	if following && accountID != "" {
		if r.deps != nil {
			// Get list of accounts this user is following
			followingList, _, err := r.deps.GetFollowing(ctx, accountID, 1000, "")
			if err != nil {
				r.logger.Warn("failed to get following list for filter",
					zap.String("account_id", accountID),
					zap.Error(err))
			} else {
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
				results = filteredResults
			}
		} else {
			r.logger.Debug("following filter requested but dependencies not set", 
				zap.String("account_id", accountID))
		}
	}

	// Sort results by relevance
	sort.Slice(results, func(i, j int) bool {
		// Exact matches first
		iExact := strings.ToLower(results[i].PreferredUsername) == normalizedQuery
		jExact := strings.ToLower(results[j].PreferredUsername) == normalizedQuery
		if iExact != jExact {
			return iExact
		}
		
		// Then prefix matches
		iPrefix := strings.HasPrefix(strings.ToLower(results[i].PreferredUsername), normalizedQuery)
		jPrefix := strings.HasPrefix(strings.ToLower(results[j].PreferredUsername), normalizedQuery)
		if iPrefix != jPrefix {
			return iPrefix
		}
		
		// Then by username length (shorter is better)
		return len(results[i].PreferredUsername) < len(results[j].PreferredUsername)
	})

	// Apply offset and limit
	if offset < len(results) {
		results = results[offset:]
	} else {
		results = []*activitypub.Actor{}
	}
	
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
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
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return []*storage.StatusSearchResult{}, nil
	}

	// Apply default limit if not specified
	if options.Limit <= 0 {
		options.Limit = 20
	}

	results := make([]*storage.StatusSearchResult, 0)
	resultMap := make(map[string]*storage.StatusSearchResult)
	
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Strategy 1: URL search - exact URL match
	if strings.HasPrefix(normalizedQuery, "http://") || strings.HasPrefix(normalizedQuery, "https://") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			var object models.Object
			err := r.db.WithContext(ctx).Model(&models.Object{}).
				Where("URL", "=", normalizedQuery).
				First(&object)
			
			if err == nil && object.Type == "Note" {
				result := r.objectToSearchResult(&object, 1.0, "url_match")
				mu.Lock()
				resultMap[object.ID] = result
				mu.Unlock()
			}
		}()
	}

	// Strategy 2: Hashtag search
	hashtags := r.extractHashtags(normalizedQuery)
	if len(hashtags) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for _, hashtag := range hashtags {
				// Search for statuses with this hashtag
				// This would require a hashtag index or scanning status content
				// For now, we'll skip this strategy
				r.logger.Debug("hashtag search not fully implemented", 
					zap.String("hashtag", hashtag))
			}
		}()
	}

	// Strategy 3: Content search - scan recent statuses
	// In the legacy system, this uses GSI7 (content-word-index)
	// Without full-text search, we need to scan and filter
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		// Get recent statuses and filter by content
		var recentStatuses []models.Object
		err := r.db.WithContext(ctx).Model(&models.Object{}).
			Index("gsi2-index").
			Where("GSI2PK", "=", "object#type#Note").
			Limit(100). // Scan last 100 statuses
			All(&recentStatuses)
		
		if err != nil {
			r.logger.Warn("content search failed", zap.Error(err))
			return
		}
		
		for _, status := range recentStatuses {
			if status.Content == "" {
				continue
			}
			
			// Simple content matching
			contentLower := strings.ToLower(status.Content)
			if strings.Contains(contentLower, normalizedQuery) {
				score := r.calculateContentScore(contentLower, normalizedQuery)
				result := r.objectToSearchResult(&status, score, "content_match")
				
				mu.Lock()
				if existing, ok := resultMap[status.ID]; !ok || result.Score > existing.Score {
					resultMap[status.ID] = result
				}
				mu.Unlock()
			}
		}
	}()

	// Wait for all strategies to complete
	wg.Wait()

	// Convert map to slice
	for _, result := range resultMap {
		results = append(results, result)
	}

	// Apply filters
	if options.AccountID != "" {
		filtered := make([]*storage.StatusSearchResult, 0)
		for _, result := range results {
			if result.AuthorID == options.AccountID {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	if options.LocalOnly {
		filtered := make([]*storage.StatusSearchResult, 0)
		for _, result := range results {
			if r.isLocalStatus(result.StatusID) {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	// Skip MinEngagement filter as StatusSearchResult doesn't have engagement fields

	// Sort by score and recency
	sort.Slice(results, func(i, j int) bool {
		// Higher score wins
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		// More recent wins
		return results[i].Published.After(results[j].Published)
	})

	// Apply limit
	if len(results) > options.Limit {
		results = results[:options.Limit]
	}

	return results, nil
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
func (r *SearchRepository) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	results := &storage.SearchResults{
		Accounts: make([]*activitypub.Actor, 0),
		Statuses: make([]*storage.StatusSearchResult, 0),
		Hashtags: make([]*storage.HashtagSearchResult, 0),
	}

	// Search accounts
	accounts, err := r.SearchAccounts(ctx, query, limit, false, 0)
	if err != nil {
		r.logger.Warn("account search failed in SearchAll", zap.Error(err))
	} else {
		results.Accounts = accounts
	}

	// Search statuses
	statuses, err := r.SearchStatuses(ctx, query, limit)
	if err != nil {
		r.logger.Warn("status search failed in SearchAll", zap.Error(err))
	} else {
		results.Statuses = statuses
	}

	// Search hashtags
	hashtags, err := r.SearchHashtags(ctx, query, limit)
	if err != nil {
		r.logger.Warn("hashtag search failed in SearchAll", zap.Error(err))
	} else {
		// Convert to HashtagSearchResult
		for _, ht := range hashtags {
			results.Hashtags = append(results.Hashtags, &storage.HashtagSearchResult{
				Name: ht.Name,
				URL:  ht.URL,
			})
		}
	}

	return results, nil
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
			UsageCount: ht.UsageCount,
			FirstSeen:  ht.FirstSeen,
			LastUsed:   ht.LastUsed,
		})
	}

	return results, nil
}

// SearchHashtagsAdvanced searches for hashtags with advanced filtering
func (r *SearchRepository) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	hashtags, err := r.SearchHashtags(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	// Convert to HashtagSearchResult format
	results := make([]*storage.HashtagSearchResult, 0, len(hashtags))
	for _, ht := range hashtags {
		result := &storage.HashtagSearchResult{
			Name: ht.Name,
			URL:  ht.URL,
			// History would need to be populated from trending data
		}
		results = append(results, result)
	}

	return results, nil
}

// Helper methods

func (r *SearchRepository) objectToSearchResult(obj *models.Object, score float64, strategy string) *storage.StatusSearchResult {
	if obj == nil || obj.Type != "Note" {
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
		Highlights:     map[string]string{"strategy": strategy},
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
	if updates == nil || len(updates) == 0 {
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
func (r *SearchRepository) IndexStatus(ctx context.Context, status *models.Object) error {
	if status == nil || status.Type != "Note" {
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

// SearchStatusesByHashtag searches for statuses containing a specific hashtag
func (r *SearchRepository) SearchStatusesByHashtag(ctx context.Context, hashtag string, limit int) ([]*storage.StatusSearchResult, error) {
	normalizedHashtag := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	results := make([]*storage.StatusSearchResult, 0)

	// In the legacy system, this would use a hashtag index
	// For now, we scan recent statuses and filter
	var statuses []models.Object

	err := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi2-index").
		Where("GSI2PK", "=", "object#type#Note").
		Limit(limit * 2). // Get more to filter
		All(&statuses)

	if err != nil {
		return nil, fmt.Errorf("failed to search statuses by hashtag: %w", err)
	}

	// Filter by hashtag presence
	for _, status := range statuses {
		if strings.Contains(strings.ToLower(status.Content), "#"+normalizedHashtag) {
			result := r.objectToSearchResult(&status, 1.0, "hashtag_match")
			if result != nil {
				results = append(results, result)
			}
		}

		if len(results) >= limit {
			break
		}
	}

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
		Filter("Type", "=", "Note").
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

	// Update keys
	event.UpdateKeys()

	// Create the analytics record
	err := r.db.WithContext(ctx).Model(event).Create()
	if err != nil {
		r.logger.Error("failed to record search analytics",
			zap.String("query", event.Query),
			zap.Error(err))
		return fmt.Errorf("failed to record search analytics: %w", err)
	}

	return nil
}

// GetSearchAnalytics retrieves search analytics for a date range
func (r *SearchRepository) GetSearchAnalytics(ctx context.Context, startDate, endDate time.Time) ([]*models.SearchAnalytics, error) {
	analytics := make([]*models.SearchAnalytics, 0)

	// Query each day in the range
	current := startDate
	for !current.After(endDate) {
		dateStr := current.Format("2006-01-02")
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

// GetPopularSearches retrieves popular search queries
func (r *SearchRepository) GetPopularSearches(ctx context.Context, limit int, timeWindow time.Duration) ([]*models.SearchQueryStats, error) {
	// Get analytics for time window
	endDate := time.Now()
	startDate := endDate.Add(-timeWindow)

	analytics, err := r.GetSearchAnalytics(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Aggregate by query
	queryCount := make(map[string]int64)
	lastQueried := make(map[string]time.Time)

	for _, event := range analytics {
		queryCount[event.Query]++
		if event.Timestamp.After(lastQueried[event.Query]) {
			lastQueried[event.Query] = event.Timestamp
		}
	}

	// Convert to SearchQueryStats
	stats := make([]*models.SearchQueryStats, 0, len(queryCount))
	for query, count := range queryCount {
		stat := &models.SearchQueryStats{
			Query:       query,
			QueryCount:  count,
			LastQueried: lastQueried[query],
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
		day := event.Timestamp.Format("2006-01-02")
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
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding cannot be empty")
	}

	if threshold < 0 || threshold > 1 {
		threshold = 0.7 // Default threshold for semantic similarity
	}

	if limit <= 0 || limit > 100 {
		limit = 20 // Default limit
	}

	r.logger.Debug("searching by embedding",
		zap.Int("embedding_dim", len(queryEmbedding)),
		zap.Float64("threshold", threshold),
		zap.Int("limit", limit))

	// For optimal vector search, we'd use a dedicated vector database like Pinecone or Weaviate
	// However, for this implementation, we'll use DynamoDB with optimizations:
	// 1. Scan in batches to avoid loading all embeddings into memory
	// 2. Use early termination when we have enough high-quality results
	// 3. Implement approximate nearest neighbor with bucketing

	var allResults []*models.SearchEmbedding
	batchSize := 50
	processed := 0
	maxScan := 500 // Limit total embeddings scanned

	// Use pagination to process embeddings in batches
	var lastKey map[string]interface{}
	
	for processed < maxScan {
		var embeddings []models.SearchEmbedding
		
		query := r.db.WithContext(ctx).Model(&models.SearchEmbedding{}).
			Limit(batchSize)
		
		if lastKey != nil {
			// TODO: Add exclusive start key support when available in DynamORM
			// For now, we'll do a basic scan
		}
		
		err := query.All(&embeddings)
		if err != nil {
			return nil, fmt.Errorf("failed to search embeddings: %w", err)
		}

		if len(embeddings) == 0 {
			break // No more results
		}

		// Process this batch
		batchResults := make([]*models.SearchEmbedding, 0)
		for i := range embeddings {
			embedding := &embeddings[i]
			
			// Skip if embedding dimensions don't match
			if len(embedding.Embedding) != len(queryEmbedding) {
				r.logger.Warn("embedding dimension mismatch",
					zap.String("content_id", embedding.ContentID),
					zap.Int("expected", len(queryEmbedding)),
					zap.Int("actual", len(embedding.Embedding)))
				continue
			}

			similarity := r.cosineSimilarity(queryEmbedding, embedding.Embedding)

			if similarity >= threshold {
				embedding.Score = similarity
				batchResults = append(batchResults, embedding)
			}
		}

		// Add batch results to overall results
		allResults = append(allResults, batchResults...)
		processed += len(embeddings)

		// Early termination if we have enough high-quality results
		if len(allResults) >= limit*2 && len(batchResults) == 0 {
			break
		}

		// Break if we got fewer results than batch size (end of data)
		if len(embeddings) < batchSize {
			break
		}
	}

	r.logger.Debug("completed embedding search scan",
		zap.Int("processed", processed),
		zap.Int("candidates", len(allResults)))

	// Sort all results by similarity (descending)
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	// Apply final limit
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	r.logger.Info("embedding search completed",
		zap.Int("results", len(allResults)),
		zap.Float64("top_score", func() float64 {
			if len(allResults) > 0 {
				return allResults[0].Score
			}
			return 0.0
		}()))

	return allResults, nil
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