package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
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

	if resp, err := validateCreateStatusRequest(ctx, &req); resp != nil || err != nil {
		return resp, err
	}

	// Authenticate with write scope
	claims, authResp, authErr := h.authenticateCreateStatus(ctx)
	if authResp != nil || authErr != nil {
		return authResp, authErr
	}

	// Default visibility
	if req.Visibility == "" {
		req.Visibility = VisibilityPublic
	}

	agentAttribution, resp, prepErr := h.prepareAgentStatusCreate(ctx, claims, &req)
	if resp != nil || prepErr != nil {
		return resp, prepErr
	}

	// Create a scheduled status instead of publishing immediately.
	if req.ScheduledAt != nil {
		return h.handleCreateScheduledStatus(ctx, claims, &req)
	}

	if req.Visibility == VisibilityDirect {
		return h.handleCreateDirectStatus(ctx, claims, &req, agentAttribution)
	}

	createCmd := createNoteCommandFromStatusRequest(claims, &req, agentAttribution)

	// Call Notes service
	result, err := h.registry.Notes().CreateNote(ctx.Context(), createCmd)
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

func (h *Handler) handleCreateDirectStatus(ctx *apptheory.Context, claims *auth.Claims, req *models.CreateStatusRequest, agentAttribution *activitypub.AgentPostAttribution) (*apptheory.Response, error) {
	conversationsService := h.registry.Conversations()
	if conversationsService == nil {
		h.logger.Error("conversations service not available for direct status create")
		return common.RespondServiceUnavailable(ctx, "conversations service")
	}

	sendCmd, err := buildDirectMessageCommandFromStatusRequest(claims, req, agentAttribution)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	result, err := conversationsService.SendDirectMessage(ctx.Context(), sendCmd)
	if err != nil {
		h.logger.Error("failed to create direct message via conversations service", zap.Error(err))
		if _, ok := commonerrors.AsAppError(err); ok {
			return common.RespondValidationError(ctx, err)
		}
		return common.RespondInternalServerError(ctx, "failed to create status")
	}

	if claims.IsAgent {
		h.recordAgentMemoryEvent(ctx, claims.Username, result.Message.StatusID, req)
	}

	apiStatus, err := h.convertStorageStatusToAPI(result.Message, claims.Username)
	if err != nil {
		h.logger.Error("failed to convert created direct status", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to create status")
	}

	h.logger.Info("created direct status",
		zap.String("id", result.Message.StatusID),
		zap.String("content", req.Status))

	h.recordAgentAuditEvent(ctx, claims, "agent.status.create", result.Message.StatusID, map[string]any{
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

func validateCreateStatusRequest(ctx *apptheory.Context, req *models.CreateStatusRequest) (*apptheory.Response, error) {
	if req == nil {
		return common.RespondBadRequest(ctx, "invalid request")
	}

	// Validate status parameters using comprehensive validation
	statusParams := buildStatusValidationParams(req)
	if err := common.ValidateStatusParams(statusParams); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	return nil, nil
}

func buildStatusValidationParams(req *models.CreateStatusRequest) map[string]any {
	statusParams := map[string]any{
		"status":         req.Status,
		"visibility":     req.Visibility,
		"sensitive":      req.Sensitive,
		"spoiler_text":   req.SpoilerText,
		"language":       req.Language,
		"in_reply_to_id": req.InReplyToID,
	}

	if len(req.MediaIDs) > 0 {
		mediaIDs := make([]any, 0, len(req.MediaIDs))
		for _, id := range req.MediaIDs {
			mediaIDs = append(mediaIDs, id)
		}
		statusParams["media_ids"] = mediaIDs
	}

	if req.ScheduledAt != nil {
		statusParams["scheduled_at"] = *req.ScheduledAt
	}

	if req.Poll != nil {
		options := make([]any, 0, len(req.Poll.Options))
		for _, opt := range req.Poll.Options {
			options = append(options, opt)
		}
		statusParams["poll"] = map[string]any{
			"options":     options,
			"expires_in":  req.Poll.ExpiresIn,
			"multiple":    req.Poll.Multiple,
			"hide_totals": req.Poll.HideTotals,
		}
	}

	return statusParams
}

func (h *Handler) authenticateCreateStatus(ctx *apptheory.Context) (*auth.Claims, *apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return nil, resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx)
		return nil, resp, respErr
	}
	return claims, nil, nil
}

func (h *Handler) prepareAgentStatusCreate(ctx *apptheory.Context, claims *auth.Claims, req *models.CreateStatusRequest) (*activitypub.AgentPostAttribution, *apptheory.Response, error) {
	if claims == nil || !claims.IsAgent {
		return nil, nil, nil
	}

	if resp, normErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req); resp != nil || normErr != nil {
		return nil, resp, normErr
	}
	if resp, railErr := h.enforceAgentStatusCreateRails(ctx, claims, req); resp != nil || railErr != nil {
		return nil, resp, railErr
	}

	attr, resp, buildErr := h.buildAgentStatusAttribution(ctx, claims, req)
	if resp != nil || buildErr != nil {
		return nil, resp, buildErr
	}
	return attr, nil, nil
}

func (h *Handler) handleCreateScheduledStatus(ctx *apptheory.Context, claims *auth.Claims, req *models.CreateStatusRequest) (*apptheory.Response, error) {
	scheduledService := h.registry.Scheduled()
	if scheduledService == nil {
		h.logger.Error("scheduled service not available")
		return common.RespondServiceUnavailable(ctx, "scheduled service")
	}

	scheduledAt, err := parseScheduledAt(req.ScheduledAt)
	if err != nil {
		return common.RespondBadRequest(ctx, "scheduled_at must be a valid RFC3339 timestamp")
	}

	scheduledResult, schedErr := scheduledService.CreateScheduledStatus(ctx.Context(), &scheduled.CreateScheduledStatusCommand{
		Username:    claims.Username,
		Status:      req.Status,
		MediaIDs:    req.MediaIDs,
		Sensitive:   req.Sensitive,
		SpoilerText: req.SpoilerText,
		Visibility:  req.Visibility,
		Language:    req.Language,
		InReplyToID: req.InReplyToID,
		Poll:        buildScheduledPoll(req.Poll),
		ScheduledAt: scheduledAt,
	})
	if schedErr != nil {
		h.logger.Error("failed to create scheduled status", zap.Error(schedErr))
		return common.RespondInternalServerError(ctx, "failed to create scheduled status")
	}

	apiScheduled := h.convertScheduledStatusToAPIWithMedia(ctx, scheduledResult.ScheduledStatus, scheduledResult.MediaAttachments)
	return createdJSON(apiScheduled)
}

func parseScheduledAt(value *string) (time.Time, error) {
	if value == nil {
		return time.Time{}, fmt.Errorf("scheduled_at is nil")
	}

	scheduledAt, err := time.Parse(time.RFC3339Nano, *value)
	if err == nil {
		return scheduledAt, nil
	}

	return time.Parse(time.RFC3339, *value)
}

func buildScheduledPoll(poll *models.Poll) map[string]any {
	if poll == nil {
		return nil
	}

	return map[string]any{
		"options":     poll.Options,
		"expires_in":  poll.ExpiresIn,
		"multiple":    poll.Multiple,
		"hide_totals": poll.HideTotals,
	}
}

func createNoteCommandFromStatusRequest(claims *auth.Claims, req *models.CreateStatusRequest, agentAttribution *activitypub.AgentPostAttribution) *notes.CreateNoteCommand {
	createCmd := &notes.CreateNoteCommand{
		AuthorID:         claims.Username,
		Content:          req.Status,
		Visibility:       req.Visibility,
		Sensitive:        req.Sensitive,
		SpoilerText:      req.SpoilerText,
		Language:         req.Language,
		InReplyToID:      req.InReplyToID,
		MediaIDs:         req.MediaIDs,
		AgentAttribution: agentAttribution,
	}
	if req.Poll != nil {
		createCmd.PollOptions = req.Poll.Options
		createCmd.PollExpiresIn = req.Poll.ExpiresIn
		createCmd.PollMultiple = req.Poll.Multiple
		createCmd.PollHideTotals = req.Poll.HideTotals
	}
	return createCmd
}

func buildDirectMessageCommandFromStatusRequest(claims *auth.Claims, req *models.CreateStatusRequest, agentAttribution *activitypub.AgentPostAttribution) (*conversations.SendDirectMessageCommand, error) {
	recipients := directMessageRecipientsFromStatusRequest(req)
	switch len(recipients) {
	case 0:
		return nil, common.ValidationError{Field: "status", Message: "direct messages must mention exactly one recipient"}
	case 1:
	default:
		return nil, common.ValidationError{Field: "status", Message: "direct messages support exactly one recipient"}
	}

	return &conversations.SendDirectMessageCommand{
		SenderID:         claims.Username,
		Recipients:       recipients,
		Content:          req.Status,
		Sensitive:        req.Sensitive,
		SpoilerText:      req.SpoilerText,
		Language:         req.Language,
		MediaIDs:         req.MediaIDs,
		InReplyToID:      req.InReplyToID,
		AgentAttribution: agentAttribution,
	}, nil
}

func directMessageRecipientsFromStatusRequest(req *models.CreateStatusRequest) []string {
	if req == nil {
		return nil
	}

	rawMentions := conversations.ExtractMentionHandles(req.Status)
	recipients := make([]string, 0, len(rawMentions))
	seen := make(map[string]struct{}, len(rawMentions))
	for _, mention := range rawMentions {
		mention = strings.TrimSpace(mention)
		if mention == "" {
			continue
		}
		if _, exists := seen[mention]; exists {
			continue
		}
		seen[mention] = struct{}{}
		recipients = append(recipients, mention)
	}

	return recipients
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
	status, err := h.registry.Notes().GetNoteWithViewer(ctx.Context(), &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: claims.Username,
	})
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
	object, resp, err := h.getAndVerifyStatusOwnership(ctx, objectID, claims.Username)
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
	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return common.RespondUnauthorized(ctx, "missing token")
		},
		func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return common.RespondUnauthorized(ctx, "invalid token")
		},
	)
	if resp != nil || err != nil {
		return nil, nil, resp, err
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

// getAndVerifyStatusOwnership retrieves the object for update and ensures the viewer can access it.
func (h *Handler) getAndVerifyStatusOwnership(ctx *apptheory.Context, objectID, viewerUsername string) (any, *apptheory.Response, error) {
	object, err := h.registry.Notes().GetNoteWithViewer(ctx.Context(), &notes.GetNoteQuery{
		StatusID: objectID,
		ViewerID: viewerUsername,
	})
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
		note.Content = common.SanitizeContent(req.Status)
	}
	if req.SpoilerText != "" {
		note.Summary = common.SanitizeContent(req.SpoilerText)
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
func (h *Handler) buildUpdateStatusResponse(_ *apptheory.Context, note *activitypub.Note, actor *activitypub.Actor, req *models.UpdateStatusRequest) (*apptheory.Response, error) {
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
	statusID, err := normalizeReadableStatusID(ctx.Param("id"))
	if err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	viewerUsername := h.getOptionalAuthenticatedUser(ctx)

	// Get status using Notes service
	status, err := h.registry.Notes().GetNoteWithViewer(ctx.Context(), &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: viewerUsername,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "status not found")
		}
		h.logger.Error("failed to get status", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to retrieve status")
	}

	mastodonStatus, err := h.convertStorageStatusToAPI(status, viewerUsername)
	if err != nil {
		h.logger.Error("failed to convert status", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to retrieve status")
	}

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
	viewerUsername := h.getOptionalAuthenticatedUser(ctx)

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
	var viewerUsername string
	isAgent := false
	if claims := h.optionalAuthenticatedClaimsLift(ctx); claims != nil {
		viewerUsername = claims.Username
		isAgent = claims.IsAgent
	}

	root, resp, err := h.validateStatusIDForContext(ctx, viewerUsername)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get ancestors and descendants
	ancestors := h.getStatusAncestors(ctx.Context(), root, viewerUsername, 200)
	if isAgent && len(ancestors) > 20 {
		// Root + last 20 to cap agent context expansion.
		ancestors = append([]models.Status{ancestors[0]}, ancestors[len(ancestors)-20:]...)
	}
	descendants := h.getStatusDescendants(ctx.Context(), root, viewerUsername)

	// Return context response
	payload := models.StatusContext{
		Ancestors:   ancestors,
		Descendants: descendants,
	}

	return okJSON(payload)
}

// validateStatusIDForContext validates the status ID, checks it exists, and enforces viewer privacy.
func (h *Handler) validateStatusIDForContext(ctx *apptheory.Context, viewerUsername string) (*storageMods.Status, *apptheory.Response, error) {
	statusID, err := normalizeReadableStatusID(ctx.Param("id"))
	if err != nil {
		resp, respErr := common.RespondBadRequest(ctx, err.Error())
		return nil, resp, respErr
	}

	objectID := fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)

	// Get the status with viewer-aware privacy enforcement.
	status, err := h.registry.Notes().GetNoteWithViewer(ctx.Context(), &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: viewerUsername,
	})
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
				return nil, resp, respErr
			}
			// Fallback if we can't get tombstone details
			resp, respErr := apptheory.JSON(http.StatusGone, map[string]string{
				"error":             "status deleted",
				"error_description": "This status has been deleted and its context is no longer available",
			})
			return nil, resp, respErr
		}
		// Regular 404 for genuinely missing objects
		resp, respErr := common.RespondNotFound(ctx, "status not found")
		return nil, resp, respErr
	}

	return status, nil, nil
}

