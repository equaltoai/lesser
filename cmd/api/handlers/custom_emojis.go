package handlers

import (
	"encoding/json"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// HandleGetCustomEmojisLift handles GET /api/v1/custom_emojis
// This endpoint is public and doesn't require authentication
func (h *Handler) HandleGetCustomEmojisLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get emoji service
	emojiService := h.registry.Emoji()
	if emojiService == nil {
		h.logger.Error("emoji service not available")
		return common.RespondServiceUnavailable(ctx, "emoji")
	}

	// List all visible emojis
	result, err := emojiService.ListEmojis(ctx.Context(), &emoji.ListEmojisQuery{
		OnlyVisible:     true,
		IncludeDisabled: false,
	})
	if err != nil {
		h.logger.Error("failed to get custom emojis", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Convert to API format
	apiEmojis := make([]models.CustomEmoji, 0, len(result.Emojis))
	for _, emoji := range result.Emojis {
		apiEmoji := models.CustomEmoji{
			Shortcode:       emoji.Shortcode,
			URL:             emoji.URL,
			StaticURL:       emoji.StaticURL,
			VisibleInPicker: emoji.VisibleInPicker,
			Category:        emoji.Category,
		}
		apiEmojis = append(apiEmojis, apiEmoji)
	}

	return okJSON(apiEmojis)
}

// HandleCreateCustomEmojiLift handles POST /api/v1/admin/custom_emojis (admin only)
func (h *Handler) HandleCreateCustomEmojiLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Require admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		if err.Error() == common.ErrorAdminAccessRequired {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse request
	var req models.CreateCustomEmojiRequest
	if err := h.parseEmojiRequest(ctx, &req); err != nil {
		return common.RespondInvalidRequest(ctx)
	}

	// Validate required fields
	if err := common.ValidateRequiredParam("shortcode", req.Shortcode); err != nil {
		return common.RespondUnprocessableEntity(ctx, err.Error())
	}
	if err := common.ValidateRequiredParam("url", req.URL); err != nil {
		return common.RespondUnprocessableEntity(ctx, err.Error())
	}

	// Get emoji service
	emojiService := h.registry.Emoji()
	if emojiService == nil {
		h.logger.Error("emoji service not available")
		return common.RespondServiceUnavailable(ctx, "emoji")
	}

	// Create emoji using service
	result, err := emojiService.CreateEmoji(ctx.Context(), &emoji.CreateEmojiCommand{
		Shortcode:       req.Shortcode,
		ImageURL:        req.URL,
		Category:        req.Category,
		VisibleInPicker: true, // Default to visible
	})
	if err != nil {
		h.logger.Error("failed to create custom emoji",
			zap.String("shortcode", req.Shortcode),
			zap.Error(err))
		// Check for specific error types
		if err.Error() == "emoji with shortcode "+req.Shortcode+" already exists" {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		return common.RespondInternalServerError(ctx)
	}

	// Return the created emoji
	apiEmoji := models.CustomEmoji{
		Shortcode:       result.Emoji.Shortcode,
		URL:             result.Emoji.URL,
		StaticURL:       result.Emoji.StaticURL,
		VisibleInPicker: result.Emoji.VisibleInPicker,
		Category:        result.Emoji.Category,
	}

	return okJSON(apiEmoji)
}

// HandleUpdateCustomEmojiLift handles PUT /api/v1/admin/custom_emojis/:shortcode (admin only)
func (h *Handler) HandleUpdateCustomEmojiLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get shortcode from path parameter
	shortcode := ctx.Param("shortcode")
	if err := common.ValidateRequiredParam("shortcode", shortcode); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Require admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		if err.Error() == common.ErrorAdminAccessRequired {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse request
	var req models.UpdateCustomEmojiRequest
	if err := h.parseEmojiRequest(ctx, &req); err != nil {
		return common.RespondInvalidRequest(ctx)
	}

	// Get emoji service
	emojiService := h.registry.Emoji()
	if emojiService == nil {
		h.logger.Error("emoji service not available")
		return common.RespondServiceUnavailable(ctx, "emoji")
	}

	// Update emoji using service
	result, err := emojiService.UpdateEmoji(ctx.Context(), &emoji.UpdateEmojiCommand{
		Shortcode:       shortcode,
		Category:        req.Category,
		VisibleInPicker: req.VisibleInPicker,
		Disabled:        req.Disabled,
	})
	if err != nil {
		h.logger.Error("failed to update custom emoji",
			zap.String("shortcode", shortcode),
			zap.Error(err))
		// Check for specific error types
		if err.Error() == "emoji not found: "+shortcode {
			return common.RespondNotFound(ctx, "custom emoji not found")
		}
		return common.RespondInternalServerError(ctx)
	}

	// Return the updated emoji
	apiEmoji := models.CustomEmoji{
		Shortcode:       result.Emoji.Shortcode,
		URL:             result.Emoji.URL,
		StaticURL:       result.Emoji.StaticURL,
		VisibleInPicker: result.Emoji.VisibleInPicker,
		Category:        result.Emoji.Category,
	}

	return okJSON(apiEmoji)
}

// HandleDeleteCustomEmojiLift handles DELETE /api/v1/admin/custom_emojis/:shortcode (admin only)
func (h *Handler) HandleDeleteCustomEmojiLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get shortcode from path parameter
	shortcode := ctx.Param("shortcode")
	if err := common.ValidateRequiredParam("shortcode", shortcode); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Require admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		if err.Error() == common.ErrorAdminAccessRequired {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get emoji service
	emojiService := h.registry.Emoji()
	if emojiService == nil {
		h.logger.Error("emoji service not available")
		return common.RespondServiceUnavailable(ctx, "emoji")
	}

	// Delete emoji using service
	err = emojiService.DeleteEmoji(ctx.Context(), &emoji.DeleteEmojiCommand{
		Shortcode: shortcode,
	})
	if err != nil {
		h.logger.Error("failed to delete custom emoji",
			zap.String("shortcode", shortcode),
			zap.Error(err))
		// Check for specific error types
		if err.Error() == "emoji not found: "+shortcode {
			return common.RespondNotFound(ctx, "custom emoji not found")
		}
		return common.RespondInternalServerError(ctx)
	}

	// Return empty object
	return okJSON(models.EmptyObject{})
}

// Helper methods

// parseEmojiRequest parses emoji request with fallback for test environments
func (h *Handler) parseEmojiRequest(ctx *apptheory.Context, req interface{}) error {
	if err := common.ParseRequestWithFallback(ctx, req); err != nil {
		// Fallback for test environment - try parsing directly from request body
		if len(ctx.Request.Body) > 0 {
			if jsonErr := json.Unmarshal(ctx.Request.Body, req); jsonErr != nil {
				h.logger.Debug("invalid emoji request",
					zap.Error(err),
					zap.Error(jsonErr))
				return jsonErr
			}
			return nil
		}
		return err
	}
	return nil
}
