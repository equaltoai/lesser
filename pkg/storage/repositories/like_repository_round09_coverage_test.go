package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func newTestLikeRepo(mockDB *mocks.MockDB) *LikeRepository {
	repo := NewLikeRepository(mockDB, "tbl", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)
	return repo
}

func TestLikeRepository_round09_create_delete_get_and_counts(t *testing.T) {
	ctx := context.Background()

	t.Run("create_like_success_duplicate_and_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Create").Return(nil).Once()
		like, err := repo.CreateLike(ctx, "actor", "object", "author")
		require.NoError(t, err)
		require.NotNil(t, like)

		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		like, err = repo.CreateLike(ctx, "actor", "object", "author")
		require.NoError(t, err)
		require.NotNil(t, like)

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		_, err = repo.CreateLike(ctx, "actor", "object", "author")
		assert.Error(t, err)
	})

	t.Run("constructor_with_cost_tracking", func(t *testing.T) {
		repo := NewLikeRepositoryWithCostTracking(new(mocks.MockDB), "tbl", zap.NewNop(), &cost.TrackingService{})
		require.NotNil(t, repo)
	})

	t.Run("delete_like_not_found_and_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("Delete").Return(dynamormerrors.ErrItemNotFound).Once()
		require.NoError(t, repo.DeleteLike(ctx, "actor", "object"))

		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
		assert.Error(t, repo.DeleteLike(ctx, "actor", "object"))
	})

	t.Run("get_like_success_and_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Like)
			*dest = *models.NewLike("actor", "object", "author")
		}).Return(nil).Once()

		like, err := repo.GetLike(ctx, "actor", "object")
		require.NoError(t, err)
		require.NotNil(t, like)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		_, err = repo.GetLike(ctx, "actor", "object")
		assert.Error(t, err)
	})

	t.Run("get_like_count_and_boost_count", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("Count").Return(int64(4), nil).Once()
		n, err := repo.GetLikeCount(ctx, "s1")
		require.NoError(t, err)
		assert.EqualValues(t, 4, n)

		mockQuery.On("Count").Return(int64(0), fmt.Errorf("boom")).Once()
		_, err = repo.GetLikeCount(ctx, "s1")
		assert.Error(t, err)

		mockQuery.On("Count").Return(int64(9), nil).Once()
		n, err = repo.GetBoostCount(ctx, "s1")
		require.NoError(t, err)
		assert.EqualValues(t, 9, n)

		mockQuery.On("Count").Return(int64(0), fmt.Errorf("boom")).Once()
		_, err = repo.GetBoostCount(ctx, "s1")
		assert.Error(t, err)
	})
}

func TestLikeRepository_round09_pagination_and_reblog_paths(t *testing.T) {
	ctx := context.Background()

	t.Run("get_object_likes_and_actor_likes", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		// FindWithPagination: return 2 likes but limit=1 so hasMore.
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Like)
			*dest = []*models.Like{
				models.NewLike("a1", "o1", "auth"),
				models.NewLike("a2", "o1", "auth"),
			}
		}).Return(nil).Once()

		likes, cursor, err := repo.GetObjectLikes(ctx, "o1", 1, "")
		require.NoError(t, err)
		require.Len(t, likes, 1)
		_ = cursor

		// Actor likes via GSI query helper (QueryCollectionWithConversion)
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "actor#a1#likes").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi1SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1SK", ">", "cur").Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Like)
			*dest = []*models.Like{
				models.NewLike("a1", "o1", "auth"),
				models.NewLike("a1", "o2", "auth"),
			}
		}).Return(nil).Once()

		out, next, err := repo.GetActorLikes(ctx, "a1", 1, "cur")
		require.NoError(t, err)
		assert.Len(t, out, 1)
		_ = next
	})

	t.Run("increment_reblog_and_has_reblogged", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		require.NoError(t, repo.IncrementReblogCount(ctx, "s1"))

		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
		assert.Error(t, repo.IncrementReblogCount(ctx, "s1"))

		mockQuery.On("Limit", 1).Return(mockQuery).Times(3)
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		ok, err := repo.HasReblogged(ctx, "a1", "s1")
		require.NoError(t, err)
		assert.False(t, ok)

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Announce)
			*dest = []models.Announce{{PK: "x"}}
		}).Return(nil).Once()
		ok, err = repo.HasReblogged(ctx, "a1", "s1")
		require.NoError(t, err)
		assert.True(t, ok)

		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
		_, err = repo.HasReblogged(ctx, "a1", "s1")
		assert.Error(t, err)
	})
}

