package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
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

	byStatus, nextByStatus, err := repo.ListDraftsByStatusPaginated(ctx, "failed", 1, "cursor")
	require.NoError(t, err)
	require.Len(t, byStatus, 1)
	require.NotEmpty(t, nextByStatus)

	_, _, err = repo.ListDraftsByStatusPaginated(ctx, "   ", 1, "")
	require.Error(t, err)
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

// TestRound10_ListDraftsByStatusPaginatedRealCursorSemantics proves the real
// ListDraftsByStatusPaginated follows cursor semantics through the
// real-expression fakedb: ascending gsi4SK ordering, a second page continuing
// after the first page's cursor, and a past-end cursor returning an empty page
// with an empty next cursor. The permissive-mock round10 case above cannot
// assert ordering or the gsi4SK > predicate; this test does.
func TestRound10_ListDraftsByStatusPaginatedRealCursorSemantics(t *testing.T) {
	ctx := context.Background()
	client := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	// Seed three failed drafts with distinct, ascending UpdatedAt so their
	// GSI4SK sort keys are strictly ordered. UpdateKeys must run explicitly:
	// the fakedb Create does not auto-run model key hooks.
	base := time.Now().UTC().Add(-25 * time.Hour)
	for i, id := range []string{"f1", "f2", "f3"} {
		draft := &models.Draft{
			AuthorID: "owner", ID: id, Status: "failed", Content: "c", ContentFormat: "markdown",
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
			UpdatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		require.NoError(t, draft.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(draft).Create())
	}

	page1, next1, err := repo.ListDraftsByStatusPaginated(ctx, "failed", 2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2, "the first page must carry the first two drafts in gsi4SK order")
	require.Equal(t, []string{"f1", "f2"}, []string{page1[0].ID, page1[1].ID},
		"the real query must order by gsi4SK ascending, not insertion order")
	require.NotEmpty(t, next1)
	require.Equal(t, page1[1].GSI4SK, next1, "the next cursor must be the last returned gsi4SK value")

	page2, next2, err := repo.ListDraftsByStatusPaginated(ctx, "failed", 2, next1)
	require.NoError(t, err)
	require.Len(t, page2, 1, "the second page must continue strictly after the first page's cursor")
	require.Equal(t, "f3", page2[0].ID)
	require.Empty(t, next2, "the final page must not emit a next cursor")

	page3, next3, err := repo.ListDraftsByStatusPaginated(ctx, "failed", 2, "TIME#2999~past-end")
	require.NoError(t, err)
	require.Empty(t, page3, "a past-end cursor must return an empty page")
	require.Empty(t, next3, "a past-end cursor must terminate with an empty next cursor")
}
