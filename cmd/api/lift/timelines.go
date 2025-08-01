package lift

import (
	"fmt"
	"strconv"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetHomeTimelineLift handles GET /api/v1/timelines/home
func (h *Handler) HandleGetHomeTimelineLift(ctx *lift.Context) error {
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

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
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

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Parse query parameters
	limit := 20
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := ctx.Query("max_id")
	if maxID == "" && ctx.Request != nil && ctx.Request.Request != nil {
		maxID = ctx.Request.Request.QueryParams["max_id"]
	}

	// Get home timeline items (posts from followed accounts)
	entries, cursor, err := h.store.GetHomeTimeline(ctx.Context, username, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get home timeline", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	h.logger.Info("timeline entries fetched",
		zap.String("username", username),
		zap.Int("count", len(entries)),
		zap.String("cursor", cursor))

	// Convert objects to statuses
	statuses := []models.Status{}
	for _, entry := range entries {
		// Extract object ID from PostID URL
		objectID := h.converter.ExtractIDFromURL(entry.PostID)

		// Get the actual object
		obj, err := h.store.GetObject(ctx.Context, objectID)
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
		case map[string]any:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
			if objUsername != "" {
				objActor, _ = h.store.GetActor(ctx.Context, objUsername)
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

		// Check if blocked
		if objActor != nil {
			if _, err := h.store.GetBlock(ctx.Context, actor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		// Get interaction counts using the full PostID URL
		if entry.PostID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx.Context, entry.PostID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, entry.PostID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted
			if _, err := h.store.GetLike(ctx.Context, actor.ID, entry.PostID); err == nil {
				status.Favourited = true
			}
			if _, err := h.store.GetAnnounce(ctx.Context, actor.ID, entry.PostID); err == nil {
				status.Reblogged = true
			}
			bookmarked, _ := h.store.IsBookmarked(ctx.Context, username, entry.PostID)
			status.Bookmarked = bookmarked
		}

		statuses = append(statuses, status)
	}

	// Add Link header for pagination if there's a cursor
	if cursor != "" {
		params := make(map[string]string)
		if limit != 20 {
			params["limit"] = strconv.Itoa(limit)
		}
		linkURL := h.buildLinkURL("/api/v1/timelines/home", cursor, params)
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(statuses)
}

// HandleGetPublicTimelineLift handles GET /api/v1/timelines/public
func (h *Handler) HandleGetPublicTimelineLift(ctx *lift.Context) error {
	// Public timeline doesn't require authentication, but check if user is authenticated
	var currentActor *activitypub.Actor
	
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Test mode - get actor if username provided
		currentActor, _ = h.store.GetActor(ctx.Context, testUsername)
	} else {
		// Try to authenticate user but don't fail if not authenticated
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

		if token, err := auth.ExtractBearerToken(authHeader); err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
			if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
				currentActor, _ = h.store.GetActor(ctx.Context, claims.Username)
			}
		}
	}

	// Parse query parameters
	limit := 20
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := ctx.Query("max_id")
	if maxID == "" && ctx.Request != nil && ctx.Request.Request != nil {
		maxID = ctx.Request.Request.QueryParams["max_id"]
	}

	local := ctx.Query("local") == "true"
	if !local && ctx.Request != nil && ctx.Request.Request != nil {
		local = ctx.Request.Request.QueryParams["local"] == "true"
	}

	remote := ctx.Query("remote") == "true"
	if !remote && ctx.Request != nil && ctx.Request.Request != nil {
		remote = ctx.Request.Request.QueryParams["remote"] == "true"
	}

	onlyMedia := ctx.Query("only_media") == "true"
	if !onlyMedia && ctx.Request != nil && ctx.Request.Request != nil {
		onlyMedia = ctx.Request.Request.QueryParams["only_media"] == "true"
	}

	// Get timeline items
	entries, cursor, err := h.store.GetPublicTimeline(ctx.Context, local, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get public timeline", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert objects to statuses
	statuses := []models.Status{}
	for _, entry := range entries {
		// Apply quick filters from timeline entry
		if onlyMedia && !entry.HasMedia {
			continue
		}

		// Get the actual object
		obj, err := h.store.GetObject(ctx.Context, entry.PostID)
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
		case map[string]any:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
			if objUsername != "" {
				objActor, _ = h.store.GetActor(ctx.Context, objUsername)
			}
		}

		// Check if blocked (only if authenticated)
		if currentActor != nil && objActor != nil {
			if _, err := h.store.GetBlock(ctx.Context, currentActor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

		// Get interaction counts
		objectID := entry.PostID

		if objectID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted (if authenticated)
			if currentActor != nil {
				if _, err := h.store.GetLike(ctx.Context, currentActor.ID, objectID); err == nil {
					status.Favourited = true
				}
				if _, err := h.store.GetAnnounce(ctx.Context, currentActor.ID, objectID); err == nil {
					status.Reblogged = true
				}
			}
		}

		statuses = append(statuses, status)
	}

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
		linkURL := h.buildLinkURL("/api/v1/timelines/public", cursor, params)
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(statuses)
}

// HandleGetTagTimelineLift handles GET /api/v1/timelines/tag/:hashtag
func (h *Handler) HandleGetTagTimelineLift(ctx *lift.Context) error {
	hashtag := ctx.Param("hashtag")
	if hashtag == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing hashtag"})
	}

	// Public timeline doesn't require authentication, but check if user is authenticated
	var currentActor *activitypub.Actor
	var currentUsername string

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Test mode - get actor if username provided
		currentActor, _ = h.store.GetActor(ctx.Context, testUsername)
		currentUsername = testUsername
	} else {
		// Try to authenticate user but don't fail if not authenticated
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

		if token, err := auth.ExtractBearerToken(authHeader); err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
			if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
				currentActor, _ = h.store.GetActor(ctx.Context, claims.Username)
				currentUsername = claims.Username
			}
		}
	}

	// Parse query parameters
	limit := 20
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := ctx.Query("max_id")
	if maxID == "" && ctx.Request != nil && ctx.Request.Request != nil {
		maxID = ctx.Request.Request.QueryParams["max_id"]
	}

	local := ctx.Query("local") == "true"
	if !local && ctx.Request != nil && ctx.Request.Request != nil {
		local = ctx.Request.Request.QueryParams["local"] == "true"
	}

	onlyMedia := ctx.Query("only_media") == "true"
	if !onlyMedia && ctx.Request != nil && ctx.Request.Request != nil {
		onlyMedia = ctx.Request.Request.QueryParams["only_media"] == "true"
	}

	// Get timeline items
	entries, cursor, err := h.store.GetHashtagTimeline(ctx.Context, hashtag, local, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get hashtag timeline",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert objects to statuses
	statuses := []models.Status{}
	for _, entry := range entries {
		// Apply quick filters from timeline entry
		if onlyMedia && !entry.HasMedia {
			continue
		}

		// Get the actual object
		obj, err := h.store.GetObject(ctx.Context, entry.PostID)
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
		case map[string]any:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
			if objUsername != "" {
				objActor, _ = h.store.GetActor(ctx.Context, objUsername)
			}
		}

		// Check if blocked (only if authenticated)
		if currentActor != nil && objActor != nil {
			if _, err := h.store.GetBlock(ctx.Context, currentActor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

		// Get interaction counts
		objectID := entry.PostID

		if objectID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted (if authenticated)
			if currentActor != nil {
				if _, err := h.store.GetLike(ctx.Context, currentActor.ID, objectID); err == nil {
					status.Favourited = true
				}
				if _, err := h.store.GetAnnounce(ctx.Context, currentActor.ID, objectID); err == nil {
					status.Reblogged = true
				}
				if currentUsername != "" {
					bookmarked, _ := h.store.IsBookmarked(ctx.Context, currentUsername, objectID)
					status.Bookmarked = bookmarked
				}
			}
		}

		statuses = append(statuses, status)
	}

	// Add Link header for pagination if there's a cursor
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
		linkURL := h.buildLinkURL(fmt.Sprintf("/api/v1/timelines/tag/%s", hashtag), cursor, params)
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(statuses)
}

