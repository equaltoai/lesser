package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestUserRepository_GetUserPreferences_NotFoundReturnsDefaults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	prefs, err := repo.GetUserPreferences(context.Background(), "alice")
	assert.NoError(t, err)
	assert.NotNil(t, prefs)
	assert.Equal(t, "en", prefs.Language)
}

func TestUserRepository_GetUserLanguagePreference_DefaultsToEnglishWhenEmpty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.UserPreferences)
		out.Username = "alice"
		out.UpdateKeys()
		out.Language = ""
	}).Return(nil)

	lang, err := repo.GetUserLanguagePreference(context.Background(), "alice")
	assert.NoError(t, err)
	assert.Equal(t, "en", lang)
}

func TestUserRepository_SetUserLanguagePreference_WhenGetFailsCreatesDefaultAndStores(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
	mockQuery.On("Create").Return(nil)

	err := repo.SetUserLanguagePreference(context.Background(), "alice", "es")
	assert.NoError(t, err)
}

func TestUserRepository_UpdateUserPreferences_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.UpdateUserPreferences(context.Background(), "alice", &storage.UserPreferences{
		Language: "en",
	})
	assert.Error(t, err)
}

func TestUserRepository_SetPreference_UnknownKeyDoesNotError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	// GetUserPreferences -> not found -> defaults.
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)
	// UpdateUserPreferences -> Create.
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	err := repo.SetPreference(context.Background(), "alice", "custom_key", "custom_value")
	assert.NoError(t, err)
}

func TestUserRepository_GetPreference_UnknownKeyReturnsError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	val, err := repo.GetPreference(context.Background(), "alice", "unknown")
	assert.Error(t, err)
	assert.Nil(t, val)
}

func TestUserRepository_UpdatePreferences_UpdatesMultipleKeys(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	// GetUserPreferences -> not found -> defaults.
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)
	// UpdateUserPreferences -> Create.
	mockQuery.On("Create").Return(nil)

	err := repo.UpdatePreferences(context.Background(), "alice", map[string]any{
		PrefKeyDefaultMediaSensitive: true,
		"custom_key":                 "custom_value",
	})
	assert.NoError(t, err)
}

func TestUserRepository_setStringPreference_TypeMismatch(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	var s string
	err := repo.setStringPreference(&s, true, PrefKeyLanguage)
	assert.Error(t, err)
}

func TestUserRepository_setBoolPreference_TypeMismatch(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	var b bool
	err := repo.setBoolPreference(&b, "true", PrefKeyDefaultMediaSensitive)
	assert.Error(t, err)
}

func TestUserRepository_setReblogFiltersPreference_TypeMismatch(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	var filters map[string]bool
	err := repo.setReblogFiltersPreference(&filters, map[string]interface{}{"bob": false}, PrefKeyReblogFilters)
	assert.Error(t, err)
}

func TestUserRepository_PreferenceSwitches_CoverAllKnownKeys(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	prefs := &storage.UserPreferences{
		Preferences:   make(map[string]string),
		ReblogFilters: make(map[string]bool),
	}

	cases := []struct {
		key   string
		value any
	}{
		{PrefKeyLanguage, "fr"},
		{PrefKeyDefaultPostingVisibility, "unlisted"},
		{PrefKeyDefaultMediaSensitive, true},
		{PrefKeyExpandSpoilers, false},
		{PrefKeyExpandMedia, "hide"},
		{PrefKeyAutoplayGifs, true},
		{PrefKeyShowFollowCounts, true},
		{PrefKeyPreferredTimelineOrder, "newest"},
		{PrefKeySearchSuggestionsEnabled, true},
		{PrefKeyPersonalizedSearchEnabled, false},
		{PrefKeyReblogFilters, map[string]bool{"bob": false}},
		{PrefKeyStreamingDefaultQuality, "AUTO"},
		{PrefKeyStreamingAutoQuality, true},
		{PrefKeyStreamingPreloadNext, false},
		{PrefKeyStreamingDataSaver, true},
	}

	for _, tc := range cases {
		assert.NoError(t, repo.updatePreferenceField(prefs, tc.key, tc.value))
		assert.NoError(t, repo.updateSinglePreference(prefs, tc.key, tc.value, "alice"))
	}
}

func TestUserRepository_GetPreference_KnownKeys(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	// GetUserPreferences -> not found -> defaults (repeatable for each GetPreference call).
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	keys := []string{
		PrefKeyLanguage,
		PrefKeyDefaultPostingVisibility,
		PrefKeyDefaultMediaSensitive,
		PrefKeyExpandSpoilers,
		PrefKeyExpandMedia,
		PrefKeyAutoplayGifs,
		PrefKeyShowFollowCounts,
		PrefKeyPreferredTimelineOrder,
		PrefKeySearchSuggestionsEnabled,
		PrefKeyPersonalizedSearchEnabled,
		PrefKeyReblogFilters,
	}

	for _, key := range keys {
		val, err := repo.GetPreference(context.Background(), "alice", key)
		assert.NoError(t, err, "key=%s val=%v", key, val)
	}
}
