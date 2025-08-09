package lift

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetStatusFavouritedByLift handles GET /api/v1/statuses/:id/favourited_by
func (h *Handler) HandleGetStatusFavouritedByLift(ctx *lift.Context) error {
	// Extract status ID from URL parameter
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "status ID is required"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	_, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}

	// Parse query parameters for pagination
	limit := 20
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	cursor := ctx.Query("max_id")
	if cursor == "" && ctx.Request != nil && ctx.Request.Request != nil {
		cursor = ctx.Request.Request.QueryParams["max_id"]
	}

	// Get likes for the object
	likes, nextCursor, err := h.repos.Like().GetObjectLikes(ctx.Context, objectID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get object likes",
			zap.String("object_id", objectID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get likes"})
	}

	// Initialize converter
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	// Convert actors to accounts
	accounts := make([]models.Account, 0, len(likes))
	for _, like := range likes {
		// Extract username from actor ID
		username := converter.ExtractUsernameFromActorID(like.Actor)
		if username == "" {
			continue
		}

		// Get the actor
		actor, err := h.repos.Actor().GetActor(ctx.Context, username)
		if err != nil {
			h.logger.Warn("failed to get actor for like",
				zap.String("actor_id", like.Actor),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := converter.ActorToAccount(actor)
		accounts = append(accounts, account)
	}

	// Set Link header for pagination if there's a next cursor
	if nextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/statuses/%s/favourited_by?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), statusID, nextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(accounts)
}

// HandleGetStatusRebloggedByLift handles GET /api/v1/statuses/:id/reblogged_by
func (h *Handler) HandleGetStatusRebloggedByLift(ctx *lift.Context) error {
	// Extract status ID from URL parameter
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "status ID is required"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	_, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}

	// Parse query parameters for pagination
	limit := 20
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	cursor := ctx.Query("max_id")
	if cursor == "" && ctx.Request != nil && ctx.Request.Request != nil {
		cursor = ctx.Request.Request.QueryParams["max_id"]
	}

	// Get announces for the object
	announces, nextCursor, err := h.repos.Social().GetStatusAnnounces(ctx.Context, objectID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get object announces",
			zap.String("object_id", objectID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get reblogs"})
	}

	// Initialize converter
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	// Convert actors to accounts
	accounts := make([]models.Account, 0, len(announces))
	for _, announce := range announces {
		// Extract username from actor ID
		username := converter.ExtractUsernameFromActorID(announce.Actor)
		if username == "" {
			continue
		}

		// Get the actor
		actor, err := h.repos.Actor().GetActor(ctx.Context, username)
		if err != nil {
			h.logger.Warn("failed to get actor for announce",
				zap.String("actor_id", announce.Actor),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := converter.ActorToAccount(actor)
		accounts = append(accounts, account)
	}

	// Set Link header for pagination if there's a next cursor
	if nextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/statuses/%s/reblogged_by?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), statusID, nextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(accounts)
}