func TestLikeRepository_round09_cascade_and_tombstone_paths(t *testing.T) {
	ctx := context.Background()

	t.Run("cascade_delete_single_page", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		// Query returns 1 like, then delete succeeds.
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Like)
			like := models.NewLike("actor", "obj", "author")
			_ = like.UpdateKeys()
			*dest = []*models.Like{like}
		}).Return(nil).Once()

		mockQuery.On("Delete").Return(nil).Once()
		require.NoError(t, repo.CascadeDeleteLikes(ctx, "obj"))
	})

	t.Run("cascade_delete_empty_page_and_batch_delete_fallback", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		// First: empty query breaks loop.
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Like)
			*dest = []*models.Like{}
		}).Return(nil).Once()
		require.NoError(t, repo.CascadeDeleteLikes(ctx, "obj-empty"))

		// Second: query one like, BatchDelete fails (Delete error), fallback delete succeeds.
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Like)
			like := models.NewLike("actor", "obj2", "author")
			_ = like.UpdateKeys()
			*dest = []*models.Like{like}
		}).Return(nil).Once()

		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
		mockQuery.On("Delete").Return(nil).Once()
		require.NoError(t, repo.CascadeDeleteLikes(ctx, "obj2"))
	})

	t.Run("tombstone_create_and_get", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("Create").Return(nil).Once()
		require.NoError(t, repo.TombstoneObject(ctx, "obj", "deleter"))

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		assert.Error(t, repo.TombstoneObject(ctx, "obj2", "deleter"))

		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err := repo.GetTombstone(ctx, "obj")
		assert.Error(t, err)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Tombstone)
			*dest = models.Tombstone{ID: "obj", Type: "Tombstone", FormerType: "Object", Deleted: time.Now(), DeletedBy: "deleter"}
		}).Return(nil).Once()
		ts, err := repo.GetTombstone(ctx, "obj")
		require.NoError(t, err)
		assert.Equal(t, "obj", ts.ID)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		_, err = repo.GetTombstone(ctx, "obj")
		assert.Error(t, err)
	})

	t.Run("compatibility_methods", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestLikeRepo(mockDB)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Count").Return(int64(1), nil).Once()
		n, err := repo.CountForObject(ctx, "o1")
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)

		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Once()
		_, _, err = repo.GetForObject(ctx, "o1", 1, "")
		require.NoError(t, err)

		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "actor#a1#likes").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi1SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Once()
		_, _, err = repo.GetLikedObjects(ctx, "a1", 1, "")
		require.NoError(t, err)

		// CountActorLikes + HasLiked (success/error)
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "actor#a1#likes").Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(2), nil).Once()
		count, err := repo.CountActorLikes(ctx, "a1")
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		mockQuery.On("Where", "PK", "=", "object#o1#likes").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "actor#a1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		liked, err := repo.HasLiked(ctx, "a1", "o1")
		assert.Error(t, err)
		assert.False(t, liked)

		mockQuery.On("Where", "PK", "=", "object#o2#likes").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "actor#a1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Like)
			*dest = *models.NewLike("a1", "o2", "auth")
		}).Return(nil).Once()
		liked, err = repo.HasLiked(ctx, "a1", "o2")
		require.NoError(t, err)
		assert.True(t, liked)

		_ = storage.ErrNotFound
	})
}
