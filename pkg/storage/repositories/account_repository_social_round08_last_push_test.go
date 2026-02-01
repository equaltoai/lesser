package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepository_Social_LastPush(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("GetMutes query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.Mute")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetMutes(ctx, "alice")
		require.Error(t, err)
	})

	t.Run("Mute condition failed then updateMute error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(errors.New("first failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.Mute(ctx, "alice", "bob", true, time.Minute))
	})

	t.Run("GetBookmarks propagates repository error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		bookmarkRepo := NewBookmarkRepository(mockDB, "test-table", zaptest.NewLogger(t))
		bookmarkRepo.queryTimeBookmarksFn = func(context.Context, string, int, string) ([]models.Bookmark, string, error) {
			return nil, "", errors.New("boom")
		}
		repo.SetBookmarkRepository(bookmarkRepo)

		_, _, err := repo.GetBookmarks(ctx, "alice", 10, "")
		require.Error(t, err)
	})
}
