package lift

import (
	"encoding/json"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetMarkersLift handles GET /api/v1/markers
// Returns saved timeline positions
func (h *Handler) HandleGetMarkersLift(ctx *lift.Context) error {
	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	var claims *auth.Claims

	if testUsername != "" {
		// Test mode - use provided username
		username = testUsername
		h.logger.Debug("test mode: using provided username", zap.String("username", username))
	} else {
		// Extract token from Authorization header
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
		}

		// Check read scope
		if !claims.HasScope(auth.ScopeRead) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get timeline parameter (can be comma-separated)
	timelineParam := ctx.Query("timeline[]")

	// Fallback to direct query param access if ctx.Query doesn't work
	if timelineParam == "" && ctx.Request != nil && ctx.Request.Request != nil {
		timelineParam = ctx.Request.Request.QueryParams["timeline[]"]
	}

	var timelines []string
	if timelineParam != "" {
		timelines = strings.Split(timelineParam, ",")
	}

	// Get markers from storage
	markers, err := h.repos.Marker().GetMarkers(ctx.Context, username, timelines)
	if err != nil {
		h.logger.Error("failed to get markers", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "internal server error"})
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

	return ctx.JSON(response)
}

// HandleSaveMarkersLift handles POST /api/v1/markers
// Saves timeline positions
func (h *Handler) HandleSaveMarkersLift(ctx *lift.Context) error {
	// Authenticate user
	username, err := h.authenticateMarkersRequest(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Parse and validate request
	req, err := h.parseMarkersRequest(ctx)
	if err != nil {
		return err
	}

	// Validate markers
	if err := h.validateMarkers(req); err != nil {
		return ctx.Status(400).JSON(map[string]string{"error": err.Error()})
	}

	// Save markers
	h.saveMarkers(ctx, username, req)

	// Get and return updated markers
	return h.returnUpdatedMarkers(ctx, username)
}

// authenticateMarkersRequest authenticates the markers request
func (h *Handler) authenticateMarkersRequest(ctx *lift.Context, requiredScope string) (string, error) {
	// Check for test mode
	testUsername := h.getMarkersTestUsername(ctx)
	if testUsername != "" {
		h.logger.Debug("test mode: using provided username", zap.String("username", testUsername))
		return testUsername, nil
	}

	// Normal authentication flow
	return h.authenticateMarkersWithScope(ctx, requiredScope)
}

// getMarkersTestUsername extracts test username from headers
func (h *Handler) getMarkersTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// authenticateMarkersWithScope authenticates and checks for the required scope
func (h *Handler) authenticateMarkersWithScope(ctx *lift.Context, requiredScope string) (string, error) {
	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		return "", ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
	}

	// Check required scope
	if !claims.HasScope(requiredScope) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// parseMarkersRequest parses the markers request body
func (h *Handler) parseMarkersRequest(ctx *lift.Context) (map[string]struct{ LastReadID string `json:"last_read_id"` }, error) {
	var req map[string]struct {
		LastReadID string `json:"last_read_id"`
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environment
		return h.parseMarkersRequestFallback(ctx, err)
	}

	return req, nil
}

// parseMarkersRequestFallback handles fallback parsing for test environments
func (h *Handler) parseMarkersRequestFallback(ctx *lift.Context, originalErr error) (map[string]struct{ LastReadID string `json:"last_read_id"` }, error) {
	if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
		var req map[string]struct {
			LastReadID string `json:"last_read_id"`
		}
		if jsonErr := json.Unmarshal(ctx.Request.Body, &req); jsonErr != nil {
			h.logger.Debug("invalid markers request",
				zap.Error(originalErr),
				zap.Error(jsonErr))
			return nil, ctx.Status(400).JSON(map[string]string{"error": "invalid request body"})
		}
		return req, nil
	}
	return nil, ctx.Status(400).JSON(map[string]string{"error": "invalid request body"})
}

// validateMarkers validates the markers request
func (h *Handler) validateMarkers(req map[string]struct{ LastReadID string `json:"last_read_id"` }) error {
	// Check that at least one timeline is provided
	if len(req) == 0 {
		return &markerValidationError{message: "no markers provided"}
	}

	// Validate timeline types
	for timeline := range req {
		if !h.isValidTimelineType(timeline) {
			return &markerValidationError{message: "invalid timeline type: " + timeline}
		}
	}

	return nil
}

// markerValidationError represents a validation error for markers
type markerValidationError struct {
	message string
}

func (e *markerValidationError) Error() string {
	return e.message
}

// isValidTimelineType checks if the timeline type is valid
func (h *Handler) isValidTimelineType(timeline string) bool {
	return timeline == "home" || timeline == "notifications"
}

// saveMarkers saves all markers from the request
func (h *Handler) saveMarkers(ctx *lift.Context, username string, req map[string]struct{ LastReadID string `json:"last_read_id"` }) {
	// Get current markers to determine versions
	currentMarkers, _ := h.repos.Marker().GetMarkers(ctx.Context, username, nil)

	// Save each marker
	for timeline, markerData := range req {
		h.saveSingleMarker(ctx, username, timeline, markerData.LastReadID, currentMarkers)
	}
}

// saveSingleMarker saves a single marker
func (h *Handler) saveSingleMarker(ctx *lift.Context, username, timeline, lastReadID string, currentMarkers map[string]*storage.Marker) {
	if lastReadID == "" {
		return
	}

	// Determine version
	version := h.calculateMarkerVersion(timeline, currentMarkers)

	// Save marker
	if err := h.repos.Marker().SaveMarker(ctx.Context, username, timeline, lastReadID, version); err != nil {
		h.logger.Error("failed to save marker",
			zap.String("timeline", timeline),
			zap.Error(err))
		// Continue with other markers even if one fails
	}
}

// calculateMarkerVersion calculates the new version for a marker
func (h *Handler) calculateMarkerVersion(timeline string, currentMarkers map[string]*storage.Marker) int {
	if current, ok := currentMarkers[timeline]; ok {
		return current.Version + 1
	}
	return 1
}

// returnUpdatedMarkers gets and returns the updated markers
func (h *Handler) returnUpdatedMarkers(ctx *lift.Context, username string) error {
	// Get updated markers
	updatedMarkers, err := h.repos.Marker().GetMarkers(ctx.Context, username, nil)
	if err != nil {
		h.logger.Error("failed to get updated markers", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "internal server error"})
	}

	// Convert to response format
	response := h.buildMarkersResponse(updatedMarkers)

	return ctx.JSON(response)
}

// buildMarkersResponse builds the markers response
func (h *Handler) buildMarkersResponse(markers map[string]*storage.Marker) models.MarkersResponse {
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

	return response
}
