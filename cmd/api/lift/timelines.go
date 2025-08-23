package lift

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Helper functions

// convertStatusesToTimeline converts status models to timeline entries

// homeTimelineParams holds parameters for home timeline requests (unused)
// type homeTimelineParams struct {
//	limit int
//	maxID string
// }

// authenticateHomeTimeline authenticates the user for home timeline access


// extractAuthorizationHeader extracts authorization header from request

// getUserActorForTimeline gets the user's actor for timeline operations

// parseHomeTimelineParams parses query parameters for home timeline

// fetchHomeTimelineEntries fetches timeline entries from the repository

// convertHomeTimelineEntries converts timeline entries to API statuses

// convertSingleTimelineEntry converts a single timeline entry to a status

// getObjectActor retrieves the actor for an object

// isActorBlocked checks if an actor is blocked

// enrichStatusWithInteractions adds interaction data to a status

// setHomeTimelinePagination sets pagination headers for home timeline

// PublicTimelineParams holds parameters for public timeline requests
type PublicTimelineParams struct {
	Limit     int
	MaxID     string
	Local     bool
	Remote    bool
	OnlyMedia bool
}

// getOptionalCurrentActor attempts to get current actor but doesn't fail if not authenticated

// parsePublicTimelineParams parses query parameters for public timeline

// processTimelineEntries converts timeline entries to status responses

// processTimelineEntry processes a single timeline entry

// isBlocked checks if the current actor has blocked the object actor

// addInteractionData adds like/reblog counts and user interaction state

// addPublicTimelinePagination adds pagination header to the response

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
	if err := common.ValidateRequiredParam("hashtag", hashtag); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Get authenticated user (if any)
	username := ""
	authHeader := h.getAuthHeader(ctx)
	if authHeader != "" {
		if token, err := auth.ExtractBearerToken(authHeader); err == nil {
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
			if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
				username = claims.Username
			}
		}
	}

	// Parse query parameters
	params, err := h.parseTagTimelineParams(ctx, hashtag)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

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
		return common.RespondInternalServerError(ctx)
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

// parseTagTimelineParams extracts query parameters
func (h *Handler) parseTagTimelineParams(ctx *lift.Context, hashtag string) (*TagTimelineParams, error) {
	params := &TagTimelineParams{
		Hashtag: hashtag,
		Limit:   20,
	}

	// Build parameter map for centralized validation
	paramMap := map[string]interface{}{
		"limit":      ctx.Query("limit"),
		"max_id":     ctx.Query("max_id"),
		"local":      ctx.Query("local"),
		"only_media": ctx.Query("only_media"),
	}

	// Validate timeline parameters using centralized validation
	if err := common.ValidateMastodonTimeline(paramMap); err != nil {
		h.logger.Debug("invalid timeline parameters", zap.Error(err))
		// Continue with defaults on validation error
	}

	// Parse limit
	if limitStr := h.getQueryParam(ctx, "limit"); limitStr != "" {
		parsedLimit, err := common.ParseTimelineLimit(limitStr)
		if err != nil {
			return nil, err
		}
		params.Limit = parsedLimit
	}

	// Parse other parameters
	params.MaxID = h.getQueryParam(ctx, "max_id")
	params.Local = h.getQueryParam(ctx, "local") == boolTrue
	params.OnlyMedia = h.getQueryParam(ctx, "only_media") == boolTrue

	return params, nil
}

// processTagTimelineEntries converts timeline entries to statuses

// processTagTimelineEntry processes a single timeline entry

// isUserBlocked checks if current user has blocked the object author

// addStatusInteractions adds interaction counts and user interaction state

// addTagTimelinePaginationHeader adds Link header for pagination
func (h *Handler) addTagTimelinePaginationHeader(ctx *lift.Context, params *TagTimelineParams, cursor string) {
	if common.ValidateRequiredParam(cursor, "cursor") != nil {
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
	if err := common.ValidateRequiredParam("list_id", listID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate and get username
	username, err := h.authenticateRequestWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Parse timeline parameters
	limit, cursor, err := h.parseTimelineParams(ctx)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Use the Lists service to get list timeline
	listService := h.registry.Lists()
	if listService == nil {
		return common.RespondInternalServerError(ctx, "Lists service not available")
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
			return common.RespondNotFound(ctx, "list not found")
		}
		if strings.Contains(err.Error(), "unauthorized") {
			return common.RespondForbidden(ctx, "unauthorized")
		}
		return common.RespondInternalServerError(ctx)
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


// parseTimelineParams parses limit and cursor from query parameters
func (h *Handler) parseTimelineParams(ctx *lift.Context) (int, string, error) {
	// Build parameter map for centralized validation
	paramMap := map[string]interface{}{
		"limit":  ctx.Query("limit"),
		"max_id": ctx.Query("max_id"),
	}

	// Validate timeline parameters using centralized validation
	if err := common.ValidateMastodonTimeline(paramMap); err != nil {
		h.logger.Debug("invalid timeline parameters", zap.Error(err))
		// Continue with defaults on validation error
	}

	// Parse limit parameter
	limit := 20
	limitStr := ctx.Query("limit")
	if common.ValidateRequiredParam(limitStr, "limitStr") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		l, err := common.ParseTimelineLimit(limitStr)
		if err != nil {
			return 0, "", err
		}
		limit = l
	}

	// Parse cursor parameter
	cursor := ctx.Query("max_id")
	if common.ValidateRequiredParam(cursor, "cursor") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		cursor = ctx.Request.Request.QueryParams["max_id"]
	}

	return limit, cursor, nil
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
		return common.RespondInternalServerError(ctx)
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
	testUsername := h.getTestUsername(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Extract and validate token
	authHeader := h.extractDirectTimelineAuthHeader(ctx)
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return "", common.RespondInsufficientScope(ctx)
	}

	return claims.Username, nil
}


// extractDirectTimelineAuthHeader extracts authorization header from request
func (h *Handler) extractDirectTimelineAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam(authHeader, "authHeader") != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if common.ValidateRequiredParam(authHeader, "authHeader") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if common.ValidateRequiredParam(authHeader, "authHeader") != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// getUserActorForDirectTimeline gets the user's actor for direct timeline operations

// parseDirectTimelineParams parses query parameters for direct timeline
func (h *Handler) parseDirectTimelineParams(ctx *lift.Context) directTimelineParams {
	params := directTimelineParams{
		limit: 20,
	}

	// Build parameter map for centralized validation
	paramMap := map[string]interface{}{
		"limit":  ctx.Query("limit"),
		"max_id": ctx.Query("max_id"),
	}

	// Validate timeline parameters using centralized validation
	if err := common.ValidateMastodonTimeline(paramMap); err != nil {
		h.logger.Debug("invalid timeline parameters", zap.Error(err))
		// Continue with defaults on validation error
	}

	// Parse limit
	limitStr := ctx.Query("limit")
	if common.ValidateRequiredParam(limitStr, "limitStr") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if parsedLimit, err := common.ParseTimelineLimit(limitStr); err == nil {
			params.limit = parsedLimit
		}
	}

	// Parse max_id
	params.maxID = ctx.Query("max_id")
	if common.ValidateRequiredParam(params.maxID, "params.maxID") != nil && ctx.Request != nil && ctx.Request.Request != nil {
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
