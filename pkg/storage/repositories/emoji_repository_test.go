package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestEmojiRepository_CreateCustomEmoji_AlreadyExists(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "EMOJI#party").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "EMOJI").Return(mockQuery)
	mockQuery.On("Count").Return(int64(1), nil)

	err := repo.CreateCustomEmoji(context.Background(), &storage.CustomEmoji{Shortcode: "party"})
	assert.ErrorIs(t, err, storage.ErrAlreadyExists)
}

func TestEmojiRepository_CreateCustomEmoji_ExistsError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Count").Return(int64(0), errors.New("db error"))

	err := repo.CreateCustomEmoji(context.Background(), &storage.CustomEmoji{Shortcode: "party"})
	assert.Error(t, err)
}

func TestEmojiRepository_CreateCustomEmoji_Success_LocalAndRemote(t *testing.T) {
	ctx := context.Background()

	t.Run("local", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Count").Return(int64(0), nil)
		mockQuery.On("Create").Return(nil)

		emoji := &storage.CustomEmoji{Shortcode: "party", URL: "https://cdn.local/party.png"}
		require.NoError(t, repo.CreateCustomEmoji(ctx, emoji))
		assert.False(t, emoji.CreatedAt.IsZero())
		assert.False(t, emoji.UpdatedAt.IsZero())
		assert.False(t, emoji.ImageUpdatedAt.IsZero())
	})

	t.Run("remote_domain_changes_pk", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "EMOJI#party@remote.example").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "EMOJI").Return(mockQuery)
		mockQuery.On("Count").Return(int64(0), nil)
		mockQuery.On("Create").Return(nil)

		emoji := &storage.CustomEmoji{Shortcode: "party", Domain: "remote.example"}
		require.NoError(t, repo.CreateCustomEmoji(ctx, emoji))
	})
}

func TestEmojiRepository_GetCustomEmoji_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Return(dynamormerrors.ErrItemNotFound)

	emoji, err := repo.GetCustomEmoji(context.Background(), "missing")
	assert.Error(t, err)
	assert.Nil(t, emoji)
}

func TestEmojiRepository_GetCustomEmojis_GracefulDegradationOnMissingGSI(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Return(dynamormerrors.ErrItemNotFound)

	emojis, err := repo.GetCustomEmojis(context.Background())
	require.NoError(t, err)
	assert.Empty(t, emojis)
}

func TestEmojiRepository_GetCustomEmojis_GracefulDegradationOnNotFoundString(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Return(errors.New("index not found"))

	emojis, err := repo.GetCustomEmojis(context.Background())
	require.NoError(t, err)
	assert.Empty(t, emojis)
}

func TestEmojiRepository_GetCustomEmojis_FiltersDisabledLocalKeepsDisabledRemote(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.EmojiModel)
		*dest = []*models.EmojiModel{
			{Shortcode: "local_disabled", Disabled: true, Domain: ""},
			{Shortcode: "remote_disabled", Disabled: true, Domain: "remote.example"},
			{Shortcode: "enabled", Disabled: false, Domain: ""},
		}
	}).Return(nil)

	emojis, err := repo.GetCustomEmojis(context.Background())
	require.NoError(t, err)
	require.Len(t, emojis, 2)
	assert.Equal(t, "remote_disabled", emojis[0].Shortcode)
	assert.Equal(t, "enabled", emojis[1].Shortcode)
}

func TestEmojiRepository_SearchEmojis_InvalidInputReturnsEmpty(t *testing.T) {
	repo := &EmojiRepository{}

	out, err := repo.SearchEmojis(context.Background(), "", 10)
	require.NoError(t, err)
	assert.Empty(t, out)

	out, err = repo.SearchEmojis(context.Background(), "party", 0)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestEmojiRepository_SearchEmojis_PrefixThenBroadSearchAndScore(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.EmojiModel)
		*dest = []*models.EmojiModel{
			{PK: "EMOJI#party", SK: "EMOJI", Shortcode: "party", PopularityScore: 1.0},
		}
	}).Return(nil).Once()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.EmojiModel)
		*dest = []*models.EmojiModel{
			{PK: "EMOJI#party", SK: "EMOJI", Shortcode: "party", PopularityScore: 1.0},
			{PK: "EMOJI#partypop", SK: "EMOJI", Shortcode: "partypop", PopularityScore: 0.0, SearchKeywords: []string{"party"}},
			{PK: "EMOJI#other", SK: "EMOJI", Shortcode: "other", PopularityScore: 10.0},
		}
	}).Return(nil).Once()

	results, err := repo.SearchEmojis(context.Background(), "party", 2)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "party", results[0].Shortcode)
}

