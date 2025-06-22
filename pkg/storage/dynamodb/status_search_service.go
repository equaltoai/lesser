package dynamodb

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aron23/lesser/pkg/cost"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"go.uber.org/zap"
)

// StatusSearchService provides advanced status search capabilities
type StatusSearchService struct {
	dynamo     DynamoDBAPI
	tableName  string
	logger     *zap.Logger
	cache      *StatusSearchCache
	analytics  *SearchAnalytics
	storage    *dynamoDBStorage
	embeddings *EmbeddingService
	comprehend *comprehend.Client
}

// NewStatusSearchService creates a new status search service
func NewStatusSearchService(
	dynamo DynamoDBAPI,
	tableName string,
	logger *zap.Logger,
	storage *dynamoDBStorage,
) *StatusSearchService {
	return &StatusSearchService{
		dynamo:    dynamo,
		tableName: tableName,
		logger:    logger,
		cache:     NewStatusSearchCache(dynamo, tableName),
		analytics: NewSearchAnalytics(dynamo, tableName, logger),
		storage:   storage,
		// embeddings service will be initialized when AI features are enabled
	}
}

// StatusSearchStrategy defines the interface for status search strategies
type StatusSearchStrategy interface {
	Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error)
	Name() string
}

// StatusSearchOptions configures status search behavior
type StatusSearchOptions struct {
	Limit         int
	Offset        int
	AccountID     string    // Filter by specific account
	FollowingOnly bool      // Only from accounts user follows
	LocalOnly     bool      // Only local statuses
	MediaOnly     bool      // Only statuses with media
	Language      string    // Filter by language
	MinEngagement int       // Minimum likes/boosts
	TimeRange     TimeRange // Last hour/day/week/month
}

// TimeRange represents a time-based filter
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// StatusSearchResult represents a status search result
type StatusSearchResult struct {
	StatusID       string
	Content        string
	URL            string
	AuthorID       string
	AuthorUsername string
	Published      time.Time
	Score          float64
	MatchedFields  []string
	Highlights     map[string]string
	Strategy       string
	// Engagement metrics
	LikesCount   int
	BoostsCount  int
	RepliesCount int
	// Additional metadata
	Language   string
	HasMedia   bool
	Visibility string
}

// SearchContext provides context for personalized ranking
type SearchContext struct {
	UserID       string
	UserFollows  map[string]bool
	UserLanguage string
	SearchTime   time.Time
}

// Search performs a multi-strategy status search
func (s *StatusSearchService) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	startTime := time.Now()

	// Check cache first
	cacheKey := s.buildCacheKey(query, options)
	if cached, found := s.cache.Get(ctx, cacheKey); found {
		return cached, nil
	}

	// Analyze query
	analyzed := s.analyzeQuery(ctx, query)

	// Select strategies based on query
	strategies := s.selectStrategies(analyzed, options)

	// Execute strategies in parallel
	results := s.executeStrategies(ctx, strategies, analyzed, options)

	// Build search context for ranking
	searchContext := s.buildSearchContext(ctx, options)

	// Merge and rank results
	merged := s.mergeAndRankResults(results, searchContext, options)

	// Apply filters
	filtered := s.applyFilters(ctx, merged, options)

	// Boost personalized results
	personalized := s.personalizeResults(ctx, filtered, searchContext)

	// Apply limit
	if len(personalized) > options.Limit {
		personalized = personalized[:options.Limit]
	}

	// Track analytics
	searchTime := time.Since(startTime).Milliseconds()
	go func() {
		if err := s.analytics.TrackStatusSearch(context.Background(), query, personalized, searchTime, searchContext.UserID); err != nil {
			s.logger.Warn("failed to track status search", zap.Error(err))
		}
	}()

	// Cache results
	s.cache.Set(ctx, cacheKey, personalized)

	return personalized, nil
}

// AnalyzedStatusQuery represents an analyzed status search query
type AnalyzedStatusQuery struct {
	Original   string
	Normalized string
	Language   string
	Intent     StatusSearchIntent
	Keywords   []string
	Hashtags   []string
	Mentions   []string
	HasURL     bool
}

// StatusSearchIntent represents the inferred intent of a status search
type StatusSearchIntent string

