package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageMods "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// Visibility constants
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
	VisibilityDirect   = "direct"
)

// HandleCreateStatusLift creates a new status using the Notes service
func (h *Handler) HandleCreateStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request
	var req models.CreateStatusRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request format")
	}

	// Validate status parameters using comprehensive validation
	statusParams := map[string]interface{}{
		"status":         req.Status,
		"visibility":     req.Visibility,
		"sensitive":      req.Sensitive,
		"spoiler_text":   req.SpoilerText,
		"language":       req.Language,
		"in_reply_to_id": req.InReplyToID,
	}
	if len(req.MediaIDs) > 0 {
		mediaIDs := make([]interface{}, 0, len(req.MediaIDs))
		for _, id := range req.MediaIDs {
			mediaIDs = append(mediaIDs, id)
		}
		statusParams["media_ids"] = mediaIDs
	}
	if req.ScheduledAt != nil {
		statusParams["scheduled_at"] = *req.ScheduledAt
	}
	if req.Poll != nil {
		options := make([]interface{}, 0, len(req.Poll.Options))
		for _, opt := range req.Poll.Options {
			options = append(options, opt)
		}
		statusParams["poll"] = map[string]interface{}{
			"options":     options,
			"expires_in":  req.Poll.ExpiresIn,
			"multiple":    req.Poll.Multiple,
			"hide_totals": req.Poll.HideTotals,
		}
	}
	if err := common.ValidateStatusParams(statusParams); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Default visibility
	if req.Visibility == "" {
		req.Visibility = VisibilityPublic
	}

	// Agent-specific safety rails (Phase 1).
	if claims.IsAgent {
		if resp, normErr := h.normalizeAgentMemoryEventRequest(ctx, claims, &req); resp != nil || normErr != nil {
			return resp, normErr
		}
		if resp, railErr := h.enforceAgentStatusCreateRails(ctx, claims, &req); resp != nil || railErr != nil {
			return resp, railErr
		}
	}

	var agentAttribution *activitypub.AgentPostAttribution
	if claims.IsAgent {
		attr, resp, buildErr := h.buildAgentStatusAttribution(ctx, claims, &req)
		if resp != nil || buildErr != nil {
			return resp, buildErr
		}
		agentAttribution = attr
	}

	// Call Notes service
	result, err := h.registry.Notes().CreateNote(ctx.Context(), &notes.CreateNoteCommand{
		AuthorID:         claims.Username,
		Content:          req.Status,
		Visibility:       req.Visibility,
		Sensitive:        req.Sensitive,
		SpoilerText:      req.SpoilerText,
		Language:         req.Language,
		InReplyToID:      req.InReplyToID,
		MediaIDs:         req.MediaIDs,
		AgentAttribution: agentAttribution,
	})
	if err != nil {
		h.logger.Error("failed to create note", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to create status")
	}

	if claims.IsAgent {
		h.recordAgentMemoryEvent(ctx, claims.Username, result.Note.StatusID, &req)
	}

	// Convert to Mastodon API format with storage-aware helper
	apiStatus, err := h.convertStorageStatusToAPI(result.Note, claims.Username)
	if err != nil {
		h.logger.Error("failed to convert created status", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to create status")
	}

	h.logger.Info("created status",
		zap.String("id", result.Note.StatusID),
		zap.String("content", req.Status))

	h.recordAgentAuditEvent(ctx, claims, "agent.status.create", result.Note.StatusID, map[string]any{
		"visibility":       req.Visibility,
		"in_reply_to_id":   req.InReplyToID,
		"has_media":        len(req.MediaIDs) > 0,
		"has_poll":         req.Poll != nil,
		"content_length":   len(req.Status),
		"spoiler_length":   len(req.SpoilerText),
		"language":         req.Language,
		"sensitive":        req.Sensitive,
		"scheduled":        req.ScheduledAt != nil,
		"requested_scopes": strings.Join(claims.Scopes, " "),
	})

	return createdJSON(apiStatus)
}

// HandleDeleteStatusLift deletes a status using the Notes service
func (h *Handler) HandleDeleteStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get the status first to return it (Mastodon API returns deleted status)
	status, err := h.registry.Notes().GetNote(ctx.Context(), statusID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "status not found")
		}
		h.logger.Error("failed to get status for deletion", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to delete status")
	}

	// Delete using Notes service
	err = h.registry.Notes().DeleteNote(ctx.Context(), &notes.DeleteNoteCommand{
		StatusID:  statusID,
		DeleterID: claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "status not found")
		}
		if strings.Contains(err.Error(), "not authorized") {
			return common.RespondForbidden(ctx, "not authorized to delete this status")
		}
		h.logger.Error("failed to delete status", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to delete status")
	}

	// Return the deleted status in Mastodon format
	mastodonStatus, err := h.convertStorageStatusToAPI(status, claims.Username)
	if err != nil {
		h.logger.Error("failed to convert deleted status", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to delete status")
	}

	h.logger.Info("deleted status", zap.String("id", statusID))

	h.recordAgentAuditEvent(ctx, claims, "agent.status.delete", statusID, map[string]any{
		"status_id": statusID,
	})

	if h.repos != nil && h.repos.User() != nil && status != nil && status.AuthorUsername != "" {
		if author, _ := h.repos.User().GetUser(ctx.Context(), status.AuthorUsername); author != nil && author.IsAgent {
			h.recordAgentMemoryTombstone(ctx.Context(), status.AuthorUsername, statusID, "")
		}
	}

	return okJSON(mastodonStatus)
}

