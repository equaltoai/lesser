package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
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

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	obj, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found: %s", statusID)), nil
	}

	// Add bookmark
	if err := h.store.CreateBookmark(ctx, claims.Username, objectID); err != nil {
		h.logger.Error("failed to create bookmark",
			zap.String("username", claims.Username),
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to bookmark status")), nil
	}

	// Convert object to status using the proper converter
	status, err := h.convertBookmarkedObjectToStatus(ctx, obj, objectID, claims.Username, true)
	if err != nil {
		h.logger.Error("failed to convert object to status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to convert status")), nil
	}

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

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	obj, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found: %s", statusID)), nil
	}

	// Remove bookmark
	if err := h.store.RemoveBookmark(ctx, claims.Username, objectID); err != nil {
		h.logger.Error("failed to remove bookmark",
			zap.String("username", claims.Username),
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to unbookmark status")), nil
	}

	// Convert object to status using the proper converter
	status, err := h.convertBookmarkedObjectToStatus(ctx, obj, objectID, claims.Username, false)
	if err != nil {
		h.logger.Error("failed to convert object to status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to convert status")), nil
	}

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

		status, err := h.convertBookmarkedObjectToStatus(ctx, obj, objectID, claims.Username, true)
		if err != nil {
			h.logger.Warn("failed to convert bookmarked object to status",
				zap.String("object_id", objectID),
				zap.Error(err))
			continue
		}

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

// convertBookmarkedObjectToStatus is a helper that properly converts objects to status format
func (h *Handler) convertBookmarkedObjectToStatus(ctx context.Context, obj any, objectID string, currentUsername string, isBookmarked bool) (*models.Status, error) {
	// Extract actor ID from object
	var attributedTo string
	switch v := obj.(type) {
	case *dynamodb.Object:
		attributedTo = v.AttributedTo
	case map[string]any:
		if attr, ok := v["attributedTo"].(string); ok {
			attributedTo = attr
		}
	default:
		// Try to get the attributed from object metadata
		if objRecord, ok := obj.(interface{ GetAttributedTo() string }); ok {
			attributedTo = objRecord.GetAttributedTo()
		}
	}

	if attributedTo == "" {
		return nil, fmt.Errorf("object has no attributedTo field")
	}

	// Extract username from actor ID
	actorUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
	if actorUsername == "" {
		return nil, fmt.Errorf("could not extract username from actor ID: %s", attributedTo)
	}

	// Get the actor
	actor, err := h.store.GetActor(ctx, actorUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Get current user's actor ID
	currentUserActorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), currentUsername)

	// Get counts
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
	reblogCount, _ := h.store.CountObjectAnnounces(ctx, objectID)

	// Check if user favorited or reblogged
	favorited := false
	if _, err := h.store.GetLike(ctx, currentUserActorID, objectID); err == nil {
		favorited = true
	}

	reblogged := false
	if _, err := h.store.GetAnnounce(ctx, currentUserActorID, objectID); err == nil {
		reblogged = true
	}

	// Convert to status using the proper converter with all context
	status := h.converter.ObjectToStatusWithContext(ctx, obj, actor, likeCount, reblogCount, favorited, reblogged, isBookmarked)

	return &status, nil
}

// Add import for dynamodb package at the top
// import "github.com/aron23/lesser/pkg/storage/dynamodb"