const (
	StatusIntentGeneral      StatusSearchIntent = "general"
	StatusIntentHashtag      StatusSearchIntent = "hashtag"
	StatusIntentMention      StatusSearchIntent = "mention"
	StatusIntentURL          StatusSearchIntent = "url"
	StatusIntentPhrase       StatusSearchIntent = "phrase"
	StatusIntentConversation StatusSearchIntent = "conversation"
)

// analyzeQuery processes the status search query
func (s *StatusSearchService) analyzeQuery(ctx context.Context, query string) *AnalyzedStatusQuery {
	analyzed := &AnalyzedStatusQuery{
		Original:   query,
		Normalized: strings.ToLower(strings.TrimSpace(query)),
		Language:   "en", // Default
		Keywords:   []string{},
		Hashtags:   []string{},
		Mentions:   []string{},
	}

	// Detect language using AWS Comprehend
	if s.comprehend != nil && len(analyzed.Normalized) >= 20 {
		// Track AWS Comprehend usage for cost tracking
		// Using DynamoDB read as proxy since specific Comprehend tracking not implemented yet
		cost.TrackDynamoReadContext(ctx, 1)

		langInput := &comprehend.DetectDominantLanguageInput{
			Text: aws.String(query),
		}

		langResp, err := s.comprehend.DetectDominantLanguage(ctx, langInput)
		if err == nil && len(langResp.Languages) > 0 {
			// Use the most confident language
			var bestLang string
			var bestScore float32
			for _, lang := range langResp.Languages {
				if lang.Score != nil && lang.LanguageCode != nil && *lang.Score > bestScore {
					bestScore = *lang.Score
					bestLang = *lang.LanguageCode
				}
			}
			if bestLang != "" {
				analyzed.Language = bestLang
				s.logger.Debug("detected query language",
					zap.String("query", query),
					zap.String("language", bestLang),
					zap.Float32("confidence", bestScore))
			}
		} else if err != nil {
			s.logger.Debug("language detection failed",
				zap.String("query", query),
				zap.Error(err))
		}
	}

	// Extract hashtags
	for _, match := range hashtagRegex.FindAllString(analyzed.Normalized, -1) {
		analyzed.Hashtags = append(analyzed.Hashtags, strings.TrimPrefix(match, "#"))
	}

	// Extract mentions
	for _, match := range mentionRegex.FindAllString(analyzed.Normalized, -1) {
		analyzed.Mentions = append(analyzed.Mentions, strings.TrimPrefix(match, "@"))
	}

	// Check for URLs
	analyzed.HasURL = urlRegex.MatchString(analyzed.Normalized)

	// Determine intent
	if len(analyzed.Hashtags) > 0 {
		analyzed.Intent = StatusIntentHashtag
	} else if len(analyzed.Mentions) > 0 {
		analyzed.Intent = StatusIntentMention
	} else if analyzed.HasURL {
		analyzed.Intent = StatusIntentURL
	} else if strings.Contains(analyzed.Normalized, "\"") {
		analyzed.Intent = StatusIntentPhrase
	} else {
		analyzed.Intent = StatusIntentGeneral
	}

	// Extract keywords (remove hashtags, mentions, and common words)
	words := strings.Fields(analyzed.Normalized)
	for _, word := range words {
		if !strings.HasPrefix(word, "#") && !strings.HasPrefix(word, "@") &&
			!isStopWord(word) && len(word) > 2 {
			analyzed.Keywords = append(analyzed.Keywords, word)
		}
	}

	return analyzed
}

// selectStrategies chooses appropriate search strategies
func (s *StatusSearchService) selectStrategies(query *AnalyzedStatusQuery, options StatusSearchOptions) []StatusSearchStrategy {
	strategies := []StatusSearchStrategy{}

	// URL search strategy - highest priority for URL queries
	if query.HasURL {
		strategies = append(strategies, &URLSearchStrategy{service: s})
	}

	// Content word search - for keyword matching
	if len(query.Keywords) > 0 {
		strategies = append(strategies, &ContentWordSearchStrategy{service: s})
	}

	// Hashtag search - for hashtag queries
	if len(query.Hashtags) > 0 {
		strategies = append(strategies, &HashtagSearchStrategy{service: s})
	}

	// Author search - when searching for specific user's posts
	if len(query.Mentions) > 0 || options.AccountID != "" {
		strategies = append(strategies, &AuthorSearchStrategy{service: s})
	}

	// Trending search - include popular content
	if options.MinEngagement > 0 || len(strategies) == 0 {
		strategies = append(strategies, &TrendingSearchStrategy{service: s})
	}

	return strategies
}

