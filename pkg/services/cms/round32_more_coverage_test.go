package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDraftService_SimpleCRUDWrappers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemDraftRepo()
	svc := &DraftService{
		draftRepo: repo,
		logger:    zap.NewNop(),
	}

	draft := &models.Draft{
		ID:          "draft-1",
		AuthorID:    "alice",
		ContentType: "Article",
		Title:       "t",
		Slug:        "t",
		Content:     "c",
		Status:      "draft",
	}

	require.True(t, draft.CreatedAt.IsZero())
	require.NoError(t, svc.CreateDraft(ctx, draft))
	require.False(t, draft.CreatedAt.IsZero())
	require.False(t, draft.UpdatedAt.IsZero())
	require.False(t, draft.LastSavedAt.IsZero())

	require.Error(t, svc.UpdateDraft(ctx, "bob", draft))

	beforeUpdated := draft.UpdatedAt
	time.Sleep(1 * time.Millisecond)
	require.NoError(t, svc.UpdateDraft(ctx, "alice", draft))
	require.True(t, draft.UpdatedAt.After(beforeUpdated))
	require.True(t, draft.LastSavedAt.After(beforeUpdated))

	require.Error(t, svc.Autosave(ctx, "bob", draft))
	autosaveBefore := draft.AutosaveVersion
	time.Sleep(1 * time.Millisecond)
	require.NoError(t, svc.Autosave(ctx, "alice", draft))
	require.Equal(t, autosaveBefore+1, draft.AutosaveVersion)

	got, err := svc.GetDraft(ctx, "alice", "draft-1")
	require.NoError(t, err)
	require.Equal(t, "draft-1", got.ID)

	require.NoError(t, svc.DeleteDraft(ctx, "alice", "draft-1"))
	_, err = svc.GetDraft(ctx, "alice", "draft-1")
	require.Error(t, err)
}

func TestNewDraftService_TrimsDomainAndSetsFlags(t *testing.T) {
	t.Parallel()

	svc := NewDraftService((*repositories.DraftRepository)(nil), (*ArticleService)(nil), "  example.com  ", true, zap.NewNop())
	require.Equal(t, "example.com", svc.domain)
	require.True(t, svc.scheduling)
}

func TestCategoryService_GetCategory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := &fakeCategoryRepo{categories: map[string]*models.Category{
		"c1": {ID: "c1", Name: "name", Slug: "slug"},
	}}
	svc := NewCategoryService(repo, zap.NewNop())

	got, err := svc.GetCategory(ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "c1", got.ID)

	_, err = svc.GetCategory(ctx, "missing")
	require.Error(t, err)
}

func TestSeriesService_DeleteSeries_PropagatesErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	seriesRepo := &fakeSeriesRepo{deleteErr: errors.New("boom"), series: map[string]*models.Series{}}
	articleRepo := &fakeArticleSeriesRepo{articles: map[string]*models.Article{}, getErrIDs: map[string]error{}}

	svc := NewSeriesService(seriesRepo, articleRepo, zap.NewNop())

	err := svc.DeleteSeries(ctx, "author", "series-1")
	require.Error(t, err)
}
