package dynamodb

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SearchService provides advanced search capabilities
type SearchService struct {
	dynamo    DynamoDBAPI
	tableName string
	logger    *zap.Logger
	cache     *SearchCache
	analytics *SearchAnalytics
	storage   *dynamoDBStorage // For filters
	domain    string
}

// NewSearchService creates a new search service
func NewSearchService(dynamo DynamoDBAPI, tableName string, logger *zap.Logger, storage *dynamoDBStorage, domain string) *SearchService {
	return &SearchService{
		dynamo:    dynamo,
		tableName: tableName,
		logger:    logger,
		cache:     NewSearchCache(dynamo, tableName),
		analytics: NewSearchAnalytics(dynamo, tableName, logger),
		storage:   storage,
		domain:    domain,
	}
}

// SearchStrategy defines different search approaches
type SearchStrategy interface {
	Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error)
	Name() string
}

// SearchOptions configures the search behavior
type SearchOptions struct {
	Limit         int
	Offset        int
	FollowingOnly bool
	Fuzzy         bool
	Semantic      bool
	IncludeRemote bool
	Language      string
}

// SearchResult represents a search result with scoring
type SearchResult struct {
	Actor         *activitypub.Actor
	Score         float64
	MatchedFields []string
	Highlights    map[string]string
	Strategy      string // Which strategy found this result
}

// AnalyzedQuery represents a processed search query
type AnalyzedQuery struct {
	Original string
	Query    string // Normalized query
	Language string
	Intent   SearchIntent
	IsHandle bool   // Is it a @username or @user@domain query
	Domain   string // For federated searches
}

// SearchIntent represents what the user is looking for
type SearchIntent string

const (
	IntentUsername    SearchIntent = "username"
	IntentDisplayName SearchIntent = "display_name"
	IntentBio         SearchIntent = "bio"
	IntentGeneral     SearchIntent = "general"
)

// Search performs a multi-strategy search
func (s *SearchService) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
	startTime := time.Now()

	// Check cache first
	cacheKey := s.cache.BuildKey(query, options)
	if cached, found := s.cache.Get(ctx, cacheKey); found {
		s.logger.Debug("search cache hit", zap.String("query", query))
		return cached, nil
	}

	// Analyze query
	analyzedQuery := s.analyzeQuery(ctx, query)

	// Initialize strategies based on query analysis
	strategies := s.selectStrategies(ctx, analyzedQuery, options)

	// Execute strategies in parallel
	resultsChan := make(chan []*SearchResult, len(strategies))
	errorsChan := make(chan error, len(strategies))

	for _, strategy := range strategies {
		go func(st SearchStrategy) {
			results, err := st.Search(ctx, analyzedQuery.Query, options)
			if err != nil {
				s.logger.Warn("search strategy failed",
					zap.String("strategy", st.Name()),
					zap.Error(err))
				errorsChan <- err
				resultsChan <- nil
			} else {
				// Tag results with strategy name
				for _, r := range results {
					r.Strategy = st.Name()
				}
				resultsChan <- results
			}
		}(strategy)
	}

	// Collect results with timeout
	timeout := time.After(2 * time.Second)
	allResults := make([]*SearchResult, 0)
	completed := 0

	for completed < len(strategies) {
		select {
		case results := <-resultsChan:
			if results != nil {
				allResults = append(allResults, results...)
			}
			completed++
		case <-errorsChan:
			completed++
		case <-timeout:
			s.logger.Warn("search timeout reached",
				zap.Int("completed", completed),
				zap.Int("total", len(strategies)))
			goto MERGE
		}
	}