// executeStrategies runs search strategies in parallel
func (s *StatusSearchService) executeStrategies(
	ctx context.Context,
	strategies []StatusSearchStrategy,
	query *AnalyzedStatusQuery,
	options StatusSearchOptions,
) []*StatusSearchResult {
	var wg sync.WaitGroup
	resultsChan := make(chan []*StatusSearchResult, len(strategies))

	// Create a context with timeout
	searchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Execute each strategy in a goroutine
	for _, strategy := range strategies {
		wg.Add(1)
		go func(str StatusSearchStrategy) {
			defer wg.Done()

			results, err := str.Search(searchCtx, query.Original, options)
			if err != nil {
				s.logger.Warn("strategy failed",
					zap.String("strategy", str.Name()),
					zap.Error(err))
				return
			}

			// Tag results with strategy name
			for _, result := range results {
				result.Strategy = str.Name()
			}

			resultsChan <- results
		}(strategy)
	}

	// Wait for all strategies to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect all results
	allResults := []*StatusSearchResult{}
	for results := range resultsChan {
		allResults = append(allResults, results...)
	}

	return allResults
}

// buildSearchContext creates context for personalized ranking
func (s *StatusSearchService) buildSearchContext(ctx context.Context, options StatusSearchOptions) SearchContext {
	context := SearchContext{
		UserFollows:  make(map[string]bool),
		SearchTime:   time.Now(),
		UserLanguage: options.Language,
	}

	// Extract user ID from context (set by auth middleware)
	if userID, ok := ctx.Value("user_id").(string); ok {
		context.UserID = userID

		// Load user's following list
		if follows, err := s.loadUserFollowing(ctx, userID); err == nil {
			for _, followedID := range follows {
				context.UserFollows[followedID] = true
			}
			s.logger.Debug("loaded user following list",
				zap.String("user_id", userID),
				zap.Int("following_count", len(follows)))
		} else {
			s.logger.Warn("failed to load user following",
				zap.String("user_id", userID),
				zap.Error(err))
		}

		// Try to get user's preferred language if not specified
		if context.UserLanguage == "" {
			if lang, err := s.storage.GetUserLanguagePreference(ctx, userID); err == nil {
				context.UserLanguage = lang
			} else {
				// Default to English
				context.UserLanguage = "en"
			}
		}
	}

	return context
}

