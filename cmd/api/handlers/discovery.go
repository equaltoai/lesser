package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/search"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleGetDirectoryLift handles GET /api/v1/directory
// Returns profile directory of discoverable accounts
func (h *Handler) HandleGetDirectoryLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse query parameters
	params := h.parseDirectoryParams(ctx)

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{"error": "search service unavailable"})
	}

	// Get directory using service
	result, err := searchService.GetDirectory(ctx.Context(), &search.DirectoryQuery{
		Local:  params.local,
		Order:  params.order,
		Limit:  params.limit,
		Offset: params.offset,
	})
	if err != nil {
		h.logger.Error("failed to get directory", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get directory",
		})
	}

	// Convert account results to API format
	accounts := h.convertDirectoryResultsToAPI(ctx.Context(), result.Accounts)

	return okJSON(accounts)
}

// directoryParams holds parsed directory query parameters
type directoryParams struct {
	limit  int
	offset int
	local  bool
	order  string
}

// parseDirectoryParams parses query parameters for directory endpoint
func (h *Handler) parseDirectoryParams(ctx *apptheory.Context) directoryParams {
	params := directoryParams{
		limit:  40,
		offset: 0,
		local:  false,
		order:  queryValue(ctx, "order"),
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
func (h *Handler) parseDirectoryLimit(ctx *apptheory.Context) int {
	limitStr := h.extractQueryParam(ctx, "limit")
	limit, err := common.ParseSearchLimit(limitStr)
	if err != nil {
		return 40
	}
	return limit
}

// parseDirectoryOffset parses the offset parameter
func (h *Handler) parseDirectoryOffset(ctx *apptheory.Context) int {
	offsetStr := h.extractQueryParam(ctx, "offset")
	offset, err := common.ParseSearchOffset(offsetStr)
	if err != nil {
		return 0
	}
	return offset
}

// parseDirectoryLocal parses the local filter parameter
func (h *Handler) parseDirectoryLocal(ctx *apptheory.Context) bool {
	localStr := h.extractQueryParam(ctx, "local")
	return localStr == boolTrue
}

// extractQueryParam extracts a query parameter with test mode fallback
func (h *Handler) extractQueryParam(ctx *apptheory.Context, param string) string {
	value := queryValue(ctx, param)

	// Test mode fallback - extract from path query string
	if err := common.ValidateRequiredParam("value", value); err != nil && strings.Contains(ctx.Request.Path, "?") {
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
func (h *Handler) convertDirectoryResultsToAPI(ctx context.Context, results []search.AccountResult) []apimodels.Account {
	accounts := make([]apimodels.Account, 0, len(results))

	for _, result := range results {
		account := h.buildDirectoryAccountFromResult(ctx, result)
		accounts = append(accounts, account)
	}

	return accounts
}

// buildDirectoryAccountFromResult builds a single directory account entry from service result
func (h *Handler) buildDirectoryAccountFromResult(_ context.Context, result search.AccountResult) apimodels.Account {
	actor := result.Actor
	return apimodels.Account{
		ID:             actor.ID,
		Username:       actor.PreferredUsername,
		Acct:           h.getAccountAcctLift(actor),
		DisplayName:    actor.Name,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == actorTypeService,
		Discoverable:   true, // Only showing discoverable accounts
		Group:          actor.Type == actorTypeGroup,
		CreatedAt:      h.formatActorCreatedAt(actor),
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         h.getActorAvatarURL(actor),
		AvatarStatic:   h.getActorAvatarURL(actor),
		Header:         h.getHeaderURLLift(actor),
		HeaderStatic:   h.getHeaderURLLift(actor),
		FollowersCount: result.FollowersCount,
		FollowingCount: result.FollowingCount,
		StatusesCount:  result.StatusesCount,
		LastStatusAt:   result.LastStatusAt,
		Emojis:         []any{},
		Fields:         []any{},
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
func (h *Handler) HandleGetSuggestionsV1Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract token from Authorization header
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return common.RespondMissingAuth(ctx)
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondInvalidToken(ctx)
	}

	// Get limit from query params
	limitStr := queryValue(ctx, "limit")
	limit, err := common.ParseSearchLimit(limitStr)
	if err != nil {
		limit = 40
	}

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{"error": "search service unavailable"})
	}

	// Get suggestions using service
	result, err := searchService.GetSuggestions(ctx.Context(), &search.SuggestionsQuery{
		Username: claims.Username,
		Limit:    limit,
		Version:  1,
	})
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get suggestions",
		})
	}

	// Convert service suggestions to API format (V1)
	suggestionsList := make([]apimodels.SuggestionV1, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		actor := item.Account.Actor
		account := apimodels.Account{
			ID:             actor.ID,
			Username:       actor.PreferredUsername,
			Acct:           h.getAccountAcctLift(actor),
			DisplayName:    actor.Name,
			Locked:         actor.ManuallyApprovesFollowers,
			Bot:            actor.Type == actorTypeService,
			Discoverable:   actor.Discoverable,
			Group:          actor.Type == actorTypeGroup,
			CreatedAt:      h.formatActorCreatedAt(actor),
			Note:           actor.Summary,
			URL:            actor.URL,
			Avatar:         h.getActorAvatarURL(actor),
			AvatarStatic:   h.getActorAvatarURL(actor),
			Header:         h.getHeaderURLLift(actor),
			HeaderStatic:   h.getHeaderURLLift(actor),
			FollowersCount: item.Account.FollowersCount,
			FollowingCount: item.Account.FollowingCount,
			StatusesCount:  item.Account.StatusesCount,
			Emojis:         []any{},
			Fields:         []any{},
		}
		suggestionsList = append(suggestionsList, apimodels.SuggestionV1{Account: account})
	}

	return okJSON(suggestionsList)
}

