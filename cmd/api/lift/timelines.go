package lift

import (
	"fmt"
	"strconv"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetHomeTimelineLift handles GET /api/v1/timelines/home
func (h *Handler) HandleGetHomeTimelineLift(ctx *lift.Context) error {
	// Authenticate user
	username, err := h.authenticateHomeTimeline(ctx)
	if err != nil {
		return err
	}

	// Get the user's actor
	actor, err := h.getUserActorForTimeline(ctx, username)
	if err != nil {
		return err
	}

	// Parse query parameters
	params := h.parseHomeTimelineParams(ctx)

	// Get timeline entries
	entries, cursor, err := h.fetchHomeTimelineEntries(ctx, username, params)
	if err != nil {
		return err
	}

	// Convert entries to statuses
	statuses := h.convertHomeTimelineEntries(ctx, entries, actor, username)

	// Add pagination header
	h.setHomeTimelinePagination(ctx, cursor, params.limit)

	return ctx.JSON(statuses)
}

// homeTimelineParams holds parameters for home timeline requests
type homeTimelineParams struct {
	limit int
	maxID string
}

// authenticateHomeTimeline authenticates the user for home timeline access
func (h *Handler) authenticateHomeTimeline(ctx *lift.Context) (string, error) {
	// Test hook - check for test username header
	testUsername := h.extractTestUsernameForTimeline(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Extract and validate token
	authHeader := h.extractAuthorizationHeader(ctx)
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// extractTestUsernameForTimeline extracts test username from headers
func (h *Handler) extractTestUsernameForTimeline(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// extractAuthorizationHeader extracts authorization header from request
func (h *Handler) extractAuthorizationHeader(ctx *lift.Context) string {
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

	return authHeader
}

// getUserActorForTimeline gets the user's actor for timeline operations
func (h *Handler) getUserActorForTimeline(ctx *lift.Context, username string) (*activitypub.Actor, error) {
	actor, err := h.repos.Account().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	return actor, nil
}

// parseHomeTimelineParams parses query parameters for home timeline
func (h *Handler) parseHomeTimelineParams(ctx *lift.Context) homeTimelineParams {
	params := homeTimelineParams{
		limit: 20,
	}

	// Parse limit
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			params.limit = parsedLimit
		}
	}

	// Parse max_id
	params.maxID = ctx.Query("max_id")
	if params.maxID == "" && ctx.Request != nil && ctx.Request.Request != nil {
		params.maxID = ctx.Request.Request.QueryParams["max_id"]
	}

	return params
}

// fetchHomeTimelineEntries fetches timeline entries from the repository
func (h *Handler) fetchHomeTimelineEntries(ctx *lift.Context, username string, params homeTimelineParams) ([]*storageModels.Timeline, string, error) {
	entries, cursor, err := h.repos.Timeline().GetHomeTimeline(ctx.Context, username, params.limit, params.maxID)
	if err != nil {
		h.logger.Error("failed to get home timeline", zap.Error(err))
		return nil, "", ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	h.logger.Info("timeline entries fetched",
		zap.String("username", username),
		zap.Int("count", len(entries)),
		zap.String("cursor", cursor))

	return entries, cursor, nil
}

// convertHomeTimelineEntries converts timeline entries to API statuses
func (h *Handler) convertHomeTimelineEntries(ctx *lift.Context, entries []*storageModels.Timeline, actor *activitypub.Actor, username string) []models.Status {
	statuses := []models.Status{}
	
	for _, entry := range entries {
		status := h.convertSingleTimelineEntry(ctx, entry, actor, username)
		if status != nil {
			statuses = append(statuses, *status)
		}
	}
	
	return statuses
}

// convertSingleTimelineEntry converts a single timeline entry to a status
func (h *Handler) convertSingleTimelineEntry(ctx *lift.Context, entry *storageModels.Timeline, actor *activitypub.Actor, username string) *models.Status {
	// Extract object ID from PostID URL
	objectID := h.converter.ExtractIDFromURL(entry.PostID)

	// Get the actual object
	obj, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		h.logger.Warn("failed to get object from timeline",
			zap.String("post_id", entry.PostID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil
	}

	// Get the actor who created the object
	objActor := h.getObjectActor(ctx, obj)
	
	// Check if blocked
	if h.isActorBlocked(ctx, actor, objActor) {
		return nil
	}

	// Convert to status
	status := h.converter.ObjectToStatus(obj, objActor)

	// Add interaction data
	h.enrichStatusWithInteractions(ctx, &status, entry, actor, username)

	// Parse and add emojis
	h.enrichStatusWithEmojis(ctx.Context, &status)

	return &status
}

// getObjectActor retrieves the actor for an object
func (h *Handler) getObjectActor(ctx *lift.Context, obj any) *activitypub.Actor {
	var attributedTo string

	switch o := obj.(type) {
	case *activitypub.Note:
		attributedTo = o.AttributedTo
	case map[string]any:
		if attr, ok := o["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo == "" {
		return nil
	}

	// Extract username from actor ID
	objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
	if objUsername == "" {
		return nil
	}

	objActor, _ := h.repos.Account().GetActor(ctx.Context, objUsername)
	return objActor
}

// isActorBlocked checks if an actor is blocked
func (h *Handler) isActorBlocked(ctx *lift.Context, actor, objActor *activitypub.Actor) bool {
	if objActor == nil {
		return false
	}
	
	if _, err := h.repos.Social().GetBlock(ctx.Context, actor.ID, objActor.ID); err == nil {
		// Blocked user
		return true
	}
	
	return false
}

// enrichStatusWithInteractions adds interaction data to a status
func (h *Handler) enrichStatusWithInteractions(ctx *lift.Context, status *models.Status, entry *storageModels.Timeline, actor *activitypub.Actor, username string) {
	if entry.PostID == "" {
		return
	}

	// Get interaction counts
	likeCount64, _ := h.repos.Like().GetLikeCount(ctx.Context, entry.PostID)
	announceCount, _ := h.repos.Social().CountObjectAnnounces(ctx.Context, entry.PostID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = announceCount

	// Check if current user has interacted
	if _, err := h.repos.Like().GetLike(ctx.Context, actor.ID, entry.PostID); err == nil {
		status.Favourited = true
	}
	if _, err := h.repos.Social().GetAnnounce(ctx.Context, actor.ID, entry.PostID); err == nil {
		status.Reblogged = true
	}
	bookmarked, _ := h.repos.User().IsBookmarked(ctx.Context, username, entry.PostID)
	status.Bookmarked = bookmarked
}

// setHomeTimelinePagination sets pagination headers for home timeline
func (h *Handler) setHomeTimelinePagination(ctx *lift.Context, cursor string, limit int) {
	if cursor == "" {
		return
	}

	params := make(map[string]string)
	if limit != 20 {
		params["limit"] = strconv.Itoa(limit)
	}
	linkURL := h.buildLinkURL("/api/v1/timelines/home", cursor, params)
	ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
}

// HandleGetPublicTimelineLift handles GET /api/v1/timelines/public
func (h *Handler) HandleGetPublicTimelineLift(ctx *lift.Context) error {
	// Get current user (optional - public timeline doesn't require auth)
	currentActor := h.getOptionalCurrentActor(ctx)

	// Parse request parameters
	params := h.parsePublicTimelineParams(ctx)

	// Get timeline entries
	entries, cursor, err := h.repos.Timeline().GetPublicTimeline(ctx.Context, params.Local, params.Limit, params.MaxID)
	if err != nil {
		h.logger.Error("failed to get public timeline", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Process entries into statuses
	statuses := h.processTimelineEntries(ctx, entries, currentActor, params)

	// Add pagination header
	h.addPublicTimelinePagination(ctx, cursor, params)

	return ctx.JSON(statuses)
}

// PublicTimelineParams holds parameters for public timeline requests
type PublicTimelineParams struct {
	Limit     int
	MaxID     string
	Local     bool
	Remote    bool
	OnlyMedia bool
}

// getOptionalCurrentActor attempts to get current actor but doesn't fail if not authenticated
func (h *Handler) getOptionalCurrentActor(ctx *lift.Context) *activitypub.Actor {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Test mode - get actor if username provided
		actor, _ := h.repos.Account().GetActor(ctx.Context, testUsername)
		return actor
	}

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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			actor, _ := h.repos.Account().GetActor(ctx.Context, claims.Username)
			return actor
		}
	}

	return nil
}

// parsePublicTimelineParams parses query parameters for public timeline
func (h *Handler) parsePublicTimelineParams(ctx *lift.Context) PublicTimelineParams {
	params := PublicTimelineParams{
		Limit: 20, // Default limit
	}

	// Parse limit
	limitStr := h.getQueryParam(ctx, "limit")
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			params.Limit = parsedLimit
		}
	}

	// Parse other parameters
	params.MaxID = h.getQueryParam(ctx, "max_id")
	params.Local = h.getQueryParam(ctx, "local") == boolTrue
	params.Remote = h.getQueryParam(ctx, "remote") == boolTrue
	params.OnlyMedia = h.getQueryParam(ctx, "only_media") == boolTrue

	return params
}


// processTimelineEntries converts timeline entries to status responses
func (h *Handler) processTimelineEntries(ctx *lift.Context, entries []*storageModels.Timeline, currentActor *activitypub.Actor, params PublicTimelineParams) []models.Status {
	statuses := make([]models.Status, 0, len(entries))

	for _, entry := range entries {
		// Apply filters
		if params.OnlyMedia && !entry.HasMedia {
			continue
		}

		// Process the entry
		status, skip := h.processTimelineEntry(ctx, entry, currentActor)
		if skip {
			continue
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// processTimelineEntry processes a single timeline entry
func (h *Handler) processTimelineEntry(ctx *lift.Context, entry *storageModels.Timeline, currentActor *activitypub.Actor) (models.Status, bool) {
	// Get the actual object
	obj, err := h.repos.Object().GetObject(ctx.Context, entry.PostID)
	if err != nil {
		h.logger.Warn("failed to get object from timeline", zap.String("id", entry.PostID), zap.Error(err))
		return models.Status{}, true // skip
	}

	// Get the actor who created the object
	objActor := h.getObjectActor(ctx, obj)

	// Check if blocked (only if authenticated)
	if h.isBlocked(ctx, currentActor, objActor) {
		return models.Status{}, true // skip
	}

	// Convert to status
	status := h.converter.ObjectToStatus(obj, objActor)

	// Add interaction data
	h.addInteractionData(ctx, &status, entry.PostID, currentActor)

	// Parse and add emojis
	h.enrichStatusWithEmojis(ctx.Context, &status)

	return status, false // don't skip
}


// isBlocked checks if the current actor has blocked the object actor
func (h *Handler) isBlocked(ctx *lift.Context, currentActor, objActor *activitypub.Actor) bool {
	if currentActor != nil && objActor != nil {
		if _, err := h.repos.Social().GetBlock(ctx.Context, currentActor.ID, objActor.ID); err == nil {
			return true // blocked user
		}
	}
	return false
}

// addInteractionData adds like/reblog counts and user interaction state
func (h *Handler) addInteractionData(ctx *lift.Context, status *models.Status, objectID string, currentActor *activitypub.Actor) {
	if objectID == "" {
		return
	}

	// Get interaction counts
	likeCount64, _ := h.repos.Like().GetLikeCount(ctx.Context, objectID)
	announceCount, _ := h.repos.Social().CountObjectAnnounces(ctx.Context, objectID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = announceCount

	// Check if current user has interacted (if authenticated)
	if currentActor != nil {
		if _, err := h.repos.Like().GetLike(ctx.Context, currentActor.ID, objectID); err == nil {
			status.Favourited = true
		}
		if _, err := h.repos.Social().GetAnnounce(ctx.Context, currentActor.ID, objectID); err == nil {
			status.Reblogged = true
		}
	}
}

// addPublicTimelinePagination adds pagination header to the response
func (h *Handler) addPublicTimelinePagination(ctx *lift.Context, cursor string, params PublicTimelineParams) {
	if cursor == "" {
		return
	}

	queryParams := make(map[string]string)
	if params.Limit != 20 {
		queryParams["limit"] = strconv.Itoa(params.Limit)
	}
	if params.Local {
		queryParams["local"] = boolTrue
	}
	if params.Remote {
		queryParams["remote"] = boolTrue
	}
	if params.OnlyMedia {
		queryParams["only_media"] = boolTrue
	}

	linkURL := h.buildLinkURL("/api/v1/timelines/public", cursor, queryParams)
	ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
}

// TagTimelineParams holds parameters for hashtag timeline requests
type TagTimelineParams struct {
	Hashtag   string
	Limit     int
	MaxID     string
	Local     bool
	OnlyMedia bool
}

// TagTimelineUser holds authenticated user information
type TagTimelineUser struct {
	Actor    *activitypub.Actor
	Username string
}

// HandleGetTagTimelineLift handles GET /api/v1/timelines/tag/:hashtag
func (h *Handler) HandleGetTagTimelineLift(ctx *lift.Context) error {
	hashtag := ctx.Param("hashtag")
	if hashtag == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing hashtag"})
	}

	// Get authenticated user (if any)
	user := h.getTagTimelineUser(ctx)
	
	// Parse query parameters
	params := h.parseTagTimelineParams(ctx, hashtag)

	// Get timeline entries
	entries, cursor, err := h.repos.Timeline().GetHashtagTimeline(ctx.Context, params.Hashtag, params.Local, params.Limit, params.MaxID)
	if err != nil {
		h.logger.Error("failed to get hashtag timeline",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert entries to statuses
	statuses := h.processTagTimelineEntries(ctx, entries, params, user)

	// Add pagination header
	h.addTagTimelinePaginationHeader(ctx, params, cursor)

	return ctx.JSON(statuses)
}

// getTagTimelineUser extracts and authenticates user from request
func (h *Handler) getTagTimelineUser(ctx *lift.Context) *TagTimelineUser {
	// Check test mode first
	testUsername := h.getTestUsername(ctx)
	if testUsername != "" {
		actor, _ := h.repos.Account().GetActor(ctx.Context, testUsername)
		return &TagTimelineUser{Actor: actor, Username: testUsername}
	}

	// Try to authenticate regular user
	authHeader := h.getAuthHeader(ctx)
	if authHeader == "" {
		return &TagTimelineUser{}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return &TagTimelineUser{}
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return &TagTimelineUser{}
	}

	actor, _ := h.repos.Account().GetActor(ctx.Context, claims.Username)
	return &TagTimelineUser{Actor: actor, Username: claims.Username}
}

// parseTagTimelineParams extracts query parameters
func (h *Handler) parseTagTimelineParams(ctx *lift.Context, hashtag string) *TagTimelineParams {
	params := &TagTimelineParams{
		Hashtag: hashtag,
		Limit:   20,
	}

	// Parse limit
	if limitStr := h.getQueryParam(ctx, "limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			params.Limit = parsedLimit
		}
	}

	// Parse other parameters
	params.MaxID = h.getQueryParam(ctx, "max_id")
	params.Local = h.getQueryParam(ctx, "local") == boolTrue
	params.OnlyMedia = h.getQueryParam(ctx, "only_media") == boolTrue

	return params
}

// processTagTimelineEntries converts timeline entries to statuses
func (h *Handler) processTagTimelineEntries(ctx *lift.Context, entries []*storageModels.Timeline, params *TagTimelineParams, user *TagTimelineUser) []models.Status {
	statuses := []models.Status{}
	
	for _, entry := range entries {
		if params.OnlyMedia && !entry.HasMedia {
			continue
		}

		status, skip := h.processTagTimelineEntry(ctx, entry, user)
		if skip {
			continue
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// processTagTimelineEntry processes a single timeline entry
func (h *Handler) processTagTimelineEntry(ctx *lift.Context, entry *storageModels.Timeline, user *TagTimelineUser) (models.Status, bool) {
	// Get the actual object
	obj, err := h.repos.Object().GetObject(ctx.Context, entry.PostID)
	if err != nil {
		h.logger.Warn("failed to get object from timeline", zap.String("id", entry.PostID), zap.Error(err))
		return models.Status{}, true
	}

	// Get object author
	objActor := h.getObjectActor(ctx, obj)

	// Check if blocked
	if h.isUserBlocked(ctx, user.Actor, objActor) {
		return models.Status{}, true
	}

	// Convert to status
	status := h.converter.ObjectToStatus(obj, objActor)

	// Add interaction data
	h.addStatusInteractions(ctx, &status, entry.PostID, user)

	// Parse and add emojis
	h.enrichStatusWithEmojis(ctx.Context, &status)

	return status, false
}


// isUserBlocked checks if current user has blocked the object author
func (h *Handler) isUserBlocked(ctx *lift.Context, currentActor, objActor *activitypub.Actor) bool {
	if currentActor == nil || objActor == nil {
		return false
	}

	_, err := h.repos.Social().GetBlock(ctx.Context, currentActor.ID, objActor.ID)
	return err == nil
}

// addStatusInteractions adds interaction counts and user interaction state
func (h *Handler) addStatusInteractions(ctx *lift.Context, status *models.Status, objectID string, user *TagTimelineUser) {
	if objectID == "" {
		return
	}

	// Get counts
	likeCount64, _ := h.repos.Like().GetLikeCount(ctx.Context, objectID)
	announceCount, _ := h.repos.Social().CountObjectAnnounces(ctx.Context, objectID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = announceCount

	// Check user interactions if authenticated
	if user.Actor == nil {
		return
	}

	if _, err := h.repos.Like().GetLike(ctx.Context, user.Actor.ID, objectID); err == nil {
		status.Favourited = true
	}
	if _, err := h.repos.Social().GetAnnounce(ctx.Context, user.Actor.ID, objectID); err == nil {
		status.Reblogged = true
	}
	if user.Username != "" {
		bookmarked, _ := h.repos.User().IsBookmarked(ctx.Context, user.Username, objectID)
		status.Bookmarked = bookmarked
	}
}

// addTagTimelinePaginationHeader adds Link header for pagination
func (h *Handler) addTagTimelinePaginationHeader(ctx *lift.Context, params *TagTimelineParams, cursor string) {
	if cursor == "" {
		return
	}

	queryParams := make(map[string]string)
	if params.Limit != 20 {
		queryParams["limit"] = strconv.Itoa(params.Limit)
	}
	if params.Local {
		queryParams["local"] = boolTrue
	}
	if params.OnlyMedia {
		queryParams["only_media"] = boolTrue
	}

	linkURL := h.buildLinkURL(fmt.Sprintf("/api/v1/timelines/tag/%s", params.Hashtag), cursor, queryParams)
	ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
}

// HandleGetListTimelineLift handles GET /api/v1/timelines/list/:list_id
func (h *Handler) HandleGetListTimelineLift(ctx *lift.Context) error {
	listID := ctx.Param("list_id")
	if listID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing list id"})
	}

	// Authenticate and get username
	username, err := h.authenticateTimelineRequest(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Verify list ownership
	_, err = h.validateListOwnership(ctx, listID, username)
	if err != nil {
		return err
	}

	// Parse timeline parameters
	limit, cursor := h.parseTimelineParams(ctx)

	// Build timeline response
	statuses, nextCursor, err := h.buildListTimelineResponse(ctx, listID, username, limit, cursor)
	if err != nil {
		return err
	}

	// Add pagination headers and return response
	return h.returnTimelineResponse(ctx, statuses, nextCursor, fmt.Sprintf("/api/v1/timelines/list/%s", listID), limit)
}

// Helper functions for HandleGetListTimelineLift

// authenticateTimelineRequest handles authentication for timeline requests
func (h *Handler) authenticateTimelineRequest(ctx *lift.Context, requiredScope string) (string, error) {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		return testUsername, nil
	}

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
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check required scope
	if !claims.HasScope(requiredScope) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// validateListOwnership validates that the user owns the list
func (h *Handler) validateListOwnership(ctx *lift.Context, listID, username string) (*storage.List, error) {
	list, err := h.repos.List().GetList(ctx.Context, listID)
	if err != nil {
		return nil, ctx.Status(404).JSON(map[string]string{"error": "list not found"})
	}

	if list.Username != username {
		return nil, ctx.Status(404).JSON(map[string]string{"error": "list not found"})
	}

	return list, nil
}

// parseTimelineParams parses limit and cursor from query parameters
func (h *Handler) parseTimelineParams(ctx *lift.Context) (int, string) {
	// Parse limit parameter
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

	// Parse cursor parameter
	cursor := ctx.Query("max_id")
	if cursor == "" && ctx.Request != nil && ctx.Request.Request != nil {
		cursor = ctx.Request.Request.QueryParams["max_id"]
	}

	return limit, cursor
}

// buildListTimelineResponse builds the timeline response with status objects
func (h *Handler) buildListTimelineResponse(ctx *lift.Context, listID, username string, limit int, cursor string) ([]models.Status, string, error) {
	// Get list timeline entries
	entries, nextCursor, err := h.repos.List().GetListTimeline(ctx.Context, listID, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get list timeline",
			zap.String("list_id", listID),
			zap.Error(err))
		return nil, "", ctx.Status(500).JSON(map[string]string{"error": "failed to get list timeline"})
	}

	// Get the user's actor
	actor, err := h.repos.Account().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, "", ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Build status objects from timeline entries
	statuses := h.buildStatusesFromEntries(ctx, entries, actor, username)

	return statuses, nextCursor, nil
}

// buildStatusesFromEntries converts timeline entries to status objects
func (h *Handler) buildStatusesFromEntries(ctx *lift.Context, entries []*storage.TimelineEntry, actor *activitypub.Actor, username string) []models.Status {
	statuses := make([]models.Status, 0, len(entries))

	for _, entry := range entries {
		status, err := h.buildSingleStatusFromEntry(ctx, entry, actor, username)
		if err != nil {
			// Log error but continue processing other entries
			h.logger.Warn("failed to build status from entry",
				zap.String("object_id", entry.PostID),
				zap.Error(err))
			continue
		}

		if status != nil {
			statuses = append(statuses, *status)
		}
	}

	return statuses
}

// buildSingleStatusFromEntry builds a single status from a timeline entry
func (h *Handler) buildSingleStatusFromEntry(ctx *lift.Context, entry *storage.TimelineEntry, actor *activitypub.Actor, username string) (*models.Status, error) {
	// Get the object
	obj, err := h.repos.Object().GetObject(ctx.Context, entry.PostID)
	if err != nil {
		return nil, err
	}

	// Get the object's actor
	objActor := h.getListObjectActor(ctx, obj)

	// Check if user is blocked
	if objActor != nil {
		if h.isListUserBlocked(ctx, actor, objActor) {
			return nil, nil // Skip blocked users
		}
	}

	// Convert to status
	status := h.converter.ObjectToStatus(obj, objActor)

	// Add interaction data
	h.addListInteractionData(ctx, &status, entry.PostID, actor.ID, username)

	// Parse and add emojis
	h.enrichStatusWithEmojis(ctx.Context, &status)

	return &status, nil
}

// getListObjectActor retrieves the actor who created the object in list timeline
func (h *Handler) getListObjectActor(ctx *lift.Context, obj any) *activitypub.Actor {
	var attributedTo string

	switch o := obj.(type) {
	case *activitypub.Note:
		attributedTo = o.AttributedTo
	case map[string]any:
		if attr, ok := o["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo == "" {
		return nil
	}

	// Extract username from actor ID
	objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
	if objUsername == "" {
		return nil
	}

	objActor, _ := h.repos.Account().GetActor(ctx.Context, objUsername)
	return objActor
}

// isListUserBlocked checks if the current user has blocked the object's author in list timeline
func (h *Handler) isListUserBlocked(ctx *lift.Context, currentActor, objActor *activitypub.Actor) bool {
	_, err := h.repos.Social().GetBlock(ctx.Context, currentActor.ID, objActor.ID)
	return err == nil // If no error, block exists
}

// addListInteractionData adds interaction counts and user interaction status to a list timeline status
func (h *Handler) addListInteractionData(ctx *lift.Context, status *models.Status, objectID, actorID, username string) {
	if objectID == "" {
		return
	}

	// Get counts
	likeCount64, _ := h.repos.Like().GetLikeCount(ctx.Context, objectID)
	announceCount, _ := h.repos.Social().CountObjectAnnounces(ctx.Context, objectID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = announceCount

	// Check user interactions
	if _, err := h.repos.Like().GetLike(ctx.Context, actorID, objectID); err == nil {
		status.Favourited = true
	}
	if _, err := h.repos.Social().GetAnnounce(ctx.Context, actorID, objectID); err == nil {
		status.Reblogged = true
	}
	bookmarked, _ := h.repos.User().IsBookmarked(ctx.Context, username, objectID)
	status.Bookmarked = bookmarked
}

// returnTimelineResponse adds pagination headers and returns the timeline response
func (h *Handler) returnTimelineResponse(ctx *lift.Context, statuses []models.Status, nextCursor, baseURL string, limit int) error {
	// Add Link header for pagination if there's a cursor
	if nextCursor != "" && len(statuses) > 0 {
		linkURL := h.buildLinkURL(baseURL, nextCursor, map[string]string{"limit": strconv.Itoa(limit)})
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(statuses)
}

// HandleGetDirectTimelineLift handles GET /api/v1/timelines/direct
func (h *Handler) HandleGetDirectTimelineLift(ctx *lift.Context) error {
	// Authenticate user
	username, err := h.authenticateDirectTimeline(ctx)
	if err != nil {
		return err
	}

	// Get the user's actor
	actor, err := h.getUserActorForDirectTimeline(ctx, username)
	if err != nil {
		return err
	}

	// Parse query parameters
	params := h.parseDirectTimelineParams(ctx)

	// Get direct timeline entries
	entries, cursor, err := h.fetchDirectTimelineEntries(ctx, username, params)
	if err != nil {
		return err
	}

	// Convert entries to statuses
	statuses := h.convertDirectTimelineEntries(ctx, entries, actor, username)

	// Add pagination header
	h.setDirectTimelinePagination(ctx, cursor, params.limit)

	return ctx.JSON(statuses)
}

// directTimelineParams holds parameters for direct timeline requests
type directTimelineParams struct {
	limit int
	maxID string
}

// authenticateDirectTimeline authenticates the user for direct timeline access
func (h *Handler) authenticateDirectTimeline(ctx *lift.Context) (string, error) {
	// Test hook - check for test username header
	testUsername := h.extractTestUsernameForDirectTimeline(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Extract and validate token
	authHeader := h.extractDirectTimelineAuthHeader(ctx)
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// extractTestUsernameForDirectTimeline extracts test username from headers
func (h *Handler) extractTestUsernameForDirectTimeline(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// extractDirectTimelineAuthHeader extracts authorization header from request
func (h *Handler) extractDirectTimelineAuthHeader(ctx *lift.Context) string {
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

	return authHeader
}

// getUserActorForDirectTimeline gets the user's actor for direct timeline operations
func (h *Handler) getUserActorForDirectTimeline(ctx *lift.Context, username string) (*activitypub.Actor, error) {
	actor, err := h.repos.Account().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	return actor, nil
}

// parseDirectTimelineParams parses query parameters for direct timeline
func (h *Handler) parseDirectTimelineParams(ctx *lift.Context) directTimelineParams {
	params := directTimelineParams{
		limit: 20,
	}

	// Parse limit
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			params.limit = parsedLimit
		}
	}

	// Parse max_id
	params.maxID = ctx.Query("max_id")
	if params.maxID == "" && ctx.Request != nil && ctx.Request.Request != nil {
		params.maxID = ctx.Request.Request.QueryParams["max_id"]
	}

	return params
}

// fetchDirectTimelineEntries fetches direct timeline entries from the repository
func (h *Handler) fetchDirectTimelineEntries(ctx *lift.Context, username string, params directTimelineParams) ([]*storageModels.Timeline, string, error) {
	entries, cursor, err := h.repos.Timeline().GetDirectTimeline(ctx.Context, username, params.limit, params.maxID)
	if err != nil {
		h.logger.Error("failed to get direct timeline", zap.Error(err))
		return nil, "", ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	h.logger.Info("direct timeline entries fetched",
		zap.String("username", username),
		zap.Int("count", len(entries)),
		zap.String("cursor", cursor))

	return entries, cursor, nil
}

// convertDirectTimelineEntries converts direct timeline entries to API statuses
func (h *Handler) convertDirectTimelineEntries(ctx *lift.Context, entries []*storageModels.Timeline, actor *activitypub.Actor, username string) []models.Status {
	statuses := []models.Status{}
	
	for _, entry := range entries {
		status := h.convertSingleDirectTimelineEntry(ctx, entry, actor, username)
		if status != nil {
			statuses = append(statuses, *status)
		}
	}
	
	return statuses
}

// convertSingleDirectTimelineEntry converts a single direct timeline entry to a status
func (h *Handler) convertSingleDirectTimelineEntry(ctx *lift.Context, entry *storageModels.Timeline, actor *activitypub.Actor, username string) *models.Status {
	// Extract object ID from PostID URL
	objectID := h.converter.ExtractIDFromURL(entry.PostID)

	// Get the actual object
	obj, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		h.logger.Warn("failed to get object from direct timeline",
			zap.String("post_id", entry.PostID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil
	}

	// Get the actor who created the object
	objActor := h.getDirectTimelineObjectActor(ctx, obj)
	
	// Check if blocked
	if h.isActorBlockedForDirect(ctx, actor, objActor) {
		return nil
	}

	// Convert to status
	status := h.converter.ObjectToStatus(obj, objActor)

	// Add interaction data
	h.enrichDirectStatusWithInteractions(ctx, &status, entry, actor, username)

	// Parse and add emojis
	h.enrichStatusWithEmojis(ctx.Context, &status)

	return &status
}

// getDirectTimelineObjectActor retrieves the actor for a direct timeline object
func (h *Handler) getDirectTimelineObjectActor(ctx *lift.Context, obj any) *activitypub.Actor {
	var attributedTo string

	switch o := obj.(type) {
	case *activitypub.Note:
		attributedTo = o.AttributedTo
	case map[string]any:
		if attr, ok := o["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo == "" {
		return nil
	}

	// Extract username from actor ID
	objUsername := h.converter.ExtractUsernameFromActorID(attributedTo)
	if objUsername == "" {
		return nil
	}

	objActor, _ := h.repos.Account().GetActor(ctx.Context, objUsername)
	return objActor
}

// isActorBlockedForDirect checks if an actor is blocked for direct timeline
func (h *Handler) isActorBlockedForDirect(ctx *lift.Context, actor, objActor *activitypub.Actor) bool {
	if objActor == nil {
		return false
	}
	
	if _, err := h.repos.Social().GetBlock(ctx.Context, actor.ID, objActor.ID); err == nil {
		// Blocked user
		return true
	}
	
	return false
}

// enrichDirectStatusWithInteractions adds interaction data to a direct timeline status
func (h *Handler) enrichDirectStatusWithInteractions(ctx *lift.Context, status *models.Status, entry *storageModels.Timeline, actor *activitypub.Actor, username string) {
	if entry.PostID == "" {
		return
	}

	// Get interaction counts
	likeCount64, _ := h.repos.Like().GetLikeCount(ctx.Context, entry.PostID)
	announceCount, _ := h.repos.Social().CountObjectAnnounces(ctx.Context, entry.PostID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = announceCount

	// Check if current user has interacted
	if _, err := h.repos.Like().GetLike(ctx.Context, actor.ID, entry.PostID); err == nil {
		status.Favourited = true
	}
	if _, err := h.repos.Social().GetAnnounce(ctx.Context, actor.ID, entry.PostID); err == nil {
		status.Reblogged = true
	}
	bookmarked, _ := h.repos.User().IsBookmarked(ctx.Context, username, entry.PostID)
	status.Bookmarked = bookmarked
}

// setDirectTimelinePagination sets pagination headers for direct timeline
func (h *Handler) setDirectTimelinePagination(ctx *lift.Context, cursor string, limit int) {
	if cursor == "" {
		return
	}

	params := make(map[string]string)
	if limit != 20 {
		params["limit"] = strconv.Itoa(limit)
	}
	linkURL := h.buildLinkURL("/api/v1/timelines/direct", cursor, params)
	ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
}

// buildLinkURL is a helper function to build Link header URLs
func (h *Handler) buildLinkURL(path, cursor string, params map[string]string) string {
	url := fmt.Sprintf("%s%s?max_id=%s", h.cfg.BaseURL(), path, cursor)
	for key, value := range params {
		url += fmt.Sprintf("&%s=%s", key, value)
	}
	return url
}
