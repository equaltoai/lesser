package lift

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleAccountSearchLift handles GET /api/v1/accounts/search
// Search for accounts by username, display name, or domain
func (h *Handler) HandleAccountSearchLift(ctx *lift.Context) error {
	// Parse search parameters
	params, err := h.parseAccountSearchParams(ctx)
	if err != nil {
		return err
	}

	// Authenticate user if needed
	authenticatedUser, err := h.authenticateAccountSearch(ctx, params.followingOnly)
	if err != nil {
		return err
	}

	// Perform the search with privacy enforcement
	actors, err := h.performAccountSearch(ctx.Context, params, authenticatedUser)
	if err != nil {
		return err
	}

	// Add remote results if resolve is enabled
	if params.resolve {
		h.addRemoteSearchResults(ctx.Context, &actors, params.query, params.limit)
	}

	// Convert results to API format
	accounts := h.convertSearchResultsToAccounts(actors)

	// Record privacy-safe analytics and set response headers
	h.finalizeAccountSearchResponse(ctx, params, accounts, authenticatedUser)

	return ctx.JSON(accounts)
}

// accountSearchParams holds parsed search parameters
type accountSearchParams struct {
	query         string
	limit         int
	offset        int
	followingOnly bool
	resolve       bool
}

// parseAccountSearchParams parses and validates search parameters
func (h *Handler) parseAccountSearchParams(ctx *lift.Context) (*accountSearchParams, error) {
	params := &accountSearchParams{
		limit:  40,
		offset: 0,
	}

	// Extract query
	params.query = h.extractSearchQuery(ctx)
	if params.query == "" {
		return nil, ctx.Status(400).JSON(map[string]string{"error": "q parameter is required"})
	}

	// Parse limit
	params.limit = h.parseSearchLimit(ctx)

	// Parse offset
	params.offset = h.parseSearchOffset(ctx)

	// Parse following filter
	params.followingOnly = h.parseSearchFollowing(ctx)

	// Parse resolve option
	params.resolve = h.parseSearchResolve(ctx)

	return params, nil
}

// extractSearchQuery extracts the search query parameter
func (h *Handler) extractSearchQuery(ctx *lift.Context) string {
	query := ctx.Query("q")
	if query == "" && ctx.Request != nil && ctx.Request.Request != nil {
		query = ctx.Request.Request.QueryParams["q"]
	}
	return query
}

// parseSearchLimit parses and validates the limit parameter
func (h *Handler) parseSearchLimit(ctx *lift.Context) int {
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}

	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			if parsedLimit > 80 {
				return 80
			} else if parsedLimit < 1 {
				return 1
			}
			return parsedLimit
		}
	}
	return 40
}

// parseSearchOffset parses the offset parameter
func (h *Handler) parseSearchOffset(ctx *lift.Context) int {
	offsetStr := ctx.Query("offset")
	if offsetStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		offsetStr = ctx.Request.Request.QueryParams["offset"]
	}

	if offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil {
			return parsedOffset
		}
	}
	return 0
}

// parseSearchFollowing parses the following filter parameter
func (h *Handler) parseSearchFollowing(ctx *lift.Context) bool {
	followingParam := ctx.Query("following")
	if followingParam == "" && ctx.Request != nil && ctx.Request.Request != nil {
		followingParam = ctx.Request.Request.QueryParams["following"]
	}
	return followingParam == boolTrue
}

// parseSearchResolve parses the resolve parameter
func (h *Handler) parseSearchResolve(ctx *lift.Context) bool {
	resolveParam := ctx.Query("resolve")
	if resolveParam == "" && ctx.Request != nil && ctx.Request.Request != nil {
		resolveParam = ctx.Request.Request.QueryParams["resolve"]
	}
	return resolveParam == boolTrue
}

