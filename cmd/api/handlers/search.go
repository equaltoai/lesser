package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// HandleAccountSearchLift handles GET /api/v1/accounts/search
// Search for accounts by username, display name, or domain
func (h *Handler) HandleAccountSearchLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse search parameters
	params, resp, err := h.parseAccountSearchParams(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Authenticate user if needed
	authenticatedUser, resp, err := h.authenticateAccountSearch(ctx, params.followingOnly)
	if resp != nil || err != nil {
		return resp, err
	}

	// Perform the search with privacy enforcement
	actors, err := h.performAccountSearch(ctx.Context(), params, authenticatedUser)
	if err != nil {
		return common.RespondInternalServerError(ctx, "search failed")
	}

	// Add remote results if resolve is enabled
	if params.resolve {
		h.addRemoteSearchResults(ctx.Context(), &actors, params.query, params.limit)
	}

	// Convert results to API format
	accounts := h.convertSearchResultsToAccounts(actors)

	resp, err = okJSON(accounts)
	if err != nil {
		return nil, err
	}
	h.finalizeAccountSearchResponse(ctx, resp, params, accounts, authenticatedUser)

	return resp, nil
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
func (h *Handler) parseAccountSearchParams(ctx *apptheory.Context) (*accountSearchParams, *apptheory.Response, error) {
	params := &accountSearchParams{
		limit:  40,
		offset: 0,
	}

	// Extract query
	params.query = h.extractSearchQuery(ctx)
	if err := common.ValidateSearchQuery(params.query); err != nil {
		resp, respErr := common.RespondValidationError(ctx, err)
		return nil, resp, respErr
	}

	// Parse limit
	params.limit = h.parseSearchLimit(ctx)

	// Parse offset
	offsetStr := queryValue(ctx, "offset")
	if offset, err := common.ParseSearchOffset(offsetStr); err == nil {
		// Additional validation to ensure offset is non-negative
		if err := common.ValidateOffset(offset); err != nil {
			h.logger.Debug("invalid offset value", zap.Error(err))
			params.offset = 0 // Use default on validation error
		} else {
			params.offset = offset
		}
	}

	// Parse following filter
	params.followingOnly = h.parseSearchFollowing(ctx)

	// Parse resolve option
	params.resolve = h.parseSearchResolve(ctx)

	return params, nil, nil
}

// extractSearchQuery extracts the search query parameter
func (h *Handler) extractSearchQuery(ctx *apptheory.Context) string {
	return queryValue(ctx, "q")
}

// parseSearchLimit parses and validates the limit parameter
func (h *Handler) parseSearchLimit(ctx *apptheory.Context) int {
	// Extract limit string from query parameters
	limitStr := queryValue(ctx, "limit")

	// Use centralized search limit parsing
	limit, err := common.ParseSearchLimit(limitStr)
	if err != nil {
		h.logger.Debug("invalid limit parameter", zap.Error(err))
		return 40 // Return default on validation error
	}

	// Hard limit for agents (Phase 1 safety rail).
	if claims, ok := ctx.Get("claims").(*auth.Claims); ok && claims != nil && claims.IsAgent && limit > 50 {
		limit = 50
	}

	return limit
}

// parseSearchFollowing parses the following filter parameter
func (h *Handler) parseSearchFollowing(ctx *apptheory.Context) bool {
	return h.parseBoolParam(ctx, "following")
}

// parseSearchResolve parses the resolve parameter
func (h *Handler) parseSearchResolve(ctx *apptheory.Context) bool {
	return h.parseBoolParam(ctx, "resolve")
}

// authenticateAccountSearch handles authentication for account search
func (h *Handler) authenticateAccountSearch(ctx *apptheory.Context, followingOnly bool) (string, *apptheory.Response, error) {
	// Try to authenticate from header
	authenticatedUser := h.authenticateFromSearchHeader(ctx, followingOnly)

	// If following filter is requested but no auth, return error
	if followingOnly {
		if err := common.ValidateRequiredParam("authenticated_user", authenticatedUser); err != nil {
			resp, respErr := common.RespondUnauthorized(ctx, "authentication required for following filter")
			return "", resp, respErr
		}
	}

	return authenticatedUser, nil, nil
}

// authenticateFromSearchHeader authenticates from Authorization header
func (h *Handler) authenticateFromSearchHeader(ctx *apptheory.Context, _ bool) string {
	// Try to authenticate without requiring it (optional auth)
	username, err := h.authenticateUser(ctx, []string{})
	if err != nil {
		return "" // Authentication is optional for search
	}
	return username
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
			return nil, errors.Join(privacyAwareSearchFailed(), err)
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
		return nil, errors.Join(searchFailed(), err)
	}
	return actors, nil
}

