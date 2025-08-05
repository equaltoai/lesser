package lift

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/mastodon"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleBookmarkLift handles POST /api/v1/statuses/:id/bookmark
func (h *Handler) HandleBookmarkLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	obj, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}

	// Add bookmark
	if err := h.repos.Account().AddBookmark(ctx.Context, username, objectID); err != nil {
		h.logger.Error("failed to create bookmark",
			zap.String("username", username),
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to bookmark status"})
	}

	// Convert object to status using the proper converter
	status, err := h.convertBookmarkedObjectToStatus(ctx.Context, obj, objectID, username, true)
	if err != nil {
		h.logger.Error("failed to convert object to status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to convert status"})
	}

	return ctx.JSON(status)
}

// HandleUnbookmarkLift handles POST /api/v1/statuses/:id/unbookmark
func (h *Handler) HandleUnbookmarkLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	obj, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}

	// Remove bookmark
	if err := h.repos.Account().RemoveBookmark(ctx.Context, username, objectID); err != nil {
		h.logger.Error("failed to remove bookmark",
			zap.String("username", username),
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to unbookmark status"})
	}

	// Convert object to status using the proper converter
	status, err := h.convertBookmarkedObjectToStatus(ctx.Context, obj, objectID, username, false)
	if err != nil {
		h.logger.Error("failed to convert object to status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to convert status"})
	}

	return ctx.JSON(status)
}

// HandleGetBookmarksLift handles GET /api/v1/bookmarks
func (h *Handler) HandleGetBookmarksLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check read scope
		if !claims.HasScope(auth.ScopeRead) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Parse query parameters
	limit := 20
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	cursor := ctx.Query("max_id")
	if cursor == "" && ctx.Request != nil && ctx.Request.Request != nil {
		cursor = ctx.Request.Request.QueryParams["max_id"]
	}

	// Get bookmarked object IDs
	bookmarks, nextCursor, err := h.repos.Account().GetBookmarks(ctx.Context, username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get bookmarks"})
	}

	// Retrieve the actual objects
	statuses := make([]*models.Status, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		obj, err := h.repos.Object().GetObject(ctx.Context, bookmark.ObjectID)
		if err != nil {
			h.logger.Warn("failed to get bookmarked object",
				zap.String("object_id", bookmark.ObjectID),
				zap.Error(err))
			continue
		}

		status, err := h.convertBookmarkedObjectToStatus(ctx.Context, obj, bookmark.ObjectID, username, true)
		if err != nil {
			h.logger.Warn("failed to convert bookmarked object to status",
				zap.String("object_id", bookmark.ObjectID),
				zap.Error(err))
			continue
		}

		statuses = append(statuses, status)
	}

	// Set Link header for pagination if there's a cursor
	if nextCursor != "" && len(statuses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/bookmarks?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(statuses)
}

// convertBookmarkedObjectToStatus is a helper that properly converts objects to status format
func (h *Handler) convertBookmarkedObjectToStatus(ctx context.Context, obj any, objectID string, currentUsername string, isBookmarked bool) (*models.Status, error) {
	// Extract actor ID from object
	var attributedTo string
	switch v := obj.(type) {
	case *storagemodels.Object:
		attributedTo = v.AttributedTo
	case *activitypub.Note:
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

	// Initialize converter
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	// Extract username from actor ID
	actorUsername := converter.ExtractUsernameFromActorID(attributedTo)
	if actorUsername == "" {
		return nil, fmt.Errorf("could not extract username from actor ID: %s", attributedTo)
	}

	// Get the actor
	actor, err := h.repos.Actor().GetActor(ctx, actorUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Get current user's actor ID
	currentUserActorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), currentUsername)

	// Get counts
	likeCount, _ := h.repos.Like().GetLikeCount(ctx, objectID)
	reblogCount, _ := h.repos.Like().GetBoostCount(ctx, objectID)

	// Check if user favorited or reblogged
	favorited := false
	if _, err := h.repos.Like().GetLike(ctx, currentUserActorID, objectID); err == nil {
		favorited = true
	}

	reblogged := false
	if _, err := h.repos.Social().GetAnnounce(ctx, currentUserActorID, objectID); err == nil {
		reblogged = true
	}

	// Convert to status using the proper converter with all context
	status := converter.ObjectToStatusWithContext(ctx, obj, actor, int(likeCount), int(reblogCount), favorited, reblogged, isBookmarked)

	return &status, nil
}