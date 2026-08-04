package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// HandleGetMarkersLift handles GET /api/v1/markers
// Returns saved timeline positions
func (h *Handler) HandleGetMarkersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	username, err := h.authenticateUser(ctx, []string{auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	var timelines []string
	for _, timelineParam := range ctx.QueryAll("timeline[]") {
		for _, timeline := range strings.Split(timelineParam, ",") {
			timeline = strings.TrimSpace(timeline)
			if timeline != "" {
				timelines = append(timelines, timeline)
			}
		}
	}

	// Get markers using Accounts service
	result, err := h.registry.Accounts().GetMarkers(ctx.Context(), &accounts.GetMarkersQuery{
		Username:  username,
		Timelines: timelines,
	})
	if err != nil {
		h.logger.Error("failed to get markers", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	markers := result.Markers

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

	return okJSON(response)
}

// HandleSaveMarkersLift handles POST /api/v1/markers
// Saves timeline positions
func (h *Handler) HandleSaveMarkersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, resp, err := h.authenticateMarkersRequest(ctx, auth.ScopeWrite)
	if resp != nil || err != nil {
		return resp, err
	}

	// Parse and validate request
	req, resp, err := h.parseMarkersRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Validate markers
	if err := h.validateMarkers(req); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Save markers
	h.saveMarkers(ctx, username, req)

	// Get and return updated markers
	return h.returnUpdatedMarkers(ctx, username)
}

// authenticateMarkersRequest authenticates the markers request
func (h *Handler) authenticateMarkersRequest(ctx *apptheory.Context, requiredScope string) (string, *apptheory.Response, error) {
	// Check for test mode
	testUsername := h.getMarkersTestUsername(ctx)
	if testUsername != "" {
		h.logger.Debug("test mode: using provided username", zap.String("username", testUsername))
		return testUsername, nil, nil
	}

	// Normal authentication flow
	return h.authenticateMarkersWithScope(ctx, requiredScope)
}

// getMarkersTestUsername extracts test username from headers
func (h *Handler) getMarkersTestUsername(ctx *apptheory.Context) string {
	if !common.RunningUnitTests() {
		return ""
	}

	return ctx.Header("X-Test-Username")
}

// authenticateMarkersWithScope authenticates and checks for the required scope
func (h *Handler) authenticateMarkersWithScope(ctx *apptheory.Context, requiredScope string) (string, *apptheory.Response, error) {
	username, err := h.authenticateUser(ctx, []string{requiredScope})
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return "", resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	return username, nil, nil
}

// parseMarkersRequest parses the markers request body
func (h *Handler) parseMarkersRequest(ctx *apptheory.Context) (map[string]struct {
	LastReadID string `json:"last_read_id"`
}, *apptheory.Response, error) {
	req, err := apptheory.BindRequest[map[string]struct {
		LastReadID string `json:"last_read_id"`
	}](ctx, apptheory.BindConfig[map[string]struct {
		LastReadID string `json:"last_read_id"`
	}]{
		Body: true,
	})
	if err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return nil, resp, respErr
	}

	return req, nil, nil
}

// validateMarkers validates the markers request
func (h *Handler) validateMarkers(req map[string]struct {
	LastReadID string `json:"last_read_id"`
}) error {
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
func (h *Handler) saveMarkers(ctx *apptheory.Context, username string, req map[string]struct {
	LastReadID string `json:"last_read_id"`
}) {
	// Get current markers to determine versions
	result, err := h.registry.Accounts().GetMarkers(ctx.Context(), &accounts.GetMarkersQuery{
		Username: username,
	})
	currentMarkers := map[string]*storage.Marker{}
	if err == nil && result != nil && result.Markers != nil {
		currentMarkers = result.Markers
	}

	// Save each marker
	for timeline, markerData := range req {
		h.saveSingleMarker(ctx, username, timeline, markerData.LastReadID, currentMarkers)
	}
}

// saveSingleMarker saves a single marker
func (h *Handler) saveSingleMarker(ctx *apptheory.Context, username, timeline, lastReadID string, currentMarkers map[string]*storage.Marker) {
	if err := common.ValidateRequiredParam("last_read_id", lastReadID); err != nil {
		return
	}

	// Determine version
	version := h.calculateMarkerVersion(timeline, currentMarkers)

	// Save marker using Accounts service
	if _, err := h.registry.Accounts().SaveMarker(ctx.Context(), &accounts.SaveMarkerCommand{
		Username:   username,
		Timeline:   timeline,
		LastReadID: lastReadID,
		Version:    version,
	}); err != nil {
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
func (h *Handler) returnUpdatedMarkers(ctx *apptheory.Context, username string) (*apptheory.Response, error) {
	// Get updated markers using Accounts service
	result, err := h.registry.Accounts().GetMarkers(ctx.Context(), &accounts.GetMarkersQuery{
		Username: username,
	})
	if err != nil {
		h.logger.Error("failed to get updated markers", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	updatedMarkers := result.Markers

	// Convert to response format
	response := h.buildMarkersResponse(updatedMarkers)

	return okJSON(response)
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