// getStatusAncestors retrieves the ancestors (parent statuses) of a status, enforcing viewer privacy.
func (h *Handler) getStatusAncestors(ctx context.Context, root *storageMods.Status, viewerUsername string, maxDepth int) []models.Status {
	if maxDepth <= 0 {
		maxDepth = 10
	}
	if root == nil {
		return []models.Status{}
	}

	parents := make([]*storageMods.Status, 0, maxDepth)
	currentID := strings.TrimSpace(root.InReplyToID)

	for i := 0; i < maxDepth; i++ {
		if currentID == "" {
			break
		}

		parent, err := h.registry.Notes().GetNoteWithViewer(ctx, &notes.GetNoteQuery{
			StatusID: currentID,
			ViewerID: viewerUsername,
		})
		if err != nil || parent == nil {
			break
		}

		parents = append(parents, parent)
		currentID = strings.TrimSpace(parent.InReplyToID)
	}

	ancestors := make([]models.Status, 0, len(parents))
	for i := len(parents) - 1; i >= 0; i-- {
		actor := h.getActorForObject(ctx, parents[i])
		status := transformations.ObjectToStatusAny(parents[i], actor, h.cfg.BaseURL())
		ancestors = append(ancestors, status)
	}

	return ancestors
}

// getParentStatusID gets the parent status ID from an object
//
//nolint:unused // Used by tests and retained for future thread/context refactors.
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
//
//nolint:unused // Used by tests and retained for future thread/context refactors.
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
//
//nolint:unused // Used by tests and retained for future thread/context refactors.
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
//
//nolint:unused // Used by tests and retained for future thread/context refactors.
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

