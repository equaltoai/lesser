package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestAnnouncementRepository_Round09_FinalPush(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 2, 3, 4, 0, time.UTC)

	t.Run("DismissAnnouncement create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.DismissAnnouncement(ctx, "u", "a1"))
	})

	t.Run("IsDismissed true path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		ok, err := repo.IsDismissed(ctx, "u", "a1")
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("GetDismissedAnnouncements not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		ids, err := repo.GetDismissedAnnouncements(ctx, "u")
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("RemoveAnnouncementReaction success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.NoError(t, repo.RemoveAnnouncementReaction(ctx, "u", "a1", "thumbsup"))
	})

	t.Run("DeleteAnnouncement base delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Delete").Return(ErrTestMockError).Once()

		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.DeleteAnnouncement(ctx, "a1"))
	})

	t.Run("DeleteAnnouncement cleanup query errors are tolerated", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Delete").Return(nil).Once()

		// Reactions cleanup query returns non-notfound error => warn path.
		mockQuery.On("All", mockMatchedByType[*[]*models.AnnouncementReaction]()).Return(ErrTestMockError).Once()
		// Dismissals cleanup scan returns non-notfound error => warn path.
		mockQuery.On("All", mockMatchedByType[*[]*models.AnnouncementDismissal]()).Return(ErrTestMockError).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.NoError(t, repo.DeleteAnnouncement(ctx, "a1"))
	})
}