// HandleUpdateStatusLift updates an existing status
func (h *Handler) HandleUpdateStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Validate status ID
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Authenticate and authorize user
	claims, actor, resp, err := h.authenticateStatusUpdate(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Agents must not silently edit; use explicit corrections/retractions via new posts.
	if claims != nil && claims.IsAgent {
		h.recordAgentAuditEvent(ctx, claims, "agent.status.update.blocked", statusID, map[string]any{
			"reason": "silent_edit_disallowed",
		})
		return apptheory.JSON(http.StatusForbidden, map[string]any{
			"error":             "agent_edit_disallowed",
			"error_description": "agents must not silently edit posts; publish a correction or retraction as a new status",
		})
	}

	// Get and verify object ownership
	objectID := h.normalizeStatusIDForUpdate(statusID)
	object, resp, err := h.getAndVerifyStatusOwnership(ctx, objectID, actor.ID)
	if resp != nil || err != nil {
		return resp, err
	}

	// Parse update request
	req, resp, err := h.parseUpdateStatusRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Convert object to Note and verify ownership
	note, resp, err := h.convertObjectToNoteWithOwnershipCheck(ctx, object, actor.ID)
	if resp != nil || err != nil {
		return resp, err
	}

	// Apply updates to note
	h.applyStatusUpdates(note, req)

	// Save updated note
	resp, err = h.saveUpdatedStatus(ctx, note)
	if resp != nil || err != nil {
		return resp, err
	}

	// Create and deliver update activity
	resp, err = h.createStatusUpdateActivity(ctx, note, actor)
	if resp != nil || err != nil {
		return resp, err
	}

	// Build and return response
	return h.buildUpdateStatusResponse(ctx, note, actor, req)
}

// authenticateStatusUpdate handles authentication for status updates
func (h *Handler) authenticateStatusUpdate(ctx *apptheory.Context) (*auth.Claims, *activitypub.Actor, *apptheory.Response, error) {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		resp, respErr := common.RespondUnauthorized(ctx, "missing token")
		return nil, nil, resp, respErr
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		resp, respErr := common.RespondUnauthorized(ctx, err.Error())
		return nil, nil, resp, respErr
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		resp, respErr := common.RespondInsufficientScope(ctx)
		return nil, nil, resp, respErr
	}

	// Get the user's actor
	account, err := h.registry.Accounts().GetAccount(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "Internal server error")
		return nil, nil, resp, respErr
	}
	actor := account.Actor

	return claims, actor, nil, nil
}

// normalizeStatusIDForUpdate normalizes a status ID to a full URL
func (h *Handler) normalizeStatusIDForUpdate(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// getAndVerifyStatusOwnership retrieves the object and initial ownership check
func (h *Handler) getAndVerifyStatusOwnership(ctx *apptheory.Context, objectID, _ string) (any, *apptheory.Response, error) {
	object, err := h.registry.Notes().GetNote(ctx.Context(), objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context(), objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context(), objectID); tErr == nil {
				resp, respErr := apptheory.JSON(http.StatusGone, map[string]interface{}{
					"error":             "status deleted",
					"error_description": "This status has been deleted",
					"deleted_at":        tombstone.Deleted.Format(time.RFC3339),
					"former_type":       tombstone.FormerType,
				})
				return nil, resp, respErr
			}
			// Fallback if we can't get tombstone details
			resp, respErr := apptheory.JSON(http.StatusGone, map[string]string{
				"error":             "status deleted",
				"error_description": "This status has been deleted",
			})
			return nil, resp, respErr
		}
		// Regular 404 for genuinely missing objects
		resp, respErr := common.RespondNotFound(ctx, "status not found")
		return nil, resp, respErr
	}
	return object, nil, nil
}

// parseUpdateStatusRequest parses the update status request
func (h *Handler) parseUpdateStatusRequest(ctx *apptheory.Context) (*models.UpdateStatusRequest, *apptheory.Response, error) {
	var req models.UpdateStatusRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request format")
		return nil, resp, respErr
	}
	return &req, nil, nil
}

// convertObjectToNoteWithOwnershipCheck converts object to Note and verifies ownership
func (h *Handler) convertObjectToNoteWithOwnershipCheck(ctx *apptheory.Context, object any, actorID string) (*activitypub.Note, *apptheory.Response, error) {
	switch obj := object.(type) {
	case *activitypub.Note:
		if obj.AttributedTo != actorID {
			resp, respErr := common.RespondForbidden(ctx, "you can only update your own statuses")
			return nil, resp, respErr
		}
		return obj, nil, nil

	case *storageMods.Status:
		if obj.AuthorID != "" && obj.AuthorID != actorID {
			resp, respErr := common.RespondForbidden(ctx, "you can only update your own statuses")
			return nil, resp, respErr
		}
		if obj.AuthorID == "" && obj.AuthorUsername != "" && transformations.ExtractUsernameFromActorID(actorID) != obj.AuthorUsername {
			resp, respErr := common.RespondForbidden(ctx, "you can only update your own statuses")
			return nil, resp, respErr
		}
		if obj.Note == nil {
			h.logger.Error("status missing ActivityPub note", zap.String("status_id", obj.StatusID))
			resp, respErr := common.RespondInternalServerError(ctx, "failed to load status")
			return nil, resp, respErr
		}
		note := obj.Note
		if note.AttributedTo != "" && note.AttributedTo != actorID {
			resp, respErr := common.RespondForbidden(ctx, "you can only update your own statuses")
			return nil, resp, respErr
		}
		return note, nil, nil

	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok && attr != actorID {
			resp, respErr := common.RespondForbidden(ctx, "you can only update your own statuses")
			return nil, resp, respErr
		}
		return h.convertMapToNote(ctx, obj)

	default:
		return h.convertUnknownObjectToNote(ctx, object, actorID)
	}
}

// convertMapToNote converts a map to a Note
func (h *Handler) convertMapToNote(ctx *apptheory.Context, obj map[string]any) (*activitypub.Note, *apptheory.Response, error) {
	noteBytes, err := json.Marshal(obj)
	if err != nil {
		h.logger.Error("failed to marshal object to JSON", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "Internal server error")
		return nil, resp, respErr
	}

	note := &activitypub.Note{}
	if err := json.Unmarshal(noteBytes, note); err != nil {
		h.logger.Error("failed to unmarshal JSON to Note", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "Internal server error")
		return nil, resp, respErr
	}

	return note, nil, nil
}

