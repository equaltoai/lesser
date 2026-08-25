package handlers

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

// HandleGetScheduledStatusesLift handles GET /api/v1/scheduled_statuses
func (h *Handler) HandleGetScheduledStatusesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate request
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	// Parse pagination parameters
	limit, cursor := h.parseScheduledStatusPagination(ctx)

	// Get scheduled service
	scheduledService := h.registry.Scheduled()
	if scheduledService == nil {
		h.logger.Error("scheduled service not available")
		return common.RespondServiceUnavailable(ctx, "scheduled service")
	}

	// Get scheduled statuses using service
	result, err := scheduledService.ListScheduledStatuses(ctx.Context(), &scheduled.ListScheduledStatusesQuery{
		Username: username,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		h.logger.Error("failed to get scheduled statuses",
			zap.String("username", username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Convert to API format
	apiStatuses := h.convertScheduledStatusesToAPI(ctx, result.ScheduledStatuses)

	// Set pagination header if needed
	nextCursor := ""
	if result.Pagination != nil {
		nextCursor = result.Pagination.NextCursor
	}
	resp, err := okJSON(apiStatuses)
	if err != nil {
		return nil, err
	}
	h.setScheduledStatusPaginationHeader(resp, nextCursor, limit)
	return resp, nil
}

// HandleGetScheduledStatusLift handles GET /api/v1/scheduled_statuses/:id
func (h *Handler) HandleGetScheduledStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get ID from path parameter
	id := ctx.Param("id")
	if common.ValidateRequiredParam("id", id) != nil {
		return common.RespondBadRequest(ctx, "ID required")
	}

	// Authenticate request
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	// Get scheduled service
	scheduledService := h.registry.Scheduled()
	if scheduledService == nil {
		h.logger.Error("scheduled service not available")
		return common.RespondServiceUnavailable(ctx, "scheduled service")
	}

	// Get scheduled status using service
	result, err := scheduledService.GetScheduledStatus(ctx.Context(), &scheduled.GetScheduledStatusQuery{
		ID:       id,
		Username: username, // For ownership verification
	})
	if err != nil {
		h.logger.Debug("scheduled status not found",
			zap.String("id", id),
			zap.Error(err))
		return common.RespondNotFound(ctx, "Record not found")
	}

	// Convert to API format with media attachments
	apiStatus := h.convertScheduledStatusToAPIWithMedia(ctx, result.ScheduledStatus, result.MediaAttachments)

	return okJSON(apiStatus)
}

// HandleUpdateScheduledStatusLift handles PUT /api/v1/scheduled_statuses/:id
func (h *Handler) HandleUpdateScheduledStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get ID from path parameter
	id := ctx.Param("id")
	if common.ValidateRequiredParam("id", id) != nil {
		return common.RespondBadRequest(ctx, "ID required")
	}

	// Authenticate request
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	// Parse request body
	var req apimodels.ScheduledStatusUpdateRequest
	if err := h.parseScheduledStatusRequest(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "Invalid request body")
	}

	// Validate scheduled_at if provided
	var scheduledAt *time.Time
	if req.ScheduledAt != "" {
		if err := common.ValidateScheduledTime(req.ScheduledAt); err != nil {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		if err := common.ValidateTimestamp(req.ScheduledAt, "scheduled_at"); err != nil {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		t, _ := time.Parse(time.RFC3339, req.ScheduledAt) // Validation passed, parse is safe
		scheduledAt = &t
	}

	// Get scheduled service
	scheduledService := h.registry.Scheduled()
	if scheduledService == nil {
		h.logger.Error("scheduled service not available")
		return common.RespondServiceUnavailable(ctx, "scheduled service")
	}

	// Update scheduled status using service
	result, err := scheduledService.UpdateScheduledStatus(ctx.Context(), &scheduled.UpdateScheduledStatusCommand{
		ID:          id,
		Username:    username,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		h.logger.Error("failed to update scheduled status",
			zap.String("id", id),
			zap.Error(err))
		// Check error type
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "Record not found")
		}
		if strings.Contains(err.Error(), "cannot update published") {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		if strings.Contains(err.Error(), "must be at least") {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		return common.RespondInternalServerError(ctx)
	}

	// Convert to API format with media attachments
	apiStatus := h.convertScheduledStatusToAPIWithMedia(ctx, result.ScheduledStatus, result.MediaAttachments)

	return okJSON(apiStatus)
}

// HandleDeleteScheduledStatusLift handles DELETE /api/v1/scheduled_statuses/:id
func (h *Handler) HandleDeleteScheduledStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get ID from path parameter
	id := ctx.Param("id")
	if common.ValidateRequiredParam("id", id) != nil {
		return common.RespondBadRequest(ctx, "ID required")
	}

	// Authenticate request
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	// Get scheduled service
	scheduledService := h.registry.Scheduled()
	if scheduledService == nil {
		h.logger.Error("scheduled service not available")
		return common.RespondServiceUnavailable(ctx, "scheduled service")
	}

	// Delete scheduled status using service
	err = scheduledService.DeleteScheduledStatus(ctx.Context(), &scheduled.DeleteScheduledStatusCommand{
		ID:       id,
		Username: username,
	})
	if err != nil {
		h.logger.Debug("failed to delete scheduled status",
			zap.String("id", id),
			zap.Error(err))
		// Check error type
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "Record not found")
		}
		if strings.Contains(err.Error(), "cannot delete published") {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		return common.RespondInternalServerError(ctx)
	}

	// Return empty object
	return okJSON(apimodels.EmptyObject{})
}

// HandleCreateScheduledStatusLift handles POST /api/v1/statuses (with scheduled_at)
func (h *Handler) HandleCreateScheduledStatusLift(ctx *apptheory.Context, statusReq *apimodels.CreateStatusRequest) (*apimodels.ScheduledStatus, *apptheory.Response, error) {
	// This is called from the main status creation handler when scheduled_at is present
	// Authenticate request and extract client ID for application tracking
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return nil, resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx)
		return nil, resp, respErr
	}
	username := claims.Username
	clientID := claims.ClientID

	// Parse scheduled time
	if statusReq.ScheduledAt == nil || common.ValidateRequiredParam("scheduledAt", *statusReq.ScheduledAt) != nil {
		resp, respErr := common.RespondUnprocessableEntity(ctx, "scheduled_at is required")
		return nil, resp, respErr
	}
	// Validate scheduled time format and constraints
	if err := common.ValidateScheduledTime(*statusReq.ScheduledAt); err != nil {
		resp, respErr := common.RespondUnprocessableEntity(ctx, fmt.Sprintf("invalid scheduled_at: %s", err.Error()))
		return nil, resp, respErr
	}
	scheduledAt, _ := time.Parse(time.RFC3339, *statusReq.ScheduledAt) // ValidateScheduledTime already validated this

	// Get scheduled service
	scheduledService := h.registry.Scheduled()
	if scheduledService == nil {
		h.logger.Error("scheduled service not available")
		resp, respErr := common.RespondServiceUnavailable(ctx, "scheduled service")
		return nil, resp, respErr
	}

	// Parse poll if provided
	var poll map[string]any
	if statusReq.Poll != nil {
		poll = h.convertAPIPollToMap(statusReq.Poll)

		// Validate poll parameters using centralized validation
		if err := common.ValidatePollParams(poll); err != nil {
			resp, respErr := common.RespondUnprocessableEntity(ctx, fmt.Sprintf("invalid poll parameters: %v", err))
			return nil, resp, respErr
		}
	}

	// Create scheduled status using service
	result, err := scheduledService.CreateScheduledStatus(ctx.Context(), &scheduled.CreateScheduledStatusCommand{
		Username:      username,
		Status:        statusReq.Status,
		MediaIDs:      statusReq.MediaIDs,
		Sensitive:     statusReq.Sensitive,
		SpoilerText:   statusReq.SpoilerText,
		Visibility:    statusReq.Visibility,
		Language:      statusReq.Language,
		InReplyToID:   statusReq.InReplyToID,
		Poll:          poll,
		ScheduledAt:   scheduledAt,
		ApplicationID: clientID,
	})
	if err != nil {
		h.logger.Error("failed to create scheduled status",
			zap.String("username", username),
			zap.Error(err))
		// Check error type
		if strings.Contains(err.Error(), "must be at least") {
			resp, respErr := common.RespondUnprocessableEntity(ctx, err.Error())
			return nil, resp, respErr
		}
		resp, respErr := common.RespondInternalServerError(ctx, "failed to create scheduled status")
		return nil, resp, respErr
	}

	// Convert to API format with media attachments
	apiStatus := h.convertScheduledStatusToAPIWithMedia(ctx, result.ScheduledStatus, result.MediaAttachments)

	return &apiStatus, nil, nil
}

