package lift

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetDirectoryLift handles GET /api/v1/directory
// Returns profile directory of discoverable accounts
func (h *Handler) HandleGetDirectoryLift(ctx *lift.Context) error {
	// Parse query parameters
	params := h.parseDirectoryParams(ctx)

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "search service unavailable"})
	}

	// Get directory using service
	result, err := searchService.GetDirectory(ctx.Context, &search.DirectoryQuery{
		Local:  params.local,
		Order:  params.order,
		Limit:  params.limit,
		Offset: params.offset,
	})
	if err != nil {
		h.logger.Error("failed to get directory", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get directory",
		})
	}

	// Convert account results to API format
	accounts := h.convertDirectoryResultsToAPI(ctx.Context, result.Accounts)

	ctx.Status(http.StatusOK)
	return ctx.JSON(accounts)
}

// directoryParams holds parsed directory query parameters
type directoryParams struct {
	limit  int
	offset int
	local  bool
	order  string
}

// parseDirectoryParams parses query parameters for directory endpoint
func (h *Handler) parseDirectoryParams(ctx *lift.Context) directoryParams {
	params := directoryParams{
		limit:  40,
		offset: 0,
		local:  false,
		order:  ctx.Query("order"),
	}

	// Parse limit
	params.limit = h.parseDirectoryLimit(ctx)

	// Parse offset
	params.offset = h.parseDirectoryOffset(ctx)

	// Parse local filter
	params.local = h.parseDirectoryLocal(ctx)

	return params
}

// parseDirectoryLimit parses the limit parameter
func (h *Handler) parseDirectoryLimit(ctx *lift.Context) int {
	limitStr := h.extractQueryParam(ctx, "limit")
	limit, err := common.ParseSearchLimit(limitStr)
	if err != nil {
		return 40
	}
	return limit
}

// parseDirectoryOffset parses the offset parameter
func (h *Handler) parseDirectoryOffset(ctx *lift.Context) int {
	offsetStr := h.extractQueryParam(ctx, "offset")
	offset, err := common.ParseSearchOffset(offsetStr)
	if err != nil {
		return 0
	}
	return offset
}

// parseDirectoryLocal parses the local filter parameter
func (h *Handler) parseDirectoryLocal(ctx *lift.Context) bool {
	localStr := h.extractQueryParam(ctx, "local")
	return localStr == boolTrue
}

// extractQueryParam extracts a query parameter with test mode fallback
func (h *Handler) extractQueryParam(ctx *lift.Context, param string) string {
	value := ctx.Query(param)

	// Test mode fallback - extract from path query string
	if err := common.ValidateRequiredParam("value", value); err != nil && ctx.Request != nil && strings.Contains(ctx.Request.Path, "?") {
		value = h.extractFromPathQuery(ctx.Request.Path, param)
	}

	return value
}

// extractFromPathQuery extracts a parameter from path query string
func (h *Handler) extractFromPathQuery(path, param string) string {
	parts := strings.Split(path, "?")
	if len(parts) <= 1 {
		return ""
	}

	params := strings.Split(parts[1], "&")
	for _, p := range params {
		kv := strings.Split(p, "=")
		if len(kv) == 2 && kv[0] == param {
			return kv[1]
		}
	}

	return ""
}

// convertDirectoryResultsToAPI converts search service results to API format
func (h *Handler) convertDirectoryResultsToAPI(ctx context.Context, results []search.AccountResult) []map[string]any {
	accounts := make([]map[string]any, 0, len(results))

	for _, result := range results {
		account := h.buildDirectoryAccountFromResult(ctx, result)
		accounts = append(accounts, account)
	}

	return accounts
}

