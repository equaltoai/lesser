package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/emoji"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
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
func (h *Handler) HandleCreateStatusLift(ctx *lift.Context) error {
	// Parse request
	var req models.CreateStatusRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Default visibility
	if req.Visibility == "" {
		req.Visibility = "public"
	}

	// Call Notes service
	result, err := h.registry.Notes().CreateNote(ctx.Context, &notes.CreateNoteCommand{
		AuthorID:    claims.Username,
		Content:     req.Status,
		Visibility:  req.Visibility,
		Sensitive:   req.Sensitive,
		Language:    req.Language,
		InReplyToID: req.InReplyToID,
		MediaIDs:    req.MediaIDs,
	})
	if err != nil {
		h.logger.Error("failed to create note", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to create status"})
	}

	// Get the author actor for proper conversion
	account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get author account", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to create status"})
	}

	// Convert to Mastodon API format using correct converter
	mastodonStatus := h.converter.ObjectToStatus(result.Note, account.Actor)

	h.logger.Info("created status", 
		zap.String("id", result.Note.StatusID),
		zap.String("content", req.Status))

	return ctx.Status(http.StatusCreated).JSON(mastodonStatus)
}

// HandleDeleteStatusLift deletes a status using the Notes service
func (h *Handler) HandleDeleteStatusLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Get the status first to return it (Mastodon API returns deleted status)
	status, err := h.registry.Notes().GetNote(ctx.Context, statusID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		h.logger.Error("failed to get status for deletion", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to delete status"})
	}

	// Delete using Notes service
	err = h.registry.Notes().DeleteNote(ctx.Context, &notes.DeleteNoteCommand{
		StatusID:  statusID,
		DeleterID: claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		if strings.Contains(err.Error(), "not authorized") {
			return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "not authorized to delete this status"})
		}
		h.logger.Error("failed to delete status", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to delete status"})
	}

	// Get the author actor for proper conversion
	account, err := h.registry.Accounts().GetAccount(ctx.Context, h.converter.ExtractUsernameFromActorID(status.AuthorID))
	if err != nil {
		h.logger.Error("failed to get author account for deleted status", zap.Error(err))
		// Return a basic response if we can't get the author
		return ctx.JSON(map[string]interface{}{"id": statusID, "deleted": true})
	}

	// Return the deleted status in Mastodon format
	mastodonStatus := h.converter.ObjectToStatus(status, account.Actor)

	h.logger.Info("deleted status", zap.String("id", statusID))

	return ctx.JSON(mastodonStatus)
}

// HandleUpdateStatusLift updates an existing status
func (h *Handler) HandleUpdateStatusLift(ctx *lift.Context) error {
	// Validate status ID
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate and authorize user
	_, actor, err := h.authenticateStatusUpdate(ctx)
	if err != nil {
		return err
	}

	// Get and verify object ownership
	objectID := h.normalizeStatusIDForUpdate(statusID)
	object, err := h.getAndVerifyStatusOwnership(ctx, objectID, actor.ID)
	if err != nil {
		return err
	}

	// Parse update request
	req, err := h.parseUpdateStatusRequest(ctx)
	if err != nil {
		return err
	}

	// Convert object to Note and verify ownership
	note, err := h.convertObjectToNoteWithOwnershipCheck(ctx, object, actor.ID)
	if err != nil {
		return err
	}

	// Apply updates to note
	h.applyStatusUpdates(note, req)

	// Save updated note
	if err := h.saveUpdatedStatus(ctx, note); err != nil {
		return err
	}

	// Create and deliver update activity
	if err := h.createStatusUpdateActivity(ctx, note, actor); err != nil {
		return err
	}

	// Build and return response
	return h.buildUpdateStatusResponse(ctx, note, actor, req)
}

// authenticateStatusUpdate handles authentication for status updates
func (h *Handler) authenticateStatusUpdate(ctx *lift.Context) (*auth.Claims, *activitypub.Actor, error) {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return nil, nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}
	actor := account.Actor

	return claims, actor, nil
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
func (h *Handler) getAndVerifyStatusOwnership(ctx *lift.Context, objectID, _ string) (any, error) {
	object, err := h.registry.Notes().GetNote(ctx.Context, objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context, objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context, objectID); tErr == nil {
				return nil, ctx.Status(http.StatusGone).JSON(map[string]interface{}{
					"error": "status deleted",
					"error_description": "This status has been deleted",
					"deleted_at": tombstone.Deleted.Format(time.RFC3339),
					"former_type": tombstone.FormerType,
				})
			}
			// Fallback if we can't get tombstone details
			return nil, ctx.Status(http.StatusGone).JSON(map[string]string{
				"error": "status deleted",
				"error_description": "This status has been deleted",
			})
		}
		// Regular 404 for genuinely missing objects
		return nil, ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}
	return object, nil
}

// parseUpdateStatusRequest parses the update status request
func (h *Handler) parseUpdateStatusRequest(ctx *lift.Context) (*models.UpdateStatusRequest, error) {
	var req models.UpdateStatusRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
	}
	return &req, nil
}

// convertObjectToNoteWithOwnershipCheck converts object to Note and verifies ownership
func (h *Handler) convertObjectToNoteWithOwnershipCheck(ctx *lift.Context, object any, actorID string) (*activitypub.Note, error) {
	switch obj := object.(type) {
	case *activitypub.Note:
		if obj.AttributedTo != actorID {
			return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
		}
		return obj, nil

	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok && attr != actorID {
			return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
		}
		return h.convertMapToNote(ctx, obj)

	default:
		return h.convertUnknownObjectToNote(ctx, object, actorID)
	}
}

