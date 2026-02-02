package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

func TestUserRepository_BookmarkWrappers_PropagateRepositoryErrors(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	bookmarkRepo := NewBookmarkRepository(nil, "test-table", zap.NewNop())
	bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, _, _ string) (*models.Bookmark, error) {
		return nil, assert.AnError
	}

	repo.SetBookmarkRepository(bookmarkRepo)

	assert.Error(t, repo.CreateBookmark(context.Background(), "alice", "obj1"))
	assert.Error(t, repo.RemoveBookmark(context.Background(), "alice", "obj1"))
}

func TestUserRepository_GetBookmarks_RepositoryQueryError(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	bookmarkRepo := NewBookmarkRepository(nil, "test-table", zap.NewNop())
	bookmarkRepo.queryTimeBookmarksFn = func(_ context.Context, _ string, _ int, _ string) ([]models.Bookmark, string, error) {
		return nil, "", assert.AnError
	}
	bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, _, _ string) (*models.Bookmark, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	bookmarkRepo.findTimeBookmarkFn = func(_ context.Context, _, _ string) (*models.Bookmark, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}

	repo.SetBookmarkRepository(bookmarkRepo)

	_, _, err := repo.GetBookmarks(context.Background(), "alice", 20, "")
	assert.Error(t, err)
}
