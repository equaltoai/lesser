package lift

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetScheduledStatusesLift handles GET /api/v1/scheduled_statuses
func (h *Handler) HandleGetScheduledStatusesLift(ctx *lift.Context) error {
	// Authenticate request
	username, err := h.authenticateScheduledStatusRequest(ctx)
	if err != nil {
		return err
	}

	// Parse pagination parameters
	limit, cursor := h.parseScheduledStatusPagination(ctx)

	// Get scheduled statuses
	scheduledStatuses, nextCursor, err := h.repos.ScheduledStatus().GetScheduledStatuses(ctx.Context, username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get scheduled statuses",
			zap.String("username", username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert to API format
	apiStatuses := h.convertScheduledStatusesToAPI(ctx, scheduledStatuses)

	// Set pagination header if needed
	h.setScheduledStatusPaginationHeader(ctx, nextCursor, limit)

	return ctx.JSON(apiStatuses)
}

// authenticateScheduledStatusRequest handles authentication for scheduled status requests
func (h *Handler) authenticateScheduledStatusRequest(ctx *lift.Context) (string, error) {
	// Check for test username
	testUsername := h.getScheduledStatusTestUsername(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Extract auth header
	authHeader := h.extractScheduledStatusAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and check scope
	return h.validateScheduledStatusToken(ctx, token)
}

// getScheduledStatusTestUsername extracts test username from headers
func (h *Handler) getScheduledStatusTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// extractScheduledStatusAuthHeader extracts authorization header
func (h *Handler) extractScheduledStatusAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// validateScheduledStatusToken validates the token and checks scope
func (h *Handler) validateScheduledStatusToken(ctx *lift.Context, token string) (string, error) {
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	if !claims.HasScope(auth.ScopeRead) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// parseScheduledStatusPagination parses pagination parameters
func (h *Handler) parseScheduledStatusPagination(ctx *lift.Context) (int, string) {
	// Parse limit
	limit := h.parseScheduledStatusLimit(ctx)

	// Parse cursor
	cursor := h.parseScheduledStatusCursor(ctx)

	return limit, cursor
}

// parseScheduledStatusLimit parses the limit parameter
func (h *Handler) parseScheduledStatusLimit(ctx *lift.Context) int {
	limit := 20
	limitStr := ctx.Query("limit")

	// Fallback to direct query param access if ctx.Query doesn't work (test mode)
	if limitStr == "" {
		limitStr = h.extractScheduledQueryParam(ctx, "limit")
	}

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	return limit
}

// parseScheduledStatusCursor parses pagination cursor from max_id/min_id
func (h *Handler) parseScheduledStatusCursor(ctx *lift.Context) string {
	maxID := ctx.Query("max_id")
	minID := ctx.Query("min_id")

	// Fallback to direct query param access if ctx.Query doesn't work (test mode)
	if maxID == "" && minID == "" {
		maxID = h.extractScheduledQueryParam(ctx, "max_id")
		minID = h.extractScheduledQueryParam(ctx, "min_id")
	}

	// Determine cursor
	if maxID != "" {
		return fmt.Sprintf("ID#%s", maxID)
	} else if minID != "" {
		// min_id requests newer posts, so we need reverse pagination
		return fmt.Sprintf("MIN#%s", minID)
	}

	return ""
}

// extractScheduledQueryParam extracts a query parameter from the request path (fallback for test mode)
func (h *Handler) extractScheduledQueryParam(ctx *lift.Context, param string) string {
	if ctx.Request != nil && strings.Contains(ctx.Request.Path, "?") {
		parts := strings.Split(ctx.Request.Path, "?")
		if len(parts) > 1 {
			params, _ := url.ParseQuery(parts[1])
			return params.Get(param)
		}
	}
	return ""
}

// convertScheduledStatusesToAPI converts scheduled statuses to API format
func (h *Handler) convertScheduledStatusesToAPI(ctx *lift.Context, scheduledStatuses []*storage.ScheduledStatus) []models.ScheduledStatus {
	apiStatuses := make([]models.ScheduledStatus, 0, len(scheduledStatuses))

	for _, scheduled := range scheduledStatuses {
		apiStatus := h.convertSingleScheduledStatus(ctx, scheduled)
		apiStatuses = append(apiStatuses, apiStatus)
	}

	return apiStatuses
}

// convertSingleScheduledStatus converts a single scheduled status to API format
func (h *Handler) convertSingleScheduledStatus(ctx *lift.Context, scheduled *storage.ScheduledStatus) models.ScheduledStatus {
	apiStatus := models.ScheduledStatus{
		ID:          scheduled.ID,
		ScheduledAt: scheduled.ScheduledAt.Format(time.RFC3339),
		Params: models.StatusParams{
			Text:        scheduled.Status,
			MediaIDs:    scheduled.MediaIDs,
			Sensitive:   scheduled.Sensitive,
			SpoilerText: scheduled.SpoilerText,
			Visibility:  scheduled.Visibility,
			Language:    scheduled.Language,
			InReplyToID: scheduled.InReplyToID,
			Poll:        h.convertScheduledPoll(scheduled.Poll),
		},
		MediaAttachments: h.loadScheduledMediaAttachments(ctx, scheduled.ID),
	}

	// Add application ID if present
	if scheduled.ApplicationID != "" {
		apiStatus.Params.ApplicationID = scheduled.ApplicationID
	}

	return apiStatus
}

// setScheduledStatusPaginationHeader sets the Link header for pagination
func (h *Handler) setScheduledStatusPaginationHeader(ctx *lift.Context, nextCursor string, limit int) {
	if nextCursor == "" {
		return
	}

	// Extract ID from cursor (format: "ID#<id>")
	if len(nextCursor) > 3 {
		nextID := nextCursor[3:]
		linkHeader := fmt.Sprintf(`<%s/api/v1/scheduled_statuses?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextID, limit)
		ctx.Response.Headers["Link"] = linkHeader
	}
}

// HandleGetScheduledStatusLift handles GET /api/v1/scheduled_statuses/:id
func (h *Handler) HandleGetScheduledStatusLift(ctx *lift.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
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
		// Extract token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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

	// Get scheduled status
	scheduled, err := h.repos.ScheduledStatus().GetScheduledStatus(ctx.Context, id)
	if err != nil || scheduled == nil {
		return ctx.Status(404).JSON(map[string]string{"error": "scheduled status not found"})
	}

	// Verify ownership
	if scheduled.Username != username {
		return ctx.Status(404).JSON(map[string]string{"error": "scheduled status not found"})
	}

	// Convert to API format
	apiStatus := models.ScheduledStatus{
		ID:          scheduled.ID,
		ScheduledAt: scheduled.ScheduledAt.Format(time.RFC3339),
		Params: models.StatusParams{
			Text:        scheduled.Status,
			MediaIDs:    scheduled.MediaIDs,
			Sensitive:   scheduled.Sensitive,
			SpoilerText: scheduled.SpoilerText,
			Visibility:  scheduled.Visibility,
			Language:    scheduled.Language,
			InReplyToID: scheduled.InReplyToID,
			Poll:        h.convertScheduledPoll(scheduled.Poll),
		},
		MediaAttachments: h.loadScheduledMediaAttachments(ctx, scheduled.ID),
	}

	if scheduled.ApplicationID != "" {
		apiStatus.Params.ApplicationID = scheduled.ApplicationID
	}

	return ctx.JSON(apiStatus)
}

// HandleUpdateScheduledStatusLift handles PUT /api/v1/scheduled_statuses/:id
func (h *Handler) HandleUpdateScheduledStatusLift(ctx *lift.Context) error {
	// Get and validate status ID
	id, err := h.getScheduledStatusUpdateID(ctx)
	if err != nil {
		return err
	}

	// Authenticate user
	username, err := h.authenticateScheduledUpdateRequest(ctx)
	if err != nil {
		return err
	}

	// Get and verify ownership of scheduled status
	existing, err := h.getAndVerifyScheduledStatus(ctx, id, username)
	if err != nil {
		return err
	}

	// Parse and apply updates
	if err := h.applyScheduledStatusUpdates(ctx, existing); err != nil {
		return err
	}

	// Save updated status
	if err := h.saveUpdatedScheduledStatus(ctx, existing, id); err != nil {
		return err
	}

	// Return updated status
	return h.returnUpdatedScheduledStatus(ctx, existing)
}

// getScheduledStatusUpdateID gets and validates the status ID
func (h *Handler) getScheduledStatusUpdateID(ctx *lift.Context) (string, error) {
	id := ctx.Param("id")
	if id == "" {
		return "", ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}
	return id, nil
}

// authenticateScheduledUpdateRequest authenticates the update request
func (h *Handler) authenticateScheduledUpdateRequest(ctx *lift.Context) (string, error) {
	// Check for test mode
	testUsername := h.getScheduledUpdateTestUsername(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Normal authentication flow
	return h.authenticateScheduledUpdateWithToken(ctx)
}

// getScheduledUpdateTestUsername extracts test username from headers
func (h *Handler) getScheduledUpdateTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// authenticateScheduledUpdateWithToken authenticates using bearer token
func (h *Handler) authenticateScheduledUpdateWithToken(ctx *lift.Context) (string, error) {
	// Extract auth header
	authHeader := h.extractScheduledUpdateAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and check scope
	return h.validateScheduledUpdateToken(ctx, token)
}

// extractScheduledUpdateAuthHeader extracts authorization header
func (h *Handler) extractScheduledUpdateAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// validateScheduledUpdateToken validates the token and checks scope
func (h *Handler) validateScheduledUpdateToken(ctx *lift.Context, token string) (string, error) {
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	if !claims.HasScope(auth.ScopeWrite) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// getAndVerifyScheduledStatus gets the scheduled status and verifies ownership
func (h *Handler) getAndVerifyScheduledStatus(ctx *lift.Context, id, username string) (*storage.ScheduledStatus, error) {
	existing, err := h.repos.ScheduledStatus().GetScheduledStatus(ctx.Context, id)
	if err != nil || existing == nil {
		return nil, ctx.Status(404).JSON(map[string]string{"error": "scheduled status not found"})
	}

	if existing.Username != username {
		return nil, ctx.Status(404).JSON(map[string]string{"error": "scheduled status not found"})
	}

	return existing, nil
}

// applyScheduledStatusUpdates parses the request and applies updates
func (h *Handler) applyScheduledStatusUpdates(ctx *lift.Context, existing *storage.ScheduledStatus) error {
	// Parse request
	req, err := h.parseScheduledUpdateRequest(ctx)
	if err != nil {
		return err
	}

	// Update scheduled time if provided
	if req.ScheduledAt != "" {
		if err := h.updateScheduledTime(ctx, existing, req.ScheduledAt); err != nil {
			return err
		}
	}

	return nil
}

// parseScheduledUpdateRequest parses the update request body
func (h *Handler) parseScheduledUpdateRequest(ctx *lift.Context) (*models.ScheduledStatusUpdateRequest, error) {
	var req models.ScheduledStatusUpdateRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Try fallback parsing for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return nil, ctx.Status(400).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return nil, ctx.Status(400).JSON(map[string]string{"error": "invalid request body"})
		}
	}
	return &req, nil
}

// updateScheduledTime updates the scheduled time if valid
func (h *Handler) updateScheduledTime(ctx *lift.Context, existing *storage.ScheduledStatus, scheduledAt string) error {
	scheduledTime, err := time.Parse(time.RFC3339, scheduledAt)
	if err != nil {
		return ctx.Status(422).JSON(map[string]string{"error": "invalid scheduled_at format"})
	}

	if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
		return ctx.Status(422).JSON(map[string]string{"error": "scheduled_at must be at least 5 minutes in the future"})
	}

	existing.ScheduledAt = scheduledTime
	return nil
}

// saveUpdatedScheduledStatus saves the updated scheduled status
func (h *Handler) saveUpdatedScheduledStatus(ctx *lift.Context, existing *storage.ScheduledStatus, id string) error {
	if err := h.repos.ScheduledStatus().UpdateScheduledStatus(ctx.Context, existing); err != nil {
		h.logger.Error("failed to update scheduled status",
			zap.String("id", id),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	return nil
}

// returnUpdatedScheduledStatus builds and returns the updated status response
func (h *Handler) returnUpdatedScheduledStatus(ctx *lift.Context, existing *storage.ScheduledStatus) error {
	apiStatus := models.ScheduledStatus{
		ID:          existing.ID,
		ScheduledAt: existing.ScheduledAt.Format(time.RFC3339),
		Params: models.StatusParams{
			Text:        existing.Status,
			MediaIDs:    existing.MediaIDs,
			Sensitive:   existing.Sensitive,
			SpoilerText: existing.SpoilerText,
			Visibility:  existing.Visibility,
			Language:    existing.Language,
			InReplyToID: existing.InReplyToID,
			Poll:        h.convertScheduledPoll(existing.Poll),
		},
		MediaAttachments: h.loadScheduledMediaAttachments(ctx, existing.ID),
	}

	if existing.ApplicationID != "" {
		apiStatus.Params.ApplicationID = existing.ApplicationID
	}

	return ctx.JSON(apiStatus)
}

// HandleDeleteScheduledStatusLift handles DELETE /api/v1/scheduled_statuses/:id
func (h *Handler) HandleDeleteScheduledStatusLift(ctx *lift.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
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
		// Extract token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get scheduled status to verify ownership
	scheduled, err := h.repos.ScheduledStatus().GetScheduledStatus(ctx.Context, id)
	if err != nil || scheduled == nil {
		return ctx.Status(404).JSON(map[string]string{"error": "scheduled status not found"})
	}

	// Verify ownership
	if scheduled.Username != username {
		return ctx.Status(404).JSON(map[string]string{"error": "scheduled status not found"})
	}

	// Delete the scheduled status
	if err := h.repos.ScheduledStatus().DeleteScheduledStatus(ctx.Context, id); err != nil {
		h.logger.Error("failed to delete scheduled status",
			zap.String("id", id),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Return empty object
	return ctx.JSON(map[string]any{})
}

// HandleScheduleStatusLift handles scheduling a status from the create status endpoint
func (h *Handler) HandleScheduleStatusLift(ctx *lift.Context, claims *auth.Claims, req models.CreateStatusRequest) error {
	// Parse scheduled time
	scheduledTime, err := time.Parse(time.RFC3339, *req.ScheduledAt)
	if err != nil {
		return ctx.Status(422).JSON(map[string]string{"error": "invalid scheduled_at format"})
	}

	// Validate scheduled time is in the future
	if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
		return ctx.Status(422).JSON(map[string]string{"error": "scheduled_at must be at least 5 minutes in the future"})
	}

	// Create scheduled status
	scheduled := &storage.ScheduledStatus{
		Username:    claims.Username,
		Status:      req.Status,
		MediaIDs:    req.MediaIDs,
		Sensitive:   req.Sensitive,
		SpoilerText: req.SpoilerText,
		Visibility:  req.Visibility,
		Language:    req.Language,
		InReplyToID: req.InReplyToID,
		ScheduledAt: scheduledTime,
		Published:   false,
	}

	// Store poll data if present
	if req.Poll != nil {
		pollData, err := json.Marshal(req.Poll)
		if err != nil {
			h.logger.Error("failed to marshal poll data", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		var pollMap map[string]any
		if err := json.Unmarshal(pollData, &pollMap); err != nil {
			h.logger.Error("failed to unmarshal poll data", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		scheduled.Poll = pollMap
	}

	// Store application ID if present in token
	if claims.ClientID != "" {
		scheduled.ApplicationID = claims.ClientID
	}

	// Create the scheduled status
	if err := h.repos.ScheduledStatus().CreateScheduledStatus(ctx.Context, scheduled); err != nil {
		h.logger.Error("failed to create scheduled status",
			zap.String("username", claims.Username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Return scheduled status response
	apiStatus := models.ScheduledStatus{
		ID:          scheduled.ID,
		ScheduledAt: scheduled.ScheduledAt.Format(time.RFC3339),
		Params: models.StatusParams{
			Text:        scheduled.Status,
			MediaIDs:    scheduled.MediaIDs,
			Sensitive:   scheduled.Sensitive,
			SpoilerText: scheduled.SpoilerText,
			Visibility:  scheduled.Visibility,
			Language:    scheduled.Language,
			InReplyToID: scheduled.InReplyToID,
			Poll:        req.Poll,
		},
		MediaAttachments: h.loadScheduledMediaAttachments(ctx, scheduled.ID),
	}

	if scheduled.ApplicationID != "" {
		apiStatus.Params.ApplicationID = scheduled.ApplicationID
	}

	return ctx.JSON(apiStatus)
}

// Helper methods for scheduled statuses
func (h *Handler) convertScheduledPoll(pollData map[string]any) *models.Poll {
	if pollData == nil {
		return nil
	}

	poll := &models.Poll{
		ID:         "", // Will be set when poll is actually created
		Multiple:   false,
		VotesCount: 0,
		Voted:      false, // Scheduled polls haven't been voted on yet
	}

	// Extract options and populate OptionsData (for responses)
	if optionsRaw, ok := pollData["options"]; ok {
		if optionsList, ok := optionsRaw.([]any); ok {
			optionsData := make([]models.PollOption, len(optionsList))
			for i, optRaw := range optionsList {
				if optStr, ok := optRaw.(string); ok {
					optionsData[i] = models.PollOption{
						Title:      optStr,
						VotesCount: 0,
					}
				}
			}
			poll.OptionsData = optionsData
		}
	}

	// Extract expires_at
	if expiresAtRaw, ok := pollData["expires_at"]; ok {
		if expiresAtStr, ok := expiresAtRaw.(string); ok {
			poll.ExpiresAt = expiresAtStr
		} else if expiresAtTime, ok := expiresAtRaw.(time.Time); ok {
			poll.ExpiresAt = expiresAtTime.Format(time.RFC3339)
		}
	}

	// Extract multiple
	if multipleRaw, ok := pollData["multiple"]; ok {
		if multiple, ok := multipleRaw.(bool); ok {
			poll.Multiple = multiple
		}
	}

	// Set expired status
	if poll.ExpiresAt != "" {
		if expiryTime, err := time.Parse(time.RFC3339, poll.ExpiresAt); err == nil {
			poll.Expired = time.Now().After(expiryTime)
		}
	}

	return poll
}

func (h *Handler) loadScheduledMediaAttachments(ctx *lift.Context, scheduledStatusID string) []any {
	attachments, err := h.repos.ScheduledStatus().GetScheduledStatusMedia(ctx, scheduledStatusID)
	if err != nil {
		h.logger.Warn("failed to load scheduled media attachments", zap.Error(err))
		return []any{}
	}

	result := make([]any, len(attachments))
	for i, attachment := range attachments {
		// Handle any type by asserting to map[string]any
		if attachmentMap, ok := attachment.(map[string]any); ok {
			result[i] = map[string]any{
				"id":          attachmentMap["id"],
				"type":        attachmentMap["type"],
				"url":         attachmentMap["url"],
				"preview_url": attachmentMap["preview_url"],
				"description": attachmentMap["description"],
			}
		}
	}

	return result
}
