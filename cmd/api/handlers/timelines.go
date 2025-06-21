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

	// Get home timeline items (posts from followed accounts)
	entries, cursor, err := h.store.GetHomeTimeline(ctx, claims.Username, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get home timeline", zap.Error(err))
		return common.InternalServerError(err), nil
	}
	
	h.logger.Info("timeline entries fetched",
		zap.String("username", claims.Username),
		zap.Int("count", len(entries)),
		zap.String("cursor", cursor))

	// Convert objects to statuses
	statuses := []models.Status{}
	for _, entry := range entries {
		// Extract object ID from PostID URL
		objectID := h.converter.ExtractIDFromURL(entry.PostID)
		
		// Get the actual object
		obj, err := h.store.GetObject(ctx, objectID)
		if err != nil {
			h.logger.Warn("failed to get object from timeline", 
				zap.String("post_id", entry.PostID),
				zap.String("object_id", objectID),
				zap.Error(err))
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
			username := h.converter.ExtractUsernameFromActorID(attributedTo)
			if username != "" {
				objActor, _ = h.store.GetActor(ctx, username)
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

		// Check if blocked
		if objActor != nil {
			if _, err := h.store.GetBlock(ctx, actor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		// Get interaction counts using the full PostID URL
		if entry.PostID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx, entry.PostID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx, entry.PostID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted
			if _, err := h.store.GetLike(ctx, actor.ID, entry.PostID); err == nil {
				status.Favourited = true
			}
			if _, err := h.store.GetAnnounce(ctx, actor.ID, entry.PostID); err == nil {
				status.Reblogged = true
			}
			bookmarked, _ := h.store.IsBookmarked(ctx, actor.PreferredUsername, entry.PostID)
			status.Bookmarked = bookmarked
		}

		statuses = append(statuses, status)
	}

	// Use common headers
	headers := common.GetAPIHeaders()

	// Add Link header for pagination if there's a cursor
	if cursor != "" {
		params := make(map[string]string)
		if limit != 20 {
			params["limit"] = strconv.Itoa(limit)
		}
		common.AddLinkHeader(headers, h.cfg.BaseURL(), "/api/v1/timelines/home", cursor, params)
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
			username := h.converter.ExtractUsernameFromActorID(attributedTo)
			if username != "" {
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

		status := h.converter.ObjectToStatus(obj, objActor)

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

	// Use common headers
	headers := common.GetAPIHeaders()

	// Add Link header for pagination if there's a cursor
	if cursor != "" {
		params := make(map[string]string)
		if limit != 20 {
			params["limit"] = strconv.Itoa(limit)
		}
		if local {
			params["local"] = "true"
		}
		if remote {
			params["remote"] = "true"
		}
		if onlyMedia {
			params["only_media"] = "true"
		}
		common.AddLinkHeader(headers, h.cfg.BaseURL(), "/api/v1/timelines/public", cursor, params)
	}

	body, _ := json.Marshal(statuses)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleHashtagTimeline retrieves posts with a specific hashtag
func (h *Handler) HandleHashtagTimeline(ctx context.Context, request events.APIGatewayV2HTTPRequest, hashtag string) (*events.APIGatewayV2HTTPResponse, error) {
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
	onlyMedia := request.QueryStringParameters["only_media"] == "true"

	// Get timeline items
	entries, cursor, err := h.store.GetHashtagTimeline(ctx, hashtag, local, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get hashtag timeline",
			zap.String("hashtag", hashtag),
			zap.Error(err))
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
			username := h.converter.ExtractUsernameFromActorID(attributedTo)
			if username != "" {
				objActor, _ = h.store.GetActor(ctx, username)
			}
		}

		// Check if blocked (only if authenticated)
		if currentActor != nil && objActor != nil {
			if _, err := h.store.GetBlock(ctx, currentActor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

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
				bookmarked, _ := h.store.IsBookmarked(ctx, currentActor.PreferredUsername, objectID)
				status.Bookmarked = bookmarked
			}
		}

		statuses = append(statuses, status)
	}

	// Set Link header for pagination if there's a cursor
	headers := common.GetAPIHeaders()

	if cursor != "" {
		params := make(map[string]string)
		if limit != 20 {
			params["limit"] = strconv.Itoa(limit)
		}
		if local {
			params["local"] = "true"
		}
		if onlyMedia {
			params["only_media"] = "true"
		}
		common.AddLinkHeader(headers, h.cfg.BaseURL(), fmt.Sprintf("/api/v1/timelines/tag/%s", hashtag), cursor, params)
	}

	body, _ := json.Marshal(statuses)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleListTimeline handles GET /api/v1/timelines/list/:list_id
func (h *Handler) HandleListTimeline(ctx context.Context, request events.APIGatewayV2HTTPRequest, listID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get the list to verify ownership
	list, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Verify ownership
	if list.Username != claims.Username {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	var cursor string
	// Support both max_id and since_id for cursor-based pagination
	if maxID := request.QueryStringParameters["max_id"]; maxID != "" {
		cursor = maxID
	}

	// Get list timeline entries
	entries, nextCursor, err := h.store.GetListTimeline(ctx, listID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get list timeline",
			zap.String("list_id", listID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get list timeline")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Retrieve the actual objects for each entry
	statuses := make([]models.Status, 0, len(entries))
	for _, entry := range entries {
		obj, err := h.store.GetObject(ctx, entry.PostID)
		if err != nil {
			h.logger.Warn("failed to get object from timeline",
				zap.String("object_id", entry.PostID),
				zap.Error(err))
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
			username := h.converter.ExtractUsernameFromActorID(attributedTo)
			if username != "" {
				objActor, _ = h.store.GetActor(ctx, username)
			}
		}

		// Check if blocked
		if objActor != nil {
			if _, err := h.store.GetBlock(ctx, actor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

		// Get interaction counts
		objectID := entry.PostID
		if objectID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted
			if _, err := h.store.GetLike(ctx, actor.ID, objectID); err == nil {
				status.Favourited = true
			}
			if _, err := h.store.GetAnnounce(ctx, actor.ID, objectID); err == nil {
				status.Reblogged = true
			}
			bookmarked, _ := h.store.IsBookmarked(ctx, actor.PreferredUsername, objectID)
			status.Bookmarked = bookmarked
		}

		statuses = append(statuses, status)
	}

	// Create response with Link header for pagination
	response := common.OK(statuses)
	if nextCursor != "" && len(statuses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/timelines/list/%s?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), listID, nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}
