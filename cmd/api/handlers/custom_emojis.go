package handlers

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetCustomEmojis handles GET /api/v1/custom_emojis
func (h *Handler) HandleGetCustomEmojis(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// This endpoint is public and doesn't require authentication

	// Get all custom emojis
	emojis, err := h.store.GetCustomEmojis(ctx)
	if err != nil {
		h.logger.Error("failed to get custom emojis", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get custom emojis")), nil
	}

	// Convert to API format
	apiEmojis := make([]models.CustomEmoji, 0, len(emojis))
	for _, emoji := range emojis {
		// Only show emojis that are visible in picker and not disabled (for local emojis)
		if !emoji.VisibleInPicker && emoji.Domain == "" {
			continue
		}

		apiEmoji := models.CustomEmoji{
			Shortcode:       emoji.Shortcode,
			URL:             emoji.URL,
			StaticURL:       emoji.StaticURL,
			VisibleInPicker: emoji.VisibleInPicker,
			Category:        emoji.Category,
		}

		apiEmojis = append(apiEmojis, apiEmoji)
	}

	return common.OK(apiEmojis), nil
}

// Admin endpoints for managing custom emojis would go here
// These would require admin authentication

// HandleCreateCustomEmoji handles POST /api/v1/admin/custom_emojis (admin only)
func (h *Handler) HandleCreateCustomEmoji(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin role
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || user.Role != "admin" {
		return common.Forbidden(fmt.Errorf("admin access required")), nil
	}

	// Parse request
	var req models.CreateCustomEmojiRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate required fields
	if req.Shortcode == "" || req.URL == "" {
		return common.UnprocessableEntity(fmt.Errorf("shortcode and url are required")), nil
	}

	// Create custom emoji
	emoji := &storage.CustomEmoji{
		Shortcode:       req.Shortcode,
		URL:             req.URL,
		StaticURL:       req.StaticURL,
		VisibleInPicker: true, // Default to visible
		Category:        req.Category,
		Domain:          "", // Local emoji
	}

	// If static URL is not provided, use the main URL
	if emoji.StaticURL == "" {
		emoji.StaticURL = emoji.URL
	}

	// Create the emoji
	if err := h.store.CreateCustomEmoji(ctx, emoji); err != nil {
		if err == storage.ErrAlreadyExists {
			return common.UnprocessableEntity(fmt.Errorf("emoji with shortcode %s already exists", req.Shortcode)), nil
		}
		h.logger.Error("failed to create custom emoji",
			zap.String("shortcode", req.Shortcode),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create custom emoji")), nil
	}

	// Return the created emoji
	apiEmoji := models.CustomEmoji{
		Shortcode:       emoji.Shortcode,
		URL:             emoji.URL,
		StaticURL:       emoji.StaticURL,
		VisibleInPicker: emoji.VisibleInPicker,
		Category:        emoji.Category,
	}

	return common.OK(apiEmoji), nil
}

// HandleUpdateCustomEmoji handles PUT /api/v1/admin/custom_emojis/:shortcode (admin only)
func (h *Handler) HandleUpdateCustomEmoji(ctx context.Context, request events.APIGatewayV2HTTPRequest, shortcode string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin role
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || user.Role != "admin" {
		return common.Forbidden(fmt.Errorf("admin access required")), nil
	}

	// Get existing emoji
	emoji, err := h.store.GetCustomEmoji(ctx, shortcode)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("custom emoji not found")), nil
		}
		h.logger.Error("failed to get custom emoji",
			zap.String("shortcode", shortcode),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get custom emoji")), nil
	}

	// Parse request
	var req models.UpdateCustomEmojiRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Update fields if provided
	if req.Category != nil {
		emoji.Category = *req.Category
	}
	if req.VisibleInPicker != nil {
		emoji.VisibleInPicker = *req.VisibleInPicker
	}
	if req.Disabled != nil {
		emoji.Disabled = *req.Disabled
	}

	// Update the emoji
	if err := h.store.UpdateCustomEmoji(ctx, emoji); err != nil {
		h.logger.Error("failed to update custom emoji",
			zap.String("shortcode", shortcode),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to update custom emoji")), nil
	}

	// Return the updated emoji
	apiEmoji := models.CustomEmoji{
		Shortcode:       emoji.Shortcode,
		URL:             emoji.URL,
		StaticURL:       emoji.StaticURL,
		VisibleInPicker: emoji.VisibleInPicker,
		Category:        emoji.Category,
	}

	return common.OK(apiEmoji), nil
}

// HandleDeleteCustomEmoji handles DELETE /api/v1/admin/custom_emojis/:shortcode (admin only)
func (h *Handler) HandleDeleteCustomEmoji(ctx context.Context, request events.APIGatewayV2HTTPRequest, shortcode string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin role
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || user.Role != "admin" {
		return common.Forbidden(fmt.Errorf("admin access required")), nil
	}

	// Delete the emoji
	if err := h.store.DeleteCustomEmoji(ctx, shortcode); err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("custom emoji not found")), nil
		}
		h.logger.Error("failed to delete custom emoji",
			zap.String("shortcode", shortcode),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to delete custom emoji")), nil
	}

	// Return empty object
	return common.OK(map[string]any{}), nil
}
