package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetStatusFavouritedBy handles GET /api/v1/statuses/:id/favourited_by
func (h *Handler) HandleGetStatusFavouritedBy(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	_, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found: %s", statusID)), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get likes for the object
	likes, nextCursor, err := h.store.GetObjectLikes(ctx, objectID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get object likes",
			zap.String("object_id", objectID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get likes")), nil
	}

	// Convert actors to accounts
	accounts := make([]models.Account, 0, len(likes))
	for _, like := range likes {
		// Extract username from actor ID
		username := h.converter.ExtractUsernameFromActorID(like.Actor)
		if username == "" {
			continue
		}

		// Get the actor
		actor, err := h.store.GetActor(ctx, username)
		if err != nil {
			h.logger.Warn("failed to get actor for like",
				zap.String("actor_id", like.Actor),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := h.converter.ActorToAccount(actor)
		accounts = append(accounts, account)
	}

	// Create response with Link header for pagination
	response := common.OK(accounts)
	if nextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/statuses/%s/favourited_by?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), statusID, nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}

// HandleGetStatusRebloggedBy handles GET /api/v1/statuses/:id/reblogged_by
func (h *Handler) HandleGetStatusRebloggedBy(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the status exists
	_, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found: %s", statusID)), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get announces for the object
	announces, nextCursor, err := h.store.GetObjectAnnounces(ctx, objectID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get object announces",
			zap.String("object_id", objectID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get reblogs")), nil
	}

	// Convert actors to accounts
	accounts := make([]models.Account, 0, len(announces))
	for _, announce := range announces {
		// Extract username from actor ID
		username := h.converter.ExtractUsernameFromActorID(announce.Actor)
		if username == "" {
			continue
		}

		// Get the actor
		actor, err := h.store.GetActor(ctx, username)
		if err != nil {
			h.logger.Warn("failed to get actor for announce",
				zap.String("actor_id", announce.Actor),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := h.converter.ActorToAccount(actor)
		accounts = append(accounts, account)
	}

	// Create response with Link header for pagination
	response := common.OK(accounts)
	if nextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/statuses/%s/reblogged_by?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), statusID, nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}
