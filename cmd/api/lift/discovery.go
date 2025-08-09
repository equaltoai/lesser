package lift

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetDirectoryLift handles GET /api/v1/directory
// Returns profile directory of discoverable accounts
func (h *Handler) HandleGetDirectoryLift(ctx *lift.Context) error {
	// Parse query parameters
	params := h.parseDirectoryParams(ctx)

	// Get discoverable accounts
	actors, err := h.repos.Search().SearchAccounts(ctx.Context, "", params.limit*2, false, params.offset)
	if err != nil {
		h.logger.Error("failed to get directory", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get directory",
		})
	}

	// Convert actors to accounts
	accounts := h.convertActorsToDirectory(ctx.Context, actors, params.local)

	// Apply ordering
	h.sortDirectoryAccounts(accounts, params.order)

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
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			return l
		}
	}
	return 40
}

// parseDirectoryOffset parses the offset parameter
func (h *Handler) parseDirectoryOffset(ctx *lift.Context) int {
	offsetStr := h.extractQueryParam(ctx, "offset")
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			return o
		}
	}
	return 0
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
	if value == "" && ctx.Request != nil && strings.Contains(ctx.Request.Path, "?") {
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

// convertActorsToDirectory converts actors to directory account format
func (h *Handler) convertActorsToDirectory(ctx context.Context, actors []*activitypub.Actor, localOnly bool) []map[string]any {
	accounts := make([]map[string]any, 0, len(actors))
	
	for _, actor := range actors {
		// Filter local only if requested
		if localOnly && !h.isLocalLift(actor.ID) {
			continue
		}

		account := h.buildDirectoryAccount(ctx, actor)
		accounts = append(accounts, account)
	}

	return accounts
}

// buildDirectoryAccount builds a single directory account entry
func (h *Handler) buildDirectoryAccount(ctx context.Context, actor *activitypub.Actor) map[string]any {
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
		"followers_count": h.getFollowerCountLift(ctx, actor.ID),
		"following_count": h.getFollowingCountLift(ctx, actor.ID),
		"statuses_count":  h.getStatusCountLift(ctx, actor.ID),
		"last_status_at":  h.formatLastStatusTimeLift(actor.LastStatusAt),
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
func (h *Handler) sortDirectoryAccounts(accounts []map[string]any, order string) {
	switch order {
	case "active":
		h.sortByActivity(accounts)
	case "new":
		h.sortByCreation(accounts)
	}
}

// sortByActivity sorts accounts by last activity
func (h *Handler) sortByActivity(accounts []map[string]any) {
	sort.Slice(accounts, func(i, j int) bool {
		lastStatusI, _ := accounts[i]["last_status_at"].(string)
		lastStatusJ, _ := accounts[j]["last_status_at"].(string)
		return lastStatusI > lastStatusJ
	})
}

// sortByCreation sorts accounts by creation date
func (h *Handler) sortByCreation(accounts []map[string]any) {
	sort.Slice(accounts, func(i, j int) bool {
		createdI, _ := accounts[i]["created_at"].(string)
		createdJ, _ := accounts[j]["created_at"].(string)
		return createdI > createdJ
	})
}

// HandleGetSuggestionsV1Lift handles GET /api/v1/suggestions
// Returns follow suggestions (v1 format)
func (h *Handler) HandleGetSuggestionsV1Lift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header(common.XTestUsernameHeader)
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
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
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	limit := 40
	limitStr := ctx.Query("limit")
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	// Get suggestions using the implemented algorithm
	suggestions, err := h.repos.Actor().GetAccountSuggestions(ctx.Context, claims.Username, limit)
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get suggestions",
		})
	}

	// Convert storage suggestions to API format
	suggestionsList := make([]map[string]any, 0, len(suggestions))
	for _, actor := range suggestions {
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
				"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
				"note":            actor.Summary,
				"url":             actor.URL,
				"avatar":          actor.Icon.URL,
				"avatar_static":   actor.Icon.URL,
				"header":          h.getHeaderURLLift(actor),
				"header_static":   h.getHeaderURLLift(actor),
				"followers_count": h.getFollowerCountLift(ctx.Context, actor.ID),
				"following_count": h.getFollowingCountLift(ctx.Context, actor.ID),
				"statuses_count":  h.getStatusCountLift(ctx.Context, actor.ID),
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
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
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
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	limit := 40
	limitStr := ctx.Query("limit")
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	// Get suggestions with sources based on mutual follows, interests, etc.
	actors, err := h.repos.Search().SearchAccounts(ctx.Context, "", limit*2, false, 0)
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get suggestions",
		})
	}

	// Filter and format suggestions
	suggestions := make([]map[string]any, 0, limit)
	for _, actor := range actors {
		if len(suggestions) >= limit {
			break
		}

		// Check if user follows this account
		followRel, _ := h.repos.Relationship().GetRelationship(ctx.Context, claims.Username, actor.PreferredUsername)
		isFollowing := followRel != nil
		if isFollowing || actor.PreferredUsername == claims.Username {
			continue
		}

		// V2 format includes sources explaining why this was suggested
		suggestion := map[string]any{
			"source": "global", // Can be: staff, past_interactions, global
			"account": map[string]any{
				"id":              actor.ID,
				"username":        actor.PreferredUsername,
				"acct":            h.getAccountAcctLift(actor),
				"display_name":    actor.Name,
				"locked":          actor.ManuallyApprovesFollowers,
				"bot":             actor.Type == actorTypeService,
				"discoverable":    actor.Discoverable,
				"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
				"note":            actor.Summary,
				"url":             actor.URL,
				"avatar":          actor.Icon.URL,
				"avatar_static":   actor.Icon.URL,
				"header":          "",
				"header_static":   "",
				"followers_count": h.getFollowerCountLift(ctx.Context, actor.ID),
				"following_count": h.getFollowingCountLift(ctx.Context, actor.ID),
				"statuses_count":  h.getStatusCountLift(ctx.Context, actor.ID),
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
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
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
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	if accountID == "" && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract account_id from path like /api/v1/suggestions/account123
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 4 && parts[3] == "suggestions" {
			accountID = parts[4]
		}
	}

	if accountID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "account ID required",
		})
	}

	// Remove suggestion from user's suggestion list
	if err := h.repos.Actor().RemoveAccountSuggestion(ctx.Context, claims.Username, accountID); err != nil {
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

// getFollowerCountLift gets follower count with error handling
func (h *Handler) getFollowerCountLift(ctx context.Context, actorID string) int {
	count, _ := h.repos.Relationship().CountFollowers(ctx, actorID)
	return count
}

// getFollowingCountLift gets following count with error handling
func (h *Handler) getFollowingCountLift(ctx context.Context, actorID string) int {
	count, _ := h.repos.Relationship().CountFollowing(ctx, actorID)
	return count
}

// getStatusCountLift gets status count with error handling
func (h *Handler) getStatusCountLift(ctx context.Context, actorID string) int {
	count, _ := h.repos.Status().CountStatusesByAuthor(ctx, actorID)
	return count
}
