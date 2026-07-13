package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestAnnouncementRepository_Round09_MoreCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("conversion helpers handle nil and round-trip", func(t *testing.T) {
		require.Nil(t, convertStorageReactionsToModel(nil))
		require.Nil(t, convertModelReactionsToStorage(nil))
		require.Nil(t, convertStorageEmojisToModel(nil))
		require.Nil(t, convertModelEmojisToStorage(nil))
		require.Nil(t, convertStorageMentionsToModel(nil))
		require.Nil(t, convertModelMentionsToStorage(nil))

		reactions := []storage.Reaction{{Name: "x", Count: 1, Me: true}}
		require.Len(t, convertModelReactionsToStorage(convertStorageReactionsToModel(reactions)), 1)

		emojis := []storage.CustomEmoji{{Shortcode: "x", URL: "u", StaticURL: "s", VisibleInPicker: true}}
		require.Len(t, convertModelEmojisToStorage(convertStorageEmojisToModel(emojis)), 1)

		mentions := []storage.Mention{{ID: "1", Username: "u", URL: "url", Acct: "acct"}}
		require.Len(t, convertModelMentionsToStorage(convertStorageMentionsToModel(mentions)), 1)
	})

	t.Run("constructors and list wrapper", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAnnouncementRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		require.NotNil(t, repo)

		announcements, err := repo.GetAnnouncements(ctx, true)
		require.NoError(t, err)
		require.NotNil(t, announcements)
	})

	t.Run("GetAnnouncement not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetAnnouncement(ctx, "missing")
		require.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("CreateAnnouncement create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.CreateAnnouncement(ctx, &storage.Announcement{Content: "c", Text: "t"}))
	})

	t.Run("GetAnnouncementsPaginated and GetAnnouncementsByAdmin not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Twice()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())

		items, next, err := repo.GetAnnouncementsPaginated(ctx, true, 10, "")
		require.NoError(t, err)
		require.Empty(t, items)
		require.Empty(t, next)

		items, next, err = repo.GetAnnouncementsByAdmin(ctx, "admin", 10, "")
		require.NoError(t, err)
		require.Empty(t, items)
		require.Empty(t, next)
	})

	t.Run("RemoveAnnouncementReaction delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.RemoveAnnouncementReaction(ctx, "u", "a1", "x"))
	})

	t.Run("IsDismissed get error; GetAnnouncementReactions not found and error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())

		_, err := repo.IsDismissed(ctx, "u", "a1")
		require.Error(t, err)

		m, err := repo.GetAnnouncementReactions(ctx, "a1")
		require.NoError(t, err)
		require.Empty(t, m)

		_, err = repo.GetAnnouncementReactions(ctx, "a1")
		require.Error(t, err)
	})
}
