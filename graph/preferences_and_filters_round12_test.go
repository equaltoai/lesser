package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRound12PreferencesQueriesAndMutations(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)

	queries := &queryResolver{resolver}
	mutations := &mutationResolver{resolver}

	ctx := round12AuthContext("alice")

	// User preferences query and mutations.
	prefs, err := queries.UserPreferences(ctx)
	require.NoError(t, err)
	require.NotNil(t, prefs)

	lang := "en"
	vis := model.VisibilityUnlisted
	expand := model.ExpandMediaPreferenceHideAll
	order := model.TimelineOrderOldest
	quality := model.StreamQualityHigh

	updated, err := mutations.UpdateUserPreferences(ctx, model.UpdateUserPreferencesInput{
		Language:                  &lang,
		DefaultPostingVisibility:  &vis,
		DefaultMediaSensitive:     ptrBool(true),
		ExpandSpoilers:            ptrBool(true),
		ExpandMedia:               &expand,
		AutoplayGifs:              ptrBool(true),
		ShowFollowCounts:          ptrBool(false),
		PreferredTimelineOrder:    &order,
		SearchSuggestionsEnabled:  ptrBool(true),
		PersonalizedSearchEnabled: ptrBool(true),
		ReblogFilters: []*model.ReblogFilterInput{
			nil,
			{Key: "boosts", Enabled: false},
		},
		Streaming: &model.StreamingPreferencesInput{
			DefaultQuality: &quality,
			AutoQuality:    ptrBool(false),
			PreloadNext:    ptrBool(true),
			DataSaver:      ptrBool(false),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.Streaming)

	onlyStreaming, err := mutations.UpdateStreamingPreferences(ctx, model.StreamingPreferencesInput{
		DefaultQuality: &quality,
		AutoQuality:    ptrBool(true),
		PreloadNext:    ptrBool(false),
		DataSaver:      ptrBool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, onlyStreaming)
	require.NotNil(t, onlyStreaming.Streaming)

	// Push subscription query: nil when none exist.
	sub, err := queries.PushSubscription(ctx)
	require.NoError(t, err)
	require.Nil(t, sub)

	// When the repo returns a subscription, we should expose it and include server key.
	state.autoPopulateAll = true
	state.autoPopulateCount = 1

	sub, err = queries.PushSubscription(ctx)
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.NotNil(t, sub.ServerKey)
	require.NotEmpty(t, *sub.ServerKey)
}

func TestRound12FilterResolvers_V2(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	queries := &queryResolver{resolver}

	ctx := round12AuthContext("alice")

	moderationRepo := resolver.Storage.Moderation()
	require.NotNil(t, moderationRepo)

	filter := &storage.Filter{
		Username:     "alice",
		Title:        "spoilers",
		Context:      []string{"home"},
		FilterAction: "hide",
		MatchMode:    "keyword",
		WholeWord:    true,
	}
	require.NoError(t, moderationRepo.CreateFilter(ctx, filter))

	require.NoError(t, moderationRepo.AddFilterKeyword(ctx, filter.ID, &storage.FilterKeyword{
		Keyword:   "spoiler",
		WholeWord: true,
	}))
	require.NoError(t, moderationRepo.AddFilterStatus(ctx, filter.ID, &storage.FilterStatus{
		StatusID: "status-1",
	}))

	// Filters lists the user's filters and converts them to Mastodon representation.
	results, err := queries.Filters(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.IsType(t, &mastodon.Filter{}, results[0])

	// Filter returns a single filter by ID for the correct user.
	one, err := queries.Filter(ctx, filter.ID)
	require.NoError(t, err)
	require.NotNil(t, one)
	require.Equal(t, filter.ID, one.ID)

	// Wrong user can't access it.
	otherFilter := &storage.Filter{
		Username:     "bob",
		Title:        "other",
		Context:      []string{"home"},
		FilterAction: "warn",
		MatchMode:    "keyword",
	}
	require.NoError(t, moderationRepo.CreateFilter(ctx, otherFilter))

	_, err = queries.Filter(ctx, otherFilter.ID)
	require.Error(t, err)

	// Invalid ID fails validation.
	_, err = queries.Filter(ctx, "")
	require.Error(t, err)

	// Ensure expiring filters don't panic in conversion paths.
	expires := time.Now().Add(1 * time.Hour)
	require.NoError(t, moderationRepo.UpdateFilter(ctx, filter.ID, map[string]any{
		"expires_at": &expires,
	}))
}
