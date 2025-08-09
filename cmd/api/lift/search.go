package lift

import (
	"context"
	"fmt"
	"strconv"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/mastodon"
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

	// Perform the search
	actors, err := h.performAccountSearch(ctx.Context, params)
	if err != nil {
		return err
	}

	// Add remote results if resolve is enabled
	if params.resolve {
		h.addRemoteSearchResults(ctx.Context, &actors, params.query, params.limit)
	}

	// Convert results to API format
	accounts := h.convertSearchResultsToAccounts(actors)

	// Set response headers and log analytics
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

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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

// performAccountSearch performs the actual search
func (h *Handler) performAccountSearch(ctx context.Context, params *accountSearchParams) ([]*activitypub.Actor, error) {
	actors, err := h.repos.Search().SearchAccounts(ctx, params.query, params.limit, params.followingOnly, params.offset)
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

// finalizeAccountSearchResponse sets headers and logs analytics
func (h *Handler) finalizeAccountSearchResponse(ctx *lift.Context, params *accountSearchParams, accounts []models.Account, authenticatedUser string) {
	// Add search metadata to response headers
	ctx.Response.Header("X-Total-Count", fmt.Sprintf("%d", len(accounts)))

	// Log search analytics
	h.logger.Info("account search completed",
		zap.String("query", params.query),
		zap.Int("results", len(accounts)),
		zap.Int("limit", params.limit),
		zap.Int("offset", params.offset),
		zap.Bool("authenticated", authenticatedUser != ""))
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

	// Get suggestions from storage
	suggestions, err := h.repos.Search().GetSearchSuggestions(ctx.Context, prefix, 10)
	if err != nil {
		h.logger.Error("failed to get search suggestions",
			zap.String("prefix", prefix),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "suggestions lookup failed"})
	}

	// Convert to API response format
	response := make([]map[string]any, 0, len(suggestions))
	for _, sugg := range suggestions {
		response = append(response, map[string]any{
			"type":  sugg.Type,
			"value": sugg.Term,
			"score": sugg.Score,
		})
	}

	h.logger.Debug("search suggestions returned",
		zap.String("prefix", prefix),
		zap.Int("count", len(response)))

	return ctx.JSON(response)
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
