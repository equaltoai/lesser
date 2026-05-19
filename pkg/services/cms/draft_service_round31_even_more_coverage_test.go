package cms

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type memArticleServiceGetErr struct {
	base   *memArticleService
	getErr error
}

func (s *memArticleServiceGetErr) GetArticle(ctx context.Context, articleID string) (*models.Article, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.base.GetArticle(ctx, articleID)
}

func (s *memArticleServiceGetErr) GetArticleBySlug(ctx context.Context, slug string) (*models.Article, error) {
	return s.base.GetArticleBySlug(ctx, slug)
}

func (s *memArticleServiceGetErr) CreateArticle(ctx context.Context, article *models.Article) error {
	return s.base.CreateArticle(ctx, article)
}

func (s *memArticleServiceGetErr) UpdateArticle(ctx context.Context, article *models.Article) error {
	return s.base.UpdateArticle(ctx, article)
}

func TestDraftServicePublishDraft_UpdateExistingArticleGetErrorMarksFailed(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	baseArticles := newMemArticleService()
	articles := &memArticleServiceGetErr{base: baseArticles, getErr: errors.New("get failed")}

	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	objectID := "https://example.com/objects/existing"
	draft := &models.Draft{
		ID:          "draft-1",
		AuthorID:    "alice",
		ContentType: activitypub.ArticleType,
		Title:       "Hello",
		Slug:        "hello",
		Content:     "hi",
		Status:      "draft",
		ObjectID:    &objectID,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	_, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)

	after, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
	require.Equal(t, "failed", after.Status)
}

func TestDraftServicePublishDraft_UpdatesExistingArticleAndDeletesDraft(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	objectID := "https://example.com/objects/existing"
	articles.items[objectID] = &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			Name:         "Before",
			AttributedTo: "https://example.com/users/alice",
			Content:      "before",
		},
		Slug:          "before",
		ContentFormat: "markdown",
	}

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "After",
		Slug:          "after",
		Content:       "after",
		ContentFormat: "html",
		Status:        "draft",
		ObjectID:      &objectID,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	updated, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "After", updated.Name)
	require.Equal(t, "after", updated.Slug)
	require.Equal(t, "after", updated.Content)
	require.Equal(t, "html", updated.ContentFormat)

	_, err = repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))
}