// convertUnknownObjectToNote handles conversion of unknown object types to Note
func (h *Handler) convertUnknownObjectToNote(ctx *apptheory.Context, object any, actorID string) (*activitypub.Note, *apptheory.Response, error) {
	// Try to handle any object with AttributedTo field using reflection
	v := reflect.ValueOf(object)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	var attributedTo string
	if v.Kind() == reflect.Struct {
		// Try to get AttributedTo field
		if attrField := v.FieldByName("AttributedTo"); attrField.IsValid() && attrField.Kind() == reflect.String {
			attributedTo = attrField.String()
		}
	}

	if err := common.ValidateRequiredParam("attributedTo", attributedTo); err != nil {
		h.logger.Error("unexpected object type or missing AttributedTo",
			zap.String("type", fmt.Sprintf("%T", object)),
			zap.Any("object", object))
		resp, respErr := common.RespondInternalServerError(ctx, "unexpected object type")
		return nil, resp, respErr
	}

	if attributedTo != actorID {
		resp, respErr := common.RespondForbidden(ctx, "you can only update your own statuses")
		return nil, resp, respErr
	}

	// Convert to Note via JSON marshaling
	noteBytes, _ := json.Marshal(object)
	note := &activitypub.Note{}
	if err := json.Unmarshal(noteBytes, note); err != nil {
		resp, respErr := common.RespondInternalServerError(ctx, "failed to convert object to Note")
		return nil, resp, respErr
	}

	return note, nil, nil
}

// applyStatusUpdates applies the requested updates to the note
func (h *Handler) applyStatusUpdates(note *activitypub.Note, req *models.UpdateStatusRequest) {
	if req.Status != "" {
		note.Content = req.Status
	}
	if req.SpoilerText != "" {
		note.Summary = req.SpoilerText
	}
	note.Sensitive = req.Sensitive

	// Update timestamp
	now := time.Now()
	note.Updated = &now
}

// saveUpdatedStatus saves the updated status to storage with edit history
func (h *Handler) saveUpdatedStatus(ctx *apptheory.Context, note *activitypub.Note) (*apptheory.Response, error) {
	// Extract the username from the authentication token
	username := h.extractUsernameFromToken(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return common.RespondInternalServerError(ctx, "failed to extract username for edit tracking")
	}

	// Build actor ID for edit tracking
	actorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)

	// Update object with history tracking using the new method
	if err := h.repos.Object().UpdateObjectWithHistory(ctx.Context(), note, actorID); err != nil {
		h.logger.Error("failed to update object with history", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}
	return nil, nil
}

// createStatusUpdateActivity creates and stores the update activity
func (h *Handler) createStatusUpdateActivity(ctx *apptheory.Context, note *activitypub.Note, actor *activitypub.Actor) (*apptheory.Response, error) {
	now := time.Now()
	updateActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.UpdateType,
			ID:        fmt.Sprintf("%s/activities/update-%d-%s", actor.ID, now.Unix(), generateRandomStringLift()),
			To:        note.To,
			CC:        note.CC,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: note,
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.repos.Activity().CreateActivity(ctx.Context(), updateActivity); err != nil {
		h.logger.Error("failed to create update activity", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Federation: Deliver Update activity to relevant recipients
	go func() {
		if err := h.deliverUpdateActivity(context.Background(), updateActivity, actor, note); err != nil {
			h.logger.Error("failed to deliver update activity for federation",
				zap.String("activity_id", updateActivity.ID),
				zap.Error(err))
		}
	}()

	h.logger.Info("created update activity with federation",
		zap.String("activity_id", updateActivity.ID),
		zap.String("note_id", note.ID),
		zap.String("actor", actor.ID))

	return nil, nil
}

// buildUpdateStatusResponse builds the response for the updated status
func (h *Handler) buildUpdateStatusResponse(ctx *apptheory.Context, note *activitypub.Note, actor *activitypub.Actor, req *models.UpdateStatusRequest) (*apptheory.Response, error) {
	resp := transformations.ObjectToStatusAny(note, actor, h.cfg.BaseURL())
	if req.Visibility != "" {
		resp.Visibility = req.Visibility
	}
	if req.Language != "" {
		resp.Language = req.Language
	}

	return okJSON(resp)
}

// HandleGetStatusLift retrieves a status by ID using the Notes service
func (h *Handler) HandleGetStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Optional authentication (public statuses can be viewed without auth)
	var viewerUsername string
	token := h.getBearerTokenLift(ctx)
	if token != "" {
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			viewerUsername = claims.Username
		}
	}

	// Get status using Notes service
	status, err := h.registry.Notes().GetNote(ctx.Context(), statusID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "status not found")
		}
		h.logger.Error("failed to get status", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to retrieve status")
	}

	// Get the author actor for proper conversion
	account, err := h.registry.Accounts().GetAccount(ctx.Context(), transformations.ExtractUsernameFromActorID(status.AuthorID))
	if err != nil {
		h.logger.Error("failed to get author account", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to retrieve status")
	}

	// Check user-specific metadata if viewer is authenticated
	var favorited, reblogged, bookmarked bool
	if viewerUsername != "" {
		// Check if the viewer has liked/favorited this status
		if likeRepo := h.repos.Like(); likeRepo != nil {
			favorited, _ = likeRepo.HasLiked(ctx.Context(), viewerUsername, statusID)
		}

		// Check if the viewer has reblogged this status
		if likeRepo := h.repos.Like(); likeRepo != nil {
			reblogged, _ = likeRepo.HasReblogged(ctx.Context(), viewerUsername, statusID)
		}

		// Check if the viewer has bookmarked this status
		if userRepo := h.repos.User(); userRepo != nil {
			bookmarked, _ = userRepo.IsBookmarked(ctx.Context(), viewerUsername, statusID)
		}
	}

	// Convert to Mastodon API format using correct converter with user context
	mastodonStatus := transformations.ObjectToStatusWithContextAndCounts(ctx.Context(), status, account.Actor, 0, 0, favorited, reblogged, bookmarked, h.cfg.BaseURL())

	return okJSON(mastodonStatus)
}

