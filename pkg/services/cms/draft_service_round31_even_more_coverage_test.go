package cms

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
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

type memArticleServiceSlugErr struct {
	base      *memArticleService
	slugError error
}

func (s *memArticleServiceSlugErr) GetArticle(ctx context.Context, articleID string) (*models.Article, error) {
	return s.base.GetArticle(ctx, articleID)
}

func (s *memArticleServiceSlugErr) GetArticleBySlug(ctx context.Context, slug string) (*models.Article, error) {
	return nil, s.slugError
}

func (s *memArticleServiceSlugErr) CreateArticle(ctx context.Context, article *models.Article) error {
	return s.base.CreateArticle(ctx, article)
}

func (s *memArticleServiceSlugErr) UpdateArticle(ctx context.Context, article *models.Article) error {
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

func TestDraftServiceResolveExistingArticleBySlug_LegacyFallback(t *testing.T) {
	t.Parallel()

	articles := newMemArticleService()
	svc := &DraftService{
		articleService: articles,
	}

	slug := "legacy-article"
	legacyID := common.GenerateObjectID("example.com", "articles", slug)
	articles.items[legacyID] = &models.Article{
		Object: models.Object{
			ID:           legacyID,
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/alice",
		},
		Slug: slug,
	}

	got, err := svc.resolveExistingArticleBySlug(context.Background(), "example.com", slug)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, legacyID, got.ID)
}

func TestDraftServiceResolveExistingArticleBySlug_PropagatesUnexpectedGetBySlugError(t *testing.T) {
	t.Parallel()

	articles := &memArticleServiceSlugErr{
		base:      newMemArticleService(),
		slugError: errors.New("unexpected slug error"),
	}
	svc := &DraftService{
		articleService: articles,
	}

	_, err := svc.resolveExistingArticleBySlug(context.Background(), "example.com", "slug")
	require.Error(t, err)
}