func TestEmojiRepository_GetPopularEmojis_DefaultsAndFiltering(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", "gsi4").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.EmojiModel)
		*dest = []*models.EmojiModel{
			{Shortcode: "local_disabled", Disabled: true},
			{Shortcode: "ok", Disabled: false},
			{Shortcode: "remote_disabled", Disabled: true, Domain: "remote.example"},
		}
	}).Return(nil)

	results, err := repo.GetPopularEmojis(context.Background(), "", 0)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "ok", results[0].Shortcode)
	assert.Equal(t, "remote_disabled", results[1].Shortcode)
}

func TestEmojiRepository_IncrementEmojiUsage_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Return(dynamormerrors.ErrItemNotFound)

	err := repo.IncrementEmojiUsage(context.Background(), "missing")
	assert.Error(t, err)
}

func TestEmojiRepository_IncrementEmojiUsage_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.EmojiModel)
		out.Shortcode = "party"
		out.UsageCount = 3
		out.CreatedAt = time.Now().Add(-24 * time.Hour)
		out.UpdatedAt = time.Now().Add(-time.Hour)
		out.ImageUpdatedAt = time.Now().Add(-time.Hour)
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.IncrementEmojiUsage(context.Background(), "party")
	require.NoError(t, err)
}

func TestEmojiRepository_PureHelpers_MatchingAndScoring(t *testing.T) {
	repo := &EmojiRepository{}

	model := &models.EmojiModel{
		Shortcode:       "party",
		Category:        "fun",
		SearchKeywords:  []string{"celebration", "partytime"},
		AltText:         "Party popper",
		PopularityScore: 2.0,
		LastUsedAt:      time.Now().Add(-24 * time.Hour),
	}

	assert.True(t, repo.matchesSearchQuery(model, "party"))
	assert.True(t, repo.matchesSearchQuery(model, "fun"))
	assert.True(t, repo.matchesSearchQuery(model, "celebration"))
	assert.True(t, repo.matchesSearchQuery(model, "popper"))
	assert.False(t, repo.matchesSearchQuery(model, "zzz"))

	scoreExact := repo.calculateSearchScore(&models.EmojiModel{Shortcode: "party", PopularityScore: 0}, "party")
	scorePrefix := repo.calculateSearchScore(&models.EmojiModel{Shortcode: "partytime", PopularityScore: 0}, "party")
	scoreContains := repo.calculateSearchScore(&models.EmojiModel{Shortcode: "timeparty", PopularityScore: 0}, "party")
	assert.Greater(t, scoreExact, scorePrefix)
	assert.Greater(t, scorePrefix, scoreContains)

	scored := repo.scoreSearchResults([]*models.EmojiModel{
		{Shortcode: "zzz", PopularityScore: 0},
		{Shortcode: "party", PopularityScore: 0},
	}, "party")
	require.Len(t, scored, 2)
	assert.Equal(t, "party", scored[0].Shortcode)
}

func TestEmojiRepository_UpdateCustomEmoji_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Count").Return(int64(0), nil)

	err := repo.UpdateCustomEmoji(context.Background(), &storage.CustomEmoji{Shortcode: "party"})
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestEmojiRepository_UpdateCustomEmoji_Success_WithDomainPk(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)

	mockQuery.On("Where", "PK", "=", "EMOJI#party@remote.example").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "EMOJI").Return(mockQuery)
	mockQuery.On("Count").Return(int64(1), nil)
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.UpdateCustomEmoji(context.Background(), &storage.CustomEmoji{Shortcode: "party", Domain: "remote.example"})
	require.NoError(t, err)
}

func TestEmojiRepository_GetRemoteEmoji_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "EMOJI#party@remote.example").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "EMOJI").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Return(dynamormerrors.ErrItemNotFound)

	emoji, err := repo.GetRemoteEmoji(context.Background(), "party", "remote.example")
	assert.Error(t, err)
	assert.Nil(t, emoji)
}

