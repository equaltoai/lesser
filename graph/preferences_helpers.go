package graph

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// preferenceState aggregates preference values across storage systems for easier conversions.
type preferenceState struct {
	Language                  string
	DefaultVisibility         string
	DefaultSensitive          bool
	DefaultLanguage           string
	ExpandSpoilers            bool
	ExpandMedia               string
	AutoplayGifs              bool
	ShowFollowCounts          bool
	TimelineOrder             string
	SearchSuggestionsEnabled  bool
	PersonalizedSearchEnabled bool
	ReblogFilters             map[string]bool
	StreamingDefaultQuality   string
	StreamingAutoQuality      bool
	StreamingPreloadNext      bool
	StreamingDataSaver        bool
}

// loadPreferenceState merges account-level preferences with user repository preferences.
func (r *Resolver) loadPreferenceState(ctx context.Context, username string) *preferenceState {
	state := &preferenceState{
		Language:                  "en",
		DefaultVisibility:         "public",
		DefaultSensitive:          false,
		DefaultLanguage:           "en",
		ExpandSpoilers:            false,
		ExpandMedia:               "default",
		AutoplayGifs:              false,
		ShowFollowCounts:          true,
		TimelineOrder:             "newest",
		SearchSuggestionsEnabled:  true,
		PersonalizedSearchEnabled: true,
		ReblogFilters:             make(map[string]bool),
		StreamingDefaultQuality:   "AUTO",
		StreamingAutoQuality:      true,
		StreamingPreloadNext:      true,
		StreamingDataSaver:        false,
	}

	// Populate from Accounts service preferences (mirrors REST behaviour)
	if accountsService := r.Registry.Accounts(); accountsService != nil {
		result, err := accountsService.GetPreferences(ctx, &accounts.GetPreferencesQuery{
			Username: username,
		})
		if err != nil {
			r.Logger.Warn("failed to load account preferences", zap.String("username", username), zap.Error(err))
		} else if result != nil {
			prefs := result.Preferences
			state.Language = getStringPref(prefs, repositories.PrefKeyLanguage, state.Language)
			state.DefaultVisibility = getStringPref(prefs, repositories.PrefKeyDefaultPostingVisibility, state.DefaultVisibility)
			state.DefaultSensitive = getBoolPref(prefs, repositories.PrefKeyDefaultMediaSensitive, state.DefaultSensitive)
			state.DefaultLanguage = state.Language
			state.ExpandSpoilers = getBoolPref(prefs, repositories.PrefKeyExpandSpoilers, state.ExpandSpoilers)
			state.ExpandMedia = getStringPref(prefs, repositories.PrefKeyExpandMedia, state.ExpandMedia)
			// REST originally keyed autoplay gifs as autoplay_gifs; fall back to legacy key.
			autoPlay := getBoolPref(prefs, repositories.PrefKeyAutoplayGifs, state.AutoplayGifs)
			if !autoPlay {
				autoPlay = getBoolPref(prefs, "auto_play_gif", state.AutoplayGifs)
			}
			state.AutoplayGifs = autoPlay
			state.ShowFollowCounts = getBoolPref(prefs, repositories.PrefKeyShowFollowCounts, state.ShowFollowCounts)
			state.TimelineOrder = getStringPref(prefs, repositories.PrefKeyPreferredTimelineOrder, state.TimelineOrder)
			state.SearchSuggestionsEnabled = getBoolPref(prefs, repositories.PrefKeySearchSuggestionsEnabled, state.SearchSuggestionsEnabled)
			state.PersonalizedSearchEnabled = getBoolPref(prefs, repositories.PrefKeyPersonalizedSearchEnabled, state.PersonalizedSearchEnabled)
			if mapVal, ok := prefs[repositories.PrefKeyReblogFilters].(map[string]any); ok {
				for k, v := range mapVal {
					state.ReblogFilters[k] = boolFromAny(v, state.ReblogFilters[k])
				}
			}
			// Streaming keys may be present if REST ever persists them.
			state.StreamingDefaultQuality = getStringPref(prefs, repositories.PrefKeyStreamingDefaultQuality, state.StreamingDefaultQuality)
			state.StreamingAutoQuality = getBoolPref(prefs, repositories.PrefKeyStreamingAutoQuality, state.StreamingAutoQuality)
			state.StreamingPreloadNext = getBoolPref(prefs, repositories.PrefKeyStreamingPreloadNext, state.StreamingPreloadNext)
			state.StreamingDataSaver = getBoolPref(prefs, repositories.PrefKeyStreamingDataSaver, state.StreamingDataSaver)
		}
	} else {
		r.Logger.Warn("accounts service unavailable while loading preferences", zap.String("username", username))
	}

	// Merge with user repository preferences (source of truth for streaming-specific settings)
	if repo := r.Registry.GetStorage().User(); repo != nil {
		if stored, err := repo.GetUserPreferences(ctx, username); err == nil && stored != nil {
			state.Language = stored.Language
			state.DefaultVisibility = stored.DefaultPostingVisibility
			state.DefaultSensitive = stored.DefaultMediaSensitive
			state.DefaultLanguage = stored.Language
			state.ExpandSpoilers = stored.ExpandSpoilers
			state.ExpandMedia = stored.ExpandMedia
			state.AutoplayGifs = stored.AutoplayGifs
			state.ShowFollowCounts = stored.ShowFollowCounts
			state.TimelineOrder = stored.PreferredTimelineOrder
			state.SearchSuggestionsEnabled = stored.SearchSuggestionsEnabled
			state.PersonalizedSearchEnabled = stored.PersonalizedSearchEnabled
			if len(stored.ReblogFilters) > 0 {
				state.ReblogFilters = stored.ReblogFilters
			}
			state.StreamingDefaultQuality = stored.StreamingDefaultQuality
			state.StreamingAutoQuality = stored.StreamingAutoQuality
			state.StreamingPreloadNext = stored.StreamingPreloadNext
			state.StreamingDataSaver = stored.StreamingDataSaver
		} else if err != nil {
			r.Logger.Warn("failed to load user repository preferences", zap.String("username", username), zap.Error(err))
		}
	}

	return state
}

