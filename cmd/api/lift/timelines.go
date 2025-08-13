package lift

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Helper functions

// convertStatusesToTimeline converts status models to timeline entries
func convertStatusesToTimeline(statuses []*storageModels.Status) []*storageModels.Timeline {
	timeline := make([]*storageModels.Timeline, len(statuses))
	for i, status := range statuses {
		timeline[i] = &storageModels.Timeline{
			PostID: status.StatusID,
			// Add other fields as needed
		}
	}
	return timeline
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
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
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
	account, err := h.registry.Accounts().GetAccount(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get account", zap.Error(err))
		return nil, ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	actor := account.Actor
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
	result, err := h.registry.Notes().ListNotes(ctx.Context, &notes.ListNotesQuery{
		TimelineType: "home",
		ViewerID:     username,
		Pagination: interfaces.PaginationOptions{
			Limit:  params.limit,
			Cursor: params.maxID,
		},
	})
	if err != nil {
		h.logger.Error("failed to get home timeline", zap.Error(err))
		return nil, "", err
	}
	entries := convertStatusesToTimeline(result.Notes)
	cursor := result.Pagination.NextCursor

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
	obj, err := h.registry.Notes().GetNote(ctx.Context, objectID)
	if err != nil {
		h.logger.Warn("failed to get note from timeline",
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

	// Check visibility - only show statuses the user is allowed to see
	if !h.canActorSeeStatus(&status, actor.ID) {
		return nil // Skip this status if user can't see it
	}

	// Filter out direct messages from home timeline (they belong in conversations)
	if status.Visibility == VisibilityDirect {
		return nil
	}

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

	account, err := h.registry.Accounts().GetAccount(ctx.Context, objUsername)
	if err != nil {
		return nil
	}
	return account.Actor
}

// isActorBlocked checks if an actor is blocked
func (h *Handler) isActorBlocked(ctx *lift.Context, actor, objActor *activitypub.Actor) bool {
	if objActor == nil {
		return false
	}

	if isBlocked, err := h.registry.Relationships().IsBlocked(ctx.Context, actor.ID, objActor.ID); err == nil && isBlocked {
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
	likeCount64, _ := h.registry.Notes().GetLikeCount(ctx.Context, entry.PostID)
	announceCount64, _ := h.registry.Notes().GetBoostCount(ctx.Context, entry.PostID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = int(announceCount64)

	// Check if current user has interacted
	if hasLiked, err := h.registry.Notes().HasLiked(ctx.Context, actor.ID, entry.PostID); err == nil && hasLiked {
		status.Favourited = true
	}
	if hasReblogged, err := h.registry.Notes().HasReblogged(ctx.Context, actor.ID, entry.PostID); err == nil && hasReblogged {
		status.Reblogged = true
	}
	bookmarked, _ := h.registry.Notes().IsBookmarked(ctx.Context, username, entry.PostID)
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
		account, err := h.registry.Accounts().GetAccount(ctx.Context, testUsername)
		if err != nil {
			return nil
		}
		return account.Actor
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
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
			if err != nil {
				return nil
			}
			return account.Actor
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
	obj, err := h.registry.Notes().GetNote(ctx.Context, entry.PostID)
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

	// Only show public and unlisted statuses in public timeline
	if status.Visibility != VisibilityPublic && status.Visibility != VisibilityUnlisted {
		return models.Status{}, true // skip non-public statuses
	}

	// For unlisted posts, only show if user is authenticated (or it's truly public)
	if status.Visibility == VisibilityUnlisted && currentActor == nil {
		return models.Status{}, true // skip unlisted posts for unauthenticated users
	}

	// Check if the authenticated user can see this status (if authenticated)
	if currentActor != nil && !h.canActorSeeStatus(&status, currentActor.ID) {
		return models.Status{}, true // skip if user can't see it
	}

	// Add interaction data
	h.addInteractionData(ctx, &status, entry.PostID, currentActor)

	// Parse and add emojis
	h.enrichStatusWithEmojis(ctx.Context, &status)

	return status, false // don't skip
}

// isBlocked checks if the current actor has blocked the object actor
func (h *Handler) isBlocked(ctx *lift.Context, currentActor, objActor *activitypub.Actor) bool {
	if currentActor != nil && objActor != nil {
		if isBlocked, err := h.registry.Relationships().IsBlocked(ctx.Context, currentActor.ID, objActor.ID); err == nil && isBlocked {
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
	likeCount64, _ := h.registry.Notes().GetLikeCount(ctx.Context, objectID)
	announceCount64, _ := h.registry.Notes().GetBoostCount(ctx.Context, objectID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = int(announceCount64)

	// Check if current user has interacted (if authenticated)
	if currentActor != nil {
		if hasLiked, err := h.registry.Notes().HasLiked(ctx.Context, currentActor.ID, objectID); err == nil && hasLiked {
			status.Favourited = true
		}
		if hasReblogged, err := h.registry.Notes().HasReblogged(ctx.Context, currentActor.ID, objectID); err == nil && hasReblogged {
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
	username := ""
	authHeader := h.getAuthHeader(ctx)
	if authHeader != "" {
		if token, err := auth.ExtractBearerToken(authHeader); err == nil {
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
			if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
				username = claims.Username
			}
		}
	}

	// Parse query parameters
	params := h.parseTagTimelineParams(ctx, hashtag)

	// Use the Notes service to get hashtag timeline
	query := &notes.ListNotesQuery{
		ViewerID:     username, // May be empty for unauthenticated requests
		TimelineType: "hashtag",
		Hashtag:      hashtag,
		Pagination: interfaces.PaginationOptions{
			Limit:  params.Limit,
			Cursor: params.MaxID,
		},
		OnlyMedia: params.OnlyMedia,
	}

	result, err := h.registry.Notes().ListNotes(ctx.Context, query)
	if err != nil {
		h.logger.Error("failed to get hashtag timeline",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert and filter statuses
	statuses := make([]*models.Status, 0, len(result.Notes))
	for _, storageStatus := range result.Notes {
		// Apply local filter if specified
		if params.Local && !h.isLocal(storageStatus.AuthorUsername) {
			continue
		}

		// Convert to API format
		apiStatus, err := h.convertStorageStatusToAPI(storageStatus, username)
		if err != nil {
			h.logger.Warn("failed to convert status to API format",
				zap.String("status_id", storageStatus.StatusID),
				zap.Error(err))
			continue
		}
		statuses = append(statuses, apiStatus)
	}

	// Add pagination header
	if result.Pagination != nil && result.Pagination.NextCursor != "" {
		h.addTagTimelinePaginationHeader(ctx, params, result.Pagination.NextCursor)
	}

	return ctx.JSON(statuses)
}

// getTagTimelineUser extracts and authenticates user from request
func (h *Handler) getTagTimelineUser(ctx *lift.Context) *TagTimelineUser {
	// Check test mode first
	testUsername := h.getTestUsername(ctx)
	if testUsername != "" {
		account, err := h.registry.Accounts().GetAccount(ctx.Context, testUsername)
		if err != nil {
			return &TagTimelineUser{}
		}
		return &TagTimelineUser{Actor: account.Actor, Username: testUsername}
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

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return &TagTimelineUser{}
	}

	account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		return &TagTimelineUser{}
	}
	return &TagTimelineUser{Actor: account.Actor, Username: claims.Username}
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
	obj, err := h.registry.Notes().GetNote(ctx.Context, entry.PostID)
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

	isBlocked, err := h.registry.Relationships().IsBlocked(ctx.Context, currentActor.ID, objActor.ID)
	return err == nil && isBlocked
}

// addStatusInteractions adds interaction counts and user interaction state
func (h *Handler) addStatusInteractions(ctx *lift.Context, status *models.Status, objectID string, user *TagTimelineUser) {
	if objectID == "" {
		return
	}

	// Get counts
	likeCount64, _ := h.registry.Notes().GetLikeCount(ctx.Context, objectID)
	announceCount64, _ := h.registry.Notes().GetBoostCount(ctx.Context, objectID)
	status.FavouritesCount = int(likeCount64)
	status.ReblogsCount = int(announceCount64)

	// Check user interactions if authenticated
	if user.Actor == nil {
		return
	}

	if hasLiked, err := h.registry.Notes().HasLiked(ctx.Context, user.Actor.ID, objectID); err == nil && hasLiked {
		status.Favourited = true
	}
	if hasReblogged, err := h.registry.Notes().HasReblogged(ctx.Context, user.Actor.ID, objectID); err == nil && hasReblogged {
		status.Reblogged = true
	}
	if user.Username != "" {
		bookmarked, _ := h.registry.Notes().IsBookmarked(ctx.Context, user.Username, objectID)
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

	// Parse timeline parameters
	limit, cursor := h.parseTimelineParams(ctx)

	// Use the Lists service to get list timeline
	listService := h.registry.Lists()
	if listService == nil {
		return ctx.Status(500).JSON(map[string]string{"error": "Lists service not available"})
	}

	// Get list timeline through the service
	timelineResult, err := listService.GetListTimeline(ctx.Context, &lists.GetListTimelineQuery{
		ListID:   listID,
		ViewerID: username,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		h.logger.Error("failed to get list timeline",
			zap.String("list_id", listID),
			zap.String("username", username),
			zap.Error(err))

		// Handle specific error cases
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(404).JSON(map[string]string{"error": "list not found"})
		}
		if strings.Contains(err.Error(), "unauthorized") {
			return ctx.Status(403).JSON(map[string]string{"error": "unauthorized"})
		}
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert storage statuses to API format
	apiStatuses := make([]*models.Status, 0, len(timelineResult.Statuses))
	for _, storageStatus := range timelineResult.Statuses {
		apiStatus, err := h.convertStorageStatusToAPI(storageStatus, username)
		if err != nil {
			h.logger.Warn("failed to convert status to API format",
				zap.String("status_id", storageStatus.StatusID),
				zap.Error(err))
			continue
		}
		apiStatuses = append(apiStatuses, apiStatus)
	}

	// Add pagination header
	if timelineResult.Pagination != nil && timelineResult.Pagination.NextCursor != "" && len(apiStatuses) > 0 {
		linkURL := h.buildLinkURL(fmt.Sprintf("/api/v1/timelines/list/%s", listID), timelineResult.Pagination.NextCursor, map[string]string{"limit": strconv.Itoa(limit)})
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(apiStatuses)
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
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
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

// HandleGetDirectTimelineLift handles GET /api/v1/timelines/direct
func (h *Handler) HandleGetDirectTimelineLift(ctx *lift.Context) error {
	// Authenticate user
	username, err := h.authenticateDirectTimeline(ctx)
	if err != nil {
		return err
	}

	// Parse query parameters
	params := h.parseDirectTimelineParams(ctx)

	// Use the Notes service to get direct timeline
	query := &notes.ListNotesQuery{
		ViewerID:     username,
		TimelineType: "direct",
		Pagination: interfaces.PaginationOptions{
			Limit:  params.limit,
			Cursor: params.maxID,
		},
	}

	result, err := h.registry.Notes().ListNotes(ctx.Context, query)
	if err != nil {
		h.logger.Error("failed to get direct timeline",
			zap.String("username", username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert storage statuses to API format
	apiStatuses := make([]*models.Status, 0, len(result.Notes))
	for _, storageStatus := range result.Notes {
		apiStatus, err := h.convertStorageStatusToAPI(storageStatus, username)
		if err != nil {
			h.logger.Warn("failed to convert status to API format",
				zap.String("status_id", storageStatus.StatusID),
				zap.Error(err))
			continue
		}
		apiStatuses = append(apiStatuses, apiStatus)
	}

	// Add pagination header
	if result.Pagination != nil && result.Pagination.NextCursor != "" && len(apiStatuses) > 0 {
		linkURL := h.buildLinkURL("/api/v1/timelines/direct", result.Pagination.NextCursor, map[string]string{"limit": strconv.Itoa(params.limit)})
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, linkURL))
	}

	return ctx.JSON(apiStatuses)
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
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
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
	account, err := h.registry.Accounts().GetAccount(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	return account.Actor, nil
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

// buildLinkURL is a helper function to build Link header URLs
func (h *Handler) buildLinkURL(path, cursor string, params map[string]string) string {
	url := fmt.Sprintf("%s%s?max_id=%s", h.cfg.BaseURL(), path, cursor)
	for key, value := range params {
		url += fmt.Sprintf("&%s=%s", key, value)
	}
	return url
}

// NOTE: convertStorageStatusToAPI has been moved to helpers.go to be shared across all handlers