// getStatusDescendants retrieves the descendants (replies) of a status, enforcing viewer privacy.
func (h *Handler) getStatusDescendants(ctx context.Context, root *storageMods.Status, viewerUsername string) []models.Status {
	descendants := []models.Status{}
	if root == nil || strings.TrimSpace(root.StatusID) == "" {
		return descendants
	}

	replies, err := h.repos.Status().GetReplies(ctx, root.StatusID, interfaces.PaginationOptions{Limit: 100})
	if err != nil || replies == nil {
		h.logger.Warn("failed to get replies for context",
			zap.String("status_id", root.StatusID),
			zap.Error(err))
		return descendants
	}

	for _, reply := range replies.Items {
		if reply == nil || strings.TrimSpace(reply.StatusID) == "" {
			continue
		}

		status, err := h.registry.Notes().GetNoteWithViewer(ctx, &notes.GetNoteQuery{
			StatusID: reply.StatusID,
			ViewerID: viewerUsername,
		})
		if err != nil || status == nil {
			continue
		}

		actor := h.getActorForObject(ctx, status)
		apiStatus := transformations.ObjectToStatusAny(status, actor, h.cfg.BaseURL())
		descendants = append(descendants, apiStatus)
	}

	h.logger.Debug("fetched descendants for context",
		zap.String("status_id", root.StatusID),
		zap.Int("count", len(descendants)))

	return descendants
}

