package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepository_BookmarkWrapper_Errors(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	bookmarkRepo := NewBookmarkRepository(mockDB, "test-table", zaptest.NewLogger(t))
	bookmarkRepo.getObjectBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, errors.New("boom")
	}
	repo.SetBookmarkRepository(bookmarkRepo)

	require.Error(t, repo.AddBookmark(ctx, "alice", "object#status-1"))
	require.Error(t, repo.RemoveBookmark(ctx, "alice", "object#status-1"))
}