MERGE:
	// Merge and rank results
	finalResults := s.mergeAndRankResults(allResults, options.Limit)

	// Apply filters if needed
	userID := "" // TODO: Extract from context when we implement auth
	if options.FollowingOnly || !options.IncludeRemote {
		var err error
		finalResults, err = ApplySearchFilters(ctx, finalResults, options, userID, s.storage, s.domain, s.logger)
		if err != nil {
			s.logger.Warn("failed to apply search filters", zap.Error(err))
		}
	}

	// Track search analytics
	searchTime := time.Since(startTime).Milliseconds()
	if s.analytics != nil {
		go func() {
			if err := s.analytics.TrackSearch(context.Background(), query, finalResults, searchTime, userID); err != nil {
				s.logger.Warn("failed to track search", zap.Error(err))
			}
		}()
	}

	// Cache results
	s.cache.Set(ctx, cacheKey, finalResults)

	return finalResults, nil
}

// analyzeQuery processes the search query to understand intent
func (s *SearchService) analyzeQuery(ctx context.Context, query string) *AnalyzedQuery {
	// Explicitly ignore context for now (may be used for language detection in future)
	_ = ctx

	analyzed := &AnalyzedQuery{
		Original: query,
		Query:    strings.ToLower(strings.TrimSpace(query)),
		Language: "en", // TODO: Detect language
	}

	// Check if it's a handle query
	if strings.HasPrefix(analyzed.Query, "@") {
		analyzed.IsHandle = true
		parts := strings.Split(analyzed.Query[1:], "@")
		if len(parts) > 1 {
			analyzed.Domain = parts[1]
		}
		analyzed.Intent = IntentUsername
	} else if strings.Contains(analyzed.Query, " ") {
		// Multiple words suggest display name or bio search
		analyzed.Intent = IntentDisplayName
	} else {
		// Single word could be username or name
		analyzed.Intent = IntentGeneral
	}

	return analyzed
}

// selectStrategies chooses which search strategies to use based on the query
func (s *SearchService) selectStrategies(ctx context.Context, query *AnalyzedQuery, options SearchOptions) []SearchStrategy {
	strategies := make([]SearchStrategy, 0)

	// Always try exact match first
	strategies = append(strategies, &ExactMatchStrategy{service: s})

	// Add prefix search for partial username matches
	if len(query.Query) >= 2 {
		strategies = append(strategies, &PrefixSearchStrategy{service: s})
	}

	// Add display name search if query suggests it
	if query.Intent == IntentDisplayName || query.Intent == IntentGeneral {
		if len(query.Query) >= 2 {
			strategies = append(strategies, &DisplayNameSearchStrategy{service: s})
		}
	}

	// Add popularity search - useful for discovering popular accounts
	// Can work with or without a search term
	strategies = append(strategies, &PopularitySearchStrategy{service: s})

	// Fuzzy search disabled - OpenSearch removed to reduce costs
	if options.Fuzzy && len(query.Query) >= 3 {
		s.logger.Debug("fuzzy search requested but not available (OpenSearch removed)")
	}

	// Add semantic search if enabled
	if options.Semantic && len(query.Query) >= 3 {
		// Get AWS config from somewhere - we'll need to pass this properly
		// For now, we'll create it here (in production, this would be passed from the handler)
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			s.logger.Warn("failed to load AWS config for semantic search", zap.Error(err))
		} else {
			if semanticStrategy, err := NewSemanticSearchStrategy(s, cfg); err == nil {
				strategies = append(strategies, semanticStrategy)
				s.logger.Debug("semantic search strategy enabled")
			} else {
				s.logger.Warn("failed to create semantic search strategy", zap.Error(err))
			}
		}
	}

	return strategies
}

