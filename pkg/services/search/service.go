// Package search provides search and discovery services for the Lesser ActivityPub server.
//
// This service handles all operations related to searching and discovering content including:
// - Account search and suggestions
// - Content search (statuses, hashtags)
// - Profile directory browsing
// - Follow suggestions and recommendations
package search

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Service provides business logic for search and discovery operations
type Service struct {
	searchRepo       *repositories.SearchRepository
	actorRepo        *repositories.ActorRepository
	relationshipRepo *repositories.RelationshipRepository
	statusRepo       *repositories.StatusRepository
	hashtagRepo      *repositories.HashtagRepository
	publisher        streaming.Publisher
	logger           *zap.Logger
	domain           string
}

// NewService creates a new search service
func NewService(
	searchRepo *repositories.SearchRepository,
	actorRepo *repositories.ActorRepository,
	relationshipRepo *repositories.RelationshipRepository,
	statusRepo *repositories.StatusRepository,
	hashtagRepo *repositories.HashtagRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
	domain string,
) *Service {
	return &Service{
		searchRepo:       searchRepo,
		actorRepo:        actorRepo,
		relationshipRepo: relationshipRepo,
		statusRepo:       statusRepo,
		hashtagRepo:      hashtagRepo,
		publisher:        publisher,
		logger:           logger,
		domain:           domain,
	}
}

// Query and Command types for CQRS pattern

// Query contains parameters for searching content
type Query struct {
	Query             string `json:"query" validate:"required,min=1,max=500"`
	AccountID         string `json:"account_id,omitempty"`
	Type              string `json:"type,omitempty"` // accounts, hashtags, statuses
	Resolve           bool   `json:"resolve"`        // Whether to resolve remote accounts
	Following         bool   `json:"following"`      // Only from accounts the user follows
	ExcludeUnreviewed bool   `json:"exclude_unreviewed"`
	Limit             int    `json:"limit"`
	Offset            int    `json:"offset"`
}

