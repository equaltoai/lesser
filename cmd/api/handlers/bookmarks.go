package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleBookmark handles POST /api/v1/statuses/:id/bookmark
func (h *Handler) HandleBookmark(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Check if the status exists
	obj, err := h.store.GetObject(ctx, statusID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found: %s", statusID)), nil
	}

	// Add bookmark
	if err := h.store.CreateBookmark(ctx, claims.Username, statusID); err != nil {
		h.logger.Error("failed to create bookmark",
			zap.String("username", claims.Username),
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to bookmark status")), nil
	}

	// Convert object to status and set bookmarked flag
	status, err := h.convertObjectToStatus(ctx, obj, claims.Username)
	if err != nil {
		h.logger.Error("failed to convert object to status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to convert status")), nil
	}

	status.Bookmarked = true

	return common.OK(status), nil
}

// HandleUnbookmark handles POST /api/v1/statuses/:id/unbookmark
func (h *Handler) HandleUnbookmark(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Check if the status exists
	obj, err := h.store.GetObject(ctx, statusID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found: %s", statusID)), nil
	}

	// Remove bookmark
	if err := h.store.RemoveBookmark(ctx, claims.Username, statusID); err != nil {
		h.logger.Error("failed to remove bookmark",
			zap.String("username", claims.Username),
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to unbookmark status")), nil
	}

	// Convert object to status and ensure bookmarked is false
	status, err := h.convertObjectToStatus(ctx, obj, claims.Username)
	if err != nil {
		h.logger.Error("failed to convert object to status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to convert status")), nil
	}

	status.Bookmarked = false

	return common.OK(status), nil
}

// HandleGetBookmarks handles GET /api/v1/bookmarks
func (h *Handler) HandleGetBookmarks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get bookmarked object IDs
	objectIDs, nextCursor, err := h.store.GetBookmarks(ctx, claims.Username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get bookmarks",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get bookmarks")), nil
	}

	// Retrieve the actual objects
	statuses := make([]*models.Status, 0, len(objectIDs))
	for _, objectID := range objectIDs {
		obj, err := h.store.GetObject(ctx, objectID)
		if err != nil {
			h.logger.Warn("failed to get bookmarked object",
				zap.String("object_id", objectID),
				zap.Error(err))
			continue
		}

		status, err := h.convertObjectToStatus(ctx, obj, claims.Username)
		if err != nil {
			h.logger.Warn("failed to convert bookmarked object to status",
				zap.String("object_id", objectID),
				zap.Error(err))
			continue
		}

		// Mark as bookmarked
		status.Bookmarked = true
		statuses = append(statuses, status)
	}

	// Create response with Link header for pagination
	response := common.OK(statuses)
	if nextCursor != "" && len(statuses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/bookmarks?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}

// convertObjectToStatus is a helper method that should be shared across handlers
// For now, we'll use a simplified version
func (h *Handler) convertObjectToStatus(ctx context.Context, obj interface{}, currentUsername string) (*models.Status, error) {
	// This is a simplified conversion - in a real implementation,
	// this would be more complex and handle different object types

	// Type assert to map for now
	objMap, ok := obj.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid object type")
	}

	// Extract basic fields
	id, _ := objMap["id"].(string)
	content, _ := objMap["content"].(string)
	published, _ := objMap["published"].(string)
	attributedTo, _ := objMap["attributedTo"].(string)

	// Get the actor
	actorUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
	actor, err := h.store.GetActor(ctx, actorUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Create account from actor
	account := h.converter.ActorToAccount(actor)

	// Check if bookmarked
	bookmarked, _ := h.store.IsBookmarked(ctx, currentUsername, id)

	// Check if favourited
	// TODO: Implement favourite checking

	// Build status
	status := &models.Status{
		ID:               id,
		CreatedAt:        published,
		Content:          content,
		Visibility:       "public", // TODO: Parse from object
		Language:         "en",     // TODO: Detect language
		URI:              id,
		URL:              fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), id),
		Account:          account,
		MediaAttachments: []interface{}{},
		Mentions:         []interface{}{},
		Tags:             []interface{}{},
		Emojis:           []interface{}{},
		Bookmarked:       bookmarked,
		// TODO: Set other fields
	}

	return status, nil
}