// authenticateAccountSearch handles authentication for account search
func (h *Handler) authenticateAccountSearch(ctx *lift.Context, followingOnly bool) (string, error) {
	// Check test mode first
	testUsername := h.extractTestUsernameForSearch(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Try to authenticate from header
	authenticatedUser := h.authenticateFromSearchHeader(ctx, followingOnly)

	// If following filter is requested but no auth, return error
	if followingOnly && authenticatedUser == "" {
		return "", ctx.Status(401).JSON(map[string]string{"error": "authentication required for following filter"})
	}

	return authenticatedUser, nil
}

// extractTestUsernameForSearch extracts test username from headers
func (h *Handler) extractTestUsernameForSearch(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// authenticateFromSearchHeader authenticates from Authorization header
func (h *Handler) authenticateFromSearchHeader(ctx *lift.Context, followingOnly bool) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		return ""
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ""
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ""
	}

	// Check read scope if following filter is requested
	if followingOnly && !claims.HasScope(auth.ScopeRead) {
		return ""
	}

	return claims.Username
}

// performAccountSearch performs the actual search with privacy enforcement
func (h *Handler) performAccountSearch(ctx context.Context, params *accountSearchParams, authenticatedUser string) ([]*activitypub.Actor, error) {
	// Use privacy-aware search method
	searchRepo := h.repos.Search()
	
	// Convert username to actor ID for privacy checks
	var searcherActorID string
	if authenticatedUser != "" {
		// Build actor ID from username - adjust format as needed for your system
		searcherActorID = fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, authenticatedUser)
	}

	// Use privacy-aware search if we have an authenticated user
	if searcherActorID != "" {
		actors, err := searchRepo.SearchAccountsWithPrivacy(ctx, params.query, params.limit, params.followingOnly, params.offset, searcherActorID)
		if err != nil {
			h.logger.Error("privacy-aware account search failed",
				zap.String("query", params.query),
				zap.Int("limit", params.limit),
				zap.Int("offset", params.offset),
				zap.Bool("following", params.followingOnly),
				zap.String("searcher", authenticatedUser),
				zap.Error(err))
			return nil, fmt.Errorf("privacy-aware search failed: %w", err)
		}
		return actors, nil
	}

	// Fallback to regular search for unauthenticated users
	actors, err := searchRepo.SearchAccounts(ctx, params.query, params.limit, params.followingOnly, params.offset)
	if err != nil {
		h.logger.Error("account search failed",
			zap.String("query", params.query),
			zap.Int("limit", params.limit),
			zap.Int("offset", params.offset),
			zap.Bool("following", params.followingOnly),
			zap.Error(err))
		return nil, fmt.Errorf("search failed: %w", err)
	}
	return actors, nil
}

// addRemoteSearchResults adds remote search results if resolve is enabled
func (h *Handler) addRemoteSearchResults(ctx context.Context, actors *[]*activitypub.Actor, query string, limit int) {
	if !isValidHandle(query) {
		return
	}

	remoteSearchSvc := federation.NewRemoteSearchService(h.repos)
	remoteResults, err := remoteSearchSvc.SearchRemoteActors(ctx, query, limit)
	if err != nil {
		h.logger.Debug("remote search failed",
			zap.String("query", query),
			zap.Error(err))
		return
	}

	for _, result := range remoteResults {
		if result.Actor != nil {
			*actors = append(*actors, result.Actor)
		}
	}
}

// convertSearchResultsToAccounts converts actors to API account format
func (h *Handler) convertSearchResultsToAccounts(actors []*activitypub.Actor) []models.Account {
	converter := mastodon.NewConverter(h.cfg.BaseURL())
	accounts := make([]models.Account, 0, len(actors))

	for _, actor := range actors {
		account := converter.ActorToAccount(actor)
		accounts = append(accounts, account)
	}

	return accounts
}

// finalizeAccountSearchResponse sets headers and logs privacy-safe analytics
func (h *Handler) finalizeAccountSearchResponse(ctx *lift.Context, params *accountSearchParams, accounts []models.Account, authenticatedUser string) {
	// Add search metadata to response headers
	ctx.Response.Header("X-Total-Count", fmt.Sprintf("%d", len(accounts)))

	// Record privacy-safe search analytics
	var userID *string
	if authenticatedUser != "" && len(authenticatedUser) > 0 {
		userID = &authenticatedUser
	}

	// Record the search event for analytics
	searchRepo := h.repos.Search()
	// Record search analytics
	if searchRepo != nil {
		// We don't have search time here, so we'll use 0
		err := searchRepo.RecordSearchWithPrivacy(ctx.Context, params.query, "accounts", len(accounts), 0, userID)
		if err != nil {
			h.logger.Warn("failed to record search analytics",
				zap.String("search_type", "accounts"),
				zap.Error(err))
		}
	}

	// Log search completion (without sensitive query data)
	h.logger.Info("account search completed",
		zap.String("query_length", fmt.Sprintf("%d_chars", len(params.query))),
		zap.Int("results", len(accounts)),
		zap.Int("limit", params.limit),
		zap.Int("offset", params.offset),
		zap.Bool("authenticated", authenticatedUser != ""),
		zap.Bool("following_filter", params.followingOnly))
}

