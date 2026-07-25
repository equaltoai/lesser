package handlers

import (
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

// HandleGetPreferencesLift handles GET /api/v1/preferences
// Returns user preferences in Mastodon format
func (h *Handler) HandleGetPreferencesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	username, err := h.authenticateUser(ctx, []string{auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get user preferences using Accounts service
	result, err := h.registry.Accounts().GetPreferences(ctx.Context(), &accounts.GetPreferencesQuery{
		Username: username,
	})
	if err != nil {
		h.logger.Error("failed to get user preferences", zap.Error(err))
		// Return defaults if preferences don't exist
		return okJSON(models.Preferences{
			PostingDefaultVisibility: "public",
			PostingDefaultSensitive:  false,
			PostingDefaultLanguage:   "en",
			ReadingExpandMedia:       "default",
			ReadingExpandSpoilers:    false,
			ReadingAutoplayGifs:      true,
		})
	}

	// Convert service result to Mastodon format
	prefs := result.Preferences
	preferences := models.Preferences{
		PostingDefaultVisibility: h.getStringPreference(prefs, "default_posting_visibility", "public"),
		PostingDefaultSensitive:  h.getBoolPreference(prefs, "default_media_sensitive", false),
		PostingDefaultLanguage:   h.getStringPreference(prefs, "language", "en"),
		ReadingExpandMedia:       h.mapExpandMediaPreference(h.getStringPreference(prefs, "expand_media", "default")),
		ReadingExpandSpoilers:    h.getBoolPreference(prefs, "expand_spoilers", false),
		ReadingAutoplayGifs:      h.getBoolPreference(prefs, "auto_play_gif", true),
	}

	return okJSON(preferences)
}

// HandleUpdatePreferencesLift handles PATCH /api/v1/preferences
// Updates user preferences and returns the updated preferences
func (h *Handler) HandleUpdatePreferencesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse update request
	updateReq, resp, err := h.parsePreferencesUpdateRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get or create user preferences
	prefs := h.getOrCreateUserPreferences(ctx, username)

	// Apply updates to preferences
	h.applyPreferenceUpdates(prefs, updateReq)

	// Save updated preferences
	if resp, err := h.saveUserPreferences(ctx, username, prefs); resp != nil || err != nil {
		return resp, err
	}

	// Return updated preferences in Mastodon format
	return h.returnUpdatedPreferences(ctx, prefs)
}

// parsePreferencesUpdateRequest parses the preferences update request
func (h *Handler) parsePreferencesUpdateRequest(ctx *apptheory.Context) (map[string]interface{}, *apptheory.Response, error) {
	var updateReq map[string]interface{}
	if err := common.ParseRequestWithFallback(ctx, &updateReq); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return nil, resp, respErr
	}
	return updateReq, nil, nil
}

// getOrCreateUserPreferences gets existing preferences or creates defaults
func (h *Handler) getOrCreateUserPreferences(ctx *apptheory.Context, username string) *storage.UserPreferences {
	// Use Accounts service to get preferences
	result, err := h.registry.Accounts().GetPreferences(ctx.Context(), &accounts.GetPreferencesQuery{
		Username: username,
	})
	if err != nil {
		h.logger.Warn("failed to get existing preferences, using defaults", zap.Error(err))
		return h.createDefaultPreferences()
	}

	// Convert from map to UserPreferences struct
	prefs := &storage.UserPreferences{
		Language:                  h.getStringPreference(result.Preferences, "language", "en"),
		DefaultPostingVisibility:  h.getStringPreference(result.Preferences, "default_posting_visibility", "public"),
		DefaultMediaSensitive:     h.getBoolPreference(result.Preferences, "default_media_sensitive", false),
		ExpandSpoilers:            h.getBoolPreference(result.Preferences, "expand_spoilers", false),
		ExpandMedia:               h.getStringPreference(result.Preferences, "expand_media", "default"),
		AutoplayGifs:              h.getBoolPreference(result.Preferences, "auto_play_gif", false),
		ShowFollowCounts:          h.getBoolPreference(result.Preferences, "show_follow_counts", true),
		PreferredTimelineOrder:    h.getStringPreference(result.Preferences, "preferred_timeline_order", "newest"),
		SearchSuggestionsEnabled:  h.getBoolPreference(result.Preferences, "search_suggestions_enabled", true),
		PersonalizedSearchEnabled: h.getBoolPreference(result.Preferences, "personalized_search_enabled", true),
		StreamingDefaultQuality:   h.getStringPreference(result.Preferences, "streaming_default_quality", "AUTO"),
		StreamingAutoQuality:      h.getBoolPreference(result.Preferences, "streaming_auto_quality", true),
		StreamingPreloadNext:      h.getBoolPreference(result.Preferences, "streaming_preload_next", true),
		StreamingDataSaver:        h.getBoolPreference(result.Preferences, "streaming_data_saver", false),
	}
	return prefs
}