// mergeAndRankResults combines results from multiple strategies and ranks them
func (s *SearchService) mergeAndRankResults(results []*SearchResult, limit int) []*SearchResult {
	// Deduplicate by actor ID
	seen := make(map[string]*SearchResult)
	for _, result := range results {
		if result.Actor == nil {
			continue
		}

		actorID := result.Actor.ID
		if existing, ok := seen[actorID]; ok {
			// Keep the result with higher score
			if result.Score > existing.Score {
				seen[actorID] = result
			} else if result.Score == existing.Score {
				// Merge matched fields
				existing.MatchedFields = append(existing.MatchedFields, result.MatchedFields...)
			}
		} else {
			seen[actorID] = result
		}
	}

	// Convert to slice for sorting
	merged := make([]*SearchResult, 0, len(seen))
	for _, result := range seen {
		merged = append(merged, result)
	}

	// Sort by score descending
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		// Secondary sort by username length (shorter = better)
		return len(merged[i].Actor.PreferredUsername) < len(merged[j].Actor.PreferredUsername)
	})

	// Apply limit
	if len(merged) > limit {
		merged = merged[:limit]
	}

	return merged
}

// ExactMatchStrategy performs exact username matching
type ExactMatchStrategy struct {
	service *SearchService
}

func (s *ExactMatchStrategy) Name() string {
	return "exact_match"
}

func (s *ExactMatchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
	// For exact match, remove @ prefix if present
	username := strings.TrimPrefix(query, "@")

	// Build the key for exact username lookup
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// Get the item
	result, err := s.service.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.service.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("exact match query failed: %w", err)
	}

	if result.Item == nil {
		return []*SearchResult{}, nil
	}

	// Unmarshal the actor
	var record ActorRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal actor: %w", err)
	}

	if record.Actor == nil {
		return []*SearchResult{}, nil
	}

	// Perfect match gets highest score
	return []*SearchResult{{
		Actor:         record.Actor,
		Score:         1.0,
		MatchedFields: []string{"username"},
		Highlights: map[string]string{
			"username": fmt.Sprintf("<em>%s</em>", record.Actor.PreferredUsername),
		},
	}}, nil
}

// PrefixSearchStrategy performs prefix matching on usernames
type PrefixSearchStrategy struct {
	service *SearchService
}

func (s *PrefixSearchStrategy) Name() string {
	return "prefix_search"
}

func (s *PrefixSearchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
	prefix := strings.ToLower(strings.TrimPrefix(query, "@"))

	// Need at least 2 characters for GSI1 prefix
	if len(prefix) < 2 {
		return []*SearchResult{}, nil
	}

	// Use GSI1 for optimized prefix search
	gsi1pk := fmt.Sprintf("USERNAME_SEARCH#%s", prefix[:2])

	// Build key condition for GSI query
	expr, err := expression.NewBuilder().
		WithKeyCondition(
			expression.Key("GSI1PK").Equal(expression.Value(gsi1pk)).
				And(expression.Key("GSI1SK").BeginsWith(prefix)),
		).
		Build()

	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(s.service.tableName),
		IndexName:                 aws.String("GSI1"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(int32(options.Limit)),
	}

	result, err := s.service.dynamo.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("prefix search query failed: %w", err)
	}

	searchResults := make([]*SearchResult, 0)
	for _, item := range result.Items {
		var record ActorRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			continue
		}

		if record.Actor != nil {
			// Calculate score based on match position and length
			usernameLen := len(record.Actor.PreferredUsername)
			prefixLen := len(prefix)

			// Higher score for exact length matches
			lengthPenalty := float64(usernameLen-prefixLen) / 10.0
			score := 0.8 - lengthPenalty
			if score < 0.5 {
				score = 0.5
			}

			// Exact match (just different case) gets higher score
			if strings.ToLower(record.Actor.PreferredUsername) == prefix {
				score = 0.95
			}

			searchResults = append(searchResults, &SearchResult{
				Actor:         record.Actor,
				Score:         score,
				MatchedFields: []string{"username"},
				Highlights: map[string]string{
					"username": fmt.Sprintf("<em>%s</em>%s",
						record.Actor.PreferredUsername[:prefixLen],
						record.Actor.PreferredUsername[prefixLen:]),
				},
			})
		}
	}

	// Sort by score within this strategy
	sort.Slice(searchResults, func(i, j int) bool {
		if searchResults[i].Score != searchResults[j].Score {
			return searchResults[i].Score > searchResults[j].Score
		}
		// Secondary sort by username length (shorter = better)
		return len(searchResults[i].Actor.PreferredUsername) < len(searchResults[j].Actor.PreferredUsername)
	})

	return searchResults, nil
}