// HandleGetHomeTimelineLift returns the home timeline using the Notes service
func (h *Handler) HandleGetHomeTimelineLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate with read scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse pagination parameters
	limit, _ := common.ParseStatusTimelineLimit(queryValue(ctx, "limit"))

	cursor := queryValue(ctx, "max_id")
	excludeAgents := strings.EqualFold(strings.TrimSpace(queryValue(ctx, "exclude_agents")), boolTrue)

	// Get home timeline using Notes service
	result, err := h.registry.Notes().ListNotes(ctx.Context(), &notes.ListNotesQuery{
		ViewerID:     claims.Username,
		TimelineType: "home",
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		h.logger.Error("failed to get home timeline", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to retrieve timeline")
	}

	// Convert to Mastodon API format
	timeline := make([]*models.Status, 0, len(result.Notes))
	for _, status := range result.Notes {
		apiStatus, err := h.convertStorageStatusToAPI(status, claims.Username)
		if err != nil {
			h.logger.Warn("failed to convert home timeline status",
				zap.String("status_id", status.StatusID),
				zap.Error(err))
			continue
		}
		if excludeAgents && apiStatus.Account.Bot {
			continue
		}
		timeline = append(timeline, apiStatus)
	}

	return okJSON(timeline)
}

// HandleGetPublicTimelineLift returns the public timeline using the Notes service
func (h *Handler) HandleGetPublicTimelineLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Optional authentication (public timeline can be viewed without auth)
	var viewerUsername string
	token := h.getBearerTokenLift(ctx)
	if token != "" {
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			viewerUsername = claims.Username
		}
	}

	// Parse pagination parameters
	limit, _ := common.ParseStatusTimelineLimit(queryValue(ctx, "limit"))

	cursor := queryValue(ctx, "max_id")
	local := queryValue(ctx, "local") == "true"
	excludeAgents := strings.EqualFold(strings.TrimSpace(queryValue(ctx, "exclude_agents")), boolTrue)

	timelineType := VisibilityPublic
	if local {
		timelineType = "local"
	}

	// Get public timeline using Notes service
	result, err := h.registry.Notes().ListNotes(ctx.Context(), &notes.ListNotesQuery{
		ViewerID:     viewerUsername,
		TimelineType: timelineType,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		h.logger.Error("failed to get public timeline", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to retrieve timeline")
	}

	// Convert to Mastodon API format
	timeline := make([]*models.Status, 0, len(result.Notes))
	for _, status := range result.Notes {
		apiStatus, err := h.convertStorageStatusToAPI(status, viewerUsername)
		if err != nil {
			h.logger.Warn("failed to convert public timeline status",
				zap.String("status_id", status.StatusID),
				zap.Error(err))
			continue
		}
		if excludeAgents && apiStatus.Account.Bot {
			continue
		}
		if apiStatus.Account.Bot && h.shouldHideRemoteAgentActor(ctx.Context(), status.AuthorID) {
			continue
		}
		timeline = append(timeline, apiStatus)
	}

	return okJSON(timeline)
}

// HandleGetStatusContextLift retrieves the context (ancestors and descendants) of a status
func (h *Handler) HandleGetStatusContextLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Validate and normalize status ID
	objectID, resp, err := h.validateStatusIDForContext(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Optional auth: agent tokens receive stricter context shaping.
	isAgent := false
	if token := h.getBearerTokenLift(ctx); token != "" {
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil && claims != nil && claims.IsAgent {
			isAgent = true
		}
	}

	// Get ancestors and descendants
	ancestors := h.getStatusAncestors(ctx.Context(), objectID, 200)
	if isAgent && len(ancestors) > 20 {
		// Root + last 20 to cap agent context expansion.
		ancestors = append([]models.Status{ancestors[0]}, ancestors[len(ancestors)-20:]...)
	}
	descendants := h.getStatusDescendants(ctx.Context(), objectID)

	// Return context response
	payload := models.StatusContext{
		Ancestors:   ancestors,
		Descendants: descendants,
	}

	return okJSON(payload)
}

// validateStatusIDForContext validates the status ID and checks it exists
func (h *Handler) validateStatusIDForContext(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, err.Error())
		return "", resp, respErr
	}

	// Normalize and validate the status ID
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to check it exists
	_, err := h.registry.Notes().GetNote(ctx.Context(), objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context(), objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context(), objectID); tErr == nil {
				resp, respErr := apptheory.JSON(http.StatusGone, map[string]interface{}{
					"error":             "status deleted",
					"error_description": "This status has been deleted and its context is no longer available",
					"deleted_at":        tombstone.Deleted.Format(time.RFC3339),
					"former_type":       tombstone.FormerType,
				})
				return "", resp, respErr
			}
			// Fallback if we can't get tombstone details
			resp, respErr := apptheory.JSON(http.StatusGone, map[string]string{
				"error":             "status deleted",
				"error_description": "This status has been deleted and its context is no longer available",
			})
			return "", resp, respErr
		}
		// Regular 404 for genuinely missing objects
		resp, respErr := common.RespondNotFound(ctx, "status not found")
		return "", resp, respErr
	}

	return objectID, nil, nil
}