// HandleGetSuggestionsV2Lift handles GET /api/v2/suggestions
// Returns follow suggestions (v2 format with sources)
func (h *Handler) HandleGetSuggestionsV2Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract token from Authorization header
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return common.RespondMissingAuth(ctx)
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondInvalidToken(ctx)
	}

	// Get limit from query params
	limitStr := queryValue(ctx, "limit")
	limit, err := common.ParseSearchLimit(limitStr)
	if err != nil {
		limit = 40
	}

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{"error": "search service unavailable"})
	}

	// Get suggestions using service (V2 includes sources)
	result, err := searchService.GetSuggestions(ctx.Context(), &search.SuggestionsQuery{
		Username: claims.Username,
		Limit:    limit,
		Version:  2,
	})
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get suggestions",
		})
	}

	// Convert service suggestions to API format (V2)
	suggestions := make([]apimodels.SuggestionV2, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		actor := item.Account.Actor
		account := apimodels.Account{
			ID:             actor.ID,
			Username:       actor.PreferredUsername,
			Acct:           h.getAccountAcctLift(actor),
			DisplayName:    actor.Name,
			Locked:         actor.ManuallyApprovesFollowers,
			Bot:            actor.Type == actorTypeService,
			Discoverable:   actor.Discoverable,
			Group:          actor.Type == actorTypeGroup,
			CreatedAt:      h.formatActorCreatedAt(actor),
			Note:           actor.Summary,
			URL:            actor.URL,
			Avatar:         h.getActorAvatarURL(actor),
			AvatarStatic:   h.getActorAvatarURL(actor),
			Header:         h.getHeaderURLLift(actor),
			HeaderStatic:   h.getHeaderURLLift(actor),
			FollowersCount: item.Account.FollowersCount,
			FollowingCount: item.Account.FollowingCount,
			StatusesCount:  item.Account.StatusesCount,
			Emojis:         []any{},
			Fields:         []any{},
		}
		suggestions = append(suggestions, apimodels.SuggestionV2{
			Source:  item.Source,
			Account: account,
		})
	}

	return okJSON(suggestions)
}

// HandleRemoveSuggestionLift handles DELETE /api/v1/suggestions/:account_id
// Removes an account from suggestions
func (h *Handler) HandleRemoveSuggestionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract token from Authorization header
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return common.RespondMissingAuth(ctx)
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondInvalidToken(ctx)
	}

	// Get account ID from path
	accountID := ctx.Param("account_id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil && ctx.Request.Path != "" {
		// Extract account_id from path like /api/v1/suggestions/account123
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 4 && parts[3] == "suggestions" {
			accountID = parts[4]
		}
	}

	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error": "account ID required",
		})
	}

	// Get search service
	searchService := h.registry.Search()
	if searchService == nil {
		h.logger.Error("search service not available")
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{"error": "search service unavailable"})
	}

	// Remove suggestion using service
	if err := searchService.RemoveSuggestion(ctx.Context(), &search.RemoveSuggestionCommand{
		Username:  claims.Username,
		AccountID: accountID,
	}); err != nil {
		h.logger.Error("failed to remove suggestion", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to remove suggestion",
		})
	}

	// For now, just return success
	return okJSON(apimodels.EmptyObject{})
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