// convertPreferenceStateToModel produces the GraphQL UserPreferences model.
func (r *Resolver) convertPreferenceStateToModel(state *preferenceState, username string) *model.UserPreferences {
	if state == nil {
		state = &preferenceState{}
	}

	reblogFilters := make([]*model.ReblogFilter, 0, len(state.ReblogFilters))
	for key, enabled := range state.ReblogFilters {
		reblogFilters = append(reblogFilters, &model.ReblogFilter{
			Key:     key,
			Enabled: enabled,
		})
	}
	sort.Slice(reblogFilters, func(i, j int) bool {
		return reblogFilters[i].Key < reblogFilters[j].Key
	})

	return &model.UserPreferences{
		ActorID: username,
		Posting: &model.PostingPreferences{
			DefaultVisibility: toVisibilityEnum(state.DefaultVisibility),
			DefaultSensitive:  state.DefaultSensitive,
			DefaultLanguage:   state.DefaultLanguage,
		},
		Reading: &model.ReadingPreferences{
			ExpandSpoilers: state.ExpandSpoilers,
			ExpandMedia:    toExpandMediaEnum(state.ExpandMedia),
			AutoplayGifs:   state.AutoplayGifs,
			TimelineOrder:  toTimelineOrderEnum(state.TimelineOrder),
		},
		Discovery: &model.DiscoveryPreferences{
			ShowFollowCounts:          state.ShowFollowCounts,
			SearchSuggestionsEnabled:  state.SearchSuggestionsEnabled,
			PersonalizedSearchEnabled: state.PersonalizedSearchEnabled,
		},
		Streaming: &model.StreamingPreferences{
			DefaultQuality: toStreamQualityEnum(state.StreamingDefaultQuality),
			AutoQuality:    state.StreamingAutoQuality,
			PreloadNext:    state.StreamingPreloadNext,
			DataSaver:      state.StreamingDataSaver,
		},
		Notifications: &model.NotificationPreferences{
			Email:  false,
			Push:   true,
			InApp:  true,
			Digest: model.DigestFrequencyNever,
		},
		Privacy: &model.PrivacyPreferences{
			DefaultVisibility: toVisibilityEnum(state.DefaultVisibility),
			Indexable:         true,
			ShowOnlineStatus:  true,
		},
		ReblogFilters: reblogFilters,
	}
}