// buildDirectoryAccountFromResult builds a single directory account entry from service result
func (h *Handler) buildDirectoryAccountFromResult(_ context.Context, result search.AccountResult) map[string]any {
	actor := result.Actor
	return map[string]any{
		"id":              actor.ID,
		"username":        actor.PreferredUsername,
		"acct":            h.getAccountAcctLift(actor),
		"display_name":    actor.Name,
		"locked":          actor.ManuallyApprovesFollowers,
		"bot":             actor.Type == actorTypeService,
		"discoverable":    true, // Only showing discoverable accounts
		"created_at":      h.formatActorCreatedAt(actor),
		"note":            actor.Summary,
		"url":             actor.URL,
		"avatar":          h.getActorAvatarURL(actor),
		"avatar_static":   h.getActorAvatarURL(actor),
		"header":          h.getHeaderURLLift(actor),
		"header_static":   "",
		"followers_count": result.FollowersCount,
		"following_count": result.FollowingCount,
		"statuses_count":  result.StatusesCount,
		"last_status_at":  result.LastStatusAt,
	}
}

// formatActorCreatedAt formats the actor creation date
func (h *Handler) formatActorCreatedAt(actor *activitypub.Actor) string {
	if actor.Published != nil {
		return actor.Published.Format("2006-01-02T15:04:05.000Z")
	}
	return ""
}

// getActorAvatarURL gets the avatar URL for an actor
func (h *Handler) getActorAvatarURL(actor *activitypub.Actor) string {
	if actor.Icon != nil {
		return actor.Icon.URL
	}
	return ""
}

// sortDirectoryAccounts sorts accounts based on the order parameter

// sortByActivity sorts accounts by last activity

// sortByCreation sorts accounts by creation date

// HandleGetSuggestionsV1Lift handles GET /api/v1/suggestions
// Returns follow suggestions (v1 format)
func (h *Handler) HandleGetSuggestionsV1Lift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header(common.XTestUsernameHeader)
	if err := common.ValidateRequiredParam("test_username", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers[common.XTestUsernameHeader]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeRead},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if err := common.ValidateRequiredParam("token", token); err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}
	}

	// Get limit from query params
	limitStr := ctx.Query("limit")
	limit, err := common.ParseSearchLimit(limitStr)
	if err != nil {
		limit = 40
	}

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "search service unavailable"})
	}

	// Get suggestions using service
	result, err := searchService.GetSuggestions(ctx.Context, &search.SuggestionsQuery{
		Username: claims.Username,
		Limit:    limit,
		Version:  1,
	})
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get suggestions",
		})
	}

	// Convert service suggestions to API format (V1)
	suggestionsList := make([]map[string]any, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		actor := item.Account.Actor
		// V1 format wraps account in suggestion object
		suggestionItem := map[string]any{
			"account": map[string]any{
				"id":              actor.ID,
				"username":        actor.PreferredUsername,
				"acct":            h.getAccountAcctLift(actor),
				"display_name":    actor.Name,
				"locked":          actor.ManuallyApprovesFollowers,
				"bot":             actor.Type == actorTypeService,
				"discoverable":    actor.Discoverable,
				"created_at":      h.formatActorCreatedAt(actor),
				"note":            actor.Summary,
				"url":             actor.URL,
				"avatar":          h.getActorAvatarURL(actor),
				"avatar_static":   h.getActorAvatarURL(actor),
				"header":          h.getHeaderURLLift(actor),
				"header_static":   h.getHeaderURLLift(actor),
				"followers_count": item.Account.FollowersCount,
				"following_count": item.Account.FollowingCount,
				"statuses_count":  item.Account.StatusesCount,
			},
		}
		suggestionsList = append(suggestionsList, suggestionItem)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(suggestionsList)
}

