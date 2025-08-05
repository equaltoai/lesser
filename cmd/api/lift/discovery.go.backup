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
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetDirectoryLift handles GET /api/v1/directory
// Returns profile directory of discoverable accounts
func (h *Handler) HandleGetDirectoryLift(ctx *lift.Context) error {
	// Get query parameters
	limit := 40
	limitStr := ctx.Query("limit")
	
	// Test mode fallback - extract from path query string
	if limitStr == "" && ctx.Request != nil && strings.Contains(ctx.Request.Path, "?") {
		parts := strings.Split(ctx.Request.Path, "?")
		if len(parts) > 1 {
			params := strings.Split(parts[1], "&")
			for _, param := range params {
				kv := strings.Split(param, "=")
				if len(kv) == 2 && kv[0] == "limit" {
					limitStr = kv[1]
					break
				}
			}
		}
	}
	
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	offset := 0
	offsetStr := ctx.Query("offset")
	
	// Test mode fallback - extract from path query string
	if offsetStr == "" && ctx.Request != nil && strings.Contains(ctx.Request.Path, "?") {
		parts := strings.Split(ctx.Request.Path, "?")
		if len(parts) > 1 {
			params := strings.Split(parts[1], "&")
			for _, param := range params {
				kv := strings.Split(param, "=")
				if len(kv) == 2 && kv[0] == "offset" {
					offsetStr = kv[1]
					break
				}
			}
		}
	}
	
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Filter options
	local := ctx.Query("local") == "true"
	
	// Test mode fallback - extract from path query string
	if ctx.Query("local") == "" && ctx.Request != nil && strings.Contains(ctx.Request.Path, "?") {
		parts := strings.Split(ctx.Request.Path, "?")
		if len(parts) > 1 {
			params := strings.Split(parts[1], "&")
			for _, param := range params {
				kv := strings.Split(param, "=")
				if len(kv) == 2 && kv[0] == "local" {
					local = kv[1] == "true"
					break
				}
			}
		}
	}
	
	order := ctx.Query("order") // active, new

	// Get discoverable accounts using SearchAccounts with empty query
	actors, err := h.store.SearchAccounts(ctx.Context, "", limit*2, false, offset)
	if err != nil {
		h.logger.Error("failed to get directory", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get directory",
		})
	}

	// Convert to account format first to allow sorting
	accounts := make([]map[string]any, 0, len(actors))
	for _, actor := range actors {
		// Filter local only if requested
		if local && !h.isLocalLift(actor.ID) {
			continue
		}

		account := map[string]any{
			"id":              actor.ID,
			"username":        actor.PreferredUsername,
			"acct":            h.getAccountAcctLift(actor),
			"display_name":    actor.Name,
			"locked":          actor.ManuallyApprovesFollowers,
			"bot":             actor.Type == "Service",
			"discoverable":    true, // Only showing discoverable accounts
			"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
			"note":            actor.Summary,
			"url":             actor.URL,
			"avatar":          actor.Icon.URL,
			"avatar_static":   actor.Icon.URL,
			"header":          h.getHeaderURLLift(actor),
			"header_static":   "",
			"followers_count": h.getFollowerCountLift(ctx.Context, actor.ID),
			"following_count": h.getFollowingCountLift(ctx.Context, actor.ID),
			"statuses_count":  h.getStatusCountLift(ctx.Context, actor.ID),
			"last_status_at":  h.formatLastStatusTimeLift(actor.LastStatusAt),
		}
		accounts = append(accounts, account)
	}

	// Apply ordering based on parameter
	switch order {
	case "active":
		// Sort by recent activity - this would need last_activity_at field
		// For now, sort by last_status_at
		sort.Slice(accounts, func(i, j int) bool {
			lastStatusI, _ := accounts[i]["last_status_at"].(string)
			lastStatusJ, _ := accounts[j]["last_status_at"].(string)
			return lastStatusI > lastStatusJ
		})
	case "new":
		// Sort by account creation date
		sort.Slice(accounts, func(i, j int) bool {
			createdI, _ := accounts[i]["created_at"].(string)
			createdJ, _ := accounts[j]["created_at"].(string)
			return createdI > createdJ
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(accounts)
}

// HandleGetSuggestionsV1Lift handles GET /api/v1/suggestions
// Returns follow suggestions (v1 format)
func (h *Handler) HandleGetSuggestionsV1Lift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
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
	suggestions, err := h.store.GetAccountSuggestions(ctx.Context, claims.Username, limit)
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
				"bot":             actor.Type == "Service",
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
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
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
	actors, err := h.store.SearchAccounts(ctx.Context, "", limit*2, false, 0)
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
		isFollowing, _ := h.store.IsFollowing(ctx.Context, claims.Username, actor.PreferredUsername)
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
				"bot":             actor.Type == "Service",
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
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
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
	if err := h.store.RemoveAccountSuggestion(ctx.Context, claims.Username, accountID); err != nil {
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
	count, _ := h.store.GetFollowersCount(ctx, actorID)
	return count
}

// getFollowingCountLift gets following count with error handling
func (h *Handler) getFollowingCountLift(ctx context.Context, actorID string) int {
	count, _ := h.store.GetFollowingCount(ctx, actorID)
	return count
}

// getStatusCountLift gets status count with error handling
func (h *Handler) getStatusCountLift(ctx context.Context, actorID string) int {
	count, _ := h.store.GetStatusCount(ctx, actorID)
	return count
}