// HandleGetListTimelineLift handles GET /api/v1/timelines/list/:list_id
func (h *Handler) HandleGetListTimelineLift(ctx *lift.Context) error {
	listID := ctx.Param("list_id")
	if listID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing list id"})
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
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

	// Get the list to verify ownership
	list, err := h.store.GetList(ctx.Context, listID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "list not found"})
	}

	// Verify ownership
	if list.Username != username {
		return ctx.Status(404).JSON(map[string]string{"error": "list not found"})
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

	// Get list timeline entries
	entries, nextCursor, err := h.store.GetListTimeline(ctx.Context, listID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get list timeline",
			zap.String("list_id", listID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get list timeline"})
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Retrieve the actual objects for each entry
	statuses := make([]models.Status, 0, len(entries))
	for _, entry := range entries {
		obj, err := h.store.GetObject(ctx.Context, entry.PostID)
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
		case map[string]any:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
			if objUsername != "" {
				objActor, _ = h.store.GetActor(ctx.Context, objUsername)
			}
		}

		// Check if blocked
		if objActor != nil {
			if _, err := h.store.GetBlock(ctx.Context, actor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

		// Get interaction counts
		objectID := entry.PostID
		if objectID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted
			if _, err := h.store.GetLike(ctx.Context, actor.ID, objectID); err == nil {
				status.Favourited = true
			}
			if _, err := h.store.GetAnnounce(ctx.Context, actor.ID, objectID); err == nil {
				status.Reblogged = true
			}
			bookmarked, _ := h.store.IsBookmarked(ctx.Context, username, objectID)
			status.Bookmarked = bookmarked
		}

		statuses = append(statuses, status)
	}

	// Add Link header for pagination if there's a cursor
	if nextCursor != "" && len(statuses) > 0 {
		linkURL := h.buildLinkURL(fmt.Sprintf("/api/v1/timelines/list/%s", listID), nextCursor, map[string]string{"limit": strconv.Itoa(limit)})
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(statuses)
}

// HandleGetDirectTimelineLift handles GET /api/v1/timelines/direct
func (h *Handler) HandleGetDirectTimelineLift(ctx *lift.Context) error {
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

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
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

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Parse query parameters
	limit := 20
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := ctx.Query("max_id")
	if maxID == "" && ctx.Request != nil && ctx.Request.Request != nil {
		maxID = ctx.Request.Request.QueryParams["max_id"]
	}

	// Get direct timeline items
	entries, cursor, err := h.store.GetDirectTimeline(ctx.Context, username, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get direct timeline", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	h.logger.Info("direct timeline entries fetched",
		zap.String("username", username),
		zap.Int("count", len(entries)),
		zap.String("cursor", cursor))

	// Convert objects to statuses
	statuses := []models.Status{}
	for _, entry := range entries {
		// Extract object ID from PostID URL
		objectID := h.converter.ExtractIDFromURL(entry.PostID)

		// Get the actual object
		obj, err := h.store.GetObject(ctx.Context, objectID)
		if err != nil {
			h.logger.Warn("failed to get object from direct timeline",
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
		case map[string]any:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		}

		if attributedTo != "" {
			// Extract username from actor ID
			objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
			if objUsername != "" {
				objActor, _ = h.store.GetActor(ctx.Context, objUsername)
			}
		}

		status := h.converter.ObjectToStatus(obj, objActor)

		// Check if blocked
		if objActor != nil {
			if _, err := h.store.GetBlock(ctx.Context, actor.ID, objActor.ID); err == nil {
				// Blocked user, skip
				continue
			}
		}

		// Get interaction counts using the full PostID URL
		if entry.PostID != "" {
			likeCount, _ := h.store.CountObjectLikes(ctx.Context, entry.PostID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, entry.PostID)
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount

			// Check if current user has interacted
			if _, err := h.store.GetLike(ctx.Context, actor.ID, entry.PostID); err == nil {
				status.Favourited = true
			}
			if _, err := h.store.GetAnnounce(ctx.Context, actor.ID, entry.PostID); err == nil {
				status.Reblogged = true
			}
			bookmarked, _ := h.store.IsBookmarked(ctx.Context, username, entry.PostID)
			status.Bookmarked = bookmarked
		}

		statuses = append(statuses, status)
	}

	// Add Link header for pagination if there's a cursor
	if cursor != "" {
		params := make(map[string]string)
		if limit != 20 {
			params["limit"] = strconv.Itoa(limit)
		}
		linkURL := h.buildLinkURL("/api/v1/timelines/direct", cursor, params)
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(statuses)
}

// buildLinkURL is a helper function to build Link header URLs
func (h *Handler) buildLinkURL(path, cursor string, params map[string]string) string {
	url := fmt.Sprintf("%s%s?max_id=%s", h.cfg.BaseURL(), path, cursor)
	for key, value := range params {
		url += fmt.Sprintf("&%s=%s", key, value)
	}
	return url
}