// Helper methods

// parseScheduledStatusPagination parses pagination parameters
func (h *Handler) parseScheduledStatusPagination(ctx *apptheory.Context) (int, string) {
	// Parse limit
	limit := h.parseScheduledStatusLimit(ctx)

	// Parse cursor
	cursor := h.parseScheduledStatusCursor(ctx)

	return limit, cursor
}

// parseScheduledStatusLimit parses the limit parameter
func (h *Handler) parseScheduledStatusLimit(ctx *apptheory.Context) int {
	limitStr := queryValue(ctx, "limit")

	limit, err := common.ParseTimelineLimit(limitStr)
	if err != nil {
		limit = 20
	}

	return limit
}

// parseScheduledStatusCursor parses pagination cursor from max_id/min_id
func (h *Handler) parseScheduledStatusCursor(ctx *apptheory.Context) string {
	maxID := queryValue(ctx, "max_id")
	minID := queryValue(ctx, "min_id")

	// Use max_id as cursor (for backward pagination)
	if maxID != "" {
		return maxID
	}

	// min_id is typically used for forward pagination
	return minID
}

// setScheduledStatusPaginationHeader sets the Link header for pagination
func (h *Handler) setScheduledStatusPaginationHeader(resp *apptheory.Response, nextCursor string, limit int) {
	if common.ValidateRequiredParam("nextCursor", nextCursor) != nil {
		return
	}

	// Build next page URL
	nextURL := fmt.Sprintf("/api/v1/scheduled_statuses?max_id=%s&limit=%d", url.QueryEscape(nextCursor), limit)
	linkHeader := fmt.Sprintf(`<%s>; rel="next"`, nextURL)

	// Set the Link header
	setHeader(resp, "link", linkHeader)
}