// getStatusAncestors retrieves the ancestors (parent statuses) of a status
func (h *Handler) getStatusAncestors(ctx context.Context, objectID string, maxDepth int) []models.Status {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	parentIDs := make([]string, 0, maxDepth)
	currentID := objectID

	for i := 0; i < maxDepth; i++ {
		parentID := h.getParentStatusID(ctx, currentID)
		if err := common.ValidateRequiredParam("parentID", parentID); err != nil {
			break
		}
		parentIDs = append(parentIDs, parentID)
		currentID = parentID
	}

	ancestors := make([]models.Status, 0, len(parentIDs))
	for i := len(parentIDs) - 1; i >= 0; i-- {
		parentStatus := h.loadStatusWithActor(ctx, parentIDs[i])
		if parentStatus == nil {
			continue
		}
		ancestors = append(ancestors, *parentStatus)
	}

	return ancestors
}

// getParentStatusID gets the parent status ID from an object
func (h *Handler) getParentStatusID(ctx context.Context, objectID string) string {
	obj, err := h.registry.Notes().GetNote(ctx, objectID)
	if err != nil {
		return ""
	}

	inReplyTo := h.extractInReplyTo(obj)
	if err := common.ValidateRequiredParam("in_reply_to", inReplyTo); err != nil {
		return ""
	}

	// Verify parent exists
	_, err = h.registry.Notes().GetNote(ctx, inReplyTo)
	if err != nil {
		return ""
	}

	return inReplyTo
}

// extractInReplyTo extracts the InReplyTo field from various object types
func (h *Handler) extractInReplyTo(obj interface{}) string {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.InReplyTo
	case *storageMods.Status:
		if o.Note != nil {
			return o.Note.InReplyTo
		}
		if o.InReplyToID != "" {
			return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), o.InReplyToID)
		}
	case map[string]any:
		if reply, ok := o["inReplyTo"].(string); ok {
			return reply
		}
	default:
		return h.extractInReplyToViaReflection(obj)
	}
	return ""
}

// extractInReplyToViaReflection uses reflection to extract InReplyTo field
func (h *Handler) extractInReplyToViaReflection(obj interface{}) string {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	replyField := v.FieldByName("InReplyTo")
	if !replyField.IsValid() {
		return ""
	}

	// Handle pointer to string
	if replyField.Kind() == reflect.Ptr && !replyField.IsNil() {
		if replyField.Elem().Kind() == reflect.String {
			return replyField.Elem().String()
		}
	} else if replyField.Kind() == reflect.String {
		return replyField.String()
	}

	return ""
}

// loadStatusWithActor loads a status object and its associated actor
func (h *Handler) loadStatusWithActor(ctx context.Context, objectID string) *models.Status {
	obj, err := h.registry.Notes().GetNote(ctx, objectID)
	if err != nil {
		return nil
	}

	actor := h.getActorForObject(ctx, obj)
	status := transformations.ObjectToStatusAny(obj, actor, h.cfg.BaseURL())
	return &status
}

// getActorForObject retrieves the actor associated with an object
func (h *Handler) getActorForObject(ctx context.Context, obj interface{}) *activitypub.Actor {
	attributedTo := h.extractAttributedTo(obj)
	if err := common.ValidateRequiredParam("attributed_to", attributedTo); err != nil {
		return nil
	}

	username := transformations.ExtractUsernameFromActorID(attributedTo)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil
	}

	account, err := h.registry.Accounts().GetAccount(ctx, username)
	if err != nil {
		return nil
	}
	return account.Actor
}

// getStatusDescendants retrieves the descendants (replies) of a status
func (h *Handler) getStatusDescendants(ctx context.Context, objectID string) []models.Status {
	descendants := []models.Status{}

	// Fetch replies to this status
	replies, _, err := h.repos.Object().GetReplies(ctx, objectID, 100, "") // Get up to 100 replies
	if err != nil {
		h.logger.Warn("failed to get replies for context",
			zap.String("object_id", objectID),
			zap.Error(err))
		return descendants
	}

	for _, reply := range replies {
		if status := h.convertReplyToStatus(ctx, reply); status != nil {
			descendants = append(descendants, *status)
		}
	}

	h.logger.Debug("fetched descendants for context",
		zap.String("object_id", objectID),
		zap.Int("count", len(descendants)))

	return descendants
}

// convertReplyToStatus converts a reply object to a status
func (h *Handler) convertReplyToStatus(ctx context.Context, reply interface{}) *models.Status {
	actor := h.getActorForObject(ctx, reply)
	status := transformations.ObjectToStatusAny(reply, actor, h.cfg.BaseURL())
	return &status
}

// HandleGetAccountStatusesLift retrieves statuses for a specific account
// accountStatusesParams holds the parameters for account statuses query
type accountStatusesParams struct {
	limit          int
	maxID          string
	onlyMedia      bool
	excludeReplies bool
	excludeReblogs bool
	tagged         string
}

// HandleGetAccountStatusesLift handles requests to get an account's statuses
func (h *Handler) HandleGetAccountStatusesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Validate and get account ID
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Resolve account ID to actor
	actor, err := h.resolveAccountID(ctx.Context(), accountID)
	if err != nil {
		return common.RespondNotFound(ctx, "account not found")
	}

	// Parse query parameters
	params := h.parseAccountStatusesParams(ctx)

	// Get objects by this actor
	userTimeline, err := h.registry.Notes().GetUserTimeline(ctx.Context(), actor.ID, interfaces.PaginationOptions{
		Limit:  params.limit,
		Cursor: params.maxID,
	})
	if err != nil {
		h.logger.Error("failed to get objects by actor", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}
	statusItems := userTimeline.Items
	cursor := userTimeline.NextCursor

	// Convert status models to interface{} for compatibility
	objects := make([]any, len(statusItems))
	for i, s := range statusItems {
		objects[i] = s
	}

	// Convert and filter objects to statuses
	statuses := h.convertAndFilterObjects(ctx, objects, actor, params)

	resp, err := okJSON(statuses)
	if err != nil {
		return nil, err
	}
	if cursor != "" {
		h.setPaginationHeader(resp, accountID, cursor, params)
	}
	return resp, nil
}

