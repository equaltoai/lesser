package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

type round12Stringer string

func (s round12Stringer) String() string { return string(s) }

func TestRound12PreferencesHelpers_ValueParsing(t *testing.T) {
	require.Equal(t, "fallback", getStringPref(nil, "k", "fallback"))
	require.Equal(t, "v", getStringPref(map[string]interface{}{"k": "v"}, "k", "fallback"))
	require.Equal(t, "str", getStringPref(map[string]interface{}{"k": round12Stringer("str")}, "k", "fallback"))
	require.Equal(t, "fallback", getStringPref(map[string]interface{}{"k": ""}, "k", "fallback"))

	require.True(t, getBoolPref(map[string]interface{}{"k": true}, "k", false))
	require.True(t, getBoolPref(map[string]interface{}{"k": "on"}, "k", false))
	require.False(t, getBoolPref(map[string]interface{}{"k": "off"}, "k", true))
	require.True(t, getBoolPref(map[string]interface{}{"k": 1}, "k", false))
	require.False(t, getBoolPref(map[string]interface{}{"k": 0}, "k", true))

	b := true
	require.True(t, boolFromAny(&b, false))
	require.False(t, boolFromAny((*bool)(nil), false))
	require.True(t, boolFromAny("1", false))
	require.False(t, boolFromAny("0", true))
	require.True(t, boolFromAny(int64(2), false))
	require.False(t, boolFromAny(float64(0), true))
	require.Equal(t, true, boolFromAny("maybe", true))
}

func TestRound12PreferencesHelpers_EnumConversions(t *testing.T) {
	require.Equal(t, model.VisibilityPublic, toVisibilityEnum("public"))
	require.Equal(t, model.VisibilityUnlisted, toVisibilityEnum("unlisted"))
	require.Equal(t, model.VisibilityFollowers, toVisibilityEnum("followers"))
	require.Equal(t, model.VisibilityDirect, toVisibilityEnum("direct"))
	require.Equal(t, model.VisibilityPublic, toVisibilityEnum("unknown"))

	require.Equal(t, model.ExpandMediaPreferenceShowAll, toExpandMediaEnum("show_all"))
	require.Equal(t, model.ExpandMediaPreferenceHideAll, toExpandMediaEnum("hide_all"))
	require.Equal(t, model.ExpandMediaPreferenceDefault, toExpandMediaEnum("something"))

	require.Equal(t, model.TimelineOrderOldest, toTimelineOrderEnum("oldest"))
	require.Equal(t, model.TimelineOrderNewest, toTimelineOrderEnum("newest"))

	require.Equal(t, model.StreamQualityHigh, toStreamQualityEnum("HIGH"))
	require.Equal(t, model.StreamQualityAuto, toStreamQualityEnum("auto"))

	require.Equal(t, "fallback", fromVisibilityEnum(nil, "fallback"))
	v := model.VisibilityUnlisted
	require.Equal(t, "unlisted", fromVisibilityEnum(&v, "fallback"))

	require.Equal(t, "fallback", fromExpandMediaEnum(nil, "fallback"))
	em := model.ExpandMediaPreferenceShowAll
	require.Equal(t, "show_all", fromExpandMediaEnum(&em, "fallback"))

	require.Equal(t, "fallback", fromTimelineOrderEnum(nil, "fallback"))
	order := model.TimelineOrderOldest
	require.Equal(t, "oldest", fromTimelineOrderEnum(&order, "fallback"))

	require.Equal(t, "fallback", fromStreamQualityEnum(nil, "fallback"))
	q := model.StreamQualityUltra
	require.Equal(t, "ULTRA", fromStreamQualityEnum(&q, "fallback"))
}

func TestRound12PreferencesHelpers_LoadAndConvert(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := context.Background()

	state := resolver.loadPreferenceState(ctx, "alice")
	require.NotNil(t, state)
	require.Equal(t, "en", state.Language)
	require.Equal(t, "public", state.DefaultVisibility)

	userPrefs := &storage.UserPreferences{
		Username:                  "bob",
		Language:                  "fr",
		DefaultPostingVisibility:  "unlisted",
		DefaultMediaSensitive:     true,
		ExpandSpoilers:            true,
		ExpandMedia:               "hide_all",
		AutoplayGifs:              true,
		ShowFollowCounts:          false,
		PreferredTimelineOrder:    "oldest",
		SearchSuggestionsEnabled:  false,
		PersonalizedSearchEnabled: false,
		ReblogFilters:             map[string]bool{"ads": true, "spoilers": false},
		StreamingDefaultQuality:   "HIGH",
		StreamingAutoQuality:      false,
		StreamingPreloadNext:      false,
		StreamingDataSaver:        true,
	}

	require.NoError(t, storageRepo.User().UpdateUserPreferences(ctx, "bob", userPrefs))

	state = resolver.loadPreferenceState(ctx, "bob")
	require.Equal(t, "fr", state.Language)
	require.Equal(t, "unlisted", state.DefaultVisibility)
	require.True(t, state.DefaultSensitive)
	require.True(t, state.ExpandSpoilers)
	require.Equal(t, "hide_all", state.ExpandMedia)
	require.True(t, state.AutoplayGifs)
	require.False(t, state.ShowFollowCounts)
	require.Equal(t, "oldest", state.TimelineOrder)
	require.False(t, state.SearchSuggestionsEnabled)
	require.False(t, state.PersonalizedSearchEnabled)
	require.Equal(t, map[string]bool{"ads": true, "spoilers": false}, state.ReblogFilters)
	require.Equal(t, "HIGH", state.StreamingDefaultQuality)
	require.False(t, state.StreamingAutoQuality)
	require.False(t, state.StreamingPreloadNext)
	require.True(t, state.StreamingDataSaver)

	modelPrefs := resolver.convertPreferenceStateToModel(state, "bob")
	require.NotNil(t, modelPrefs)
	require.Equal(t, "bob", modelPrefs.ActorID)
	require.NotNil(t, modelPrefs.Posting)
	require.Equal(t, model.VisibilityUnlisted, modelPrefs.Posting.DefaultVisibility)
	require.NotNil(t, modelPrefs.ReblogFilters)
	require.Len(t, modelPrefs.ReblogFilters, 2)
	require.Equal(t, "ads", modelPrefs.ReblogFilters[0].Key)
}

