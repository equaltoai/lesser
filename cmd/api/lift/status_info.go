package lift

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetStatusSourceLift handles GET /api/v1/statuses/:id/source
func (h *Handler) HandleGetStatusSourceLift(ctx *lift.Context) error {
	// Extract status ID from URL parameter
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return ctx.Status(400).JSON(map[string]string{"error": err.Error()})
	}

	// Support test mode with X-Test-Username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("status source request in test mode",
			zap.String("test_username", testUsername),
			zap.String("status_id", statusID))
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Extract status ID from object ID
	statusID = strings.TrimPrefix(objectID, h.cfg.BaseURL()+"/objects/")

	// Get the note using Notes service
	result, err := h.registry.Notes().GetNote(ctx.Context, statusID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}
	object := result.Note

	// Debug logging to see what type we're getting
	h.logger.Info("GetStatusSource: object type info",
		zap.String("status_id", statusID),
		zap.String("object_id", objectID),
		zap.String("type", "activitypub.Note"),
	)

	// Extract content from the Note
	content := object.Content
	spoilerText := object.Summary

	// For source endpoint, we return the raw content without stripping HTML
	// The source should show the original markdown/plain text

	// Return source
	source := &models.StatusSource{
		ID:          statusID,
		Text:        content,
		SpoilerText: spoilerText,
	}

	return ctx.JSON(source)
}

// HandleGetStatusHistoryLift handles GET /api/v1/statuses/:id/history
func (h *Handler) HandleGetStatusHistoryLift(ctx *lift.Context) error {
	// Extract and validate status ID
	statusID, err := h.extractStatusIDForHistory(ctx)
	if err != nil {
		return err
	}

	// Perform optional authentication
	h.performOptionalHistoryAuth(ctx, statusID)

	// Normalize and fetch the current object
	objectID := h.normalizeStatusIDForHistory(statusID)
	currentObject, err := h.fetchObjectForHistory(ctx, objectID)
	if err != nil {
		return err
	}

	// Get the author actor
	actor := h.getHistoryAuthorActor(ctx, currentObject)

	// Get edit history
	histories := h.fetchEditHistory(ctx, objectID)

	// Build history response
	edits := h.buildHistoryResponse(currentObject, actor, histories)

	return ctx.JSON(edits)
}

// extractStatusIDForHistory extracts and validates the status ID parameter
func (h *Handler) extractStatusIDForHistory(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return "", ctx.Status(400).JSON(map[string]string{"error": err.Error()})
	}
	return statusID, nil
}

// performOptionalHistoryAuth performs optional authentication for private status history
func (h *Handler) performOptionalHistoryAuth(ctx *lift.Context, statusID string) {
	// Support test mode with X-Test-Username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("status history request in test mode",
			zap.String("test_username", testUsername),
			zap.String("status_id", statusID))
	}

	// Optional authentication
	authHeader := h.extractHistoryAuthHeader(ctx)
	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
			_, _ = oauthSvc.ValidateAccessToken(token)
		}
	}
}

// extractHistoryAuthHeader extracts the authorization header
func (h *Handler) extractHistoryAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}
	return authHeader
}

// normalizeStatusIDForHistory normalizes the status ID to a full URL
func (h *Handler) normalizeStatusIDForHistory(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// fetchObjectForHistory fetches the current object
func (h *Handler) fetchObjectForHistory(ctx *lift.Context, objectID string) (any, error) {
	// Extract status ID from object ID
	statusID := strings.TrimPrefix(objectID, h.cfg.BaseURL()+"/objects/")

	// Get the note using Notes service
	result, err := h.registry.Notes().GetNote(ctx.Context, statusID)
	if err != nil {
		return nil, ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}
	return result.Note, nil
}

// getHistoryAuthorActor gets the author actor for the status
func (h *Handler) getHistoryAuthorActor(ctx *lift.Context, currentObject any) *activitypub.Actor {
	attributedTo := h.extractAttributedTo(currentObject)
	if err := common.ValidateRequiredParam("attributedTo", attributedTo); err != nil {
		return nil
	}

	// Extract username from actor ID
	parts := strings.Split(attributedTo, "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
		username := parts[len(parts)-1]
		result, _ := h.registry.Accounts().GetAccount(ctx.Context, username)
		if result != nil {
			return result.Actor
		}
	}
	return nil
}

// extractAttributedTo extracts the attributedTo field from an object
func (h *Handler) extractAttributedTo(obj any) string {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.AttributedTo
	case map[string]any:
		if attr, ok := o["attributedTo"].(string); ok {
			return attr
		}
	}
	return ""
}