// parseAccountStatusesParams parses query parameters for account statuses
func (h *Handler) parseAccountStatusesParams(ctx *apptheory.Context) accountStatusesParams {
	// Parse limit parameter
	limit, _ := common.ParseAccountStatusesLimit(queryValue(ctx, "limit"))

	params := accountStatusesParams{
		limit:          limit,
		maxID:          queryValue(ctx, "max_id"),
		onlyMedia:      queryValue(ctx, "only_media") == boolTrue,
		excludeReplies: queryValue(ctx, "exclude_replies") == boolTrue,
		excludeReblogs: queryValue(ctx, "exclude_reblogs") == boolTrue,
		tagged:         queryValue(ctx, "tagged"),
	}

	return params
}

// convertAndFilterObjects converts objects to statuses with filtering
func (h *Handler) convertAndFilterObjects(_ *apptheory.Context, objects []any, actor *activitypub.Actor, params accountStatusesParams) []models.Status {
	statuses := []models.Status{}

	h.logger.Debug("converting objects to statuses",
		zap.Int("object_count", len(objects)),
		zap.String("actor_id", actor.ID))

	for _, obj := range objects {
		// Apply filters
		if h.shouldFilterObject(obj, params) {
			continue
		}

		status := transformations.ObjectToStatusAny(obj, actor, h.cfg.BaseURL())
		h.logStatusConversion(status, obj)
		statuses = append(statuses, status)
	}

	return statuses
}

// shouldFilterObject checks if an object should be filtered out
func (h *Handler) shouldFilterObject(obj any, params accountStatusesParams) bool {
	if params.onlyMedia && !h.objectHasMedia(obj) {
		return true
	}

	if params.excludeReplies && h.objectIsReply(obj) {
		return true
	}

	if params.excludeReblogs && h.objectIsReblog(obj) {
		return true
	}

	if params.tagged != "" && !h.objectHasHashtags(obj, params.tagged) {
		return true
	}

	return false
}

// objectHasMedia checks if an object has media attachments
func (h *Handler) objectHasMedia(obj any) bool {
	switch o := obj.(type) {
	case *activitypub.Note:
		return len(o.Attachment) > 0
	case map[string]any:
		if attachments, ok := o["attachment"].([]any); ok {
			return len(attachments) > 0
		}
	}
	return false
}

// objectIsReply checks if an object is a reply
func (h *Handler) objectIsReply(obj any) bool {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.InReplyTo != ""
	case map[string]any:
		if inReplyTo, ok := o["inReplyTo"].(string); ok {
			return inReplyTo != ""
		}
	}
	return false
}

// objectIsReblog checks if an object is a reblog/boost/announce activity
func (h *Handler) objectIsReblog(obj any) bool {
	switch o := obj.(type) {
	case *models.Status:
		// API models.Status has a Reblog field
		return o.Reblog != nil
	case *storageMods.Status:
		// Storage models.Status has IsReblog() method
		return o.IsReblog()
	case *activitypub.Note:
		// For ActivityPub Notes, check if this is an Announce activity
		// or if it has a reblogOfID-like field
		return false // Pure Note objects are not reblogs
	case *activitypub.Activity:
		return o.Type == activitypub.AnnounceType
	case map[string]any:
		// Check for reblog_of_id field in map representation
		if reblogOfID, ok := o["reblog_of_id"].(string); ok {
			return reblogOfID != ""
		}
		// Check if this is an Announce activity
		if actType, ok := o["type"].(string); ok {
			return actType == "Announce"
		}
	}
	return false
}

// objectHasHashtags checks if an object contains the specified hashtags
func (h *Handler) objectHasHashtags(obj any, taggedParam string) bool {
	if err := common.ValidateRequiredParam("tagged_param", taggedParam); err != nil {
		return true // No filter specified, so all objects match
	}

	requiredTags := h.parseRequiredTags(taggedParam)
	objectTags := h.extractHashtagsFromObject(obj)

	return h.containsAllRequiredTags(objectTags, requiredTags)
}

// parseRequiredTags parses comma-separated hashtags and normalizes them
func (h *Handler) parseRequiredTags(taggedParam string) []string {
	requiredTags := strings.Split(taggedParam, ",")
	for i, tag := range requiredTags {
		requiredTags[i] = strings.ToLower(common.SanitizeInput(tag))
	}
	return requiredTags
}

// extractHashtagsFromObject extracts hashtags from different object types
func (h *Handler) extractHashtagsFromObject(obj any) []string {
	switch o := obj.(type) {
	case *models.Status:
		return h.extractFromAPIStatus(o)
	case *storageMods.Status:
		return h.normalizeHashtags(o.Hashtags)
	case *activitypub.Note:
		return h.extractFromActivityPubNote(o)
	case map[string]any:
		return h.extractFromGenericMap(o)
	default:
		return []string{}
	}
}

// extractFromAPIStatus extracts hashtags from API models.Status
func (h *Handler) extractFromAPIStatus(status *models.Status) []string {
	var tags []string
	for _, tagInterface := range status.Tags {
		if tagMap, ok := tagInterface.(map[string]any); ok {
			if tagName, hasName := tagMap["name"].(string); hasName {
				name := strings.TrimPrefix(tagName, "#")
				tags = append(tags, strings.ToLower(name))
			}
		}
	}
	return tags
}

// extractFromActivityPubNote extracts hashtags from ActivityPub Note
func (h *Handler) extractFromActivityPubNote(note *activitypub.Note) []string {
	var tags []string
	for _, tag := range note.Tag {
		if tag.Type == "Hashtag" && tag.Name != "" {
			name := strings.TrimPrefix(tag.Name, "#")
			tags = append(tags, strings.ToLower(name))
		}
	}
	return tags
}