// DisplayNameSearchStrategy performs search on display names using GSI2
type DisplayNameSearchStrategy struct {
	service *SearchService
}

func (s *DisplayNameSearchStrategy) Name() string {
	return "display_name_search"
}

func (s *DisplayNameSearchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
	searchTerm := strings.ToLower(strings.TrimSpace(query))

	// Need at least 2 characters for GSI2 prefix
	if len(searchTerm) < 2 {
		return []*SearchResult{}, nil
	}

	// Use GSI2 for display name search
	gsi2pk := fmt.Sprintf("NAME_SEARCH#%s", searchTerm[:2])

	// Build key condition for GSI query
	expr, err := expression.NewBuilder().
		WithKeyCondition(
			expression.Key("GSI2PK").Equal(expression.Value(gsi2pk)).
				And(expression.Key("GSI2SK").BeginsWith(searchTerm)),
		).
		Build()

	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(s.service.tableName),
		IndexName:                 aws.String("GSI2"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(int32(options.Limit)),
	}

	result, err := s.service.dynamo.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("display name search query failed: %w", err)
	}

	searchResults := make([]*SearchResult, 0)
	for _, item := range result.Items {
		var record ActorRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			continue
		}

		if record.Actor != nil && record.Actor.Name != "" {
			displayNameLower := strings.ToLower(record.Actor.Name)

			// Calculate score based on match quality
			score := 0.7 // Base score for display name matches

			// Exact match gets highest score
			if displayNameLower == searchTerm {
				score = 0.9
			} else if strings.HasPrefix(displayNameLower, searchTerm) {
				// Prefix match gets good score
				lengthRatio := float64(len(searchTerm)) / float64(len(displayNameLower))
				score = 0.7 + (0.1 * lengthRatio)
			} else if strings.Contains(displayNameLower, searchTerm) {
				// Contains match gets lower score
				score = 0.6
			}

			// Highlight the matched portion
			startIdx := strings.Index(displayNameLower, searchTerm)
			endIdx := startIdx + len(searchTerm)

			highlight := record.Actor.Name
			if startIdx >= 0 && endIdx <= len(record.Actor.Name) {
				highlight = fmt.Sprintf("%s<em>%s</em>%s",
					record.Actor.Name[:startIdx],
					record.Actor.Name[startIdx:endIdx],
					record.Actor.Name[endIdx:])
			}

			searchResults = append(searchResults, &SearchResult{
				Actor:         record.Actor,
				Score:         score,
				MatchedFields: []string{"display_name"},
				Highlights: map[string]string{
					"display_name": highlight,
					"username":     record.Actor.PreferredUsername,
				},
			})
		}
	}

	// Sort by score within this strategy
	sort.Slice(searchResults, func(i, j int) bool {
		if searchResults[i].Score != searchResults[j].Score {
			return searchResults[i].Score > searchResults[j].Score
		}
		// Secondary sort by display name length (shorter = better)
		return len(searchResults[i].Actor.Name) < len(searchResults[j].Actor.Name)
	})

	return searchResults, nil
}

// ActorRecord represents how actors are stored in DynamoDB
type ActorRecord struct {
	PK         string             `dynamodbav:"PK"`
	SK         string             `dynamodbav:"SK"`
	Actor      *activitypub.Actor `dynamodbav:"Actor"`
	PrivateKey string             `dynamodbav:"PrivateKey,omitempty"`
	CreatedAt  time.Time          `dynamodbav:"CreatedAt"`
	UpdatedAt  time.Time          `dynamodbav:"UpdatedAt"`
}

