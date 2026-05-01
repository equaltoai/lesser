package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_DraftRepository_CRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewDraftRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	draft := &models.Draft{
		AuthorID:      "user-1",
		ID:            "draft-1",
		ContentType:   "Article",
		Content:       "hello",
		ContentFormat: "markdown",
		Status:        "draft",
		CreatedAt:     baseTime,
		UpdatedAt:     baseTime,
	}

	require.NoError(t, repo.CreateDraft(ctx, draft))

	got, err := repo.GetDraft(ctx, "user-1", "draft-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.Error(t, repo.UpdateDraft(ctx, "user-1", &models.Draft{ID: "draft-1"})) // missing AuthorID
	require.Error(t, repo.UpdateDraft(ctx, "other-user", draft))
	require.NoError(t, repo.UpdateDraft(ctx, "user-1", draft))

	require.NoError(t, repo.DeleteDraft(ctx, "user-1", "draft-1"))

	_, _, err = repo.ListDraftsByAuthorPaginated(ctx, "   ", 1, "")
	require.Error(t, err)

	items, next, err := repo.ListDraftsByAuthorPaginated(ctx, "user-1", 1, "draft-1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotEmpty(t, next)

	scheduled, nextScheduled, err := repo.ListScheduledDraftsDuePaginated(ctx, time.Time{}, 1, "cursor")
	require.NoError(t, err)
	require.Len(t, scheduled, 1)
	require.NotEmpty(t, nextScheduled)
}

func TestRound10_DraftRepository_MorePaginationBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewDraftRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	drafts, err := repo.ListDraftsByAuthor(ctx, "user-1", 1)
	require.NoError(t, err)
	require.Len(t, drafts, 1)

	scheduled, next, err := repo.ListScheduledDraftsDuePaginated(ctx, baseTime.Add(1*time.Hour), 10, "")
	require.NoError(t, err)
	require.NotEmpty(t, scheduled)
	require.Empty(t, next)
}
