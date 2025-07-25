package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/aron23/lesser/cmd/api/models"
)

// HandleAccountSearch handles GET /api/v1/accounts/search
// Search for accounts by username, display name, or domain
func (h *Handler) HandleAccountSearch(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract query parameters
	query := request.QueryStringParameters["q"]
	if query == "" {
		return common.BadRequest(errors.New("q parameter is required")), nil
	}

	// Parse limit (default 40, max 80)
	limit := 40
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil {
			if limit > 80 {
				limit = 80
			} else if limit < 1 {
				limit = 1
			}
		}
	}

	// Parse offset for pagination
	offset := 0
	if offsetStr := request.QueryStringParameters["offset"]; offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}

	// Check if we should only return accounts the user is following
	followingOnly := request.QueryStringParameters["following"] == "true"

	// Check if we should resolve remote accounts
	resolve := request.QueryStringParameters["resolve"] == "true"

	// Authentication is optional for search
	var authenticatedUser string
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if token, err := auth.ExtractBearerToken(authHeader); err == nil {
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			authenticatedUser = claims.Username

			// Check read scope if following filter is requested
			if followingOnly && !claims.HasScope(auth.ScopeRead) {
				return common.Forbidden(errors.New("insufficient scope for following filter")), nil
			}
		}
	}

	// If following filter is requested but no auth, return error
	if followingOnly && authenticatedUser == "" {
		return common.Unauthorized(errors.New("authentication required for following filter")), nil
	}

	// Perform the search
	actors, err := h.store.SearchAccounts(ctx, query, limit, followingOnly, offset)
	if err != nil {
		h.logger.Error("account search failed",
			zap.String("query", query),
			zap.Int("limit", limit),
			zap.Int("offset", offset),
			zap.Bool("following", followingOnly),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("search failed")), nil
	}

	// If resolve is true, try WebFinger lookup for federated handles
	if resolve && isValidHandle(query) {
		// Create remote search service
		remoteSearchSvc := federation.NewRemoteSearchService(h.store)

		// Search for remote actors
		remoteResults, err := remoteSearchSvc.SearchRemoteActors(ctx, query, limit)
		if err != nil {
			h.logger.Debug("remote search failed",
				zap.String("query", query),
				zap.Error(err))
		} else if len(remoteResults) > 0 {
			// Add remote actors to results
			for _, result := range remoteResults {
				if result.Actor != nil {
					actors = append(actors, result.Actor)
				}
			}
		}
	}

	// Convert actors to Mastodon account format
	accounts := make([]models.Account, 0, len(actors))
	for _, actor := range actors {
		account := h.converter.ActorToAccount(actor)
		accounts = append(accounts, account)
	}

	// Add search metadata to response headers
	headers := map[string]string{
		"Content-Type":  "application/json",
		"X-Total-Count": fmt.Sprintf("%d", len(accounts)),
	}

	// Log search analytics
	h.logger.Info("account search completed",
		zap.String("query", query),
		zap.Int("results", len(accounts)),
		zap.Int("limit", limit),
		zap.Int("offset", offset),
		zap.Bool("authenticated", authenticatedUser != ""))

	// Marshal the response
	body, _ := json.Marshal(accounts)

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleGetSearchSuggestions handles GET /api/v1/accounts/search/suggestions
// Returns search suggestions for autocomplete
func (h *Handler) HandleGetSearchSuggestions(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract query prefix
	prefix := request.QueryStringParameters["q"]
	if len(prefix) < 2 {
		// Return empty array for short prefixes
		return common.OK([]any{}), nil
	}

	// Get suggestions from storage
	suggestions, err := h.store.GetSearchSuggestions(ctx, prefix)
	if err != nil {
		h.logger.Error("failed to get search suggestions",
			zap.String("prefix", prefix),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("suggestions lookup failed")), nil
	}

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

	return common.OK(response), nil
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
