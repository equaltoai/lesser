package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetStatusSource handles GET /api/v1/statuses/:id/source
func (h *Handler) HandleGetStatusSource(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
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
	case map[string]interface{}:
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
			return common.InternalServerError(fmt.Errorf("unexpected object type")), nil
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

	return common.OK(source), nil
}

// HandleGetStatusHistory handles GET /api/v1/statuses/:id/history
func (h *Handler) HandleGetStatusHistory(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Optional authentication for private status history
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
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
	currentObject, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Check if user has permission to view history
	var attributedTo string
	switch obj := currentObject.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]interface{}:
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
			actor, _ = h.store.GetActor(ctx, username)
		}
	}

	// Get edit history
	histories, err := h.store.GetUpdateHistory(ctx, objectID, 100) // Get up to 100 edits
	if err != nil {
		h.logger.Error("failed to get update history",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Return empty history if there's an error
		return common.OK([]models.StatusEdit{}), nil
	}

	// Build history response
	edits := make([]models.StatusEdit, 0, len(histories)+1)

	// Prepare account for edits
	var editAccount models.Account
	if actor != nil {
		editAccount = h.converter.ActorToAccount(actor)
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
		MediaAttachments: []interface{}{},
		Emojis:           []interface{}{},
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
	case map[string]interface{}:
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
			MediaAttachments: []interface{}{},
			Emojis:           []interface{}{},
		}

		// Parse previous state
		if history.PreviousState != "" {
			var previousObj map[string]interface{}
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

	return common.OK(edits), nil
}
