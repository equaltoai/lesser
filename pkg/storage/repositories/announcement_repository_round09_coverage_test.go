package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestAnnouncementRepository_Round09_Coverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("CreateAnnouncement and GetAnnouncement", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("First", mockMatchedByType[*models.Announcement]()).
			Run(func(args mock.Arguments) {
				a := args.Get(0).(*models.Announcement)
				a.ID = "a1"
				a.Content = "<p>c</p>"
				a.Text = "c"
				a.PublishedAt = baseTime
				a.UpdatedAt = baseTime
				a.CreatedBy = "admin"
				_ = a.UpdateKeys()
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())

		in := &storage.Announcement{
			ID:        "", // force model BeforeCreate to generate
			Content:   "<p>hi</p>",
			Text:      "hi",
			CreatedBy: "admin",
		}
		require.NoError(t, repo.CreateAnnouncement(ctx, in))
		require.NotEmpty(t, in.ID)

		got, err := repo.GetAnnouncement(ctx, "a1")
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, "a1", got.ID)
	})

	t.Run("GetAnnouncementsPaginated and by admin (hasMore + cursor)", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mockMatchedByType[*[]*models.Announcement]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Announcement)
				a1 := &models.Announcement{ID: "a1", PublishedAt: baseTime, CreatedBy: "admin"}
				a1.UpdateKeys()
				a2 := &models.Announcement{ID: "a2", PublishedAt: baseTime.Add(time.Minute), CreatedBy: "admin"}
				a2.UpdateKeys()
				a3 := &models.Announcement{ID: "a3", PublishedAt: baseTime.Add(2 * time.Minute), CreatedBy: "admin"}
				a3.UpdateKeys()
				*out = append(*out, a1, a2, a3)
			}).
			Return(nil).
			Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())

		items, next, err := repo.GetAnnouncementsPaginated(ctx, true, 2, "")
		require.NoError(t, err)
		require.Len(t, items, 2)
		require.NotEmpty(t, next)

		items, next, err = repo.GetAnnouncementsPaginated(ctx, false, 2, "0000000000")
		require.NoError(t, err)
		require.Len(t, items, 2)
		_ = next

		items, next, err = repo.GetAnnouncementsByAdmin(ctx, "admin", 2, "")
		require.NoError(t, err)
		require.Len(t, items, 2)
		require.NotEmpty(t, next)

		items, _, err = repo.GetAnnouncementsByAdmin(ctx, "admin", 2, "cursor")
		require.NoError(t, err)
		require.Len(t, items, 2)
	})

	t.Run("UpdateAnnouncement not found vs success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.ErrorIs(t, repo.UpdateAnnouncement(ctx, &storage.Announcement{ID: "missing"}), storage.ErrNotFound)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAnnouncementRepository(mockDB2, "test-table", zap.NewNop())
		require.NoError(t, repo2.UpdateAnnouncement(ctx, &storage.Announcement{ID: "a1", CreatedBy: "admin"}))
	})

	t.Run("DeleteAnnouncement cleanup paths", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// Base delete succeeds.
		mockQuery.On("Delete").Return(nil).Once()

		// Cleanup reactions: bounded page walk (wave #1469) returns one item.
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.
			On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementReaction")).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.AnnouncementReaction)
				*out = append(*out, &models.AnnouncementReaction{Username: "u", AnnouncementID: "a1", EmojiName: "thumbsup"})
			}).
			Return(&core.PaginatedResult{}, nil).
			Once()

		// Cleanup dismissals: bounded page walk returns one matching item.
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.
			On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementDismissal")).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.AnnouncementDismissal)
				*out = append(*out, &models.AnnouncementDismissal{Username: "u", AnnouncementID: "a1"})
			}).
			Return(&core.PaginatedResult{}, nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
		require.NoError(t, repo.DeleteAnnouncement(ctx, "a1"))
	})

	t.Run("Dismissals and reactions", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		// GetDismissedAnnouncements: bounded page walk returns one dismissal.
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.
			On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementDismissal")).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.AnnouncementDismissal)
				*out = append(*out, &models.AnnouncementDismissal{Username: "u", AnnouncementID: "a1"})
			}).
			Return(&core.PaginatedResult{}, nil).
			Once()

		// GetAnnouncementReactions: bounded page walk returns three reactions.
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.
			On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementReaction")).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.AnnouncementReaction)
				*out = append(*out,
					&models.AnnouncementReaction{Username: "u1", AnnouncementID: "a1", EmojiName: "thumbsup"},
					&models.AnnouncementReaction{Username: "u2", AnnouncementID: "a1", EmojiName: "thumbsup"},
					&models.AnnouncementReaction{Username: "u3", AnnouncementID: "a1", EmojiName: "heart"},
				)
			}).
			Return(&core.PaginatedResult{}, nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())

		require.NoError(t, repo.DismissAnnouncement(ctx, "u", "a1"))
		dismissed, err := repo.IsDismissed(ctx, "u", "a1")
		require.NoError(t, err)
		require.False(t, dismissed)

		ids, err := repo.GetDismissedAnnouncements(ctx, "u")
		require.NoError(t, err)
		require.Len(t, ids, 1)

		require.NoError(t, repo.AddAnnouncementReaction(ctx, "u", "a1", "thumbsup"))

		// "already exists" is treated as success by ignoring errors on Create.
		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAnnouncementRepository(mockDB2, "test-table", zap.NewNop())
		require.NoError(t, repo2.AddAnnouncementReaction(ctx, "u", "a1", "thumbsup"))

		// Remove reaction not found is ignored.
		mockDB3 := new(mocks.MockDB)
		mockQuery3 := new(mocks.MockQuery)
		mockQuery3.On("Delete").Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB3, mockQuery3, nil, baseTime)
		repo3 := NewAnnouncementRepository(mockDB3, "test-table", zap.NewNop())
		require.NoError(t, repo3.RemoveAnnouncementReaction(ctx, "u", "a1", "thumbsup"))

		reactions, err := repo.GetAnnouncementReactions(ctx, "a1")
		require.NoError(t, err)
		require.Len(t, reactions["thumbsup"], 2)
		require.Len(t, reactions["heart"], 1)
	})
}
