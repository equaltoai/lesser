package dynamodb

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// SetPreference sets a single preference for a user
func (s *dynamoDBStorage) SetPreference(ctx context.Context, username string, key string, value any) error {
	// Get existing preferences
	prefs, err := s.GetUserPreferences(ctx, username)
	if err != nil {
		// If preferences don't exist, create new ones
		prefs = s.getDefaultPreferences()
	}

	// Map the key to the appropriate field
	switch key {
	case "posting:default:visibility":
		if v, ok := value.(string); ok {
			prefs.DefaultPostingVisibility = v
		}
	case "posting:default:sensitive":
		if v, ok := value.(bool); ok {
			prefs.DefaultMediaSensitive = v
		}
	case "posting:default:language":
		if v, ok := value.(string); ok {
			prefs.Language = v
		}
	case "reading:expand:spoilers":
		if v, ok := value.(bool); ok {
			prefs.ExpandSpoilers = v
		}
	case "reading:expand:media":
		if v, ok := value.(string); ok {
			prefs.ExpandMedia = v
		}
	case "reading:autoplay:gifs":
		if v, ok := value.(bool); ok {
			prefs.AutoplayGifs = v
		}
	case "show_follow_counts":
		if v, ok := value.(bool); ok {
			prefs.ShowFollowCounts = v
		}
	case "preferred_timeline_order":
		if v, ok := value.(string); ok {
			prefs.PreferredTimelineOrder = v
		}
	case "search_suggestions_enabled":
		if v, ok := value.(bool); ok {
			prefs.SearchSuggestionsEnabled = v
		}
	case "personalized_search_enabled":
		if v, ok := value.(bool); ok {
			prefs.PersonalizedSearchEnabled = v
		}
	default:
		// For unknown keys, we could store them in a map, but for now just log
		s.logger().Warn("unknown preference key", zap.String("key", key))
	}

	// Save the updated preferences
	return s.UpdateUserPreferences(ctx, username, prefs)
}

// GetPreference gets a single preference value for a user
func (s *dynamoDBStorage) GetPreference(ctx context.Context, username string, key string) (any, error) {
	prefs, err := s.GetUserPreferences(ctx, username)
	if err != nil {
		// Return default value if preferences don't exist
		defaults := s.getDefaultPreferences()
		return s.getPreferenceValue(defaults, key), nil
	}

	return s.getPreferenceValue(prefs, key), nil
}

// GetAllPreferences gets all preferences as a map
func (s *dynamoDBStorage) GetAllPreferences(ctx context.Context, username string) (map[string]any, error) {
	prefs, err := s.GetUserPreferences(ctx, username)
	if err != nil {
		// Return defaults if preferences don't exist
		prefs = s.getDefaultPreferences()
	}

	// Convert to map format
	result := map[string]any{
		"posting:default:visibility":  prefs.DefaultPostingVisibility,
		"posting:default:sensitive":   prefs.DefaultMediaSensitive,
		"posting:default:language":    prefs.Language,
		"reading:expand:spoilers":     prefs.ExpandSpoilers,
		"reading:expand:media":        prefs.ExpandMedia,
		"reading:autoplay:gifs":       prefs.AutoplayGifs,
		"show_follow_counts":          prefs.ShowFollowCounts,
		"preferred_timeline_order":    prefs.PreferredTimelineOrder,
		"search_suggestions_enabled":  prefs.SearchSuggestionsEnabled,
		"personalized_search_enabled": prefs.PersonalizedSearchEnabled,
	}

	return result, nil
}

// UpdatePreferences updates multiple preferences at once from a map
func (s *dynamoDBStorage) UpdatePreferences(ctx context.Context, username string, prefsMap map[string]any) error {
	// Get existing preferences
	prefs, err := s.GetUserPreferences(ctx, username)
	if err != nil {
		// Start with defaults if preferences don't exist
		prefs = s.getDefaultPreferences()
	}

	// Update each preference
	for key, value := range prefsMap {
		switch key {
		case "posting:default:visibility":
			if v, ok := value.(string); ok {
				prefs.DefaultPostingVisibility = v
			}
		case "posting:default:sensitive":
			if v, ok := value.(bool); ok {
				prefs.DefaultMediaSensitive = v
			}
		case "posting:default:language":
			if v, ok := value.(string); ok {
				prefs.Language = v
			}
		case "reading:expand:spoilers":
			if v, ok := value.(bool); ok {
				prefs.ExpandSpoilers = v
			}
		case "reading:expand:media":
			if v, ok := value.(string); ok {
				prefs.ExpandMedia = v
			}
		case "reading:autoplay:gifs":
			if v, ok := value.(bool); ok {
				prefs.AutoplayGifs = v
			}
		case "show_follow_counts":
			if v, ok := value.(bool); ok {
				prefs.ShowFollowCounts = v
			}
		case "preferred_timeline_order":
			if v, ok := value.(string); ok {
				prefs.PreferredTimelineOrder = v
			}
		case "search_suggestions_enabled":
			if v, ok := value.(bool); ok {
				prefs.SearchSuggestionsEnabled = v
			}
		case "personalized_search_enabled":
			if v, ok := value.(bool); ok {
				prefs.PersonalizedSearchEnabled = v
			}
		}
	}

	// Save the updated preferences
	return s.UpdateUserPreferences(ctx, username, prefs)
}

// Helper method to get a specific preference value from the preferences struct
func (s *dynamoDBStorage) getPreferenceValue(prefs *storage.UserPreferences, key string) any {
	switch key {
	case "posting:default:visibility":
		return prefs.DefaultPostingVisibility
	case "posting:default:sensitive":
		return prefs.DefaultMediaSensitive
	case "posting:default:language":
		return prefs.Language
	case "reading:expand:spoilers":
		return prefs.ExpandSpoilers
	case "reading:expand:media":
		return prefs.ExpandMedia
	case "reading:autoplay:gifs":
		return prefs.AutoplayGifs
	case "show_follow_counts":
		return prefs.ShowFollowCounts
	case "preferred_timeline_order":
		return prefs.PreferredTimelineOrder
	case "search_suggestions_enabled":
		return prefs.SearchSuggestionsEnabled
	case "personalized_search_enabled":
		return prefs.PersonalizedSearchEnabled
	default:
		return nil
	}
}

// Helper method to get default preferences
func (s *dynamoDBStorage) getDefaultPreferences() *storage.UserPreferences {
	return &storage.UserPreferences{
		Language:                  "en",
		DefaultPostingVisibility:  "public",
		DefaultMediaSensitive:     false,
		ExpandSpoilers:            false,
		ExpandMedia:               "default",
		AutoplayGifs:              true,
		ShowFollowCounts:          true,
		PreferredTimelineOrder:    "newest",
		SearchSuggestionsEnabled:  true,
		PersonalizedSearchEnabled: true,
	}
}
