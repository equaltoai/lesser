package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	// order := request.QueryStringParameters["order"] // active, new

	// TODO: Implement proper directory listing with discoverable flag
	// For now, use SearchAccounts as a placeholder
	actors, err := h.store.SearchAccounts(ctx, "", limit, false, offset)
	if err != nil {
		h.logger.Error("failed to get directory", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get directory: %w", err)), nil
	}

	// Convert to Mastodon API format
	response := make([]map[string]interface{}, 0, len(actors))
	for _, actor := range actors {
		// Filter local only if requested
		if local && !h.isLocal(actor.ID) {
			continue
		}

		// TODO: Check if account is discoverable
		// TODO: Apply ordering (active = recent activity, new = recently created)

		account := map[string]interface{}{
			"id":              actor.ID,
			"username":        actor.PreferredUsername,
			"acct":            actor.PreferredUsername, // TODO: Add domain for remote
			"display_name":    actor.Name,
			"locked":          false, // TODO: Get from actor.ManuallyApprovesFollowers
			"bot":             false, // TODO: Get from actor type
			"discoverable":    true,  // Only showing discoverable accounts
			"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
			"note":            actor.Summary,
			"url":             actor.URL,
			"avatar":          actor.Icon.URL,
			"avatar_static":   actor.Icon.URL,
			"header":          "", // TODO: Get header image
			"header_static":   "",
			"followers_count": 0,   // TODO: Get follower count
			"following_count": 0,   // TODO: Get following count
			"statuses_count":  0,   // TODO: Get status count
			"last_status_at":  nil, // TODO: Get last status time
		}
		response = append(response, account)
	}

	return common.OK(response), nil
}

// HandleGetSuggestionsV1 handles GET /api/v1/suggestions
// Returns follow suggestions (v1 format)
func (h *Handler) HandleGetSuggestionsV1(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user
	username, ok := ctx.Value("username").(string)
	if !ok || username == "" {
		return common.Unauthorized(fmt.Errorf("unauthorized")), nil
	}

	// Get limit from query params
	limit := 40
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	// TODO: Implement proper suggestion algorithm
	// Should consider:
	// - Users followed by people you follow
	// - Popular accounts you don't follow
	// - Accounts with similar interests (based on hashtags, interactions)
	// - Trust scores and reputation

	// For now, just get some accounts the user doesn't follow
	actors, err := h.store.SearchAccounts(ctx, "", limit*2, false, 0)
	if err != nil {
		h.logger.Error("failed to get suggestions", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get suggestions: %w", err)), nil
	}

	// Filter out accounts user already follows
	suggestions := make([]map[string]interface{}, 0, limit)
	for _, actor := range actors {
		if len(suggestions) >= limit {
			break
		}

		// Check if user follows this account
		isFollowing, _ := h.store.IsFollowing(ctx, username, actor.PreferredUsername)
		if isFollowing || actor.PreferredUsername == username {
			continue
		}

		// V1 format wraps account in suggestion object
		suggestion := map[string]interface{}{
			"account": map[string]interface{}{
				"id":              actor.ID,
				"username":        actor.PreferredUsername,
				"acct":            actor.PreferredUsername,
				"display_name":    actor.Name,
				"locked":          false,
				"bot":             false,
				"discoverable":    true,
				"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
				"note":            actor.Summary,
				"url":             actor.URL,
				"avatar":          actor.Icon.URL,
				"avatar_static":   actor.Icon.URL,
				"header":          "",
				"header_static":   "",
				"followers_count": 0,
				"following_count": 0,
				"statuses_count":  0,
			},
		}
		suggestions = append(suggestions, suggestion)
	}

	return common.OK(suggestions), nil
}

// HandleGetSuggestionsV2 handles GET /api/v2/suggestions
// Returns follow suggestions (v2 format with sources)
func (h *Handler) HandleGetSuggestionsV2(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user
	username, ok := ctx.Value("username").(string)
	if !ok || username == "" {
		return common.Unauthorized(fmt.Errorf("unauthorized")), nil
	}

	// Get limit from query params
	limit := 40
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	// TODO: Implement proper suggestion algorithm with sources
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
		isFollowing, _ := h.store.IsFollowing(ctx, username, actor.PreferredUsername)
		if isFollowing || actor.PreferredUsername == username {
			continue
		}

		// V2 format includes sources explaining why this was suggested
		suggestion := map[string]interface{}{
			"source": "global", // Can be: staff, past_interactions, global
			"account": map[string]interface{}{
				"id":              actor.ID,
				"username":        actor.PreferredUsername,
				"acct":            actor.PreferredUsername,
				"display_name":    actor.Name,
				"locked":          false,
				"bot":             false,
				"discoverable":    true,
				"created_at":      actor.Published.Format("2006-01-02T15:04:05.000Z"),
				"note":            actor.Summary,
				"url":             actor.URL,
				"avatar":          actor.Icon.URL,
				"avatar_static":   actor.Icon.URL,
				"header":          "",
				"header_static":   "",
				"followers_count": 0,
				"following_count": 0,
				"statuses_count":  0,
			},
		}
		suggestions = append(suggestions, suggestion)
	}

	return common.OK(suggestions), nil
}

// HandleRemoveSuggestion handles DELETE /api/v1/suggestions/:account_id
// Removes an account from suggestions
func (h *Handler) HandleRemoveSuggestion(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user
	username, ok := ctx.Value("username").(string)
	if !ok || username == "" {
		return common.Unauthorized(fmt.Errorf("unauthorized")), nil
	}

	if accountID == "" {
		return common.BadRequest(fmt.Errorf("account ID required")), nil
	}

	// TODO: Implement suggestion removal
	// This should store that the user dismissed this suggestion
	// so it won't appear again

	// For now, just return success
	return common.OK(map[string]interface{}{}), nil
}

// Helper function to check if an actor ID is local
func (h *Handler) isLocal(actorID string) bool {
	// Check if the actor ID contains our domain
	return strings.Contains(actorID, h.cfg.Domain)
}