// DirectoryQuery contains parameters for browsing the profile directory
type DirectoryQuery struct {
	Local  bool   `json:"local"` // Only local accounts
	Order  string `json:"order"` // active, new
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// SuggestionsQuery contains parameters for getting follow suggestions
type SuggestionsQuery struct {
	Username string `json:"username" validate:"required"`
	Limit    int    `json:"limit"`
	Version  int    `json:"version"` // API version (1 or 2)
}

// RemoveSuggestionCommand contains data to remove a suggestion
type RemoveSuggestionCommand struct {
	Username  string `json:"username" validate:"required"`
	AccountID string `json:"account_id" validate:"required"`
}

// Result types

// Result contains search results
type Result struct {
	Accounts []AccountResult    `json:"accounts"`
	Statuses []StatusResult     `json:"statuses"`
	Hashtags []HashtagResult    `json:"hashtags"`
	Events   []*streaming.Event `json:"events"`
}

// AccountResult represents an account in search results
type AccountResult struct {
	Actor          *activitypub.Actor `json:"actor"`
	FollowersCount int                `json:"followers_count"`
	FollowingCount int                `json:"following_count"`
	StatusesCount  int                `json:"statuses_count"`
	IsLocal        bool               `json:"is_local"`
	LastStatusAt   string             `json:"last_status_at,omitempty"`
}

// StatusResult represents a status in search results
type StatusResult struct {
	Status       interface{} `json:"status"` // Using interface{} as Status type varies
	ReblogsCount int         `json:"reblogs_count"`
	LikesCount   int         `json:"likes_count"`
	RepliesCount int         `json:"replies_count"`
}

// HashtagResult represents a hashtag in search results
type HashtagResult struct {
	Name      string           `json:"name"`
	URL       string           `json:"url"`
	History   []HashtagHistory `json:"history"`
	Following bool             `json:"following"`
}

// HashtagHistory represents hashtag usage statistics
type HashtagHistory struct {
	Day      string `json:"day"`
	Uses     int    `json:"uses"`
	Accounts int    `json:"accounts"`
}

// DirectoryResult contains directory listings
type DirectoryResult struct {
	Accounts []AccountResult    `json:"accounts"`
	Events   []*streaming.Event `json:"events"`
}

// SuggestionsResult contains follow suggestions
type SuggestionsResult struct {
	Suggestions []SuggestionItem   `json:"suggestions"`
	Events      []*streaming.Event `json:"events"`
}

// SuggestionItem represents a single suggestion
type SuggestionItem struct {
	Account AccountResult `json:"account"`
	Source  string        `json:"source,omitempty"` // For v2: staff, past_interactions, global
}

// Search performs a search across accounts, statuses, and hashtags
func (s *Service) Search(ctx context.Context, query *Query) (*Result, error) {
	s.logger.Info("performing search",
		zap.String("query", query.Query),
		zap.String("type", query.Type),
		zap.Bool("resolve", query.Resolve))

	result := &Result{
		Accounts: []AccountResult{},
		Statuses: []StatusResult{},
		Hashtags: []HashtagResult{},
		Events:   []*streaming.Event{},
	}

	// Set default limit
	if query.Limit <= 0 {
		query.Limit = 20
	}

	// Search based on type filter
	switch query.Type {
	case "accounts":
		accounts, err := s.searchAccounts(ctx, query)
		if err != nil {
			s.logger.Error("account search failed", zap.Error(err), zap.String("query", query.Query))
			return nil, errors.Join(pkgerrors.ErrSearchAccounts, err)
		}
		result.Accounts = accounts

	case "hashtags":
		hashtags, err := s.searchHashtags(ctx, query)
		if err != nil {
			s.logger.Error("hashtag search failed", zap.Error(err), zap.String("query", query.Query))
			return nil, errors.Join(pkgerrors.ErrSearchHashtags, err)
		}
		result.Hashtags = hashtags

	case "statuses":
		statuses, err := s.searchStatuses(ctx, query)
		if err != nil {
			s.logger.Error("status search failed", zap.Error(err), zap.String("query", query.Query))
			return nil, errors.Join(pkgerrors.ErrSearchStatuses, err)
		}
		result.Statuses = statuses

	default:
		// Search all types
		accounts, _ := s.searchAccounts(ctx, query)
		result.Accounts = accounts

		hashtags, _ := s.searchHashtags(ctx, query)
		result.Hashtags = hashtags

		statuses, _ := s.searchStatuses(ctx, query)
		result.Statuses = statuses
	}

	// Emit search event for analytics
	s.emitSearchEvent(ctx, query)

	return result, nil
}

// GetDirectory retrieves the profile directory
func (s *Service) GetDirectory(ctx context.Context, query *DirectoryQuery) (*DirectoryResult, error) {
	s.logger.Info("getting directory",
		zap.Bool("local", query.Local),
		zap.String("order", query.Order),
		zap.Int("limit", query.Limit))

	// Set defaults
	if query.Limit <= 0 || query.Limit > 80 {
		query.Limit = 40
	}

	// Get discoverable accounts
	actors, err := s.searchRepo.SearchAccounts(ctx, "", query.Limit*2, false, query.Offset)
	if err != nil {
		s.logger.Error("directory retrieval failed", zap.Error(err), zap.String("order", query.Order), zap.Int("limit", query.Limit))
		return nil, errors.Join(pkgerrors.ErrGetDirectory, err)
	}

	// Convert to account results
	accounts := make([]AccountResult, 0, len(actors))
	for _, actor := range actors {
		// Filter local only if requested
		isLocal := s.isLocal(actor.ID)
		if query.Local && !isLocal {
			continue
		}

		account := s.buildAccountResult(ctx, actor, isLocal)
		accounts = append(accounts, account)
	}

	// Sort based on order
	accounts = s.sortAccounts(accounts, query.Order)

	return &DirectoryResult{
		Accounts: accounts,
		Events:   nil,
	}, nil
}

// GetSuggestions retrieves follow suggestions for a user
func (s *Service) GetSuggestions(ctx context.Context, query *SuggestionsQuery) (*SuggestionsResult, error) {
	s.logger.Info("getting suggestions",
		zap.String("username", query.Username),
		zap.Int("limit", query.Limit),
		zap.Int("version", query.Version))

	// Set default limit
	if query.Limit <= 0 || query.Limit > 80 {
		query.Limit = 40
	}

	// Get suggestions using the actor repository
	actors, err := s.actorRepo.GetAccountSuggestions(ctx, query.Username, query.Limit)
	if err != nil {
		s.logger.Error("suggestions retrieval failed", zap.Error(err), zap.String("username", query.Username), zap.Int("limit", query.Limit))
		return nil, errors.Join(pkgerrors.ErrGetSuggestions, err)
	}

	// Convert to suggestion items
	suggestions := make([]SuggestionItem, 0, len(actors))
	for _, actor := range actors {
		isLocal := s.isLocal(actor.ID)
		account := s.buildAccountResult(ctx, actor, isLocal)

		item := SuggestionItem{
			Account: account,
		}

		// Add source for v2 API
		if query.Version == 2 {
			item.Source = s.determineSuggestionSource(ctx, query.Username, actor)
		}

		suggestions = append(suggestions, item)
	}

	return &SuggestionsResult{
		Suggestions: suggestions,
		Events:      nil,
	}, nil
}

// RemoveSuggestion removes an account from suggestions
func (s *Service) RemoveSuggestion(ctx context.Context, cmd *RemoveSuggestionCommand) error {
	s.logger.Info("removing suggestion",
		zap.String("username", cmd.Username),
		zap.String("account_id", cmd.AccountID))

	// Remove the suggestion
	if err := s.actorRepo.RemoveAccountSuggestion(ctx, cmd.Username, cmd.AccountID); err != nil {
		s.logger.Error("suggestion removal failed", zap.Error(err), zap.String("username", cmd.Username), zap.String("account_id", cmd.AccountID))
		return errors.Join(pkgerrors.ErrRemoveSuggestion, err)
	}

	// Emit event for real-time updates
	s.emitSuggestionRemovedEvent(ctx, cmd)

	return nil
}

// Helper methods

// searchAccounts searches for accounts matching the query
func (s *Service) searchAccounts(ctx context.Context, query *Query) ([]AccountResult, error) {
	actors, err := s.searchRepo.SearchAccounts(ctx, query.Query, query.Limit, query.Following, query.Offset)
	if err != nil {
		return nil, err
	}

	accounts := make([]AccountResult, 0, len(actors))
	for _, actor := range actors {
		isLocal := s.isLocal(actor.ID)
		account := s.buildAccountResult(ctx, actor, isLocal)
		accounts = append(accounts, account)
	}

	return accounts, nil
}

// searchStatuses searches for statuses matching the query
func (s *Service) searchStatuses(ctx context.Context, query *Query) ([]StatusResult, error) {
	s.logger.Info("searching statuses",
		zap.String("query", query.Query),
		zap.Int("limit", query.Limit))

	// Search for statuses that contain the query terms
	searchResults, err := s.searchRepo.SearchStatuses(ctx, query.Query, query.Limit)
	if err != nil {
		s.logger.Error("failed to search statuses", zap.Error(err))
		return nil, err
	}

	var statusResults []StatusResult
	for _, searchResult := range searchResults {
		// Get real engagement metrics from storage
		var reblogsCount, likesCount, repliesCount int

		// Extract status ID from search result (assuming it has an ID field)
		if statusID := getStatusIDFromResult(searchResult); statusID != "" {
			// Get all status counts in a single call for efficiency
			likes, reblogs, replies, err := s.statusRepo.GetStatusCounts(ctx, statusID)
			if err == nil {
				likesCount = likes
				reblogsCount = reblogs
				repliesCount = replies
			}
		}

		result := StatusResult{
			Status:       searchResult,
			ReblogsCount: reblogsCount,
			LikesCount:   likesCount,
			RepliesCount: repliesCount,
		}

		statusResults = append(statusResults, result)
	}

	s.logger.Info("status search completed",
		zap.String("query", query.Query),
		zap.Int("results", len(statusResults)))

	return statusResults, nil
}

// searchHashtags searches for hashtags matching the query
func (s *Service) searchHashtags(ctx context.Context, query *Query) ([]HashtagResult, error) {
	s.logger.Info("searching hashtags",
		zap.String("query", query.Query),
		zap.Int("limit", query.Limit))

	// Clean the query - remove # prefix if present
	hashtagQuery := strings.TrimPrefix(query.Query, "#")

	// Search for hashtags that match the query
	hashtags, err := s.searchRepo.SearchHashtags(ctx, hashtagQuery, query.Limit)
	if err != nil {
		s.logger.Error("failed to search hashtags", zap.Error(err))
		return nil, err
	}

	var hashtagResults []HashtagResult
	for _, hashtag := range hashtags {
		// Build hashtag history from available data
		history := []HashtagHistory{}
		if hashtag.UsageCount > 0 {
			// Create a simple history entry based on available data
			history = append(history, HashtagHistory{
				Day:      time.Now().Format("2006-01-02"),
				Uses:     int(hashtag.UsageCount),
				Accounts: hashtag.Accounts,
			})
		}

		// Check if current user follows this hashtag
		following := false
		if query.AccountID != "" && s.hashtagRepo != nil {
			if isFollowing, err := s.hashtagRepo.IsFollowingHashtag(ctx, query.AccountID, hashtag.Name); err == nil {
				following = isFollowing
			} else {
				s.logger.Warn("failed to check hashtag following status",
					zap.String("user_id", query.AccountID),
					zap.String("hashtag", hashtag.Name),
					zap.Error(err))
			}
		}

		// Build the result using available HashtagResult fields
		result := HashtagResult{
			Name:      hashtag.Name,
			URL:       fmt.Sprintf("https://%s/tags/%s", s.domain, hashtag.Name),
			History:   history,
			Following: following,
		}

		hashtagResults = append(hashtagResults, result)
	}

	// Sort by usage count (most popular first)
	sort.Slice(hashtagResults, func(i, j int) bool {
		if len(hashtagResults[i].History) > 0 && len(hashtagResults[j].History) > 0 {
			return hashtagResults[i].History[0].Uses > hashtagResults[j].History[0].Uses
		}
		return false
	})

	s.logger.Info("hashtag search completed",
		zap.String("query", query.Query),
		zap.Int("results", len(hashtagResults)))

	return hashtagResults, nil
}

// buildAccountResult builds an account result with counts
func (s *Service) buildAccountResult(ctx context.Context, actor *activitypub.Actor, isLocal bool) AccountResult {
	followersCount, _ := s.relationshipRepo.CountFollowers(ctx, actor.ID)
	followingCount, _ := s.relationshipRepo.CountFollowing(ctx, actor.ID)
	statusesCount, _ := s.statusRepo.CountStatusesByAuthor(ctx, actor.ID)

	result := AccountResult{
		Actor:          actor,
		FollowersCount: followersCount,
		FollowingCount: followingCount,
		StatusesCount:  statusesCount,
		IsLocal:        isLocal,
	}

	// Format last status time if available
	if actor.LastStatusAt != nil {
		result.LastStatusAt = actor.LastStatusAt.Format("2006-01-02T15:04:05.000Z")
	}

	return result
}

// isLocal checks if an actor ID is local
func (s *Service) isLocal(actorID string) bool {
	return strings.HasPrefix(actorID, "https://"+s.domain+"/") ||
		strings.HasPrefix(actorID, "http://"+s.domain+"/")
}

// sortAccounts sorts accounts based on the specified order
func (s *Service) sortAccounts(accounts []AccountResult, _ string) []AccountResult {
	// For now, return as-is - implement sorting when needed
	return accounts
}

// determineSuggestionSource determines why an account was suggested
func (s *Service) determineSuggestionSource(_ context.Context, _ string, _ *activitypub.Actor) string {
	// Check various sources
	// For now, return "global" - enhance with more logic as needed
	return "global"
}

// Helper methods

// getStatusIDFromResult extracts the status ID from a search result
func getStatusIDFromResult(result interface{}) string {
	// Handle different types of search results
	switch r := result.(type) {
	case map[string]interface{}:
		if id, ok := r["id"].(string); ok {
			return id
		}
	case struct{ ID string }:
		return r.ID
	default:
		// Try to use reflection or type assertion for other result types
		// For now, return empty string if we can't extract the ID
	}
	return ""
}

// Event emission methods

func (s *Service) emitSearchEvent(ctx context.Context, query *Query) {
	if s.publisher == nil {
		return
	}

	// Track search for analytics
	event := &streaming.Event{
		Type:      "search.performed",
		Stream:    "analytics",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"query": query.Query,
			"type":  query.Type,
		},
	}

	// Publish to analytics stream
	if err := s.publisher.PublishToStream(ctx, "analytics", event); err != nil {
		s.logger.Warn("failed to publish search event", zap.Error(err))
	}
}

func (s *Service) emitSuggestionRemovedEvent(ctx context.Context, cmd *RemoveSuggestionCommand) {
	if s.publisher == nil {
		return
	}

	event := &streaming.Event{
		Type:      "suggestion.removed",
		Stream:    fmt.Sprintf("user:%s", cmd.Username),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"account_id": cmd.AccountID,
		},
	}

	// Publish to user's stream
	if err := s.publisher.PublishToUser(ctx, cmd.Username, event); err != nil {
		s.logger.Warn("failed to publish suggestion removed event", zap.Error(err))
	}
}
