package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetMarkers handles GET /api/v1/markers
// Returns saved timeline positions
func (h *Handler) HandleGetMarkers(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get timeline parameter (can be comma-separated)
	timelineParam := request.QueryStringParameters["timeline[]"]
	var timelines []string
	if timelineParam != "" {
		timelines = strings.Split(timelineParam, ",")
	}

	// Get markers from storage
	markers, err := h.store.GetMarkers(ctx, claims.Username, timelines)
	if err != nil {
		h.logger.Error("failed to get markers", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to Mastodon format
	response := models.MarkersResponse{}

	if homeMarker, ok := markers["home"]; ok {
		response.Home = &models.Marker{
			LastReadID: homeMarker.LastReadID,
			UpdatedAt:  homeMarker.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
			Version:    homeMarker.Version,
		}
	}

	if notifMarker, ok := markers["notifications"]; ok {
		response.Notifications = &models.Marker{
			LastReadID: notifMarker.LastReadID,
			UpdatedAt:  notifMarker.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
			Version:    notifMarker.Version,
		}
	}

	return common.OK(response), nil
}

// HandleSaveMarkers handles POST /api/v1/markers
// Saves timeline positions
func (h *Handler) HandleSaveMarkers(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request body
	var req map[string]struct {
		LastReadID string `json:"last_read_id"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate that at least one timeline is provided
	if len(req) == 0 {
		return common.BadRequest(errors.New("no markers provided")), nil
	}

	// Validate timeline types
	for timeline := range req {
		if timeline != "home" && timeline != "notifications" {
			return common.BadRequest(errors.New("invalid timeline type: " + timeline)), nil
		}
	}

	// Get current markers to determine versions
	currentMarkers, _ := h.store.GetMarkers(ctx, claims.Username, nil)

	// Save each marker
	for timeline, markerData := range req {
		if markerData.LastReadID == "" {
			continue
		}

		// Determine version (increment if exists, otherwise 1)
		version := 1
		if current, ok := currentMarkers[timeline]; ok {
			version = current.Version + 1
		}

		// Save marker
		if err := h.store.SaveMarker(ctx, claims.Username, timeline, markerData.LastReadID, version); err != nil {
			h.logger.Error("failed to save marker",
				zap.String("timeline", timeline),
				zap.Error(err))
			// Continue with other markers even if one fails
		}
	}

	// Get updated markers to return
	updatedMarkers, err := h.store.GetMarkers(ctx, claims.Username, nil)
	if err != nil {
		h.logger.Error("failed to get updated markers", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to response format
	response := models.MarkersResponse{}

	if homeMarker, ok := updatedMarkers["home"]; ok {
		response.Home = &models.Marker{
			LastReadID: homeMarker.LastReadID,
			UpdatedAt:  homeMarker.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
			Version:    homeMarker.Version,
		}
	}

	if notifMarker, ok := updatedMarkers["notifications"]; ok {
		response.Notifications = &models.Marker{
			LastReadID: notifMarker.LastReadID,
			UpdatedAt:  notifMarker.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
			Version:    notifMarker.Version,
		}
	}

	return common.OK(response), nil
}