// Suggestion represents a search suggestion
type Suggestion struct {
	Type        string  `json:"type"`     // "username", "display_name", "hashtag"
	Value       string  `json:"value"`    // The suggestion text
	DisplayText string  `json:"display"`  // What to show to the user
	Username    string  `json:"username"` // For account suggestions
	Score       float64 `json:"-"`        // Internal ranking score
}

// GetSuggestions returns search suggestions for autocomplete
func (s *SearchService) GetSuggestions(ctx context.Context, prefix string) ([]Suggestion, error) {
	if len(prefix) < 2 {
		return []Suggestion{}, nil
	}

	suggestions := make([]Suggestion, 0)
	suggestionMap := make(map[string]Suggestion) // Deduplicate

	// Normalize prefix
	prefix = strings.ToLower(strings.TrimSpace(prefix))

	// Search usernames in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Search usernames using GSI1
	wg.Add(1)
	go func() {
		defer wg.Done()

		gsi1pk := fmt.Sprintf("USERNAME_SEARCH#%s", prefix[:2])

		expr, _ := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("GSI1PK").Equal(expression.Value(gsi1pk)).
					And(expression.Key("GSI1SK").BeginsWith(prefix)),
			).
			Build()

		result, err := s.dynamo.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(s.tableName),
			IndexName:                 aws.String("GSI1"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     aws.Int32(5), // Top 5 username matches
		})

		if err == nil {
			mu.Lock()
			for _, item := range result.Items {
				var record ActorRecord
				if err := attributevalue.UnmarshalMap(item, &record); err == nil && record.Actor != nil {
					username := record.Actor.PreferredUsername
					suggestion := Suggestion{
						Type:        "username",
						Value:       username,
						DisplayText: fmt.Sprintf("@%s", username),
						Username:    username,
						Score:       0.9, // High score for username matches
					}

					// Exact prefix match gets highest score
					if strings.ToLower(username) == prefix {
						suggestion.Score = 1.0
					}

					suggestionMap[username] = suggestion
				}
			}
			mu.Unlock()
		}
	}()

	// Search display names using GSI2
	wg.Add(1)
	go func() {
		defer wg.Done()

		gsi2pk := fmt.Sprintf("NAME_SEARCH#%s", prefix[:2])

		expr, _ := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("GSI2PK").Equal(expression.Value(gsi2pk)).
					And(expression.Key("GSI2SK").BeginsWith(prefix)),
			).
			Build()

		result, err := s.dynamo.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(s.tableName),
			IndexName:                 aws.String("GSI2"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     aws.Int32(5), // Top 5 display name matches
		})

		if err == nil {
			mu.Lock()
			for _, item := range result.Items {
				var record ActorRecord
				if err := attributevalue.UnmarshalMap(item, &record); err == nil && record.Actor != nil {
					// Skip if already suggested by username
					if _, exists := suggestionMap[record.Actor.PreferredUsername]; !exists {
						suggestion := Suggestion{
							Type:        "display_name",
							Value:       record.Actor.Name,
							DisplayText: fmt.Sprintf("%s (@%s)", record.Actor.Name, record.Actor.PreferredUsername),
							Username:    record.Actor.PreferredUsername,
							Score:       0.7, // Lower score for display name matches
						}

						suggestionMap[record.Actor.PreferredUsername] = suggestion
					}
				}
			}
			mu.Unlock()
		}
	}()

	// Wait for all searches to complete
	wg.Wait()

	// Convert map to slice and sort by score
	for _, suggestion := range suggestionMap {
		suggestions = append(suggestions, suggestion)
	}

	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Score != suggestions[j].Score {
			return suggestions[i].Score > suggestions[j].Score
		}
		// Secondary sort by value length (shorter = better)
		return len(suggestions[i].Value) < len(suggestions[j].Value)
	})

	// Limit to top 10 suggestions
	if len(suggestions) > 10 {
		suggestions = suggestions[:10]
	}

	return suggestions, nil
}