// addRemoteSearchResults adds remote search results if resolve is enabled
func (h *Handler) addRemoteSearchResults(ctx context.Context, actors *[]*activitypub.Actor, query string, limit int) {
	if !isValidHandle(query) {
		return
	}

	factory := h.remoteSearch
	if factory == nil {
		factory = defaultRemoteSearchServiceFactory
	}
	remoteSearchSvc := factory(h.repos)
	if remoteSearchSvc == nil {
		return
	}
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
	accounts := make([]models.Account, 0, len(actors))

	for _, actor := range actors {
		account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
		accounts = append(accounts, account)
	}

	return accounts
}

// finalizeAccountSearchResponse sets headers and logs privacy-safe analytics
func (h *Handler) finalizeAccountSearchResponse(ctx *apptheory.Context, resp *apptheory.Response, params *accountSearchParams, accounts []models.Account, authenticatedUser string) {
	// Add search metadata to response headers
	setHeader(resp, "x-total-count", fmt.Sprintf("%d", len(accounts)))

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
		err := searchRepo.RecordSearchWithPrivacy(ctx.Context(), params.query, "accounts", len(accounts), 0, userID)
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
func (h *Handler) HandleGetSearchSuggestionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract query prefix
	prefix := queryValue(ctx, "q")
	if err := common.ValidateStringLength("prefix", prefix, 2, 500); err != nil {
		// Return empty array for short prefixes
		return okJSON([]models.SearchSuggestion{})
	}

	// Get suggestions from Notes service
	result, err := h.registry.Notes().GetSearchSuggestions(ctx.Context(), &notes.GetSearchSuggestionsQuery{
		Prefix: prefix,
		Limit:  10,
	})
	if err != nil {
		h.logger.Error("failed to get search suggestions",
			zap.String("prefix", prefix),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "suggestions lookup failed")
	}
	suggestions := result.Suggestions

	// Convert to API response format
	response := make([]models.SearchSuggestion, 0, len(suggestions))
	for _, sugg := range suggestions {
		response = append(response, models.SearchSuggestion{
			Type:  sugg.Type,
			Value: sugg.Value,
			Score: sugg.Score,
		})
	}

	h.logger.Debug("search suggestions returned",
		zap.String("prefix", prefix),
		zap.Int("count", len(response)))

	return okJSON(response)
}

// HandleStatusSearchLift handles POST/GET /api/v1/search/statuses
// Search for statuses with privacy enforcement
func (h *Handler) HandleStatusSearchLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse search parameters
	params, resp, err := h.parseStatusSearchParams(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Authenticate user - status search requires authentication for privacy
	authenticatedUser, resp, err := h.authenticateStatusSearch(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Perform the status search with privacy enforcement
	statuses, err := h.performStatusSearch(ctx.Context(), params, authenticatedUser)
	if err != nil {
		return common.RespondInternalServerError(ctx, "search failed")
	}

	// Convert results to API format
	results := h.convertStatusSearchResults(ctx.Context(), statuses, authenticatedUser)

	resp, err = okJSON(results)
	if err != nil {
		return nil, err
	}
	h.finalizeStatusSearchResponse(ctx, resp, params, results, authenticatedUser)

	return resp, nil
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
func (h *Handler) parseStatusSearchParams(ctx *apptheory.Context) (*statusSearchParams, *apptheory.Response, error) {
	params := &statusSearchParams{
		limit: 20,
	}

	// Extract query
	params.query = h.extractSearchQuery(ctx)
	if err := common.ValidateSearchQuery(params.query); err != nil {
		resp, respErr := common.RespondValidationError(ctx, err)
		return nil, resp, respErr
	}

	// Parse limit
	params.limit = h.parseSearchLimit(ctx)
	if params.limit > 40 {
		params.limit = 40 // Status search has lower limit for performance
	}

	// Parse pagination parameters
	if maxID := queryValue(ctx, "max_id"); maxID != "" {
		params.maxID = &maxID
	}
	if minID := queryValue(ctx, "min_id"); minID != "" {
		params.minID = &minID
	}

	// Parse account filter
	params.accountID = queryValue(ctx, "account_id")

	// Parse local only filter
	params.localOnly = queryValue(ctx, "local") == "true"

	return params, nil, nil
}

// authenticateStatusSearch handles authentication for status search
func (h *Handler) authenticateStatusSearch(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return "", resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx, "authentication required for status search")
		return "", resp, respErr
	}

	return claims.Username, nil, nil
}

