package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetFavourites handles GET /api/v1/favourites
func (h *Handler) HandleGetFavourites(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get liked objects
	likes, nextCursor, err := h.store.GetActorLikes(ctx, actor.ID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get likes",
			zap.String("actor_id", actor.ID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get favorites")), nil
	}

	// Retrieve the actual objects
	statuses := make([]*models.Status, 0, len(likes))
	for _, like := range likes {
		obj, err := h.store.GetObject(ctx, like.Object)
		if err != nil {
			h.logger.Warn("failed to get liked object",
				zap.String("object_id", like.Object),
				zap.Error(err))
			continue
		}

		// Get the actor who created the object
		var attributedTo string
		var objActor *activitypub.Actor

		switch o := obj.(type) {
		case *activitypub.Note:
			attributedTo = o.AttributedTo
		case map[string]any:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			username := h.converter.ExtractUsernameFromActorID(attributedTo)
			if username != "" {
				objActor, _ = h.store.GetActor(ctx, username)
			}
		}

		// Convert to status with context
		likeCount, _ := h.store.CountObjectLikes(ctx, like.Object)
		announceCount, _ := h.store.CountObjectAnnounces(ctx, like.Object)

		// Check if reblogged
		reblogged := false
		if _, err := h.store.GetAnnounce(ctx, actor.ID, like.Object); err == nil {
			reblogged = true
		}

		// Check if bookmarked
		bookmarked, _ := h.store.IsBookmarked(ctx, claims.Username, like.Object)

		status := h.converter.ObjectToStatusWithContext(
			ctx,
			obj,
			objActor,
			likeCount,
			announceCount,
			true, // favorited (always true in favorites timeline)
			reblogged,
			bookmarked,
		)

		statuses = append(statuses, &status)
	}

	// Create response with Link header for pagination
	response := common.OK(statuses)
	if nextCursor != "" && len(statuses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/favourites?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}
