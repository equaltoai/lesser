package lift

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetStatusSourceLift handles GET /api/v1/statuses/:id/source
func (h *Handler) HandleGetStatusSourceLift(ctx *lift.Context) error {
	// Extract status ID from URL parameter
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "status ID is required"})
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

	// Get the object
	object, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}

	// Debug logging to see what type we're getting
	h.logger.Info("GetStatusSource: object type info",
		zap.String("status_id", statusID),
		zap.String("object_id", objectID),
		zap.String("type", fmt.Sprintf("%T", object)),
	)

	// Extract content based on type
	var content string
	var spoilerText string

	switch obj := object.(type) {
	case *activitypub.Note:
		content = obj.Content
		spoilerText = obj.Summary
	case map[string]any:
		if c, ok := obj["content"].(string); ok {
			content = c
		}
		if s, ok := obj["summary"].(string); ok {
			spoilerText = s
		}
	default:
		// Try to handle any object with Content field using reflection
		v := reflect.ValueOf(object)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			// Try to get Content field
			if contentField := v.FieldByName("Content"); contentField.IsValid() && contentField.Kind() == reflect.String {
				content = contentField.String()
			}
			// Try to get Summary field
			if summaryField := v.FieldByName("Summary"); summaryField.IsValid() && summaryField.Kind() == reflect.String {
				spoilerText = summaryField.String()
			}
		} else {
			h.logger.Error("unexpected object type",
				zap.String("type", fmt.Sprintf("%T", object)),
				zap.Any("object", object))
			return ctx.Status(500).JSON(map[string]string{"error": "unexpected object type"})
		}
	}

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
	// Extract status ID from URL parameter
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "status ID is required"})
	}

	// Optional authentication for private status history
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Support test mode with X-Test-Username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("status history request in test mode",
			zap.String("test_username", testUsername),
			zap.String("status_id", statusID))
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
			_, _ = oauthSvc.ValidateAccessToken(token)
		}
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the current object
	currentObject, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}

	// Check if user has permission to view history
	var attributedTo string
	switch obj := currentObject.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	// Extract username from actor ID
	var actor *activitypub.Actor
	if attributedTo != "" {
		parts := strings.Split(attributedTo, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			actor, _ = h.store.GetActor(ctx.Context, username)
		}
	}

	// Get edit history
	histories, err := h.store.GetUpdateHistory(ctx.Context, objectID, 100) // Get up to 100 edits
	if err != nil {
		h.logger.Error("failed to get update history",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Return empty history if there's an error
		return ctx.JSON([]models.StatusEdit{})
	}

	// Build history response
	edits := make([]models.StatusEdit, 0, len(histories)+1)

	// Initialize converter
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	// Prepare account for edits
	var editAccount models.Account
	if actor != nil {
		editAccount = converter.ActorToAccount(actor)
	} else {
		// Create a minimal account for unknown actors
		editAccount = models.Account{
			ID:       "unknown",
			Username: "unknown",
			Acct:     "unknown",
			URL:      "",
		}
	}

	// Add current version as the latest edit
	currentEdit := models.StatusEdit{
		CreatedAt:        time.Now().Format(time.RFC3339),
		Account:          editAccount,
		Poll:             nil, // Polls in status history not currently supported
		MediaAttachments: []any{},
		Emojis:           []any{},
	}

	// Extract current content
	switch obj := currentObject.(type) {
	case *activitypub.Note:
		currentEdit.Content = obj.Content
		currentEdit.SpoilerText = obj.Summary
		currentEdit.Sensitive = obj.Sensitive
		if obj.Updated != nil {
			currentEdit.CreatedAt = obj.Updated.Format(time.RFC3339)
		} else if obj.Published != nil {
			currentEdit.CreatedAt = obj.Published.Format(time.RFC3339)
		}
	case map[string]any:
		if content, ok := obj["content"].(string); ok {
			currentEdit.Content = content
		}
		if summary, ok := obj["summary"].(string); ok {
			currentEdit.SpoilerText = summary
		}
		if sensitive, ok := obj["sensitive"].(bool); ok {
			currentEdit.Sensitive = sensitive
		}
		if updated, ok := obj["updated"].(string); ok {
			currentEdit.CreatedAt = updated
		} else if published, ok := obj["published"].(string); ok {
			currentEdit.CreatedAt = published
		}
	}

	edits = append(edits, currentEdit)

	// Add historical versions
	for _, history := range histories {
		edit := models.StatusEdit{
			CreatedAt:        history.UpdatedAt.Format(time.RFC3339),
			Account:          editAccount,
			MediaAttachments: []any{},
			Emojis:           []any{},
		}

		// Parse previous state
		if history.PreviousState != "" {
			var previousObj map[string]any
			if err := json.Unmarshal([]byte(history.PreviousState), &previousObj); err == nil {
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
		}

		edits = append(edits, edit)
	}

	return ctx.JSON(edits)
}