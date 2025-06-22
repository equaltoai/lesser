package handlers

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetDirectory handles GET /api/v1/directory
// Returns profile directory of discoverable accounts
func (h *Handler) HandleGetDirectory(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get query parameters
	limit := 40
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := request.QueryStringParameters["offset"]; offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Filter options
	local := request.QueryStringParameters["local"] == "true"
	order := request.QueryStringParameters["order"] // active, new

	// Get discoverable accounts using SearchAccounts with empty query
	actors, err := h.store.SearchAccounts(ctx, "", limit*2, false, offset)
	if err != nil {
		h.logger.Error("failed to get directory", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get directory: %w", err)), nil
	}

	// Convert to account format first to allow sorting
	accounts := make([]map[string]interface{}, 0, len(actors))
	for _, actor := range actors {
		// Filter local only if requested
		if local && !h.isLocal(actor.ID) {
			continue
		}

		account := map[string]interface{}{
			"id":              actor.ID,
			"username":        actor.PreferredUsername,
			"acct":            h.getAccountAcct(actor),
			"display_name":    actor.Name,
			"locked":          actor.ManuallyApprovesFollowers,
			"bot":             actor.Type == "Service",
			"discoverable":    true, // Only showing discoverable accounts
			"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
			"note":            actor.Summary,
			"url":             actor.URL,
			"avatar":          actor.Icon.URL,
			"avatar_static":   actor.Icon.URL,
			"header":          h.getHeaderURL(actor),
			"header_static":   "",
			"followers_count": h.getFollowerCount(ctx, actor.ID),
			"following_count": h.getFollowingCount(ctx, actor.ID),
			"statuses_count":  h.getStatusCount(ctx, actor.ID),
			"last_status_at":  h.formatLastStatusTime(actor.LastStatusAt),
		}
		accounts = append(accounts, account)
	}

	// Apply ordering based on parameter
	if order == "active" {
		// Sort by recent activity - this would need last_activity_at field
		// For now, sort by last_status_at
		sort.Slice(accounts, func(i, j int) bool {
			lastStatusI, _ := accounts[i]["last_status_at"].(string)
			lastStatusJ, _ := accounts[j]["last_status_at"].(string)
			return lastStatusI > lastStatusJ
		})
	} else if order == "new" {
		// Sort by account creation date
		sort.Slice(accounts, func(i, j int) bool {
			createdI, _ := accounts[i]["created_at"].(string)
			createdJ, _ := accounts[j]["created_at"].(string)
			return createdI > createdJ
		})
	}

	return common.OK(accounts), nil
}

// HandleGetSuggestionsV1 handles GET /api/v1/suggestions
// Returns follow suggestions (v1 format)
func (h *Handler) HandleGetSuggestionsV1(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get limit from query params
	limit := 40
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	// Get suggestions using the implemented algorithm
	suggestions, err := h.store.GetAccountSuggestions(ctx, claims.Username, limit)
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get suggestions: %w", err)), nil
	}

	// Convert storage suggestions to API format
	suggestionsList := make([]map[string]interface{}, 0, len(suggestions))
	for _, actor := range suggestions {
		// V1 format wraps account in suggestion object
		suggestionItem := map[string]interface{}{
			"account": map[string]interface{}{
				"id":              actor.ID,
				"username":        actor.PreferredUsername,
				"acct":            h.getAccountAcct(actor),
				"display_name":    actor.Name,
				"locked":          actor.ManuallyApprovesFollowers,
				"bot":             actor.Type == "Service",
				"discoverable":    actor.Discoverable,
				"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
				"note":            actor.Summary,
				"url":             actor.URL,
				"avatar":          actor.Icon.URL,
				"avatar_static":   actor.Icon.URL,
				"header":          h.getHeaderURL(actor),
				"header_static":   h.getHeaderURL(actor),
				"followers_count": h.getFollowerCount(ctx, actor.ID),
				"following_count": h.getFollowingCount(ctx, actor.ID),
				"statuses_count":  h.getStatusCount(ctx, actor.ID),
			},
		}
		suggestionsList = append(suggestionsList, suggestionItem)
	}

	return common.OK(suggestionsList), nil
}

// HandleGetSuggestionsV2 handles GET /api/v2/suggestions
// Returns follow suggestions (v2 format with sources)
func (h *Handler) HandleGetSuggestionsV2(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get limit from query params
	limit := 40
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	// Get suggestions with sources based on mutual follows, interests, etc.
	actors, err := h.store.SearchAccounts(ctx, "", limit*2, false, 0)
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get suggestions: %w", err)), nil
	}

	// Filter and format suggestions
	suggestions := make([]map[string]interface{}, 0, limit)
	for _, actor := range actors {
		if len(suggestions) >= limit {
			break
		}

		// Check if user follows this account
		isFollowing, _ := h.store.IsFollowing(ctx, claims.Username, actor.PreferredUsername)
		if isFollowing || actor.PreferredUsername == claims.Username {
			continue
		}

		// V2 format includes sources explaining why this was suggested
		suggestion := map[string]interface{}{
			"source": "global", // Can be: staff, past_interactions, global
			"account": map[string]interface{}{
				"id":              actor.ID,
				"username":        actor.PreferredUsername,
				"acct":            h.getAccountAcct(actor),
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
				"followers_count": h.getFollowerCount(ctx, actor.ID),
				"following_count": h.getFollowingCount(ctx, actor.ID),
				"statuses_count":  h.getStatusCount(ctx, actor.ID),
			},
		}
		suggestions = append(suggestions, suggestion)
	}

	return common.OK(suggestions), nil
}

// HandleRemoveSuggestion handles DELETE /api/v1/suggestions/:account_id
// Removes an account from suggestions
func (h *Handler) HandleRemoveSuggestion(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	if accountID == "" {
		return common.BadRequest(fmt.Errorf("account ID required")), nil
	}

	// Remove suggestion from user's suggestion list
	if err := h.store.RemoveAccountSuggestion(ctx, claims.Username, accountID); err != nil {
		h.logger.Error("failed to remove suggestion", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// For now, just return success
	return common.OK(map[string]interface{}{}), nil
}

// Helper function to check if an actor ID is local
func (h *Handler) isLocal(actorID string) bool {
	// Check if the actor ID contains our domain
	return strings.Contains(actorID, h.cfg.Domain)
}

// getAccountAcct returns the account acct (username@domain for remote)
func (h *Handler) getAccountAcct(actor *activitypub.Actor) string {
	if h.isLocal(actor.ID) {
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

// getFollowerCount gets follower count with error handling
func (h *Handler) getFollowerCount(ctx context.Context, actorID string) int {
	count, _ := h.store.GetFollowersCount(ctx, actorID)
	return count
}

// getFollowingCount gets following count with error handling
func (h *Handler) getFollowingCount(ctx context.Context, actorID string) int {
	count, _ := h.store.GetFollowingCount(ctx, actorID)
	return count
}

// getStatusCount gets status count with error handling
func (h *Handler) getStatusCount(ctx context.Context, actorID string) int {
	count, _ := h.store.GetStatusCount(ctx, actorID)
	return count
}
