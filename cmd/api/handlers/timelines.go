package handlers

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	apptheory "github.com/theory-cloud/apptheory/runtime"
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
func (h *Handler) HandleGetTagTimelineLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	hashtag := ctx.Param("hashtag")
	if err := common.ValidateRequiredParam("hashtag", hashtag); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Get authenticated user (if any) using helper method
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

	// Parse query parameters using improved helper
	params, err := h.parseTagTimelineParams(ctx, hashtag)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}
	excludeAgents := strings.EqualFold(strings.TrimSpace(queryValue(ctx, "exclude_agents")), boolTrue)

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

	result, err := h.registry.Notes().ListNotes(ctx.Context(), query)
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
		if excludeAgents && apiStatus.Account.Bot {
			continue
		}
		if apiStatus.Account.Bot && h.shouldHideRemoteAgentActor(ctx.Context(), storageStatus.AuthorID) {
			continue
		}
		statuses = append(statuses, apiStatus)
	}

	// Add pagination header using centralized helper
	resp, err := h.respondOK(ctx, statuses)
	if err != nil {
		return nil, err
	}

	if result.Pagination != nil && result.Pagination.NextCursor != "" {
		baseURL := fmt.Sprintf("/api/v1/timelines/tag/%s", hashtag)
		paginationParams := common.PaginationParams{
			Limit: params.Limit,
			MaxID: params.MaxID,
		}
		common.SetPaginationHeaders(resp, baseURL, paginationParams, true, false, result.Pagination.NextCursor, "")
	}

	return resp, nil
}

// getTagTimelineUser extracts and authenticates user from request

// parseTagTimelineParams extracts query parameters using centralized pagination helper
func (h *Handler) parseTagTimelineParams(ctx *apptheory.Context, hashtag string) (*TagTimelineParams, error) {
	// Use centralized timeline pagination helper
	timelineParams := common.GetTimelinePaginationParams(ctx)

	params := &TagTimelineParams{
		Hashtag:   hashtag,
		Limit:     timelineParams.Limit,
		MaxID:     timelineParams.MaxID,
		Local:     timelineParams.Local,
		OnlyMedia: timelineParams.OnlyMedia,
	}

	// Build parameter map for centralized validation
	paramMap := map[string]interface{}{
		"limit":      timelineParams.Limit,
		"max_id":     timelineParams.MaxID,
		"local":      timelineParams.Local,
		"only_media": timelineParams.OnlyMedia,
	}

	// Validate timeline parameters using centralized validation
	if err := common.ValidateMastodonTimeline(paramMap); err != nil {
		h.logger.Debug("invalid timeline parameters", zap.Error(err))
		// Continue with defaults on validation error
	}

	return params, nil
}

// processTagTimelineEntries converts timeline entries to statuses

// processTagTimelineEntry processes a single timeline entry

// isUserBlocked checks if current user has blocked the object author

// addStatusInteractions adds interaction counts and user interaction state

// addTagTimelinePaginationHeader adds Link header for pagination

// HandleGetListTimelineLift handles GET /api/v1/timelines/list/:list_id
func (h *Handler) HandleGetListTimelineLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	listID := ctx.Param("list_id")
	if err := common.ValidateRequiredParam("list_id", listID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate and get username
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

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
	timelineResult, err := listService.GetListTimeline(ctx.Context(), &lists.GetListTimelineQuery{
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

	// Add pagination header using common helper
	if timelineResult.Pagination != nil && timelineResult.Pagination.NextCursor != "" && len(apiStatuses) > 0 {
		baseURL := fmt.Sprintf("/api/v1/timelines/list/%s", listID)
		paginationParams := common.PaginationParams{
			Limit: limit,
			MaxID: cursor,
		}
		resp, err := h.respondOK(ctx, apiStatuses)
		if err != nil {
			return nil, err
		}
		common.SetPaginationHeaders(resp, baseURL, paginationParams, true, false, timelineResult.Pagination.NextCursor, "")
		return resp, nil
	}

	return h.respondOK(ctx, apiStatuses)
}

// Helper functions for HandleGetListTimelineLift

// parseTimelineParams parses limit and cursor from query parameters using centralized helper
func (h *Handler) parseTimelineParams(ctx *apptheory.Context) (int, string, error) {
	// Use centralized pagination parameter extraction
	paginationParams := common.GetPaginationParams(ctx)

	// Build parameter map for centralized validation
	paramMap := map[string]interface{}{
		"limit":  paginationParams.Limit,
		"max_id": paginationParams.MaxID,
	}

	// Validate timeline parameters using centralized validation
	if err := common.ValidateMastodonTimeline(paramMap); err != nil {
		h.logger.Debug("invalid timeline parameters", zap.Error(err))
		// Continue with defaults on validation error
	}

	return paginationParams.Limit, paginationParams.MaxID, nil
}

// HandleGetDirectTimelineLift handles GET /api/v1/timelines/direct
func (h *Handler) HandleGetDirectTimelineLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, resp, err := h.authenticateDirectTimeline(ctx)
	if resp != nil || err != nil {
		return resp, err
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

	result, err := h.registry.Notes().ListNotes(ctx.Context(), query)
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

	// Add pagination header using common helper
	if result.Pagination != nil && result.Pagination.NextCursor != "" && len(apiStatuses) > 0 {
		baseURL := "/api/v1/timelines/direct"
		paginationParams := common.PaginationParams{
			Limit: params.limit,
			MaxID: params.maxID,
		}
		resp, err := h.respondOK(ctx, apiStatuses)
		if err != nil {
			return nil, err
		}
		common.SetPaginationHeaders(resp, baseURL, paginationParams, true, false, result.Pagination.NextCursor, "")
		return resp, nil
	}

	return h.respondOK(ctx, apiStatuses)
}

// directTimelineParams holds parameters for direct timeline requests
type directTimelineParams struct {
	limit int
	maxID string
}

// authenticateDirectTimeline authenticates the user for direct timeline access
func (h *Handler) authenticateDirectTimeline(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return "", resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	return claims.Username, nil, nil
}

// getUserActorForDirectTimeline gets the user's actor for direct timeline operations

// parseDirectTimelineParams parses query parameters for direct timeline using centralized helper
func (h *Handler) parseDirectTimelineParams(ctx *apptheory.Context) directTimelineParams {
	// Use centralized pagination parameter extraction
	paginationParams := common.GetPaginationParams(ctx)

	params := directTimelineParams{
		limit: paginationParams.Limit,
		maxID: paginationParams.MaxID,
	}

	// Build parameter map for centralized validation
	paramMap := map[string]interface{}{
		"limit":  paginationParams.Limit,
		"max_id": paginationParams.MaxID,
	}

	// Validate timeline parameters using centralized validation
	if err := common.ValidateMastodonTimeline(paramMap); err != nil {
		h.logger.Debug("invalid timeline parameters", zap.Error(err))
		// Continue with defaults on validation error
	}

	return params
}

// NOTE: convertStorageStatusToAPI has been moved to helpers.go to be shared across all handlers