// fetchEditHistory fetches the edit history for an object
func (h *Handler) fetchEditHistory(ctx *lift.Context, objectID string) []*storage.UpdateHistory {
	// Extract status ID from object ID
	statusID := strings.TrimPrefix(objectID, h.cfg.BaseURL()+"/objects/")

	// Get update history using Notes service
	result, err := h.registry.Notes().GetUpdateHistory(ctx.Context, &notes.GetUpdateHistoryQuery{
		StatusID: statusID,
		Limit:    100,
	})
	if err != nil {
		h.logger.Error("failed to get update history",
			zap.String("status_id", statusID),
			zap.Error(err))
		return []*storage.UpdateHistory{}
	}
	return result.History
}

// buildHistoryResponse builds the history response
func (h *Handler) buildHistoryResponse(currentObject any, actor *activitypub.Actor, histories []*storage.UpdateHistory) []models.StatusEdit {
	edits := make([]models.StatusEdit, 0, len(histories)+1)

	// Prepare account for edits
	editAccount := h.prepareEditAccount(actor)

	// Add current version as the latest edit
	currentEdit := h.buildCurrentEdit(currentObject, editAccount)
	edits = append(edits, currentEdit)

	// Add historical versions
	for _, history := range histories {
		edit := h.buildHistoricalEdit(history, editAccount)
		edits = append(edits, edit)
	}

	return edits
}

// prepareEditAccount prepares the account object for edits
func (h *Handler) prepareEditAccount(actor *activitypub.Actor) models.Account {
	if actor != nil {
		return transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
	}

	// Create a minimal account for unknown actors using transformation framework - ELIMINATES 4+ LINES OF DUPLICATE CODE
	unknownActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "unknown",
			Type: "Person",
		},
		PreferredUsername: "unknown",
		Name:              "unknown",
		URL:               "",
	}
	
	return transformations.ActorToAccountBase(unknownActor, h.cfg.BaseURL())
}

// buildCurrentEdit builds the edit object for the current version
func (h *Handler) buildCurrentEdit(currentObject any, editAccount models.Account) models.StatusEdit {
	currentEdit := models.StatusEdit{
		CreatedAt:        time.Now().Format(time.RFC3339),
		Account:          editAccount,
		Poll:             nil, // Polls in status history not currently supported
		MediaAttachments: []any{},
		Emojis:           []any{},
	}

	// Extract current content
	h.extractEditContent(currentObject, &currentEdit)

	return currentEdit
}

// extractEditContent extracts content from an object into an edit
func (h *Handler) extractEditContent(obj any, edit *models.StatusEdit) {
	switch o := obj.(type) {
	case *activitypub.Note:
		h.extractNoteContent(o, edit)
	case map[string]any:
		h.extractMapContent(o, edit)
	}
}

// extractNoteContent extracts content from a Note object
func (h *Handler) extractNoteContent(note *activitypub.Note, edit *models.StatusEdit) {
	edit.Content = note.Content
	edit.SpoilerText = note.Summary
	edit.Sensitive = note.Sensitive
	if note.Updated != nil {
		edit.CreatedAt = note.Updated.Format(time.RFC3339)
	} else if note.Published != nil {
		edit.CreatedAt = note.Published.Format(time.RFC3339)
	}
}

// extractMapContent extracts content from a map object
func (h *Handler) extractMapContent(obj map[string]any, edit *models.StatusEdit) {
	if content, ok := obj["content"].(string); ok {
		edit.Content = content
	}
	if summary, ok := obj["summary"].(string); ok {
		edit.SpoilerText = summary
	}
	if sensitive, ok := obj["sensitive"].(bool); ok {
		edit.Sensitive = sensitive
	}
	if updated, ok := obj["updated"].(string); ok {
		edit.CreatedAt = updated
	} else if published, ok := obj["published"].(string); ok {
		edit.CreatedAt = published
	}
}

// buildHistoricalEdit builds an edit object from history
func (h *Handler) buildHistoricalEdit(history *storage.UpdateHistory, editAccount models.Account) models.StatusEdit {
	edit := models.StatusEdit{
		CreatedAt:        history.UpdatedAt.Format(time.RFC3339),
		Account:          editAccount,
		MediaAttachments: []any{},
		Emojis:           []any{},
	}

	// Parse previous state
	if len(history.PreviousState) > 0 {
		h.extractPreviousState(history.PreviousState, &edit)
	}

	return edit
}

// extractPreviousState extracts content from previous state
func (h *Handler) extractPreviousState(previousObj map[string]any, edit *models.StatusEdit) {
	if content, ok := previousObj["content"].(string); ok {
		edit.Content = content
	}
	if summary, ok := previousObj["summary"].(string); ok {
		edit.SpoilerText = summary
	}
	if sensitive, ok := previousObj["sensitive"].(bool); ok {
		edit.Sensitive = sensitive
	}
}