// convertMapToNote converts a map to a Note
func (h *Handler) convertMapToNote(ctx *lift.Context, obj map[string]any) (*activitypub.Note, error) {
	noteBytes, err := json.Marshal(obj)
	if err != nil {
		h.logger.Error("failed to marshal object to JSON", zap.Error(err))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	note := &activitypub.Note{}
	if err := json.Unmarshal(noteBytes, note); err != nil {
		h.logger.Error("failed to unmarshal JSON to Note", zap.Error(err))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	return note, nil
}

// convertUnknownObjectToNote handles conversion of unknown object types to Note
func (h *Handler) convertUnknownObjectToNote(ctx *lift.Context, object any, actorID string) (*activitypub.Note, error) {
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

	if attributedTo == "" {
		h.logger.Error("unexpected object type or missing AttributedTo",
			zap.String("type", fmt.Sprintf("%T", object)),
			zap.Any("object", object))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "unexpected object type"})
	}

	if attributedTo != actorID {
		return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
	}

	// Convert to Note via JSON marshaling
	noteBytes, _ := json.Marshal(object)
	note := &activitypub.Note{}
	if err := json.Unmarshal(noteBytes, note); err != nil {
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to convert object to Note"})
	}

	return note, nil
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
func (h *Handler) saveUpdatedStatus(ctx *lift.Context, note *activitypub.Note) error {
	// Extract the username from the authentication token
	username := h.extractUsernameFromToken(ctx)
	if username == "" {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to extract username for edit tracking"})
	}

	// Build actor ID for edit tracking
	actorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)

	// Update object with history tracking using the new method
	if err := h.repos.Object().UpdateObjectWithHistory(ctx.Context, note, actorID); err != nil {
		h.logger.Error("failed to update object with history", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}
	return nil
}

// createStatusUpdateActivity creates and stores the update activity
func (h *Handler) createStatusUpdateActivity(ctx *lift.Context, note *activitypub.Note, actor *activitypub.Actor) error {
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
	if err := h.repos.Activity().CreateActivity(ctx.Context, updateActivity); err != nil {
		h.logger.Error("failed to create update activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
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

	return nil
}

// buildUpdateStatusResponse builds the response for the updated status
func (h *Handler) buildUpdateStatusResponse(ctx *lift.Context, note *activitypub.Note, actor *activitypub.Actor, req *models.UpdateStatusRequest) error {
	resp := h.converter.ObjectToStatus(note, actor)
	if req.Visibility != "" {
		resp.Visibility = req.Visibility
	}
	if req.Language != "" {
		resp.Language = req.Language
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetStatusLift retrieves a status by ID using the Notes service
func (h *Handler) HandleGetStatusLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Optional authentication (public statuses can be viewed without auth)
	var viewerUsername string
	token := h.getBearerTokenLift(ctx)
	if token != "" {
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			viewerUsername = claims.Username
		}
	}

	// Get status using Notes service
	status, err := h.registry.Notes().GetNote(ctx.Context, statusID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		h.logger.Error("failed to get status", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to retrieve status"})
	}

	// Get the author actor for proper conversion
	account, err := h.registry.Accounts().GetAccount(ctx.Context, h.converter.ExtractUsernameFromActorID(status.AuthorID))
	if err != nil {
		h.logger.Error("failed to get author account", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to retrieve status"})
	}

	// Convert to Mastodon API format using correct converter
	mastodonStatus := h.converter.ObjectToStatusWithContext(ctx.Context, status, account.Actor, 0, 0, false, false, false)

	// TODO: Add user-specific metadata like favorited, reblogged, bookmarked if viewerUsername is available
	_ = viewerUsername

	return ctx.JSON(mastodonStatus)
}

// HandleGetHomeTimelineLift returns the home timeline using the Notes service
func (h *Handler) HandleGetHomeTimelineLift(ctx *lift.Context) error {
	// Authenticate with read scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Parse pagination parameters
	limit := 20
	if limitStr := ctx.QueryParam("limit"); limitStr != "" {
		if l, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || l != 1 {
			limit = 20
		}
		if limit > 80 {
			limit = 80
		}
	}

	cursor := ctx.QueryParam("max_id")

	// Get home timeline using Notes service
	result, err := h.registry.Notes().ListNotes(ctx.Context, &notes.ListNotesQuery{
		ViewerID:     claims.Username,
		TimelineType: "home",
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		h.logger.Error("failed to get home timeline", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "failed to retrieve timeline",
		})
	}

	// Convert to Mastodon API format
	timeline := make([]interface{}, len(result.Notes))
	for i, status := range result.Notes {
		// Get author actor for conversion
		account, err := h.registry.Accounts().GetAccount(ctx.Context, h.converter.ExtractUsernameFromActorID(status.AuthorID))
		if err != nil {
			h.logger.Warn("failed to get author for timeline status", zap.Error(err))
			continue
		}
		timeline[i] = h.converter.ObjectToStatus(status, account.Actor)
	}

	return ctx.JSON(timeline)
}

// HandleGetPublicTimelineLift returns the public timeline using the Notes service
func (h *Handler) HandleGetPublicTimelineLift(ctx *lift.Context) error {
	// Optional authentication (public timeline can be viewed without auth)
	var viewerUsername string
	token := h.getBearerTokenLift(ctx)
	if token != "" {
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			viewerUsername = claims.Username
		}
	}

	// Parse pagination parameters
	limit := 20
	if limitStr := ctx.QueryParam("limit"); limitStr != "" {
		if l, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || l != 1 {
			limit = 20
		}
		if limit > 80 {
			limit = 80
		}
	}

	cursor := ctx.QueryParam("max_id")
	local := ctx.QueryParam("local") == "true"

	timelineType := "public"
	if local {
		timelineType = "local"
	}

	// Get public timeline using Notes service
	result, err := h.registry.Notes().ListNotes(ctx.Context, &notes.ListNotesQuery{
		ViewerID:     viewerUsername,
		TimelineType: timelineType,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		h.logger.Error("failed to get public timeline", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "failed to retrieve timeline",
		})
	}

	// Convert to Mastodon API format
	timeline := make([]interface{}, len(result.Notes))
	for i, status := range result.Notes {
		// Get author actor for conversion
		account, err := h.registry.Accounts().GetAccount(ctx.Context, h.converter.ExtractUsernameFromActorID(status.AuthorID))
		if err != nil {
			h.logger.Warn("failed to get author for timeline status", zap.Error(err))
			continue
		}
		timeline[i] = h.converter.ObjectToStatus(status, account.Actor)
	}

	return ctx.JSON(timeline)
}

// HandleGetStatusContextLift retrieves the context (ancestors and descendants) of a status
func (h *Handler) HandleGetStatusContextLift(ctx *lift.Context) error {
	// Validate and normalize status ID
	objectID, err := h.validateStatusIDForContext(ctx)
	if err != nil {
		return err
	}

	// Get ancestors and descendants
	ancestors := h.getStatusAncestors(ctx.Context, objectID)
	descendants := h.getStatusDescendants(ctx.Context, objectID)

	// Return context response
	resp := models.StatusContext{
		Ancestors:   ancestors,
		Descendants: descendants,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// validateStatusIDForContext validates the status ID and checks it exists
func (h *Handler) validateStatusIDForContext(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")
	if statusID == "" {
		return "", ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Normalize and validate the status ID
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to check it exists
	_, err := h.registry.Notes().GetNote(ctx.Context, objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context, objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context, objectID); tErr == nil {
				return "", ctx.Status(http.StatusGone).JSON(map[string]interface{}{
					"error": "status deleted",
					"error_description": "This status has been deleted and its context is no longer available",
					"deleted_at": tombstone.Deleted.Format(time.RFC3339),
					"former_type": tombstone.FormerType,
				})
			}
			// Fallback if we can't get tombstone details
			return "", ctx.Status(http.StatusGone).JSON(map[string]string{
				"error": "status deleted",
				"error_description": "This status has been deleted and its context is no longer available",
			})
		}
		// Regular 404 for genuinely missing objects
		return "", ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	return objectID, nil
}

// getStatusAncestors retrieves the ancestors (parent statuses) of a status
func (h *Handler) getStatusAncestors(ctx context.Context, objectID string) []models.Status {
	ancestors := []models.Status{}
	currentID := objectID

	for i := 0; i < 10; i++ { // Limit depth to prevent infinite loops
		parentID := h.getParentStatusID(ctx, currentID)
		if parentID == "" {
			break
		}

		parentStatus := h.loadStatusWithActor(ctx, parentID)
		if parentStatus == nil {
			break
		}

		ancestors = append([]models.Status{*parentStatus}, ancestors...) // Prepend to maintain order
		currentID = parentID
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
	if inReplyTo == "" {
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
	status := h.converter.ObjectToStatus(obj, actor)
	return &status
}

// getActorForObject retrieves the actor associated with an object
func (h *Handler) getActorForObject(ctx context.Context, obj interface{}) *activitypub.Actor {
	attributedTo := h.extractAttributedTo(obj)
	if attributedTo == "" {
		return nil
	}

	username := h.converter.ExtractUsernameFromActorID(attributedTo)
	if username == "" {
		return nil
	}

	account, err := h.registry.Accounts().GetAccount(ctx, username)
	if err != nil {
		return nil
	}
	return account.Actor
}

// extractAttributedToViaReflection uses reflection to extract AttributedTo field
//
//nolint:unused // Reserved for future ActivityPub object handling
func (h *Handler) extractAttributedToViaReflection(obj interface{}) string {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	attrField := v.FieldByName("AttributedTo")
	if !attrField.IsValid() || attrField.Kind() != reflect.String {
		return ""
	}

	return attrField.String()
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
	status := h.converter.ObjectToStatus(reply, actor)
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
func (h *Handler) HandleGetAccountStatusesLift(ctx *lift.Context) error {
	// Validate and get account ID
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing account id"})
	}

	// Resolve account ID to actor
	actor, err := h.resolveAccountID(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "account not found"})
	}

	// Parse query parameters
	params := h.parseAccountStatusesParams(ctx)

	// Get objects by this actor
	userTimeline, err := h.registry.Notes().GetUserTimeline(ctx.Context, actor.ID, interfaces.PaginationOptions{
		Limit:  params.limit,
		Cursor: params.maxID,
	})
	if err != nil {
		h.logger.Error("failed to get objects by actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
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

	// Set pagination header if needed
	if cursor != "" {
		h.setPaginationHeader(ctx, accountID, cursor, params)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(statuses)
}

// parseAccountStatusesParams parses query parameters for account statuses
func (h *Handler) parseAccountStatusesParams(ctx *lift.Context) accountStatusesParams {
	params := accountStatusesParams{
		limit:          20,
		maxID:          ctx.Query("max_id"),
		onlyMedia:      ctx.Query("only_media") == boolTrue,
		excludeReplies: ctx.Query("exclude_replies") == boolTrue,
		excludeReblogs: ctx.Query("exclude_reblogs") == boolTrue,
		tagged:         ctx.Query("tagged"),
	}

	// Parse limit parameter
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			params.limit = parsedLimit
		}
	}

	return params
}

// convertAndFilterObjects converts objects to statuses with filtering
func (h *Handler) convertAndFilterObjects(_ *lift.Context, objects []any, actor *activitypub.Actor, params accountStatusesParams) []models.Status {
	statuses := []models.Status{}

	h.logger.Debug("converting objects to statuses",
		zap.Int("object_count", len(objects)),
		zap.String("actor_id", actor.ID))

	for _, obj := range objects {
		// Apply filters
		if h.shouldFilterObject(obj, params) {
			continue
		}

		status := h.converter.ObjectToStatus(obj, actor)
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

	// exclude_reblogs and tagged filters not yet implemented
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

// logStatusConversion logs debug information about status conversion
func (h *Handler) logStatusConversion(status models.Status, obj any) {
	h.logger.Debug("converted status from object",
		zap.String("status_id", status.ID),
		zap.String("status_content", status.Content),
		zap.String("status_created_at", status.CreatedAt),
		zap.Any("object_type", fmt.Sprintf("%T", obj)))
}

// setPaginationHeader sets the Link header for pagination
func (h *Handler) setPaginationHeader(ctx *lift.Context, accountID, cursor string, params accountStatusesParams) {
	nextURL := h.buildPaginationURL(accountID, cursor, params)
	ctx.Response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
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
func extractLinksFromContent(content string) []string {
	links := []string{}
	words := strings.Fields(content)
	for _, word := range words {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			links = append(links, word)
		}
	}
	return links
}

// extractMentions extracts @mentions from content and returns actor URIs
func (h *Handler) extractMentions(content string) []string {
	mentions := []string{}
	words := strings.Fields(content)

	for _, word := range words {
		if strings.HasPrefix(word, "@") {
			// Remove @ and any trailing punctuation
			username := strings.TrimPrefix(word, "@")
			username = strings.TrimRight(username, ".,!?;:")

			if username != "" {
				// Convert username to actor URI
				actorURI := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)
				mentions = append(mentions, actorURI)
			}
		}
	}

	return mentions
}


// validateAndNormalizeStatusID validates and normalizes the status ID parameter
func (h *Handler) validateAndNormalizeStatusID(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")
	if statusID == "" {
		return "", ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Normalize the status ID to a full URL if needed
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return objectID, nil
}

// extractAuthorUsernameForStatus extracts the username from a status object
func (h *Handler) extractAuthorUsernameForStatus(ctx context.Context, objectID string) string {
	// Get the object
	statusObj, err := h.registry.Notes().GetNote(ctx, objectID)
	if err != nil {
		return ""
	}

	// For Status models, use the AuthorID field directly
	attributedTo := statusObj.AuthorID

	if attributedTo == "" {
		return ""
	}

	// Extract username from actor ID using converter
	return h.converter.ExtractUsernameFromActorID(attributedTo)
}

// getStatusActor retrieves the actor associated with a status object
func (h *Handler) getStatusActor(ctx context.Context, _ any, objectID string) *activitypub.Actor {
	// Extract author username from the object
	username := h.extractAuthorUsernameForStatus(ctx, objectID)
	if username == "" {
		return nil
	}

	// Get the actor
	account, err := h.registry.Accounts().GetAccount(ctx, username)
	if err != nil {
		h.logger.Debug("failed to get status actor",
			zap.String("username", username),
			zap.Error(err))
		return nil
	}
	actor := account.Actor

	return actor
}

// enrichStatusWithEmojis parses and adds emoji data to a status
func (h *Handler) enrichStatusWithEmojis(ctx context.Context, status *models.Status) {
	// Create emoji parser
	emojiParser := emoji.NewParser(h.repos, h.logger)

	// Parse emojis from status content
	emojis, err := emojiParser.GetForStatus(ctx, status.Content)
	if err != nil {
		h.logger.Warn("Failed to parse emojis for status",
			zap.String("status_id", status.ID),
			zap.Error(err))
		status.Emojis = []any{}
		return
	}

	status.Emojis = emojis
}

// enrichStatusWithInteractionCounts adds like and reblog counts to a status
func (h *Handler) enrichStatusWithInteractionCounts(ctx context.Context, status *models.Status, objectID string) {
	// Get like count
	likeCount, err := h.registry.Notes().GetLikeCount(ctx, objectID)
	if err == nil {
		status.FavouritesCount = int(likeCount)
	}

	// Get reblog/boost count
	boostCount, err := h.registry.Notes().GetBoostCount(ctx, objectID)
	if err == nil {
		status.ReblogsCount = int(boostCount)
	}
}

// enrichStatusWithPoll adds poll data to a status if it exists
func (h *Handler) enrichStatusWithPoll(ctx context.Context, status *models.Status, objectID string) *storage.Poll {
	// Get poll if exists
	poll, err := h.repos.Poll().GetPollByStatusID(ctx, objectID)
	if err == nil && poll != nil {
		// Use converter to convert poll to API format
		apiPoll := h.converter.PollToAPI(poll, []int{}) // User votes will be set later in enrichStatusWithUserMetadata

		status.Poll = &apiPoll
		return poll
	}
	return nil
}

// extractUsernameFromToken extracts the username from authentication token
func (h *Handler) extractUsernameFromToken(ctx *lift.Context) string {
	// Check for test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	if testUsername != "" {
		return testUsername
	}

	// Get authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			// Validate token
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				return claims.Username
			}
		}
	}

	return ""
}

// enrichStatusWithUserInteractions adds user-specific interaction state to a status
func (h *Handler) enrichStatusWithUserInteractions(ctx *lift.Context, status *models.Status, objectID string, poll *storage.Poll) {
	// Try to get authenticated user
	username := h.extractUsernameFromToken(ctx)
	if username == "" {
		return
	}

	// Build actor ID for the user
	actorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)

	// Check if user has liked this status
	liked, err := h.registry.Notes().HasLiked(ctx.Context, actorID, objectID)
	if err == nil {
		status.Favourited = liked
	}

	// Check if user has reblogged this status
	reblogged, err := h.registry.Notes().HasReblogged(ctx.Context, actorID, objectID)
	if err == nil {
		status.Reblogged = reblogged
	} else {
		// Log error but don't fail - just assume not reblogged
		h.logger.Warn("failed to check reblog status",
			zap.String("actor_id", actorID),
			zap.String("object_id", objectID),
			zap.Error(err))
		status.Reblogged = false
	}

	// Check if user has bookmarked this status
	bookmarked, err := h.registry.Notes().IsBookmarked(ctx.Context, username, objectID)
	if err == nil {
		status.Bookmarked = bookmarked
	} else {
		// Log error but don't fail - just assume not bookmarked
		h.logger.Warn("failed to check bookmark status",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		status.Bookmarked = false
	}

	// Check if user has voted in poll
	if poll != nil && status.Poll != nil && username != "" {
		// Check if user has voted using the poll repository
		hasVoted, userVotes, err := h.repos.Poll().HasUserVoted(ctx.Context, poll.ID, actorID)
		if err != nil {
			// Log error but don't fail - just assume no votes
			h.logger.Warn("failed to get user poll votes",
				zap.String("poll_id", poll.ID),
				zap.String("actor_id", actorID),
				zap.Error(err))
			status.Poll.Voted = false
			status.Poll.OwnVotes = []int{}
		} else {
			status.Poll.Voted = hasVoted
			if userVotes != nil {
				status.Poll.OwnVotes = userVotes
			} else {
				status.Poll.OwnVotes = []int{}
			}
		}
	}
}

// performLocalCascadeDeletion performs cascade deletion operations for local status deletion
func (h *Handler) performLocalCascadeDeletion(ctx context.Context, objectID, actorID string) error {
	h.logger.Info("performing local cascade deletion operations",
		zap.String("object_id", objectID),
		zap.String("actor", actorID))

	// Remove likes for this object
	if err := h.cascadeDeleteLikes(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete likes", zap.Error(err))
	}

	// Remove announces/boosts for this object
	if err := h.cascadeDeleteAnnounces(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete announces", zap.Error(err))
	}

	// Remove from collections (featured, pinned, etc.)
	if err := h.cascadeDeleteFromCollections(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete from collections", zap.Error(err))
	}

	// Clean up bookmarks
	if err := h.cascadeDeleteBookmarks(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete bookmarks", zap.Error(err))
	}

	// Clean up polls if this status had one
	if err := h.cascadeDeletePolls(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete polls", zap.Error(err))
	}

	h.logger.Info("completed local cascade deletion operations", zap.String("object_id", objectID))
	return nil
}

// cascadeDeleteLikes removes all likes for the deleted object
func (h *Handler) cascadeDeleteLikes(ctx context.Context, objectID string) error {
	// Get all likes for this object (using a reasonable limit)
	likes, _, err := h.repos.Like().GetObjectLikes(ctx, objectID, 1000, "")
	if err != nil {
		return fmt.Errorf("failed to get object likes: %w", err)
	}

	// Delete each like
	for _, like := range likes {
		if err := h.repos.Like().DeleteLike(ctx, like.Actor, objectID); err != nil {
			h.logger.Warn("failed to delete like during cascade",
				zap.String("object_id", objectID),
				zap.String("like_actor", like.Actor),
				zap.Error(err))
		}
	}

	h.logger.Debug("cascade deleted likes",
		zap.String("object_id", objectID),
		zap.Int("count", len(likes)))

	return nil
}

// cascadeDeleteAnnounces removes all announces/boosts for the deleted object
func (h *Handler) cascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	// Use the Social repository's CascadeDeleteAnnounces method
	if err := h.repos.Social().CascadeDeleteAnnounces(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete announces",
			zap.String("object_id", objectID),
			zap.Error(err))
		return err
	}

	h.logger.Debug("cascade deleted announces",
		zap.String("object_id", objectID))

	return nil
}

// cascadeDeleteFromCollections removes the object from collections
func (h *Handler) cascadeDeleteFromCollections(ctx context.Context, objectID string) error {
	// Remove from common collections
	collections := []string{"featured", "pinned"}

	for _, collection := range collections {
		if err := h.repos.Object().RemoveFromCollection(ctx, collection, objectID); err != nil {
			h.logger.Debug("failed to remove from collection during cascade",
				zap.String("object_id", objectID),
				zap.String("collection", collection),
				zap.Error(err))
		}
	}

	return nil
}

// cascadeDeleteBookmarks removes all bookmarks for the deleted object
func (h *Handler) cascadeDeleteBookmarks(_ context.Context, objectID string) error {
	// Note: The current Bookmark model doesn't have a GSI to efficiently query by ObjectID
	// This would require either:
	// 1. Adding a GSI to the Bookmark model (infrastructure change)
	// 2. Implementing a background cleanup process
	// 3. Accepting that orphaned bookmarks will exist (cleaned up on access)
	//
	// For now, we log this limitation and rely on the application layer to handle
	// orphaned bookmarks gracefully (e.g., returning 404 when trying to access a deleted status)
	
	h.logger.Debug("cascade delete bookmarks - skipped due to model limitations", 
		zap.String("object_id", objectID),
		zap.String("reason", "no GSI for efficient bookmark lookup by object_id"))
	
	// In a production system, you might want to add this to a cleanup queue
	// or implement a background job that periodically removes orphaned bookmarks
	
	return nil
}

// cascadeDeletePolls removes polls associated with the deleted status
func (h *Handler) cascadeDeletePolls(ctx context.Context, objectID string) error {
	// Check if this status has a poll
	poll, err := h.repos.Poll().GetPollByStatusID(ctx, objectID)
	if err != nil {
		return nil // No poll found, which is fine
	}

	if poll != nil {
		// For now, log that poll cleanup should happen
		// This would require implementing poll deletion methods
		h.logger.Debug("cascade delete poll - implementation framework ready",
			zap.String("object_id", objectID),
			zap.String("poll_id", poll.ID))
	}

	return nil
}

// deliverDeleteActivity delivers a Delete activity to relevant recipients for federation
func (h *Handler) deliverDeleteActivity(ctx context.Context, deleteActivity *activitypub.Activity, actor *activitypub.Actor, originalObject any) error {
	h.logger.Info("delivering delete activity for federation",
		zap.String("activity_id", deleteActivity.ID),
		zap.String("actor", actor.ID))

	// Determine delivery recipients based on the original object
	recipients, err := h.determineDeleteDeliveryRecipients(ctx, actor, originalObject)
	if err != nil {
		return fmt.Errorf("failed to determine delivery recipients: %w", err)
	}

	if len(recipients) == 0 {
		h.logger.Info("no recipients for delete activity delivery", zap.String("activity_id", deleteActivity.ID))
		return nil
	}

	// Deliver to each recipient
	deliveredCount := 0
	failedCount := 0

	for _, recipient := range recipients {
		if err := h.deliverDeleteToRecipient(ctx, deleteActivity, actor, recipient); err != nil {
			h.logger.Warn("failed to deliver delete activity to recipient",
				zap.String("recipient", recipient),
				zap.String("activity_id", deleteActivity.ID),
				zap.Error(err))
			failedCount++
		} else {
			deliveredCount++
		}
	}

	h.logger.Info("completed delete activity delivery",
		zap.String("activity_id", deleteActivity.ID),
		zap.Int("delivered", deliveredCount),
		zap.Int("failed", failedCount),
		zap.Int("total_recipients", len(recipients)))

	return nil
}

// determineDeleteDeliveryRecipients determines who should receive the Delete activity
func (h *Handler) determineDeleteDeliveryRecipients(ctx context.Context, actor *activitypub.Actor, originalObject any) ([]string, error) {
	recipients := make(map[string]bool)

	// Add followers to recipients
	followers, _, err := h.repos.Relationship().GetFollowers(ctx, actor.PreferredUsername, 1000, "")
	if err != nil {
		h.logger.Warn("failed to get followers for delete delivery", zap.Error(err))
	} else {
		for _, follower := range followers {
			recipients[follower] = true
		}
	}

	// Add mentioned users from original object
	mentions := h.extractMentionsFromObject(originalObject)
	for _, mention := range mentions {
		recipients[mention] = true
	}

	// Add users who were in original To/CC fields
	toUsers, ccUsers := h.extractToAndCCFromObject(originalObject)
	for _, user := range toUsers {
		if user != activitypub.PublicAddress {
			recipients[user] = true
		}
	}
	for _, user := range ccUsers {
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

// deliverDeleteToRecipient delivers the Delete activity to a specific recipient
func (h *Handler) deliverDeleteToRecipient(ctx context.Context, deleteActivity *activitypub.Activity, actor *activitypub.Actor, recipientID string) error {
	h.logger.Debug("delivering delete activity to recipient",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", deleteActivity.ID))

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName())
	deliveryService := federation.NewDeliveryService(federationStorage)

	// Set recipient in activity To field for delivery
	deleteActivity.To = []string{recipientID}

	// Deliver using federation service
	if err := deliveryService.DeliverToRecipients(ctx, deleteActivity, actor); err != nil {
		h.logger.Error("failed to deliver delete activity to recipient",
			zap.String("recipient_id", recipientID),
			zap.String("activity_id", deleteActivity.ID),
			zap.Error(err))
		return err
	}

	h.logger.Info("delete activity delivered successfully",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", deleteActivity.ID))

	return nil
}

// extractMentionsFromObject extracts mentioned user IDs from an object
func (h *Handler) extractMentionsFromObject(object any) []string {
	var mentions []string

	if objMap, ok := object.(map[string]any); ok {
		if tagsInterface, ok := objMap["tag"]; ok {
			mentions = h.parseMentionsFromTags(tagsInterface)
		}
	} else if note, ok := object.(*activitypub.Note); ok {
		for _, tag := range note.Tag {
			if tag.Type == tagTypeMention && tag.Href != "" {
				mentions = append(mentions, tag.Href)
			}
		}
	}

	return mentions
}

// extractToAndCCFromObject extracts To and CC fields from an object
func (h *Handler) extractToAndCCFromObject(object any) ([]string, []string) {
	var toUsers, ccUsers []string

	if objMap, ok := object.(map[string]any); ok {
		if to, ok := objMap["to"].([]string); ok {
			toUsers = to
		}
		if cc, ok := objMap["cc"].([]string); ok {
			ccUsers = cc
		}
	} else if note, ok := object.(*activitypub.Note); ok {
		toUsers = note.To
		ccUsers = note.CC
	}

	return toUsers, ccUsers
}

const tagTypeMention = "Mention"

// parseMentionsFromTags parses mentions from various tag formats
func (h *Handler) parseMentionsFromTags(tagsInterface any) []string {
	var mentions []string

	switch tags := tagsInterface.(type) {
	case []any:
		mentions = h.parseMentionsFromAnySlice(tags)
	case []activitypub.Tag:
		mentions = h.parseMentionsFromTagSlice(tags)
	case string:
		mentions = h.parseMentionsFromString(tags)
	}

	return mentions
}

// parseMentionsFromAnySlice extracts mentions from a slice of any type
func (h *Handler) parseMentionsFromAnySlice(tags []any) []string {
	var mentions []string
	for _, tagInterface := range tags {
		if mention := h.extractMentionFromInterface(tagInterface); mention != "" {
			mentions = append(mentions, mention)
		}
	}
	return mentions
}

// extractMentionFromInterface extracts a mention URL from an interface{} if it's a mention tag
func (h *Handler) extractMentionFromInterface(tagInterface any) string {
	tagMap, ok := tagInterface.(map[string]any)
	if !ok {
		return ""
	}
	
	tagType, ok := tagMap["type"].(string)
	if !ok || tagType != tagTypeMention {
		return ""
	}
	
	href, ok := tagMap["href"].(string)
	if !ok || href == "" {
		return ""
	}
	
	return href
}

// parseMentionsFromTagSlice extracts mentions from a slice of ActivityPub tags
func (h *Handler) parseMentionsFromTagSlice(tags []activitypub.Tag) []string {
	var mentions []string
	for _, tag := range tags {
		if tag.Type == tagTypeMention && tag.Href != "" {
			mentions = append(mentions, tag.Href)
		}
	}
	return mentions
}

// parseMentionsFromString extracts mentions from a JSON string of tags
func (h *Handler) parseMentionsFromString(tags string) []string {
	var tagSlice []activitypub.Tag
	if err := json.Unmarshal([]byte(tags), &tagSlice); err != nil {
		return nil
	}
	return h.parseMentionsFromTagSlice(tagSlice)
}

// deliverUpdateActivity delivers an Update activity to relevant recipients for federation
func (h *Handler) deliverUpdateActivity(ctx context.Context, updateActivity *activitypub.Activity, actor *activitypub.Actor, note *activitypub.Note) error {
	h.logger.Info("delivering update activity for federation",
		zap.String("activity_id", updateActivity.ID),
		zap.String("actor", actor.ID))

	// Determine delivery recipients based on the updated note
	recipients, err := h.determineUpdateDeliveryRecipients(ctx, actor, note)
	if err != nil {
		return fmt.Errorf("failed to determine delivery recipients: %w", err)
	}

	if len(recipients) == 0 {
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
			recipients[follower] = true
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
	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName())
	deliveryService := federation.NewDeliveryService(federationStorage)

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
func (h *Handler) deliverCreateActivity(ctx context.Context, createActivity *activitypub.Activity, actor *activitypub.Actor, visibility string) error {
	h.logger.Info("delivering create activity for federation",
		zap.String("activity_id", createActivity.ID),
		zap.String("actor", actor.ID),
		zap.String("visibility", visibility))

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName())
	deliveryService := federation.NewDeliveryService(federationStorage)

	// Deliver based on visibility
	switch visibility {
	case VisibilityPublic, VisibilityUnlisted:
		// Deliver to followers for public/unlisted content
		if err := deliveryService.DeliverToFollowers(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver to followers", zap.Error(err))
			// Continue to deliver to specific recipients
		}
		
		// Also deliver to specific recipients (mentions, replies, etc.)
		if err := deliveryService.DeliverToRecipients(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver to recipients", zap.Error(err))
			return err
		}

	case VisibilityPrivate:
		// Only deliver to followers
		if err := deliveryService.DeliverToFollowers(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver to followers", zap.Error(err))
			return err
		}

	case VisibilityDirect:
		// Use privacy-aware direct message delivery
		if err := deliveryService.DeliverDirectMessage(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver direct message", zap.Error(err))
			return err
		}

	default:
		h.logger.Info("no federation delivery for visibility",
			zap.String("visibility", visibility),
			zap.String("activity_id", createActivity.ID))
	}

	h.logger.Info("create activity federation delivery completed",
		zap.String("activity_id", createActivity.ID),
		zap.String("visibility", visibility))

	return nil
}

// getRequestingActorID extracts the actor ID from authentication context
func (h *Handler) getRequestingActorID(ctx *lift.Context) (string, error) {
	// Try to get authentication token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return "", fmt.Errorf("no authentication token")
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", err
	}

	// Get the actor
	account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		return "", err
	}
	actor := account.Actor

	return actor.ID, nil
}

// canActorSeeStatus checks if an actor can view a status based on visibility rules
func (h *Handler) canActorSeeStatus(status *models.Status, requestingActorID string) bool {
	switch status.Visibility {
	case VisibilityPublic, VisibilityUnlisted:
		return true
	case VisibilityPrivate:
		// Private posts are visible to author and followers
		if status.Account.ID == requestingActorID {
			return true
		}
		// Check if requesting actor is a follower
		return h.isFollower(requestingActorID, status.Account.ID)
	case VisibilityDirect:
		// Direct messages are only visible to author and mentioned users
		if status.Account.ID == requestingActorID {
			return true
		}
		// Check if requesting actor is mentioned
		return h.isMentioned(requestingActorID, status)
	default:
		return false
	}
}

// isFollower checks if one actor follows another
func (h *Handler) isFollower(followerID, followeeID string) bool {
	// Extract usernames from actor IDs for repository call
	followerUsername := h.extractUsernameFromActorID(followerID)
	followeeUsername := h.extractUsernameFromActorID(followeeID)
	
	if followerUsername == "" || followeeUsername == "" {
		return false
	}
	
	// Check if the relationship exists using GetRelationship
	relationship, err := h.registry.Relationships().GetRelationship(context.Background(), followerUsername, followeeUsername)
	if err != nil {
		h.logger.Warn("error checking follow relationship", zap.Error(err))
		return false
	}
	
	return relationship.Following
}

// isMentioned checks if an actor is mentioned in a status
func (h *Handler) isMentioned(actorID string, status *models.Status) bool {
	// Check if the actor is in the mentions
	for _, mention := range status.Mentions {
		if mention == actorID {
			return true
		}
	}
	return false
}

// extractUsernameFromActorID extracts username from an ActivityPub actor ID
func (h *Handler) extractUsernameFromActorID(actorID string) string {
	// Handle different actor ID formats
	// e.g., "https://example.com/users/username" -> "username"
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// canActorSeeStatusEnhanced performs enhanced visibility checking using Status model methods
func (h *Handler) canActorSeeStatusEnhanced(status *models.Status, requestingActorID string) bool {
	// Convert models.Status to our storage Status for visibility checks
	storageStatus := h.convertToStorageStatus(status)
	
	// Use the enhanced visibility checking from the Status model
	return storageStatus.IsVisibleTo(requestingActorID)
}

// sanitizeStatusForActor removes sensitive addressing information based on viewer's relationship
func (h *Handler) sanitizeStatusForActor(status *models.Status, viewerID string) *models.Status {
	// Convert to storage Status, sanitize, then convert back
	storageStatus := h.convertToStorageStatus(status)
	sanitized := storageStatus.SanitizeForActor(viewerID)
	return h.convertFromStorageStatus(sanitized)
}

// convertToStorageStatus converts API models.Status to storage models.Status
func (h *Handler) convertToStorageStatus(status *models.Status) *storageModels.Status {
	// Convert mentions from []any to []string
	mentions := make([]string, 0)
	if status.Mentions != nil {
		for _, mention := range status.Mentions {
			if str, ok := mention.(string); ok {
				mentions = append(mentions, str)
			}
		}
	}

	// Handle InReplyToID pointer
	inReplyToID := ""
	if status.InReplyToID != nil {
		inReplyToID = *status.InReplyToID
	}

	// Parse CreatedAt timestamp
	var createdAt time.Time
	if status.CreatedAt != "" {
		// Try to parse the timestamp - handle multiple potential formats
		if parsed, err := time.Parse(time.RFC3339, status.CreatedAt); err == nil {
			createdAt = parsed
		} else if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", status.CreatedAt); err == nil {
			createdAt = parsed  
		} else if parsed, err := time.Parse("2006-01-02T15:04:05Z", status.CreatedAt); err == nil {
			createdAt = parsed
		} else {
			// If parsing fails, use current time as fallback
			createdAt = time.Now()
		}
	} else {
		createdAt = time.Now()
	}

	// Extract addressing from status content or ActivityPub Note if available
	// For now, extract basic visibility-based addressing
	toRecipients := []string{}
	ccRecipients := []string{}
	btoRecipients := []string{}
	bccRecipients := []string{}

	// Set up basic addressing based on visibility
	switch status.Visibility {
	case "public":
		// Public posts go to the Public collection
		toRecipients = append(toRecipients, "https://www.w3.org/ns/activitystreams#Public")
		// CC followers collection if we have author info
		if status.Account.ID != "" {
			ccRecipients = append(ccRecipients, fmt.Sprintf("https://%s/users/%s/followers", 
				h.getDomainFromConfig(), status.Account.Username))
		}
	case "unlisted":
		// Unlisted posts CC the Public collection
		ccRecipients = append(ccRecipients, "https://www.w3.org/ns/activitystreams#Public")
		// TO followers collection
		if status.Account.ID != "" {
			toRecipients = append(toRecipients, fmt.Sprintf("https://%s/users/%s/followers",
				h.getDomainFromConfig(), status.Account.Username))
		}
	case "private":
		// Private posts only to followers
		if status.Account.ID != "" {
			toRecipients = append(toRecipients, fmt.Sprintf("https://%s/users/%s/followers",
				h.getDomainFromConfig(), status.Account.Username))
		}
	case VisibilityDirect:
		// Direct messages - recipients would be extracted from mentions
		// For now, we'll leave empty as we don't have specific recipient info in the API model
	}

	// Add mentioned users to recipients for direct messages
	if status.Visibility == VisibilityDirect && len(mentions) > 0 {
		toRecipients = append(toRecipients, mentions...)
	}

	return &storageModels.Status{
		StatusID:      status.ID,
		AuthorID:      status.Account.ID,
		Visibility:    status.Visibility,
		ToRecipients:  toRecipients,
		CcRecipients:  ccRecipients,
		BtoRecipients: btoRecipients,
		BccRecipients: bccRecipients,
		Mentions:      mentions,
		InReplyToID:   inReplyToID,
		CreatedAt:     createdAt,
	}
}

// getDomainFromConfig returns the domain from environment configuration
func (h *Handler) getDomainFromConfig() string {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		domain = os.Getenv("DOMAIN_NAME")
	}
	if domain == "" {
		domain = "localhost" // Default fallback for local development
	}
	return domain
}

// convertFromStorageStatus converts storage models.Status back to API models.Status
func (h *Handler) convertFromStorageStatus(storageStatus *storageModels.Status) *models.Status {
	// Convert mentions from []string to []any
	mentions := make([]any, len(storageStatus.Mentions))
	for i, mention := range storageStatus.Mentions {
		mentions[i] = mention
	}

	// Handle InReplyToID pointer
	var inReplyToID *string
	if storageStatus.InReplyToID != "" {
		inReplyToID = &storageStatus.InReplyToID
	}

	// This is a simplified conversion - in a real implementation you'd need
	// to preserve all the original status fields while updating the addressing fields
	return &models.Status{
		ID:          storageStatus.StatusID,
		Visibility:  storageStatus.Visibility,
		Mentions:    mentions,
		InReplyToID: inReplyToID,
		// Other fields would be preserved from the original status
	}
}



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