// performStatusSearch performs the actual status search with privacy enforcement
func (h *Handler) performStatusSearch(ctx context.Context, params *statusSearchParams, authenticatedUser string) ([]storage.StatusSearchResult, error) {
	searchRepo := h.repos.Search()
	if searchRepo == nil {
		return nil, statusSearchFailed()
	}

	// Convert username to actor ID for privacy checks
	searcherActorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, authenticatedUser)

	// Build search options
	options := storage.StatusSearchOptions{
		Limit:     params.limit,
		AccountID: params.accountID,
		LocalOnly: params.localOnly,
	}

	results, err := searchRepo.SearchStatusesWithPrivacy(ctx, params.query, options, searcherActorID)
	if err != nil {
		h.logger.Error("privacy-aware status search failed",
			zap.String("query", params.query),
			zap.Int("limit", params.limit),
			zap.String("searcher", authenticatedUser),
			zap.Error(err))
		return nil, errors.Join(privacyAwareStatusSearchFailed(), err)
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
func (h *Handler) convertStatusSearchResults(ctx context.Context, statuses []storage.StatusSearchResult, viewerUsername string) []models.StatusSearchResult {
	results := make([]models.StatusSearchResult, 0, len(statuses))

	for _, status := range statuses {
		result := models.StatusSearchResult{
			ID:              status.StatusID,
			Content:         status.Content,
			URL:             status.URL,
			AccountID:       status.AuthorID,
			AccountUsername: status.AuthorUsername,
			CreatedAt:       status.Published.Format(time.RFC3339),
			Score:           status.Score,
		}

		if fullStatus, err := h.resolveStatusFromSearchResult(ctx, &status); err == nil && fullStatus != nil {
			if !h.statusVisibleInSearch(fullStatus, viewerUsername) {
				continue
			}
			if apiStatus, convErr := h.convertStorageStatusToAPI(fullStatus, viewerUsername); convErr == nil && apiStatus != nil {
				result.ID = apiStatus.ID
				result.Content = apiStatus.Content
				result.URL = apiStatus.URL
				result.AccountID = apiStatus.Account.ID
				result.AccountUsername = apiStatus.Account.Username
				result.CreatedAt = apiStatus.CreatedAt
			}
		} else if !searchResultVisibleInSearch(&status, viewerUsername) {
			continue
		}

		// Add highlights if available
		if len(status.Highlights) > 0 {
			result.Highlights = status.Highlights
		}

		results = append(results, result)
	}

	return results
}

// finalizeStatusSearchResponse sets headers and logs privacy-safe analytics
func (h *Handler) finalizeStatusSearchResponse(ctx *apptheory.Context, resp *apptheory.Response, params *statusSearchParams, results []models.StatusSearchResult, authenticatedUser string) {
	// Add search metadata to response headers
	setHeader(resp, "x-total-count", fmt.Sprintf("%d", len(results)))

	// Record privacy-safe search analytics
	var userID *string
	if authenticatedUser != "" {
		userID = &authenticatedUser
	}

	// Record the search event for analytics
	searchRepo := h.repos.Search()
	if searchRepo != nil {
		err := searchRepo.RecordSearchWithPrivacy(ctx.Context(), params.query, "statuses", len(results), 0, userID)
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

// isValidHandle checks if a query looks like a remote-resolvable handle.
func isValidHandle(query string) bool {
	query = strings.TrimPrefix(query, "@")
	return strings.Count(query, "@") == 1
}