// convertReplyToStatus converts a reply object to a status
//
//nolint:unused // Retained for future thread/context refactors.
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

	viewerUsername := h.getOptionalAuthenticatedUser(ctx)

	// Get user timeline with viewer-aware privacy enforcement.
	result, err := h.registry.Notes().ListNotes(ctx.Context(), &notes.ListNotesQuery{
		ViewerID:       viewerUsername,
		TimelineType:   "user",
		AuthorID:       actor.ID,
		Pagination:     interfaces.PaginationOptions{Limit: params.limit, Cursor: params.maxID},
		OnlyMedia:      params.onlyMedia,
		ExcludeReplies: params.excludeReplies,
		ExcludeReblogs: params.excludeReblogs,
	})
	if err != nil {
		h.logger.Error("failed to get objects by actor", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}
	if result == nil {
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	cursor := ""
	if result != nil && result.Pagination != nil {
		cursor = result.Pagination.NextCursor
	}

	// Convert status models to interface{} for compatibility
	objects := make([]any, 0, len(result.Notes))
	for _, s := range result.Notes {
		objects = append(objects, s)
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
	return h.getOptionalAuthenticatedUser(ctx)
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

	if note != nil {
		updateActivity.To = append([]string(nil), note.To...)
		updateActivity.CC = append([]string(nil), note.CC...)
	}

	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName(), h.cfg.Domain, h.logger)
	deliveryService := federation.NewDeliveryService(federationStorage, h.cfg)

	if isActivityPublicOrUnlisted(updateActivity) {
		if err := deliveryService.DeliverToFollowers(ctx, updateActivity, actor); err != nil {
			h.logger.Warn("failed to deliver update activity to followers",
				zap.String("activity_id", updateActivity.ID),
				zap.Error(err))
		}
	}

	if err := deliveryService.DeliverToRecipients(ctx, updateActivity, actor); err != nil {
		h.logger.Error("failed to deliver update activity to recipients",
			zap.String("activity_id", updateActivity.ID),
			zap.Error(err))
		return err
	}

	h.logger.Info("completed update activity delivery",
		zap.String("activity_id", updateActivity.ID))

	return nil
}

func isActivityPublicOrUnlisted(activity *activitypub.Activity) bool {
	if activity == nil {
		return false
	}

	for _, recipient := range append(activity.To, activity.CC...) {
		if recipient == activitypub.PublicAddress {
			return true
		}
	}

	return false
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
