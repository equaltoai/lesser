// statuses_v2.go - Service-first implementation of status endpoints
// This file demonstrates the Phase 3 approach: direct replacement using services

package lift

import (
	"fmt"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleCreateStatusV2 creates a new status
// This is a simplified version that demonstrates the service-first approach
func (h *Handler) HandleCreateStatusV2(ctx *lift.Context) error {
	// For now, we'll create a simple status directly
	// In the full implementation, this would go through the notes service
	
	var req struct {
		Status     string `json:"status"`
		Visibility string `json:"visibility"`
	}
	
	// Parse request - Lift uses JSON binding
	if err := ctx.JSON(&req); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "invalid request",
		})
	}
	
	// Default visibility
	if req.Visibility == "" {
		req.Visibility = "public"
	}
	
	// Create a simple response
	statusID := fmt.Sprintf("%d", time.Now().UnixNano())
	
	resp := map[string]interface{}{
		"id":         statusID,
		"content":    req.Status,
		"visibility": req.Visibility,
		"created_at": time.Now().Format(time.RFC3339),
		"account": map[string]interface{}{
			"id":       "1",
			"username": "testuser",
		},
		"media_attachments": []interface{}{},
		"mentions":          []interface{}{},
		"tags":              []interface{}{},
		"emojis":            []interface{}{},
	}
	
	h.logger.Info("created status (v2)", 
		zap.String("id", statusID),
		zap.String("content", req.Status))
	
	return ctx.Status(http.StatusCreated).JSON(resp)
}

// HandleGetStatusV2 retrieves a status by ID
func (h *Handler) HandleGetStatusV2(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "missing status id",
		})
	}
	
	// For now, return a mock status
	resp := map[string]interface{}{
		"id":         statusID,
		"content":    "This is a test status",
		"visibility": "public",
		"created_at": time.Now().Format(time.RFC3339),
		"account": map[string]interface{}{
			"id":       "1",
			"username": "testuser",
		},
		"media_attachments": []interface{}{},
		"mentions":          []interface{}{},
		"tags":              []interface{}{},
		"emojis":            []interface{}{},
	}
	
	return ctx.JSON(resp)
}

// HandleGetHomeTimelineV2 returns the home timeline
func (h *Handler) HandleGetHomeTimelineV2(ctx *lift.Context) error {
	// For now, return an empty timeline
	timeline := []interface{}{}
	
	// In a real implementation, this would:
	// 1. Authenticate the user
	// 2. Call the notes service to get their timeline
	// 3. Convert to Mastodon API format
	
	return ctx.JSON(timeline)
}

// HandleGetPublicTimelineV2 returns the public timeline
func (h *Handler) HandleGetPublicTimelineV2(ctx *lift.Context) error {
	// For now, return a sample timeline with one status
	timeline := []interface{}{
		map[string]interface{}{
			"id":         "123456789",
			"content":    "Welcome to Lesser!",
			"visibility": "public",
			"created_at": time.Now().Format(time.RFC3339),
			"account": map[string]interface{}{
				"id":       "1",
				"username": "admin",
			},
			"media_attachments": []interface{}{},
			"mentions":          []interface{}{},
			"tags":              []interface{}{},
			"emojis":            []interface{}{},
		},
	}
	
	return ctx.JSON(timeline)
}

// HandleDeleteStatusV2 deletes a status
func (h *Handler) HandleDeleteStatusV2(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "missing status id",
		})
	}
	
	// For now, return the "deleted" status
	resp := map[string]interface{}{
		"id":         statusID,
		"content":    "This status was deleted",
		"visibility": "public",
		"created_at": time.Now().Format(time.RFC3339),
		"account": map[string]interface{}{
			"id":       "1",
			"username": "testuser",
		},
	}
	
	h.logger.Info("deleted status (v2)", zap.String("id", statusID))
	
	return ctx.JSON(resp)
}

// Additional helper methods for the V2 implementation

// convertModelToAPIV2 converts a storage model to Mastodon API format
// Note: This is kept for future use when we integrate with the actual storage layer
// nolint:unused
func (h *Handler) convertModelToAPIV2(status *models.Status) map[string]interface{} {
	return map[string]interface{}{
		"id":                     status.StatusID,
		"created_at":             status.PublishedAt.Format(time.RFC3339),
		"in_reply_to_id":         nil,
		"in_reply_to_account_id": nil,
		"sensitive":              status.Sensitive,
		"spoiler_text":           "",
		"visibility":             status.Visibility,
		"language":               status.Language,
		"uri":                    fmt.Sprintf("https://example.com/statuses/%s", status.StatusID),
		"url":                    fmt.Sprintf("https://example.com/@%s/%s", status.AuthorUsername, status.StatusID),
		"replies_count":          0,
		"reblogs_count":          0,
		"favourites_count":       0,
		"content":                status.Content,
		"reblog":                 nil,
		"account": map[string]interface{}{
			"id":       status.AuthorID,
			"username": status.AuthorUsername,
		},
		"media_attachments": []interface{}{},
		"mentions":          []interface{}{},
		"tags":              []interface{}{},
		"emojis":            []interface{}{},
		"card":              nil,
		"poll":              nil,
	}
}