// convertScheduledStatusesToAPI converts scheduled statuses to API format
func (h *Handler) convertScheduledStatusesToAPI(ctx *apptheory.Context, statuses []*storage.ScheduledStatus) []apimodels.ScheduledStatus {
	apiStatuses := make([]apimodels.ScheduledStatus, 0, len(statuses))

	for _, status := range statuses {
		// Get media attachments for each status
		var mediaAttachments []*models.Media
		if len(status.MediaIDs) > 0 {
			// Get scheduled service to fetch media
			if scheduledService := h.registry.Scheduled(); scheduledService != nil {
				mediaAttachments, _ = scheduledService.GetScheduledMediaAttachments(ctx.Context(), status.ID, status.Username)
			}
		}

		apiStatus := h.convertScheduledStatusToAPIWithMedia(ctx, status, mediaAttachments)
		apiStatuses = append(apiStatuses, apiStatus)
	}

	return apiStatuses
}

// convertScheduledStatusToAPIWithMedia converts a scheduled status to API format with media
func (h *Handler) convertScheduledStatusToAPIWithMedia(_ *apptheory.Context, status *storage.ScheduledStatus, mediaItems []*models.Media) apimodels.ScheduledStatus {
	// Convert media attachments to []any
	mediaAttachments := make([]any, 0, len(mediaItems))
	for _, media := range mediaItems {
		mediaAttachments = append(mediaAttachments, h.convertMediaToAPI(media))
	}

	// Build params
	params := apimodels.StatusParams{
		Text:          status.Status,
		MediaIDs:      status.MediaIDs,
		Sensitive:     status.Sensitive,
		SpoilerText:   status.SpoilerText,
		Visibility:    status.Visibility,
		Language:      status.Language,
		InReplyToID:   status.InReplyToID,
		ApplicationID: status.ApplicationID,
	}

	// Add poll if present
	if status.Poll != nil {
		params.Poll = h.convertStoragePollToAPI(status.Poll)
	}

	return apimodels.ScheduledStatus{
		ID:               status.ID,
		ScheduledAt:      status.ScheduledAt.Format(time.RFC3339),
		Params:           params,
		MediaAttachments: mediaAttachments,
	}
}

