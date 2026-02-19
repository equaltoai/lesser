package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// UpdateUserPreferences synchronises user preferences (posting, reading, discovery, streaming).
func (r *mutationResolver) UpdateUserPreferences(ctx context.Context, input model.UpdateUserPreferencesInput) (*model.UserPreferences, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	accountsService := r.Registry.Accounts()
	if accountsService == nil {
		return nil, errors.New("accounts service is not available")
	}

	storage := r.Registry.GetStorage()
	if storage == nil || storage.User() == nil {
		return nil, errors.New("user repository is not available")
	}

	state := r.loadPreferenceState(ctx, username)
	applyUserPreferencesInput(state, &input)

	cmd := &accounts.UpdatePreferencesCommand{
		Username:                  username,
		UpdaterID:                 username,
		Language:                  state.Language,
		DefaultPostingVisibility:  state.DefaultVisibility,
		DefaultMediaSensitive:     state.DefaultSensitive,
		DirectMessagesFrom:        state.DirectMessagesFrom,
		ExpandSpoilers:            state.ExpandSpoilers,
		ExpandMedia:               state.ExpandMedia,
		AutoplayGifs:              state.AutoplayGifs,
		ShowFollowCounts:          state.ShowFollowCounts,
		PreferredTimelineOrder:    state.TimelineOrder,
		SearchSuggestionsEnabled:  state.SearchSuggestionsEnabled,
		PersonalizedSearchEnabled: state.PersonalizedSearchEnabled,
		ReblogFilters:             state.ReblogFilters,
	}

	if _, err := accountsService.UpdatePreferences(ctx, cmd); err != nil {
		r.Logger.Error("Failed to update account preferences",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update preferences"), err)
	}

	if err := storage.User().UpdatePreferences(ctx, username, preferenceStateToMap(state)); err != nil {
		r.Logger.Error("Failed to persist preferences in user repository",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to persist preferences"), err)
	}

	// Reload to ensure we reflect any server-side normalization.
	state = r.loadPreferenceState(ctx, username)
	return r.convertPreferenceStateToModel(state, username), nil
}

// UpdateStreamingPreferences updates only the streaming subset of user preferences.
func (r *mutationResolver) UpdateStreamingPreferences(ctx context.Context, input model.StreamingPreferencesInput) (*model.UserPreferences, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	storage := r.Registry.GetStorage()
	if storage == nil || storage.User() == nil {
		return nil, errors.New("user repository is not available")
	}

	state := r.loadPreferenceState(ctx, username)
	applyStreamingPreferencesInput(state, &input)

	updates := map[string]any{
		repositories.PrefKeyStreamingDefaultQuality: state.StreamingDefaultQuality,
		repositories.PrefKeyStreamingAutoQuality:    state.StreamingAutoQuality,
		repositories.PrefKeyStreamingPreloadNext:    state.StreamingPreloadNext,
		repositories.PrefKeyStreamingDataSaver:      state.StreamingDataSaver,
	}

	if err := storage.User().UpdatePreferences(ctx, username, updates); err != nil {
		r.Logger.Error("Failed to update streaming preferences",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update streaming preferences"), err)
	}

	state = r.loadPreferenceState(ctx, username)
	return r.convertPreferenceStateToModel(state, username), nil
}

func applyUserPreferencesInput(state *preferenceState, input *model.UpdateUserPreferencesInput) {
	if state == nil || input == nil {
		return
	}

	if input.Language != nil {
		state.Language = *input.Language
		state.DefaultLanguage = *input.Language
	}
	if input.DefaultPostingVisibility != nil {
		state.DefaultVisibility = fromVisibilityEnum(input.DefaultPostingVisibility, state.DefaultVisibility)
	}
	if input.DirectMessagesFrom != nil {
		state.DirectMessagesFrom = input.DirectMessagesFrom.String()
	}
	if input.DefaultMediaSensitive != nil {
		state.DefaultSensitive = *input.DefaultMediaSensitive
	}
	if input.ExpandSpoilers != nil {
		state.ExpandSpoilers = *input.ExpandSpoilers
	}
	if input.ExpandMedia != nil {
		state.ExpandMedia = fromExpandMediaEnum(input.ExpandMedia, state.ExpandMedia)
	}
	if input.AutoplayGifs != nil {
		state.AutoplayGifs = *input.AutoplayGifs
	}
	if input.ShowFollowCounts != nil {
		state.ShowFollowCounts = *input.ShowFollowCounts
	}
	if input.PreferredTimelineOrder != nil {
		state.TimelineOrder = fromTimelineOrderEnum(input.PreferredTimelineOrder, state.TimelineOrder)
	}
	if input.SearchSuggestionsEnabled != nil {
		state.SearchSuggestionsEnabled = *input.SearchSuggestionsEnabled
	}
	if input.PersonalizedSearchEnabled != nil {
		state.PersonalizedSearchEnabled = *input.PersonalizedSearchEnabled
	}
	if len(input.ReblogFilters) > 0 {
		filters := make(map[string]bool, len(input.ReblogFilters))
		for _, filter := range input.ReblogFilters {
			if filter == nil {
				continue
			}
			filters[filter.Key] = filter.Enabled
		}
		state.ReblogFilters = filters
	}
	if input.Streaming != nil {
		applyStreamingPreferencesInput(state, input.Streaming)
	}
}

func applyStreamingPreferencesInput(state *preferenceState, input *model.StreamingPreferencesInput) {
	if state == nil || input == nil {
		return
	}
	state.StreamingDefaultQuality = fromStreamQualityEnum(input.DefaultQuality, state.StreamingDefaultQuality)
	if input.AutoQuality != nil {
		state.StreamingAutoQuality = *input.AutoQuality
	}
	if input.PreloadNext != nil {
		state.StreamingPreloadNext = *input.PreloadNext
	}
	if input.DataSaver != nil {
		state.StreamingDataSaver = *input.DataSaver
	}
}

func preferenceStateToMap(state *preferenceState) map[string]any {
	return map[string]any{
		repositories.PrefKeyLanguage:                  state.Language,
		repositories.PrefKeyDefaultPostingVisibility:  state.DefaultVisibility,
		repositories.PrefKeyDefaultMediaSensitive:     state.DefaultSensitive,
		repositories.PrefKeyDirectMessagesFrom:        state.DirectMessagesFrom,
		repositories.PrefKeyExpandSpoilers:            state.ExpandSpoilers,
		repositories.PrefKeyExpandMedia:               state.ExpandMedia,
		repositories.PrefKeyAutoplayGifs:              state.AutoplayGifs,
		repositories.PrefKeyShowFollowCounts:          state.ShowFollowCounts,
		repositories.PrefKeyPreferredTimelineOrder:    state.TimelineOrder,
		repositories.PrefKeySearchSuggestionsEnabled:  state.SearchSuggestionsEnabled,
		repositories.PrefKeyPersonalizedSearchEnabled: state.PersonalizedSearchEnabled,
		repositories.PrefKeyReblogFilters:             state.ReblogFilters,
		repositories.PrefKeyStreamingDefaultQuality:   state.StreamingDefaultQuality,
		repositories.PrefKeyStreamingAutoQuality:      state.StreamingAutoQuality,
		repositories.PrefKeyStreamingPreloadNext:      state.StreamingPreloadNext,
		repositories.PrefKeyStreamingDataSaver:        state.StreamingDataSaver,
	}
}