// loadUserFollowing loads the list of actors a user follows
func (s *StatusSearchService) loadUserFollowing(ctx context.Context, userID string) ([]string, error) {
	// Use the storage interface to get following list
	// Get all following (use high limit and handle pagination if needed)
	allFollowing := make([]string, 0)
	cursor := ""

	for {
		following, nextCursor, err := s.storage.GetFollowing(ctx, userID, 1000, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get following list: %w", err)
		}

		// Convert usernames to actor IDs
		for _, username := range following {
			// Following list returns usernames, convert to actor IDs
			actorID := fmt.Sprintf("https://%s/users/%s", s.storage.domain, username)
			allFollowing = append(allFollowing, actorID)
		}

		// Check if there are more results
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allFollowing, nil
}

// mergeAndRankResults combines and ranks results from multiple strategies
func (s *StatusSearchService) mergeAndRankResults(
	results []*StatusSearchResult,
	context SearchContext,
	_ StatusSearchOptions,
) []*StatusSearchResult {
	// Deduplicate by status ID
	seen := make(map[string]*StatusSearchResult)
	for _, result := range results {
		if existing, ok := seen[result.StatusID]; ok {
			// Merge results - keep higher score
			if result.Score > existing.Score {
				seen[result.StatusID] = result
			} else {
				// Merge matched fields and highlights
				existing.MatchedFields = append(existing.MatchedFields, result.MatchedFields...)
				for k, v := range result.Highlights {
					if _, hasKey := existing.Highlights[k]; !hasKey {
						existing.Highlights[k] = v
					}
				}
			}
		} else {
			seen[result.StatusID] = result
		}
	}

	// Convert to slice
	merged := make([]*StatusSearchResult, 0, len(seen))
	for _, result := range seen {
		merged = append(merged, result)
	}

	// Create ranker
	ranker := NewStatusRanker()

	// Calculate final scores
	for _, result := range merged {
		result.Score = ranker.Score(result, context)
	}

	// Sort by score
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	return merged
}

// applyFilters applies search filters to results
//
//nolint:unused // False positive - options parameter is used throughout the function
func (s *StatusSearchService) applyFilters(
	ctx context.Context,
	results []*StatusSearchResult,
	options StatusSearchOptions,
) []*StatusSearchResult {
	filtered := make([]*StatusSearchResult, 0)

	// Get search context for following filter
	var searchContext SearchContext
	if options.FollowingOnly {
		searchContext = s.buildSearchContext(ctx, options)
	}

	for _, result := range results {
		// Time range filter
		if !options.TimeRange.Start.IsZero() && result.Published.Before(options.TimeRange.Start) {
			continue
		}
		if !options.TimeRange.End.IsZero() && result.Published.After(options.TimeRange.End) {
			continue
		}

		// Engagement filter
		totalEngagement := result.LikesCount + result.BoostsCount
		if options.MinEngagement > 0 && totalEngagement < options.MinEngagement {
			continue
		}

		// Media filter
		if options.MediaOnly && !result.HasMedia {
			continue
		}

		// Language filter
		if options.Language != "" && result.Language != options.Language {
			continue
		}

		// Local only filter
		if options.LocalOnly && !s.isLocalStatus(result.StatusID) {
			continue
		}

		// Account filter
		if options.AccountID != "" && result.AuthorID != options.AccountID {
			continue
		}

		// Following only filter
		if options.FollowingOnly && searchContext.UserID != "" {
			// Only include posts from followed users
			if !searchContext.UserFollows[result.AuthorID] {
				continue
			}
		}

		filtered = append(filtered, result)
	}

	return filtered
}

// personalizeResults applies personalization to search results
func (s *StatusSearchService) personalizeResults(
	_ context.Context,
	results []*StatusSearchResult,
	context SearchContext,
) []*StatusSearchResult {
	// For now, just boost posts from followed users
	for _, result := range results {
		if context.UserFollows[result.AuthorID] {
			result.Score *= 1.2 // 20% boost for followed users
		}
	}

	// Re-sort after personalization
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// Helper methods

func (s *StatusSearchService) buildCacheKey(query string, options StatusSearchOptions) string {
	return fmt.Sprintf("status_search:%s:%+v", query, options)
}


func (s *StatusSearchService) isLocalStatus(statusID string) bool {
	// Check if status ID belongs to local instance
	return strings.Contains(statusID, s.storage.domain)
}

// StatusRanker implements the ranking algorithm
type StatusRanker struct {
	ContentRelevance   float64
	Recency            float64
	Engagement         float64
	AuthorAuthority    float64
	PersonalAffinity   float64
	SemanticSimilarity float64
}

// NewStatusRanker creates a ranker with default weights
func NewStatusRanker() *StatusRanker {
	return &StatusRanker{
		ContentRelevance:   0.3,
		Recency:            0.2,
		Engagement:         0.15,
		AuthorAuthority:    0.15,
		PersonalAffinity:   0.1,
		SemanticSimilarity: 0.1,
	}
}

// Score calculates the final score for a status result
func (r *StatusRanker) Score(status *StatusSearchResult, context SearchContext) float64 {
	score := 0.0

	// Content relevance (from search strategy)
	score += status.Score * r.ContentRelevance

	// Recency (exponential decay)
	ageHours := time.Since(status.Published).Hours()
	recencyScore := math.Exp(-ageHours / 168) // Half-life of 1 week
	score += recencyScore * r.Recency

	// Engagement (normalized by age)
	engagementCount := float64(status.LikesCount + status.BoostsCount + status.RepliesCount)
	engagementRate := engagementCount / (ageHours + 1)
	score += math.Log1p(engagementRate) * r.Engagement

	// Author authority (placeholder - would use follower count/trust score)
	// For now, just use a fixed value
	authorScore := 0.5
	score += authorScore * r.AuthorAuthority

	// Personal affinity
	if context.UserFollows[status.AuthorID] {
		score += 1.0 * r.PersonalAffinity
	}

	// Semantic similarity (if available from semantic search)
	// Placeholder for now

	return score
}