func TestEmojiRepository_GetCustomEmoji_DBError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Return(ErrTestMockError)

	emoji, err := repo.GetCustomEmoji(context.Background(), "party")
	assert.Error(t, err)
	assert.Nil(t, emoji)
}

func TestEmojiRepository_GetCustomEmoji_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.EmojiModel)
		dest.Shortcode = "party"
		dest.URL = "https://cdn.local/party.png"
		dest.VisibleInPicker = true
	}).Return(nil)

	emoji, err := repo.GetCustomEmoji(context.Background(), "party")
	require.NoError(t, err)
	require.NotNil(t, emoji)
	assert.Equal(t, "party", emoji.Shortcode)
	assert.True(t, emoji.VisibleInPicker)
}

func TestEmojiRepository_GetRemoteEmoji_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "EMOJI#party@remote.example").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "EMOJI").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.EmojiModel)
		dest.Shortcode = "party"
		dest.Domain = "remote.example"
		dest.URL = "https://remote.example/party.png"
	}).Return(nil)

	emoji, err := repo.GetRemoteEmoji(context.Background(), "party", "remote.example")
	require.NoError(t, err)
	require.NotNil(t, emoji)
	assert.Equal(t, "party", emoji.Shortcode)
	assert.Equal(t, "remote.example", emoji.Domain)
}

func TestEmojiRepository_DeleteCustomEmoji_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Count").Return(int64(0), nil)

	err := repo.DeleteCustomEmoji(context.Background(), "missing")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestEmojiRepository_DeleteCustomEmoji_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "EMOJI#party").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "EMOJI").Return(mockQuery)
	mockQuery.On("Count").Return(int64(1), nil)

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.DeleteCustomEmoji(context.Background(), "party")
	require.NoError(t, err)
}

func TestEmojiRepository_GetCustomEmojisByCategory_FiltersDisabledLocal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.EmojiModel)
		*dest = []*models.EmojiModel{
			{Shortcode: "local_disabled", Disabled: true},
			{Shortcode: "remote_disabled", Disabled: true, Domain: "remote.example"},
			{Shortcode: "ok", Disabled: false},
		}
	}).Return(nil)

	results, err := repo.GetCustomEmojisByCategory(context.Background(), "fun")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "remote_disabled", results[0].Shortcode)
	assert.Equal(t, "ok", results[1].Shortcode)
}

func TestEmojiRepository_QueryEmojiGSI_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Return(ErrTestMockError)

	_, err := repo.queryEmojiGSI(context.Background(), "gsi1", "gsi1PK", "ALL_EMOJIS", 0)
	assert.Error(t, err)
}

func TestEmojiRepository_CalculateSearchScore_RecencyBranch(t *testing.T) {
	repo := &EmojiRepository{}

	withRecent := repo.calculateSearchScore(&models.EmojiModel{
		Shortcode:  "party",
		LastUsedAt: time.Now().Add(-24 * time.Hour),
	}, "party")
	withOld := repo.calculateSearchScore(&models.EmojiModel{
		Shortcode:  "party",
		LastUsedAt: time.Now().Add(-30 * 24 * time.Hour),
	}, "party")
	assert.Greater(t, withRecent, withOld)
}

func TestEmojiRepository_IncrementEmojiUsage_UpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.EmojiModel")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.EmojiModel)
		out.Shortcode = "party"
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(ErrTestMockError)

	err := repo.IncrementEmojiUsage(context.Background(), "party")
	assert.Error(t, err)
}

func TestEmojiRepository_QueryEmojiGSI_Success_WithLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewEmojiRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmojiModel")).Return(mockQuery)
	mockQuery.On("Index", "gsi4").Return(mockQuery)
	mockQuery.On("Where", "gsi4PK", "=", "USAGE#local").Return(mockQuery)
	mockQuery.On("Limit", 3).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmojiModel")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.EmojiModel)
		*dest = []*models.EmojiModel{{Shortcode: "party"}}
	}).Return(nil)

	results, err := repo.queryEmojiGSI(context.Background(), "gsi4", "gsi4PK", "USAGE#local", 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "party", results[0].Shortcode)
}
