package handlers

import (
	"context"
	"errors"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetPreferences handles GET /api/v1/preferences
func (h *Handler) HandleGetPreferences(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get user preferences from storage
	prefs, err := h.store.GetUserPreferences(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get user preferences", zap.Error(err))
		// Return defaults if preferences don't exist
		return common.OK(models.Preferences{
			PostingDefaultVisibility: "public",
			PostingDefaultSensitive:  false,
			PostingDefaultLanguage:   "en",
			ReadingExpandMedia:       "default",
			ReadingExpandSpoilers:    false,
			ReadingAutoplayGifs:      true,
		}), nil
	}

	// Convert to Mastodon format
	preferences := models.Preferences{
		PostingDefaultVisibility: prefs.DefaultPostingVisibility,
		PostingDefaultSensitive:  prefs.DefaultMediaSensitive,
		PostingDefaultLanguage:   prefs.Language,
		ReadingExpandMedia:       "default", // TODO: Map from preferences
		ReadingExpandSpoilers:    prefs.ExpandSpoilers,
		ReadingAutoplayGifs:      true, // TODO: Add to storage preferences
	}

	return common.OK(preferences), nil
}

// HandleUpdatePreferences handles PATCH /api/v1/preferences
func (h *Handler) HandleUpdatePreferences(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request body - using map to handle partial updates
	var updateReq map[string]interface{}
	if err := common.ParseRequestBody([]byte(request.Body), &updateReq); err != nil {
		return common.BadRequest(err), nil
	}

	// Get existing preferences
	prefs, err := h.store.GetUserPreferences(ctx, claims.Username)
	if err != nil {
		h.logger.Warn("failed to get existing preferences, using defaults", zap.Error(err))
		// Start with defaults if preferences don't exist
		prefs = &storage.UserPreferences{
			Language:                  "en",
			DefaultPostingVisibility:  "public",
			DefaultMediaSensitive:     false,
			ExpandSpoilers:            false,
			ShowFollowCounts:          true,
			PreferredTimelineOrder:    "newest",
			SearchSuggestionsEnabled:  true,
			PersonalizedSearchEnabled: true,
		}
	}

	// Update preferences based on request
	// Mastodon preference keys use colons, so we need to map them
	if val, ok := updateReq["posting:default:visibility"]; ok {
		if strVal, ok := val.(string); ok {
			prefs.DefaultPostingVisibility = strVal
		}
	}
	if val, ok := updateReq["posting:default:sensitive"]; ok {
		if boolVal, ok := val.(bool); ok {
			prefs.DefaultMediaSensitive = boolVal
		}
	}
	if val, ok := updateReq["posting:default:language"]; ok {
		if strVal, ok := val.(string); ok {
			prefs.Language = strVal
		}
	}
	if val, ok := updateReq["reading:expand:spoilers"]; ok {
		if boolVal, ok := val.(bool); ok {
			prefs.ExpandSpoilers = boolVal
		}
	}
	// TODO: Map other Mastodon preferences to our storage model

	// Save updated preferences
	if err := h.store.UpdateUserPreferences(ctx, claims.Username, prefs); err != nil {
		h.logger.Error("failed to update user preferences", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return updated preferences in Mastodon format
	preferences := models.Preferences{
		PostingDefaultVisibility: prefs.DefaultPostingVisibility,
		PostingDefaultSensitive:  prefs.DefaultMediaSensitive,
		PostingDefaultLanguage:   prefs.Language,
		ReadingExpandMedia:       "default",
		ReadingExpandSpoilers:    prefs.ExpandSpoilers,
		ReadingAutoplayGifs:      true,
	}

	return common.OK(preferences), nil
}