// convertMediaToAPI converts media item to API format
func (h *Handler) convertMediaToAPI(media *models.Media) apimodels.MediaAttachment {
	// Determine media type
	mediaType := "image"
	if media.ContentType != "" {
		if strings.HasPrefix(media.ContentType, "video/") {
			mediaType = "video"
		} else if strings.HasPrefix(media.ContentType, "audio/") {
			mediaType = "audio"
		} else if media.ContentType == "image/gif" {
			mediaType = "gifv"
		}
	}

	// Build meta information
	meta := map[string]interface{}{
		"original": map[string]interface{}{
			"width":  media.Width,
			"height": media.Height,
			"size":   fmt.Sprintf("%dx%d", media.Width, media.Height),
			"aspect": h.calculateAspectRatio(media.Width, media.Height),
		},
	}

	// Add duration for video/audio
	if mediaType == "video" || mediaType == "audio" {
		if original, ok := meta["original"].(map[string]interface{}); ok {
			original["duration"] = media.Duration
		}
	}

	return apimodels.MediaAttachment{
		ID:          media.MediaID,
		Type:        mediaType,
		URL:         media.CDNUrl,
		PreviewURL:  media.CDNUrl, // Use same URL for preview by default
		RemoteURL:   nil,          // For local media
		Description: media.Description,
		Blurhash:    media.Blurhash,
		Meta:        meta,
	}
}

// calculateAspectRatio calculates aspect ratio for media
func (h *Handler) calculateAspectRatio(width, height int) float64 {
	if height == 0 {
		return 1.0
	}
	return float64(width) / float64(height)
}

// convertStoragePollToAPI converts storage poll options to API format
func (h *Handler) convertStoragePollToAPI(poll map[string]any) *apimodels.Poll {
	if poll == nil {
		return nil
	}

	result := &apimodels.Poll{}

	// Extract fields from map
	if options, ok := poll["options"].([]string); ok {
		result.Options = options
	}
	if expiresIn, ok := poll["expires_in"].(int); ok {
		result.ExpiresIn = expiresIn
	}
	if multiple, ok := poll["multiple"].(bool); ok {
		result.Multiple = multiple
	}
	if hideTotals, ok := poll["hide_totals"].(bool); ok {
		result.HideTotals = hideTotals
	}

	return result
}

// convertAPIPollToMap converts API poll request to map format
func (h *Handler) convertAPIPollToMap(poll *apimodels.Poll) map[string]any {
	if poll == nil {
		return nil
	}

	options := make([]interface{}, 0, len(poll.Options))
	for _, opt := range poll.Options {
		options = append(options, opt)
	}

	return map[string]any{
		"options":     options,
		"expires_in":  poll.ExpiresIn,
		"multiple":    poll.Multiple,
		"hide_totals": poll.HideTotals,
	}
}

// parseScheduledStatusRequest parses scheduled status request with fallback for test environments
func (h *Handler) parseScheduledStatusRequest(ctx *apptheory.Context, req interface{}) error {
	return common.ParseRequestWithFallback(ctx, req)
}
