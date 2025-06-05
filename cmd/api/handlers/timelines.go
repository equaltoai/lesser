package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleHomeTimeline retrieves the home timeline for the authenticated user
func (h *Handler) HandleHomeTimeline(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := request.QueryStringParameters["max_id"]

	// Get timeline items
	// For now, just get public timeline items (TODO: implement proper home timeline with follows)
	entries, cursor, err := h.store.GetPublicTimeline(ctx, false, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get timeline", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert objects to statuses
	statuses := []models.Status{}
	for _, entry := range entries {
		// Get the actual object
		obj, err := h.store.GetObject(ctx, entry.PostID)
		if err != nil {
			h.logger.Warn("failed to get object from timeline", zap.String("id", entry.PostID), zap.Error(err))
			continue
		}

		// Get the actor who created the object
		var attributedTo string
		var objActor *activitypub.Actor

		switch o := obj.(type) {
		case *activitypub.Note:
			attributedTo = o.AttributedTo
		case map[string]interface{}:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			parts := strings.Split(attributedTo, "/")
			if len(parts) > 0 {
				username := parts[len(parts)-1]
				objActor, _ = h.store.GetActor(ctx, username)
			}
		}

		status := ObjectToStatus(obj, objActor)

		// Check if blocked
		if objActor != nil {
			if _, err := h.store.GetBlock(ctx, actor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		statuses = append(statuses, status)
	}

	// Set Link header for pagination if there's a cursor
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if cursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/timelines/home?max_id=%s", h.cfg.BaseURL(), cursor)
		if limit != 20 {
			nextURL += fmt.Sprintf("&limit=%d", limit)
		}
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	body, _ := json.Marshal(statuses)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandlePublicTimeline retrieves the public timeline
func (h *Handler) HandlePublicTimeline(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Public timeline doesn't require authentication, but check if user is authenticated
	var currentActor *activitypub.Actor
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if token, err := auth.ExtractBearerToken(authHeader); err == nil {
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			currentActor, _ = h.store.GetActor(ctx, claims.Username)
		}
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := request.QueryStringParameters["max_id"]
	local := request.QueryStringParameters["local"] == "true"
	remote := request.QueryStringParameters["remote"] == "true"
	onlyMedia := request.QueryStringParameters["only_media"] == "true"

	// Get timeline items
	entries, cursor, err := h.store.GetPublicTimeline(ctx, local, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get public timeline", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert objects to statuses
	statuses := []models.Status{}
	for _, entry := range entries {
		// Apply quick filters from timeline entry
		if onlyMedia && !entry.HasMedia {
			continue
		}

		// Get the actual object
		obj, err := h.store.GetObject(ctx, entry.PostID)
		if err != nil {
			h.logger.Warn("failed to get object from timeline", zap.String("id", entry.PostID), zap.Error(err))
			continue
		}

		// Get the actor who created the object
		var attributedTo string
		var objActor *activitypub.Actor

		switch o := obj.(type) {
		case *activitypub.Note:
			attributedTo = o.AttributedTo
		case map[string]interface{}:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			parts := strings.Split(attributedTo, "/")
			if len(parts) > 0 {
				username := parts[len(parts)-1]
				objActor, _ = h.store.GetActor(ctx, username)
			}
		}

		// Apply local/remote filter
		if local && !strings.HasPrefix(attributedTo, h.cfg.BaseURL()) {
			continue
		}
		if remote && strings.HasPrefix(attributedTo, h.cfg.BaseURL()) {
			continue
		}

		// Check if blocked (only if authenticated)
		if currentActor != nil && objActor != nil {
			if _, err := h.store.GetBlock(ctx, currentActor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		status := ObjectToStatus(obj, objActor)

		// Get interaction counts
		objectID := entry.PostID

		if objectID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted (if authenticated)
			if currentActor != nil {
				if _, err := h.store.GetLike(ctx, currentActor.ID, objectID); err == nil {
					status.Favourited = true
				}
				if _, err := h.store.GetAnnounce(ctx, currentActor.ID, objectID); err == nil {
					status.Reblogged = true
				}
			}
		}

		statuses = append(statuses, status)
	}

	// Set Link header for pagination if there's a cursor
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if cursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/timelines/public?max_id=%s", h.cfg.BaseURL(), cursor)
		if limit != 20 {
			nextURL += fmt.Sprintf("&limit=%d", limit)
		}
		if local {
			nextURL += "&local=true"
		}
		if remote {
			nextURL += "&remote=true"
		}
		if onlyMedia {
			nextURL += "&only_media=true"
		}
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	body, _ := json.Marshal(statuses)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}
