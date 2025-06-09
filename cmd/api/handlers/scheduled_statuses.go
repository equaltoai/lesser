package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetScheduledStatuses handles GET /api/v1/scheduled_statuses
func (h *Handler) HandleGetScheduledStatuses(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Parse pagination parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	maxID := request.QueryStringParameters["max_id"]
	minID := request.QueryStringParameters["min_id"]

	// Determine cursor
	cursor := ""
	if maxID != "" {
		cursor = fmt.Sprintf("ID#%s", maxID)
	} else if minID != "" {
		// For min_id, we'd need to implement reverse pagination
		// For now, just use forward pagination
		cursor = fmt.Sprintf("ID#%s", minID)
	}

	// Get scheduled statuses
	scheduledStatuses, nextCursor, err := h.store.GetScheduledStatuses(ctx, claims.Username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get scheduled statuses",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get scheduled statuses")), nil
	}

	// Convert to API format
	apiStatuses := make([]models.ScheduledStatus, 0, len(scheduledStatuses))
	for _, scheduled := range scheduledStatuses {
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
				Poll:        nil, // TODO: Convert poll data
			},
			MediaAttachments: []interface{}{}, // TODO: Load media attachments
		}

		// Add application ID if present
		if scheduled.ApplicationID != "" {
			apiStatus.Params.ApplicationID = scheduled.ApplicationID
		}

		apiStatuses = append(apiStatuses, apiStatus)
	}

	// Build response with Link header for pagination
	resp := common.OK(apiStatuses)
	if nextCursor != "" {
		// Extract ID from cursor (format: "ID#<id>")
		if len(nextCursor) > 3 {
			nextID := nextCursor[3:]
			linkHeader := fmt.Sprintf(`<%s/api/v1/scheduled_statuses?max_id=%s&limit=%d>; rel="next"`,
				h.cfg.BaseURL(), nextID, limit)
			resp.Headers["Link"] = linkHeader
		}
	}

	return resp, nil
}

// HandleGetScheduledStatus handles GET /api/v1/scheduled_statuses/:id
func (h *Handler) HandleGetScheduledStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, id string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get scheduled status
	scheduled, err := h.store.GetScheduledStatus(ctx, id)
	if err != nil || scheduled == nil {
		return common.NotFound(fmt.Errorf("scheduled status not found")), nil
	}

	// Verify ownership
	if scheduled.Username != claims.Username {
		return common.NotFound(fmt.Errorf("scheduled status not found")), nil
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
			Poll:        nil, // TODO: Convert poll data
		},
		MediaAttachments: []interface{}{}, // TODO: Load media attachments
	}

	if scheduled.ApplicationID != "" {
		apiStatus.Params.ApplicationID = scheduled.ApplicationID
	}

	return common.OK(apiStatus), nil
}

// HandleUpdateScheduledStatus handles PUT /api/v1/scheduled_statuses/:id
func (h *Handler) HandleUpdateScheduledStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, id string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get existing scheduled status
	existing, err := h.store.GetScheduledStatus(ctx, id)
	if err != nil || existing == nil {
		return common.NotFound(fmt.Errorf("scheduled status not found")), nil
	}

	// Verify ownership
	if existing.Username != claims.Username {
		return common.NotFound(fmt.Errorf("scheduled status not found")), nil
	}

	// Parse request
	var req models.ScheduledStatusUpdateRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Update scheduled time if provided
	if req.ScheduledAt != "" {
		scheduledTime, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			return common.UnprocessableEntity(fmt.Errorf("invalid scheduled_at format")), nil
		}

		// Validate scheduled time is in the future
		if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
			return common.UnprocessableEntity(fmt.Errorf("scheduled_at must be at least 5 minutes in the future")), nil
		}

		existing.ScheduledAt = scheduledTime
	}

	// Update the scheduled status
	if err := h.store.UpdateScheduledStatus(ctx, existing); err != nil {
		h.logger.Error("failed to update scheduled status",
			zap.String("id", id),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to update scheduled status")), nil
	}

	// Return updated status
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
			Poll:        nil, // TODO: Convert poll data
		},
		MediaAttachments: []interface{}{}, // TODO: Load media attachments
	}

	if existing.ApplicationID != "" {
		apiStatus.Params.ApplicationID = existing.ApplicationID
	}

	return common.OK(apiStatus), nil
}

// HandleDeleteScheduledStatus handles DELETE /api/v1/scheduled_statuses/:id
func (h *Handler) HandleDeleteScheduledStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, id string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get scheduled status to verify ownership
	scheduled, err := h.store.GetScheduledStatus(ctx, id)
	if err != nil || scheduled == nil {
		return common.NotFound(fmt.Errorf("scheduled status not found")), nil
	}

	// Verify ownership
	if scheduled.Username != claims.Username {
		return common.NotFound(fmt.Errorf("scheduled status not found")), nil
	}

	// Delete the scheduled status
	if err := h.store.DeleteScheduledStatus(ctx, id); err != nil {
		h.logger.Error("failed to delete scheduled status",
			zap.String("id", id),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to delete scheduled status")), nil
	}

	// Return empty object
	return common.OK(map[string]interface{}{}), nil
}

// HandleScheduleStatus handles scheduling a status from the create status endpoint
func (h *Handler) HandleScheduleStatus(ctx context.Context, claims *auth.Claims, req models.CreateStatusRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse scheduled time
	scheduledTime, err := time.Parse(time.RFC3339, *req.ScheduledAt)
	if err != nil {
		return common.UnprocessableEntity(fmt.Errorf("invalid scheduled_at format")), nil
	}

	// Validate scheduled time is in the future
	if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
		return common.UnprocessableEntity(fmt.Errorf("scheduled_at must be at least 5 minutes in the future")), nil
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
		pollData, _ := json.Marshal(req.Poll)
		var pollMap map[string]interface{}
		json.Unmarshal(pollData, &pollMap)
		scheduled.Poll = pollMap
	}

	// Store application ID if present in token
	if claims.ClientID != "" {
		scheduled.ApplicationID = claims.ClientID
	}

	// Create the scheduled status
	if err := h.store.CreateScheduledStatus(ctx, scheduled); err != nil {
		h.logger.Error("failed to create scheduled status",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to schedule status")), nil
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
		MediaAttachments: []interface{}{}, // TODO: Load media attachments
	}

	if scheduled.ApplicationID != "" {
		apiStatus.Params.ApplicationID = scheduled.ApplicationID
	}

	return common.OK(apiStatus), nil
}