// HandleGetSuggestionsV2Lift handles GET /api/v2/suggestions
// Returns follow suggestions (v2 format with sources)
func (h *Handler) HandleGetSuggestionsV2Lift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header(common.XTestUsernameHeader)
	if err := common.ValidateRequiredParam("test_username", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers[common.XTestUsernameHeader]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeRead},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if err := common.ValidateRequiredParam("token", token); err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}
	}

	// Get limit from query params
	limitStr := ctx.Query("limit")
	limit, err := common.ParseSearchLimit(limitStr)
	if err != nil {
		limit = 40
	}

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "search service unavailable"})
	}

	// Get suggestions using service (V2 includes sources)
	result, err := searchService.GetSuggestions(ctx.Context, &search.SuggestionsQuery{
		Username: claims.Username,
		Limit:    limit,
		Version:  2,
	})
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get suggestions",
		})
	}

	// Convert service suggestions to API format (V2)
	suggestions := make([]map[string]any, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		actor := item.Account.Actor
		// V2 format includes sources explaining why this was suggested
		suggestion := map[string]any{
			"source": item.Source, // Can be: staff, past_interactions, global
			"account": map[string]any{
				"id":              actor.ID,
				"username":        actor.PreferredUsername,
				"acct":            h.getAccountAcctLift(actor),
				"display_name":    actor.Name,
				"locked":          actor.ManuallyApprovesFollowers,
				"bot":             actor.Type == actorTypeService,
				"discoverable":    actor.Discoverable,
				"created_at":      h.formatActorCreatedAt(actor),
				"note":            actor.Summary,
				"url":             actor.URL,
				"avatar":          h.getActorAvatarURL(actor),
				"avatar_static":   h.getActorAvatarURL(actor),
				"header":          h.getHeaderURLLift(actor),
				"header_static":   h.getHeaderURLLift(actor),
				"followers_count": item.Account.FollowersCount,
				"following_count": item.Account.FollowingCount,
				"statuses_count":  item.Account.StatusesCount,
			},
		}
		suggestions = append(suggestions, suggestion)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(suggestions)
}

// HandleRemoveSuggestionLift handles DELETE /api/v1/suggestions/:account_id
// Removes an account from suggestions
func (h *Handler) HandleRemoveSuggestionLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header(common.XTestUsernameHeader)
	if err := common.ValidateRequiredParam("test_username", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers[common.XTestUsernameHeader]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if err := common.ValidateRequiredParam("token", token); err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}
	}

	// Get account ID from path
	accountID := ctx.Param("account_id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract account_id from path like /api/v1/suggestions/account123
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 4 && parts[3] == "suggestions" {
			accountID = parts[4]
		}
	}

	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "account ID required",
		})
	}

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "search service unavailable"})
	}

	// Remove suggestion using service
	if err := searchService.RemoveSuggestion(ctx.Context, &search.RemoveSuggestionCommand{
		Username:  claims.Username,
		AccountID: accountID,
	}); err != nil {
		h.logger.Error("failed to remove suggestion", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to remove suggestion",
		})
	}

	// For now, just return success
	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{})
}

// Helper function to check if an actor ID is local
func (h *Handler) isLocalLift(actorID string) bool {
	// Check if the actor ID contains our domain
	// More precise check: ensure it starts with https:// or http:// followed by our domain
	return strings.HasPrefix(actorID, "https://"+h.cfg.Domain+"/") ||
		strings.HasPrefix(actorID, "http://"+h.cfg.Domain+"/")
}

// getAccountAcctLift returns the account acct (username@domain for remote)
func (h *Handler) getAccountAcctLift(actor *activitypub.Actor) string {
	if h.isLocalLift(actor.ID) {
		return actor.PreferredUsername
	}
	// For remote actors, extract domain from actor ID
	// Actor ID format: https://domain.com/users/username
	if strings.Contains(actor.ID, "://") {
		parts := strings.Split(actor.ID, "://")
		if len(parts) > 1 {
			domainParts := strings.Split(parts[1], "/")
			if len(domainParts) > 0 {
				return fmt.Sprintf("%s@%s", actor.PreferredUsername, domainParts[0])
			}
		}
	}
	return actor.PreferredUsername
}

// Note: Count methods have been removed as they are now handled by the Search service
