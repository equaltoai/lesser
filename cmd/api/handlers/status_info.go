package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Extract source information based on object type
	var source models.StatusSource
	switch obj := object.(type) {
	case *activitypub.Note:
		source = models.StatusSource{
			ID:          statusID,
			Text:        obj.Content,
			SpoilerText: obj.Summary,
		}
	case map[string]interface{}:
		// Extract from map
		if content, ok := obj["content"].(string); ok {
			source.Text = content
		}
		if summary, ok := obj["summary"].(string); ok {
			source.SpoilerText = summary
		}
		source.ID = statusID
	default:
		return common.InternalServerError(fmt.Errorf("unexpected object type")), nil
	}

	// Return source response
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

	// Add current version as the latest edit
	currentEdit := models.StatusEdit{
		CreatedAt:        time.Now().Format(time.RFC3339),
		Account:          h.converter.ActorToAccount(actor),
		Poll:             nil, // TODO: Add poll support
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
			Account:          h.converter.ActorToAccount(actor),
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