// HandleGetSearchSuggestionsLift handles GET /api/v1/accounts/search/suggestions
// Returns search suggestions for autocomplete
func (h *Handler) HandleGetSearchSuggestionsLift(ctx *lift.Context) error {
	// Extract query prefix
	prefix := ctx.Query("q")
	if prefix == "" && ctx.Request != nil && ctx.Request.Request != nil {
		prefix = ctx.Request.Request.QueryParams["q"]
	}
	if len(prefix) < 2 {
		// Return empty array for short prefixes
		return ctx.JSON([]any{})
	}

	// Get suggestions from Notes service
	result, err := h.registry.Notes().GetSearchSuggestions(ctx.Context, &notes.GetSearchSuggestionsQuery{
		Prefix: prefix,
		Limit:  10,
	})
	if err != nil {
		h.logger.Error("failed to get search suggestions",
			zap.String("prefix", prefix),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "suggestions lookup failed"})
	}
	suggestions := result.Suggestions

	// Convert to API response format
	response := make([]map[string]any, 0, len(suggestions))
	for _, sugg := range suggestions {
		response = append(response, map[string]any{
			"type":  sugg.Type,
			"value": sugg.Value,
			"score": sugg.Score,
		})
	}

	h.logger.Debug("search suggestions returned",
		zap.String("prefix", prefix),
		zap.Int("count", len(response)))

	return ctx.JSON(response)
}

// HandleStatusSearchLift handles POST/GET /api/v1/search/statuses
// Search for statuses with privacy enforcement
func (h *Handler) HandleStatusSearchLift(ctx *lift.Context) error {
	// Parse search parameters
	params, err := h.parseStatusSearchParams(ctx)
	if err != nil {
		return err
	}

	// Authenticate user - status search requires authentication for privacy
	authenticatedUser, err := h.authenticateStatusSearch(ctx)
	if err != nil {
		return err
	}

	// Perform the status search with privacy enforcement
	statuses, err := h.performStatusSearch(ctx.Context, params, authenticatedUser)
	if err != nil {
		return err
	}

	// Convert results to API format
	results := h.convertStatusSearchResults(statuses)

	// Set response headers and log analytics
	h.finalizeStatusSearchResponse(ctx, params, results, authenticatedUser)

	return ctx.JSON(results)
}

// statusSearchParams holds parsed status search parameters
type statusSearchParams struct {
	query     string
	limit     int
	maxID     *string
	minID     *string
	accountID string
	localOnly bool
}

// parseStatusSearchParams parses and validates status search parameters
func (h *Handler) parseStatusSearchParams(ctx *lift.Context) (*statusSearchParams, error) {
	params := &statusSearchParams{
		limit: 20,
	}

	// Extract query
	params.query = h.extractSearchQuery(ctx)
	if params.query == "" {
		return nil, ctx.Status(400).JSON(map[string]string{"error": "q parameter is required"})
	}

	// Parse limit
	params.limit = h.parseSearchLimit(ctx)
	if params.limit > 40 {
		params.limit = 40 // Status search has lower limit for performance
	}

	// Parse pagination parameters
	if maxID := ctx.Query("max_id"); maxID != "" {
		params.maxID = &maxID
	}
	if minID := ctx.Query("min_id"); minID != "" {
		params.minID = &minID
	}

	// Parse account filter
	params.accountID = ctx.Query("account_id")

	// Parse local only filter
	params.localOnly = ctx.Query("local") == boolTrue

	return params, nil
}

