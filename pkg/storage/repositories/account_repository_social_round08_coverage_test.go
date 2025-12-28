package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type round08StatusRepo struct {
	statuses []*models.Status
	err      error
}

func (s round08StatusRepo) CreateStatus(context.Context, *models.Status) error {
	panic("unexpected call")
}
func (s round08StatusRepo) CreateBoostStatus(context.Context, *models.Status) error {
	panic("unexpected call")
}
func (s round08StatusRepo) GetStatus(context.Context, string) (*models.Status, error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetStatusByURL(context.Context, string) (*models.Status, error) {
	panic("unexpected call")
}
func (s round08StatusRepo) UpdateStatus(context.Context, *models.Status) error {
	panic("unexpected call")
}
func (s round08StatusRepo) DeleteStatus(context.Context, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) DeleteBoostStatus(context.Context, string, string) (*models.Status, error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetPublicTimeline(context.Context, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetHomeTimeline(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetUserTimeline(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetConversationThread(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetReplies(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) SearchStatuses(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetStatusesByHashtag(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetTrendingStatuses(context.Context, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) LikeStatus(context.Context, string, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) UnlikeStatus(context.Context, string, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) ReblogStatus(context.Context, string, string, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) UnreblogStatus(context.Context, string, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) BookmarkStatus(context.Context, string, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) UnbookmarkStatus(context.Context, string, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) FlagStatus(context.Context, string, string, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) UnflagStatus(context.Context, string) error {
	panic("unexpected call")
}
func (s round08StatusRepo) GetFlaggedStatuses(context.Context, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetStatusesByIDs(_ context.Context, _ []string) ([]*models.Status, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.statuses, nil
}
func (s round08StatusRepo) GetStatusCounts(context.Context, string) (int, int, int, error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetStatusContext(context.Context, string) ([]*models.Status, []*models.Status, error) {
	panic("unexpected call")
}
func (s round08StatusRepo) GetStatusEngagement(context.Context, string, string) (bool, bool, bool, error) {
	panic("unexpected call")
}

func TestRound08_AccountRepository_Social_Sweep(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("clamp helpers", func(t *testing.T) {
		require.Equal(t, 40, clampFollowLimit(0))
		require.Equal(t, 200, clampFollowLimit(999))
		require.Equal(t, 10, clampFollowLimit(10))

		require.Equal(t, 40, clampBookmarkLimit(0))
		require.Equal(t, 400, clampBookmarkLimit(999))
		require.Equal(t, 12, clampBookmarkLimit(12))
	})

	t.Run("follow/unfollow/relationship queries", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("First", mock.AnythingOfType("*models.Follow")).Return(dynamormErrors.ErrItemNotFound).Twice()
		setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.NoError(t, repo.Follow(ctx, "alice", "bob"))

		mockQuery.On("Delete").Return(dynamormErrors.ErrItemNotFound).Once()
		require.NoError(t, repo.Unfollow(ctx, "alice", "bob"))

		isFollowing, err := repo.IsFollowing(ctx, "alice", "bob")
		require.NoError(t, err)
		require.False(t, isFollowing)

		actors, next, err := repo.GetFollowers(ctx, "bob", 1, "")
		require.NoError(t, err)
		require.NotEmpty(t, actors)
		require.NotEmpty(t, next)

		_, _, err = repo.GetFollowers(ctx, "bob", 1, "cursor")
		require.NoError(t, err)

		actors, next, err = repo.GetFollowing(ctx, "alice", 1, "")
		require.NoError(t, err)
		require.NotEmpty(t, actors)
		require.NotEmpty(t, next)

		_, _, err = repo.GetFollowing(ctx, "alice", 1, "cursor")
		require.NoError(t, err)
	})

	t.Run("blocks", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		mockQuery.On("Create").Return(errors.New("create failed")).Once()
		mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Block")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.NoError(t, repo.Block(ctx, "alice", "bob"))
		require.Error(t, repo.Block(ctx, "alice", "bob"))
		require.Error(t, repo.Unblock(ctx, "alice", "bob"))

		blocked, err := repo.IsBlocked(ctx, "alice", "bob")
		require.NoError(t, err)
		require.False(t, blocked)

		blocked, err = repo.IsBlocked(ctx, "alice", "bob")
		require.NoError(t, err)
		require.True(t, blocked)

		blocks, err := repo.GetBlocks(ctx, "alice")
		require.NoError(t, err)
		require.NotEmpty(t, blocks)
	})

	t.Run("mutes", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(errors.New("get failed")).Once()
		mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.NoError(t, repo.Mute(ctx, "alice", "bob", true, time.Minute))
		require.Error(t, repo.Mute(ctx, "alice", "bob", true, time.Minute))
		require.Error(t, repo.Unmute(ctx, "alice", "bob"))

		isMuted, hide, err := repo.IsMuted(ctx, "alice", "bob")
		require.NoError(t, err)
		require.False(t, isMuted)
		require.False(t, hide)

		isMuted, _, err = repo.IsMuted(ctx, "alice", "bob")
		require.NoError(t, err)
		require.True(t, isMuted)

			mutes, err := repo.GetMutes(ctx, "alice")
			require.NoError(t, err)
			require.NotEmpty(t, mutes)
		})

	t.Run("bookmarks and bookmarked statuses", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		bookmarkRepo := NewBookmarkRepository(mockDB, "test-table", zaptest.NewLogger(t))
		bookmarkRepo.transactWriteFn = func(_ context.Context, _ func(core.TransactionBuilder) error) error { return nil }
		repo.SetBookmarkRepository(bookmarkRepo)

		bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, username, objectID string) (*models.Bookmark, error) {
			return &models.Bookmark{
				PK:           "BOOKMARK#" + username,
				SK:           "OBJECT#" + objectID,
				Username:     username,
				ObjectID:     objectID,
				RecordType:   models.BookmarkRecordTypeObject,
				TimeRecordSK: "TIME#2025-01-01T00:00:00Z#" + objectID,
				CreatedAt:    baseTime,
			}, nil
		}
		bookmarkRepo.queryTimeBookmarksFn = func(_ context.Context, username string, limit int, cursor string) ([]models.Bookmark, string, error) {
			_ = limit
			_ = cursor
			return []models.Bookmark{
				{
					PK:        "BOOKMARK#" + username,
					SK:        "TIME#2025-01-01T00:00:00Z#object#status-1",
					Username:  username,
					ObjectID:  "object#status-1",
					CreatedAt: baseTime,
				},
				{
					PK:        "BOOKMARK#" + username,
					SK:        "TIME#2025-01-01T00:00:01Z#object#status-2",
					Username:  username,
					ObjectID:  "status-2",
					CreatedAt: baseTime.Add(time.Second),
				},
			}, "", nil
		}

		require.NoError(t, repo.AddBookmark(ctx, "alice", "object#status-1"))
		require.NoError(t, repo.RemoveBookmark(ctx, "alice", "object#status-1"))

		bookmarks, cursor, err := repo.GetBookmarks(ctx, "alice", 1, "")
		require.NoError(t, err)
		require.NotEmpty(t, bookmarks)
		_ = cursor

		_, err = repo.GetBookmarkedStatuses(ctx, "alice", interfaces.PaginationOptions{Limit: 10})
		require.NoError(t, err) // dependency error is logged but not returned

		repo.SetStatusRepository(round08StatusRepo{
			statuses: []*models.Status{{StatusID: "status-1"}, {StatusID: "status-2"}},
		})

		result, err := repo.GetBookmarkedStatuses(ctx, "alice", interfaces.PaginationOptions{Limit: 2})
		require.NoError(t, err)
		require.NotNil(t, result)

		bookmarkRepo.queryTimeBookmarksFn = func(context.Context, string, int, string) ([]models.Bookmark, string, error) {
			return nil, "", nil
		}
		result, err = repo.GetBookmarkedStatuses(ctx, "alice", interfaces.PaginationOptions{Limit: 2})
		require.NoError(t, err)
		require.Empty(t, result.Items)

		bookmarkRepo.queryTimeBookmarksFn = func(_ context.Context, username string, limit int, cursor string) ([]models.Bookmark, string, error) {
			_ = limit
			_ = cursor
			return []models.Bookmark{
				{
					PK:        "BOOKMARK#" + username,
					SK:        "TIME#2025-01-01T00:00:00Z#object#status-1",
					Username:  username,
					ObjectID:  "object#status-1",
					CreatedAt: baseTime,
				},
			}, "", nil
		}

		repo.SetStatusRepository(round08StatusRepo{err: errors.New("status lookup failed")})
		_, err = repo.GetBookmarkedStatuses(ctx, "alice", interfaces.PaginationOptions{Limit: 2})
		require.Error(t, err)
	})

	t.Run("pins", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.AccountPin")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.NoError(t, repo.PinAccount(ctx, "alice", "bob"))
		require.Error(t, repo.UnpinAccount(ctx, "alice", "bob"))

		actors, err := repo.GetPinnedAccounts(ctx, "alice")
		require.NoError(t, err)
		require.NotEmpty(t, actors)

		pins, err := repo.GetAccountPins(ctx, "alice")
		require.NoError(t, err)
		require.NotEmpty(t, pins)

		_, err = repo.GetAccountPin(ctx, "alice", "https://example.com/users/bob")
		require.Error(t, err)

		_, err = repo.GetAccountPin(ctx, "alice", "https://example.com/users/bob")
		require.NoError(t, err)
	})

	t.Run("best-effort actor count update", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockUpdateBuilder.On("Execute").Return(errors.New("exec failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		repo.updateActorCount(ctx, "alice", "FollowerCount", 1)
	})
}