// Helper conversion functions

func getStringPref(prefs map[string]interface{}, key string, fallback string) string {
	if prefs == nil {
		return fallback
	}
	if value, ok := prefs[key]; ok {
		switch v := value.(type) {
		case string:
			if v != "" {
				return v
			}
		case fmt.Stringer:
			str := v.String()
			if str != "" {
				return str
			}
		}
	}
	return fallback
}

func getBoolPref(prefs map[string]interface{}, key string, fallback bool) bool {
	if prefs == nil {
		return fallback
	}
	if value, ok := prefs[key]; ok {
		return boolFromAny(value, fallback)
	}
	return fallback
}

func boolFromAny(value interface{}, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		default:
			return fallback
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case *bool:
		if v == nil {
			return fallback
		}
		return *v
	default:
		return fallback
	}
}

func toVisibilityEnum(value string) model.Visibility {
	switch strings.ToLower(value) {
	case VisibilityPublic:
		return model.VisibilityPublic
	case VisibilityUnlisted:
		return model.VisibilityUnlisted
	case EventTypeFollowers:
		return model.VisibilityFollowers
	case TimelineTypeDirect:
		return model.VisibilityDirect
	default:
		return model.VisibilityPublic
	}
}

func toExpandMediaEnum(value string) model.ExpandMediaPreference {
	switch strings.ToLower(value) {
	case "show_all":
		return model.ExpandMediaPreferenceShowAll
	case "hide_all":
		return model.ExpandMediaPreferenceHideAll
	default:
		return model.ExpandMediaPreferenceDefault
	}
}

func toTimelineOrderEnum(value string) model.TimelineOrder {
	switch strings.ToLower(value) {
	case "oldest":
		return model.TimelineOrderOldest
	default:
		return model.TimelineOrderNewest
	}
}

func toStreamQualityEnum(value string) model.StreamQuality {
	switch strings.ToUpper(value) {
	case "LOW":
		return model.StreamQualityLow
	case "MEDIUM":
		return model.StreamQualityMedium
	case "HIGH":
		return model.StreamQualityHigh
	case "ULTRA":
		return model.StreamQualityUltra
	default:
		return model.StreamQualityAuto
	}
}

func fromVisibilityEnum(enum *model.Visibility, fallback string) string {
	if enum == nil {
		return fallback
	}
	return strings.ToLower(string(*enum))
}

func fromExpandMediaEnum(enum *model.ExpandMediaPreference, fallback string) string {
	if enum == nil {
		return fallback
	}
	switch *enum {
	case model.ExpandMediaPreferenceShowAll:
		return "show_all"
	case model.ExpandMediaPreferenceHideAll:
		return "hide_all"
	default:
		return "default"
	}
}

func fromTimelineOrderEnum(enum *model.TimelineOrder, fallback string) string {
	if enum == nil {
		return fallback
	}
	switch *enum {
	case model.TimelineOrderOldest:
		return "oldest"
	default:
		return "newest"
	}
}

func fromStreamQualityEnum(enum *model.StreamQuality, fallback string) string {
	if enum == nil {
		return fallback
	}
	return strings.ToUpper(string(*enum))
}