// createDefaultPreferences creates default user preferences
func (h *Handler) createDefaultPreferences() *storage.UserPreferences {
	return &storage.UserPreferences{
		Language:                  "en",
		DefaultPostingVisibility:  "public",
		DefaultMediaSensitive:     false,
		ExpandSpoilers:            false,
		ExpandMedia:               "default",
		AutoplayGifs:              false,
		ShowFollowCounts:          true,
		PreferredTimelineOrder:    "newest",
		SearchSuggestionsEnabled:  true,
		PersonalizedSearchEnabled: true,
		StreamingDefaultQuality:   "AUTO",
		StreamingAutoQuality:      true,
		StreamingPreloadNext:      true,
		StreamingDataSaver:        false,
	}
}

// applyPreferenceUpdates applies updates from the request to preferences
func (h *Handler) applyPreferenceUpdates(prefs *storage.UserPreferences, updateReq map[string]interface{}) {
	// Update posting preferences
	h.updateStringPreference(&prefs.DefaultPostingVisibility, updateReq, "posting:default:visibility")
	h.updateBoolPreference(&prefs.DefaultMediaSensitive, updateReq, "posting:default:sensitive")
	h.updateStringPreference(&prefs.Language, updateReq, "posting:default:language")

	// Update reading preferences
	h.updateBoolPreference(&prefs.ExpandSpoilers, updateReq, "reading:expand:spoilers")
	h.updateStringPreference(&prefs.ExpandMedia, updateReq, "reading:expand:media")
	h.updateBoolPreference(&prefs.AutoplayGifs, updateReq, "reading:autoplay:gifs")
}

// updateStringPreference updates a string preference if present in request
func (h *Handler) updateStringPreference(field *string, updateReq map[string]interface{}, key string) {
	if val, ok := updateReq[key]; ok {
		if strVal, ok := val.(string); ok {
			*field = strVal
		}
	}
}

// updateBoolPreference updates a boolean preference if present in request
func (h *Handler) updateBoolPreference(field *bool, updateReq map[string]interface{}, key string) {
	if val, ok := updateReq[key]; ok {
		if boolVal, ok := val.(bool); ok {
			*field = boolVal
		}
	}
}

// saveUserPreferences saves the updated preferences to storage
func (h *Handler) saveUserPreferences(ctx *apptheory.Context, username string, prefs *storage.UserPreferences) (*apptheory.Response, error) {
	// Use Accounts service to update preferences
	_, err := h.registry.Accounts().UpdatePreferences(ctx.Context(), &accounts.UpdatePreferencesCommand{
		Username:                  username,
		Language:                  prefs.Language,
		DefaultPostingVisibility:  prefs.DefaultPostingVisibility,
		DefaultMediaSensitive:     prefs.DefaultMediaSensitive,
		ExpandSpoilers:            prefs.ExpandSpoilers,
		ExpandMedia:               prefs.ExpandMedia,
		AutoplayGifs:              prefs.AutoplayGifs,
		ShowFollowCounts:          prefs.ShowFollowCounts,
		PreferredTimelineOrder:    prefs.PreferredTimelineOrder,
		SearchSuggestionsEnabled:  prefs.SearchSuggestionsEnabled,
		PersonalizedSearchEnabled: prefs.PersonalizedSearchEnabled,
		UpdaterID:                 username,
	})
	if err != nil {
		h.logger.Error("failed to update user preferences", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	return nil, nil
}

// returnUpdatedPreferences returns the updated preferences in Mastodon format
func (h *Handler) returnUpdatedPreferences(_ *apptheory.Context, prefs *storage.UserPreferences) (*apptheory.Response, error) {
	preferences := models.Preferences{
		PostingDefaultVisibility: prefs.DefaultPostingVisibility,
		PostingDefaultSensitive:  prefs.DefaultMediaSensitive,
		PostingDefaultLanguage:   prefs.Language,
		ReadingExpandMedia:       h.mapExpandMediaPreference(prefs.ExpandMedia),
		ReadingExpandSpoilers:    prefs.ExpandSpoilers,
		ReadingAutoplayGifs:      prefs.AutoplayGifs,
	}
	return okJSON(preferences)
}

// mapExpandMediaPreference maps internal expand media preference to Mastodon format
func (h *Handler) mapExpandMediaPreference(expandMedia string) string {
	switch expandMedia {
	case "show_all":
		return "show_all"
	case "hide_all":
		return "hide_all"
	default:
		return "default"
	}
}

// getStringPreference gets a string preference from the map with a default value
func (h *Handler) getStringPreference(prefs map[string]interface{}, key string, defaultValue string) string {
	if val, ok := prefs[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// getBoolPreference gets a boolean preference from the map with a default value
func (h *Handler) getBoolPreference(prefs map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := prefs[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return defaultValue
}
