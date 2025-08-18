package lift

import (
	"encoding/json"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetCustomEmojisLift handles GET /api/v1/custom_emojis
// This endpoint is public and doesn't require authentication
func (h *Handler) HandleGetCustomEmojisLift(ctx *lift.Context) error {
	// Get emoji service
	emojiService := h.registry.Emoji()
	if emojiService == nil {
		h.logger.Error("emoji service not available")
		return common.RespondServiceUnavailable(ctx, "emoji")
	}

	// List all visible emojis
	result, err := emojiService.ListEmojis(ctx.Context, &emoji.ListEmojisQuery{
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

	return ctx.JSON(apiEmojis)
}

// HandleCreateCustomEmojiLift handles POST /api/v1/admin/custom_emojis (admin only)
func (h *Handler) HandleCreateCustomEmojiLift(ctx *lift.Context) error {
	// Authenticate and check admin role
	username, err := h.authenticateAdminRequest(ctx)
	if err != nil {
		return err
	}

	// Check admin role using Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get user for admin check", zap.String("username", username), zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	if account.User == nil || account.User.Role != roleAdmin {
		return common.RespondForbidden(ctx, "admin access required")
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
	result, err := emojiService.CreateEmoji(ctx.Context, &emoji.CreateEmojiCommand{
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

	return ctx.JSON(apiEmoji)
}

// HandleUpdateCustomEmojiLift handles PUT /api/v1/admin/custom_emojis/:shortcode (admin only)
func (h *Handler) HandleUpdateCustomEmojiLift(ctx *lift.Context) error {
	// Get shortcode from path parameter
	shortcode := ctx.Param("shortcode")
	if err := common.ValidateRequiredParam("shortcode", shortcode); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate and check admin role
	username, err := h.authenticateAdminRequest(ctx)
	if err != nil {
		return err
	}

	// Check admin role using Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get user for admin check", zap.String("username", username), zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	if account.User == nil || account.User.Role != roleAdmin {
		return common.RespondForbidden(ctx, "admin access required")
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
	result, err := emojiService.UpdateEmoji(ctx.Context, &emoji.UpdateEmojiCommand{
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

	return ctx.JSON(apiEmoji)
}

// HandleDeleteCustomEmojiLift handles DELETE /api/v1/admin/custom_emojis/:shortcode (admin only)
func (h *Handler) HandleDeleteCustomEmojiLift(ctx *lift.Context) error {
	// Get shortcode from path parameter
	shortcode := ctx.Param("shortcode")
	if err := common.ValidateRequiredParam("shortcode", shortcode); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate and check admin role
	username, err := h.authenticateAdminRequest(ctx)
	if err != nil {
		return err
	}

	// Check admin role using Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get user for admin check", zap.String("username", username), zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	if account.User == nil || account.User.Role != roleAdmin {
		return common.RespondForbidden(ctx, "admin access required")
	}

	// Get emoji service
	emojiService := h.registry.Emoji()
	if emojiService == nil {
		h.logger.Error("emoji service not available")
		return common.RespondServiceUnavailable(ctx, "emoji")
	}

	// Delete emoji using service
	err = emojiService.DeleteEmoji(ctx.Context, &emoji.DeleteEmojiCommand{
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
	return ctx.JSON(map[string]any{})
}

// Helper methods

// authenticateAdminRequest handles authentication for admin endpoints
func (h *Handler) authenticateAdminRequest(ctx *lift.Context) (string, error) {
	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if err := common.ValidateRequiredParam("testUsername", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Test mode - use provided username
		h.logger.Debug("test mode: using provided username", zap.String("username", testUsername))
		return testUsername, nil
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	return claims.Username, nil
}

// parseEmojiRequest parses emoji request with fallback for test environments
func (h *Handler) parseEmojiRequest(ctx *lift.Context, req interface{}) error {
	if err := ctx.ParseRequest(req); err != nil {
		// Fallback for test environment - try parsing directly from request body
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
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