// extractFromGenericMap extracts hashtags from generic map[string]any
func (h *Handler) extractFromGenericMap(objMap map[string]any) []string {
	if hashtags, ok := objMap["hashtags"].([]string); ok {
		return h.normalizeHashtags(hashtags)
	}

	if tags, ok := objMap["tag"].([]any); ok {
		return h.extractFromTagArray(tags)
	}

	return []string{}
}

// extractFromTagArray extracts hashtags from tag array
func (h *Handler) extractFromTagArray(tags []any) []string {
	var hashtags []string
	for _, tagInterface := range tags {
		if tagMap, ok := tagInterface.(map[string]any); ok {
			if h.isHashtagType(tagMap) {
				if tagName, hasName := tagMap["name"].(string); hasName {
					name := strings.TrimPrefix(tagName, "#")
					hashtags = append(hashtags, strings.ToLower(name))
				}
			}
		}
	}
	return hashtags
}

// isHashtagType checks if a tag map represents a hashtag
func (h *Handler) isHashtagType(tagMap map[string]any) bool {
	tagType, hasType := tagMap["type"].(string)
	return hasType && tagType == "Hashtag"
}

// normalizeHashtags converts all hashtags to lowercase
func (h *Handler) normalizeHashtags(tags []string) []string {
	normalized := make([]string, len(tags))
	for i, tag := range tags {
		normalized[i] = strings.ToLower(tag)
	}
	return normalized
}

// containsAllRequiredTags checks if object contains all required tags
func (h *Handler) containsAllRequiredTags(objectTags, requiredTags []string) bool {
	for _, requiredTag := range requiredTags {
		if !h.containsTag(objectTags, requiredTag) {
			return false
		}
	}
	return true
}

// containsTag checks if a tag exists in the tag list
func (h *Handler) containsTag(tags []string, target string) bool {
	for _, tag := range tags {
		if tag == target {
			return true
		}
	}
	return false
}

// logStatusConversion logs debug information about status conversion
func (h *Handler) logStatusConversion(status models.Status, obj any) {
	h.logger.Debug("converted status from object",
		zap.String("status_id", status.ID),
		zap.String("status_content", status.Content),
		zap.String("status_created_at", status.CreatedAt),
		zap.Any("object_type", fmt.Sprintf("%T", obj)))
}

// setPaginationHeader sets the Link header for pagination
func (h *Handler) setPaginationHeader(resp *apptheory.Response, accountID, cursor string, params accountStatusesParams) {
	nextURL := h.buildPaginationURL(accountID, cursor, params)
	setHeader(resp, "link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
}

// buildPaginationURL builds the next page URL
func (h *Handler) buildPaginationURL(accountID, cursor string, params accountStatusesParams) string {
	nextURL := fmt.Sprintf("%s/api/v1/accounts/%s/statuses?max_id=%s", h.cfg.BaseURL(), accountID, cursor)

	if params.limit != 20 {
		nextURL += fmt.Sprintf("&limit=%d", params.limit)
	}
	if params.onlyMedia {
		nextURL += "&only_media=true"
	}
	if params.excludeReplies {
		nextURL += "&exclude_replies=true"
	}
	if params.excludeReblogs {
		nextURL += "&exclude_reblogs=true"
	}
	if params.tagged != "" {
		nextURL += fmt.Sprintf("&tagged=%s", params.tagged)
	}

	return nextURL
}

// Helper functions

// getStringFromMap safely gets a string from a map[string]any
func getStringFromMap(m map[string]any, key, defaultValue string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

// extractLinksFromContent extracts links from a given content

// extractMentions extracts @mentions from content and returns actor URIs

// validateAndNormalizeStatusID validates and normalizes the status ID parameter

// extractAuthorUsernameForStatus extracts the username from a status object

// getStatusActor retrieves the actor associated with a status object

// enrichStatusWithEmojis parses and adds emoji data to a status

// enrichStatusWithInteractionCounts adds like and reblog counts to a status

// enrichStatusWithPoll adds poll data to a status if it exists

// extractUsernameFromToken extracts the username from authentication token
func (h *Handler) extractUsernameFromToken(ctx *apptheory.Context) string {
	// Get authorization header
	authHeader := headerValue(ctx, "Authorization")
	if err := common.ValidateRequiredParam("auth_header", authHeader); err != nil {
		authHeader = headerValue(ctx, "authorization")
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			// Validate token
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				return claims.Username
			}
		}
	}

	return ""
}

// enrichStatusWithUserInteractions adds user-specific interaction state to a status

// performLocalCascadeDeletion performs cascade deletion operations for local status deletion

// cascadeDeleteLikes removes all likes for the deleted object

// cascadeDeleteAnnounces removes all announces/boosts for the deleted object

// cascadeDeleteFromCollections removes the object from collections

// cascadeDeleteBookmarks removes all bookmarks for the deleted object

// cascadeDeletePolls removes polls associated with the deleted status

// deliverDeleteActivity delivers a Delete activity to relevant recipients for federation

// determineDeleteDeliveryRecipients determines who should receive the Delete activity

// deliverDeleteToRecipient delivers the Delete activity to a specific recipient

// extractMentionsFromObject extracts mentioned user IDs from an object

// extractToAndCCFromObject extracts To and CC fields from an object

// const tagTypeMention = "Mention" // unused constant

// parseMentionsFromTags parses mentions from various tag formats

// parseMentionsFromAnySlice extracts mentions from a slice of any type

// extractMentionFromInterface extracts a mention URL from an interface{} if it's a mention tag

// parseMentionsFromTagSlice extracts mentions from a slice of ActivityPub tags

// parseMentionsFromString extracts mentions from a JSON string of tags