// authenticateStatusSearch handles authentication for status search
func (h *Handler) authenticateStatusSearch(ctx *lift.Context) (string, error) {
	// Check test mode first
	testUsername := h.extractTestUsernameForSearch(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Status search requires authentication for privacy
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		return "", ctx.Status(401).JSON(map[string]string{"error": "authentication required for status search"})
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "invalid authorization header"})
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "invalid access token"})
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// performStatusSearch performs the actual status search with privacy enforcement
func (h *Handler) performStatusSearch(ctx context.Context, params *statusSearchParams, authenticatedUser string) ([]storage.StatusSearchResult, error) {
	searchRepo := h.repos.Search()
	
	// Convert username to actor ID for privacy checks
	searcherActorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, authenticatedUser)

	// Build search options
	options := storage.StatusSearchOptions{
		Limit:     params.limit,
		AccountID: params.accountID,
		LocalOnly: params.localOnly,
	}

	// Use privacy-aware search
	if searchRepo != nil {
		results, err := searchRepo.SearchStatusesWithPrivacy(ctx, params.query, options, searcherActorID)
		if err != nil {
			h.logger.Error("privacy-aware status search failed",
				zap.String("query", params.query),
				zap.Int("limit", params.limit),
				zap.String("searcher", authenticatedUser),
				zap.Error(err))
			return nil, fmt.Errorf("privacy-aware status search failed: %w", err)
		}

		// Convert pointer slice to value slice
		statuses := make([]storage.StatusSearchResult, 0, len(results))
		for _, result := range results {
			if result != nil {
				statuses = append(statuses, *result)
			}
		}
		return statuses, nil
	}

	// Fallback to regular search (though this should be avoided for status search)
	results, err := searchRepo.SearchStatusesWithOptions(ctx, params.query, options)
	if err != nil {
		h.logger.Error("status search failed",
			zap.String("query", params.query),
			zap.Int("limit", params.limit),
			zap.Error(err))
		return nil, fmt.Errorf("status search failed: %w", err)
	}

	// Convert pointer slice to value slice
	statuses := make([]storage.StatusSearchResult, 0, len(results))
	for _, result := range results {
		if result != nil {
			statuses = append(statuses, *result)
		}
	}
	return statuses, nil
}

// convertStatusSearchResults converts status search results to API format
func (h *Handler) convertStatusSearchResults(statuses []storage.StatusSearchResult) []map[string]interface{} {
	results := make([]map[string]interface{}, 0, len(statuses))

	for _, status := range statuses {
		result := map[string]interface{}{
			"id":               status.StatusID,
			"content":          status.Content,
			"url":              status.URL,
			"account_id":       status.AuthorID,
			"account_username": status.AuthorUsername,
			"created_at":       status.Published.Format(time.RFC3339),
			"score":            status.Score,
		}

		// Add highlights if available
		if len(status.Highlights) > 0 {
			result["highlights"] = status.Highlights
		}

		results = append(results, result)
	}

	return results
}

// finalizeStatusSearchResponse sets headers and logs privacy-safe analytics
func (h *Handler) finalizeStatusSearchResponse(ctx *lift.Context, params *statusSearchParams, results []map[string]interface{}, authenticatedUser string) {
	// Add search metadata to response headers
	ctx.Response.Header("X-Total-Count", fmt.Sprintf("%d", len(results)))

	// Record privacy-safe search analytics
	var userID *string
	if authenticatedUser != "" {
		userID = &authenticatedUser
	}

	// Record the search event for analytics
	searchRepo := h.repos.Search()
	if searchRepo != nil {
		err := searchRepo.RecordSearchWithPrivacy(ctx.Context, params.query, "statuses", len(results), 0, userID)
		if err != nil {
			h.logger.Warn("failed to record status search analytics",
				zap.String("search_type", "statuses"),
				zap.Error(err))
		}
	}

	// Log search completion (without sensitive query data)
	h.logger.Info("status search completed",
		zap.String("query_length", fmt.Sprintf("%d_chars", len(params.query))),
		zap.Int("results", len(results)),
		zap.Int("limit", params.limit),
		zap.String("searcher", authenticatedUser),
		zap.Bool("local_only", params.localOnly))
}

// isValidHandle checks if a query looks like a federated handle (@user@domain.com)
func isValidHandle(query string) bool {
	// Simple check for @user@domain pattern
	if len(query) < 5 {
		return false
	}

	atCount := 0
	for _, ch := range query {
		if ch == '@' {
			atCount++
		}
	}

	// Should have exactly 2 @ symbols for federated handle
	return atCount == 2 || (atCount == 1 && query[0] == '@')
}