// deliverUpdateActivity delivers an Update activity to relevant recipients for federation
func (h *Handler) deliverUpdateActivity(ctx context.Context, updateActivity *activitypub.Activity, actor *activitypub.Actor, note *activitypub.Note) error {
	h.logger.Info("delivering update activity for federation",
		zap.String("activity_id", updateActivity.ID),
		zap.String("actor", actor.ID))

	// Determine delivery recipients based on the updated note
	recipients, err := h.determineUpdateDeliveryRecipients(ctx, actor, note)
	if err != nil {
		h.logger.Error("failed to determine delivery recipients",
			zap.String("activity_id", updateActivity.ID),
			zap.String("actor_id", actor.ID),
			zap.Error(err))
		return errors.Join(failedToDetermineDeliveryRecipients(), err)
	}

	if err := common.ValidateSliceNotEmpty("recipients", recipients); err != nil {
		h.logger.Info("no recipients for update activity delivery", zap.String("activity_id", updateActivity.ID))
		return nil
	}

	// Deliver to each recipient
	deliveredCount := 0
	failedCount := 0

	for _, recipient := range recipients {
		if err := h.deliverUpdateToRecipient(ctx, updateActivity, actor, recipient); err != nil {
			h.logger.Warn("failed to deliver update activity to recipient",
				zap.String("recipient", recipient),
				zap.String("activity_id", updateActivity.ID),
				zap.Error(err))
			failedCount++
		} else {
			deliveredCount++
		}
	}

	h.logger.Info("completed update activity delivery",
		zap.String("activity_id", updateActivity.ID),
		zap.Int("delivered", deliveredCount),
		zap.Int("failed", failedCount),
		zap.Int("total_recipients", len(recipients)))

	return nil
}

// determineUpdateDeliveryRecipients determines who should receive the Update activity
func (h *Handler) determineUpdateDeliveryRecipients(ctx context.Context, actor *activitypub.Actor, note *activitypub.Note) ([]string, error) {
	recipients := make(map[string]bool)

	// Add followers to recipients (they should be notified of edits)
	followers, _, err := h.repos.Relationship().GetFollowers(ctx, actor.PreferredUsername, 1000, "")
	if err != nil {
		h.logger.Warn("failed to get followers for update delivery", zap.Error(err))
	} else {
		for _, follower := range followers {
			if strings.HasPrefix(follower, "https://") || strings.HasPrefix(follower, "http://") {
				recipients[follower] = true
				continue
			}

			// Stored relationship IDs are typically usernames or federated handles; normalize to actor IDs.
			if strings.Contains(follower, "@") {
				parts := strings.SplitN(follower, "@", 2)
				if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
					recipients[fmt.Sprintf("https://%s/users/%s", parts[1], parts[0])] = true
					continue
				}
			}

			recipients[h.cfg.ActorURL(follower)] = true
		}
	}

	// Add mentioned users from the updated note
	for _, tag := range note.Tag {
		if tag.Type == "Mention" && tag.Href != "" {
			recipients[tag.Href] = true
		}
	}

	// Add users in To/CC fields
	for _, user := range note.To {
		if user != activitypub.PublicAddress {
			recipients[user] = true
		}
	}
	for _, user := range note.CC {
		if user != activitypub.PublicAddress {
			recipients[user] = true
		}
	}

	// Convert to slice
	recipientList := make([]string, 0, len(recipients))
	for recipient := range recipients {
		recipientList = append(recipientList, recipient)
	}

	return recipientList, nil
}

// deliverUpdateToRecipient delivers the Update activity to a specific recipient
func (h *Handler) deliverUpdateToRecipient(ctx context.Context, updateActivity *activitypub.Activity, actor *activitypub.Actor, recipientID string) error {
	h.logger.Debug("delivering update activity to recipient",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", updateActivity.ID))

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName(), h.logger)
	deliveryService := federation.NewDeliveryService(federationStorage, h.cfg)

	// Set recipient in activity To field for delivery
	updateActivity.To = []string{recipientID}

	// Deliver using federation service
	if err := deliveryService.DeliverToRecipients(ctx, updateActivity, actor); err != nil {
		h.logger.Error("failed to deliver update activity to recipient",
			zap.String("recipient_id", recipientID),
			zap.String("activity_id", updateActivity.ID),
			zap.Error(err))
		return err
	}

	h.logger.Info("update activity delivered successfully",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", updateActivity.ID))

	return nil
}

// deliverCreateActivity delivers a Create activity to relevant recipients for federation

// getRequestingActorID extracts the actor ID from authentication context

// canActorSeeStatus checks if an actor can view a status based on visibility rules

// isFollower checks if one actor follows another

// isMentioned checks if an actor is mentioned in a status

// extractUsernameFromActorID extracts username from an ActivityPub actor ID

// canActorSeeStatusEnhanced performs enhanced visibility checking using Status model methods

// sanitizeStatusForActor removes sensitive addressing information based on viewer's relationship

// convertToStorageStatus converts API models.Status to storage models.Status

// getDomainFromConfig returns the domain from environment configuration

// convertFromStorageStatus converts storage models.Status back to API models.Status

// CreateStatusRequest represents the enhanced request for creating a status with addressing fields
type CreateStatusRequest struct {
	Status        string   `json:"status"`
	Visibility    string   `json:"visibility"`
	InReplyToID   string   `json:"in_reply_to_id,omitempty"`
	Sensitive     bool     `json:"sensitive,omitempty"`
	SpoilerText   string   `json:"spoiler_text,omitempty"`
	Mentions      []string `json:"mentions,omitempty"`
	ToRecipients  []string `json:"to_recipients,omitempty"`  // Enhanced addressing
	CcRecipients  []string `json:"cc_recipients,omitempty"`  // Enhanced addressing
	BtoRecipients []string `json:"bto_recipients,omitempty"` // Enhanced addressing (hidden from others)
	BCCRecipients []string `json:"bcc_recipients,omitempty"` // Enhanced addressing (hidden from all)
}
